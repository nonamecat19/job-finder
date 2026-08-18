//go:build integration

package activity_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
)

func insertRun(ctx context.Context, t *testing.T, testDB *db.DB, state string, heartbeat *time.Time) pgtype.UUID {
	t.Helper()
	row, err := testDB.Queries.InsertActivityRun(ctx, sqlcgen.InsertActivityRunParams{
		Op:    "match",
		Label: "test run",
		Meta:  []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert activity run: %v", err)
	}

	var hb pgtype.Timestamp
	if heartbeat != nil {
		hb = pgtype.Timestamp{Time: heartbeat.UTC(), Valid: true}
	}
	_, err = testDB.Pool.Exec(ctx, `UPDATE "ActivityRun" SET "state" = $2, "heartbeatAt" = $3 WHERE "id" = $1`, row.ID, state, hb)
	if err != nil {
		t.Fatalf("set state: %v", err)
	}
	return row.ID
}

func TestSweeper_Integration_ExactlyStaleRowsInterrupted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	testDB := dbtest.New(t)

	stale := time.Now().Add(-10 * time.Minute)
	fresh := time.Now()

	staleID := insertRun(ctx, t, testDB, "running", &stale)
	nullHeartbeatID := insertRun(ctx, t, testDB, "running", nil)
	freshID := insertRun(ctx, t, testDB, "running", &fresh)
	succeededID := insertRun(ctx, t, testDB, "succeeded", &stale)

	sweeper := activity.NewSweeper(testDB.Queries, 2*time.Minute, time.Minute, 30*time.Minute)
	sweeper.Run(sweepOnceCtx(t))

	rows, err := testDB.Queries.ListRecentActivityRuns(ctx, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	states := map[string]string{}
	for _, r := range rows {
		states[uuidString(r.ID)] = r.State
	}

	if got := states[uuidString(staleID)]; got != "interrupted" {
		t.Errorf("stale running row: expected interrupted, got %q", got)
	}
	if got := states[uuidString(nullHeartbeatID)]; got != "interrupted" {
		t.Errorf("null-heartbeat running row: expected interrupted, got %q", got)
	}
	if got := states[uuidString(freshID)]; got != "running" {
		t.Errorf("fresh running row: expected untouched (running), got %q", got)
	}
	if got := states[uuidString(succeededID)]; got != "succeeded" {
		t.Errorf("terminal row: expected untouched (succeeded), got %q", got)
	}
}

func sweepOnceCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

func uuidString(id pgtype.UUID) string {
	b := id.Bytes
	return string(b[:])
}
