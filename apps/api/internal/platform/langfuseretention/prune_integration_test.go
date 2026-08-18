//go:build integration

package langfuseretention_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/job-finder/api/internal/platform/langfuseretention"
	"github.com/job-finder/api/internal/testinfra"
)

func TestPrune_BlanksOldPayloadKeepsRest(t *testing.T) {
	ctx := context.Background()

	dsn, err := testinfra.ClickHouseDSN(ctx)
	if err != nil {
		t.Fatalf("start clickhouse container: %v", err)
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		t.Fatalf("open clickhouse: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS traces"); err != nil {
		t.Fatalf("drop traces: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS traces")
	})

	const createTraces = `
CREATE TABLE traces (
    id String,
    timestamp DateTime64(3),
    name String,
    user_id Nullable(String),
    metadata Map(LowCardinality(String), String),
    release Nullable(String),
    version Nullable(String),
    project_id String,
    public Bool,
    bookmarked Bool,
    tags Array(String),
    input Nullable(String) CODEC(ZSTD(3)),
    output Nullable(String) CODEC(ZSTD(3)),
    session_id Nullable(String),
    created_at DateTime64(3) DEFAULT now(),
    updated_at DateTime64(3) DEFAULT now(),
    event_ts DateTime64(3),
    is_deleted UInt8
) ENGINE = ReplacingMergeTree(event_ts, is_deleted) Partition by toYYYYMM(timestamp)
PRIMARY KEY (project_id, toDate(timestamp))
ORDER BY (project_id, toDate(timestamp), id)`
	if _, err := db.ExecContext(ctx, createTraces); err != nil {
		t.Fatalf("create traces: %v", err)
	}

	now := time.Now().UTC()
	old := now.AddDate(0, 0, -45)
	recent := now.AddDate(0, 0, -1)

	insert := `INSERT INTO traces
		(id, timestamp, name, project_id, public, bookmarked, tags, input, output, event_ts, is_deleted)
		VALUES (?, ?, ?, ?, false, false, [], ?, ?, ?, 0)`
	if _, err := db.ExecContext(ctx, insert,
		"old-trace", old, "generate-resume", "proj1", "profile and resume content", "generated resume", old,
	); err != nil {
		t.Fatalf("insert old row: %v", err)
	}
	if _, err := db.ExecContext(ctx, insert,
		"recent-trace", recent, "generate-resume", "proj1", "profile and resume content", "generated resume", recent,
	); err != nil {
		t.Fatalf("insert recent row: %v", err)
	}

	pruner := langfuseretention.New(langfuseretention.Config{URL: dsn, RetentionDays: 30})
	report, err := pruner.Prune(ctx)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if report.Skipped {
		t.Fatal("report unexpectedly skipped")
	}
	if report.TablesPurged["traces"] != 1 {
		t.Fatalf("expected exactly 1 row purged in traces, got %d", report.TablesPurged["traces"])
	}

	var oldInput, oldOutput, oldName sql.NullString
	var oldTimestamp time.Time
	if err := db.QueryRowContext(ctx,
		"SELECT input, output, name, timestamp FROM traces FINAL WHERE id = 'old-trace'",
	).Scan(&oldInput, &oldOutput, &oldName, &oldTimestamp); err != nil {
		t.Fatalf("select old row: %v", err)
	}
	if oldInput.Valid || oldOutput.Valid {
		t.Fatalf("old row payload not blanked: input=%v output=%v", oldInput, oldOutput)
	}
	if oldName.String != "generate-resume" {
		t.Fatalf("old row lost non-payload column name: got %q", oldName.String)
	}
	if oldTimestamp.Unix() != old.Unix() {
		t.Fatalf("old row lost its timestamp: got %v want %v", oldTimestamp, old)
	}

	var recentInput, recentOutput sql.NullString
	if err := db.QueryRowContext(ctx,
		"SELECT input, output FROM traces FINAL WHERE id = 'recent-trace'",
	).Scan(&recentInput, &recentOutput); err != nil {
		t.Fatalf("select recent row: %v", err)
	}
	if !recentInput.Valid || recentInput.String != "profile and resume content" {
		t.Fatalf("recent row payload wrongly touched: input=%v", recentInput)
	}
	if !recentOutput.Valid || recentOutput.String != "generated resume" {
		t.Fatalf("recent row payload wrongly touched: output=%v", recentOutput)
	}

	report2, err := pruner.Prune(ctx)
	if err != nil {
		t.Fatalf("second prune: %v", err)
	}
	if len(report2.TablesPurged) != 0 {
		t.Fatalf("second run found more to purge: %v", report2.TablesPurged)
	}
}
