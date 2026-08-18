// Package langfuseretention enforces FR-018/FR-018a of feature 047: the
// self-hosted Langfuse trace store must forget the profile and resume
// content it carries as step input/output after LANGFUSE_PAYLOAD_RETENTION_DAYS,
// while step sequence, timings, model tier, token counts, cost and outcome
// survive indefinitely (FR-018).
//
// This is deliberately not the same job as internal/platform/observability
// (036 FR-008). That package deletes whole trace rows through Langfuse's
// public API — the only retention primitive the public API exposes. FR-018
// asks for something the public API cannot do: keep the row, blank only the
// payload. Langfuse's own "Data Retention" project setting cannot do it
// either — it is an Enterprise-licensed feature on self-hosted instances,
// and even when licensed it deletes whole trace/observation/score rows the
// same way the public API delete does (confirmed against Langfuse's OSS
// GitHub discussions as of 2026-08; there is no first-party self-hosted
// mechanism that purges only input/output). So this job talks to Langfuse
// v4's own ClickHouse store directly, blanking the `input`/`output` columns
// on rows older than the cutoff while leaving every other column untouched.
//
// The exact columns this targets were read from Langfuse's pinned image
// version (langfuse/langfuse:4.6.0, packages/shared/clickhouse/migrations)
// rather than guessed: `traces` and `observations` (the tables that exist
// from v3 onward) carry `input`/`output` as separate Nullable(String)
// columns from `metadata`/`usage_details`/`cost_details`/timings; the newer
// `events_full`/`events_core` tables (v4's denormalized event store) carry
// the same separation with non-nullable String columns. A deployment may
// have either pair populated depending on ingestion path, so this job
// targets all four and skips any that do not exist in this instance.
//
// The ClickHouse schema is explicitly not a stable API contract across
// Langfuse releases (its own engineering docs say as much), so a Langfuse
// upgrade can require revisiting the table/column names below.
package langfuseretention

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

// DefaultRetentionDays is the window FR-018 promises when nothing is
// configured.
const DefaultRetentionDays = 30

// payloadTables enumerates the ClickHouse tables Langfuse 4.6.0 may have
// populated with trace/observation payloads, and how to reach each one's
// cutoff timestamp and payload columns. See the package doc for how these
// were determined.
var payloadTables = []struct {
	name       string
	timeColumn string
	nullable   bool // Nullable(String) columns use NULL as "purged"; plain String columns use ''.
}{
	{name: "traces", timeColumn: "timestamp", nullable: true},
	{name: "observations", timeColumn: "start_time", nullable: true},
	{name: "events_full", timeColumn: "start_time", nullable: false},
	{name: "events_core", timeColumn: "start_time", nullable: false},
}

// Config is what the job needs to reach Langfuse's ClickHouse store.
//
// These are deliberately not named LANGFUSE_CLICKHOUSE_* on the application
// container: 036 contracts C2-2 forbids granting the application container
// any LANGFUSE_*-named variable, since that prefix is reserved for
// credentials that could act on Langfuse's ingestion API on the
// application's behalf. This job's credential reaches ClickHouse only, for
// deletion only, so it is named and granted separately — the same reasoning
// that named 036's collector-deletion key EVAL_PRUNE_* instead of LANGFUSE_*.
type Config struct {
	// URL is a ClickHouse DSN, e.g. "clickhouse://clickhouse:9000/default".
	// Empty disables the job entirely.
	URL string
	// User and Password authenticate against ClickHouse. Both optional —
	// Langfuse's own ClickHouse deployment may run without auth.
	User     string
	Password string
	// RetentionDays is the window (FR-018, LANGFUSE_PAYLOAD_RETENTION_DAYS).
	// Zero or negative means DefaultRetentionDays.
	RetentionDays int
}

func (c Config) enabled() bool {
	return c.URL != ""
}

func (c Config) window() time.Duration {
	days := c.RetentionDays
	if days <= 0 {
		days = DefaultRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// dsn builds the connection string clickhouse-go's database/sql driver
// accepts, layering in User/Password when the URL itself carries none.
func (c Config) dsn() (string, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return "", fmt.Errorf("parse clickhouse url: %w", err)
	}
	if c.User != "" && u.User == nil {
		if c.Password != "" {
			u.User = url.UserPassword(c.User, c.Password)
		} else {
			u.User = url.User(c.User)
		}
	}
	return u.String(), nil
}

// Report is what one pruning run did (K5-3: the guarantee must be
// verifiable, not merely claimed).
type Report struct {
	// Skipped is true when no ClickHouse URL is configured — the default,
	// since Langfuse itself is dev-only self-hosted (docker-compose.prod.yml
	// does not deploy the collector group).
	Skipped bool
	// Cutoff is the boundary this run used.
	Cutoff time.Time
	// TablesPurged lists the tables this run found rows to blank in, with
	// the row count blanked in each.
	TablesPurged map[string]int64
}

// Pruner blanks Langfuse trace/observation payload columns older than the
// retention window, leaving every other column in place.
type Pruner struct {
	cfg Config
	db  *sql.DB
}

// New builds a pruner. The ClickHouse connection is opened lazily on first
// Prune call so a deployment with no collector configured never dials out.
func New(cfg Config) *Pruner {
	return &Pruner{cfg: cfg}
}

func (p *Pruner) connect() error {
	if p.db != nil {
		return nil
	}
	dsn, err := p.cfg.dsn()
	if err != nil {
		return err
	}
	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return fmt.Errorf("open clickhouse: %w", err)
	}
	p.db = db
	return nil
}

// Prune blanks input/output on every payload table's rows older than the
// window and reports what it changed.
//
// Errors are returned, never swallowed: a retention job that fails silently
// is worse than none, since the documentation keeps claiming the window is
// enforced while payloads accumulate.
func (p *Pruner) Prune(ctx context.Context) (Report, error) {
	if !p.cfg.enabled() {
		return Report{Skipped: true}, nil
	}
	if err := p.connect(); err != nil {
		return Report{}, err
	}

	cutoff := time.Now().UTC().Add(-p.cfg.window())
	report := Report{Cutoff: cutoff, TablesPurged: map[string]int64{}}

	for _, t := range payloadTables {
		exists, err := p.tableExists(ctx, t.name)
		if err != nil {
			return report, fmt.Errorf("check table %s exists: %w", t.name, err)
		}
		if !exists {
			continue
		}
		n, err := p.purgeTable(ctx, t.name, t.timeColumn, t.nullable, cutoff)
		if err != nil {
			return report, fmt.Errorf("purge table %s: %w", t.name, err)
		}
		if n > 0 {
			report.TablesPurged[t.name] = n
		}
	}

	return report, nil
}

func (p *Pruner) tableExists(ctx context.Context, name string) (bool, error) {
	var n int
	err := p.db.QueryRowContext(ctx,
		"SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = ?", name,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// purgeTable blanks input/output for rows older than cutoff that still carry
// a payload. It counts affected rows first (ClickHouse mutations do not
// return a row count) purely so the run is observable (K5-3); the count
// query and the mutation can race with concurrent ingestion, so the reported
// count is a lower bound, not a guarantee of atomicity — acceptable for a
// best-effort retention sweep that reruns every tick.
func (p *Pruner) purgeTable(ctx context.Context, table, timeColumn string, nullable bool, cutoff time.Time) (int64, error) {
	var emptyPayload string
	if nullable {
		emptyPayload = "input IS NULL AND output IS NULL"
	} else {
		emptyPayload = "input = '' AND output = ''"
	}

	countQuery := fmt.Sprintf(
		"SELECT count() FROM %s WHERE %s < ? AND NOT (%s)",
		table, timeColumn, emptyPayload,
	)
	var n int64
	if err := p.db.QueryRowContext(ctx, countQuery, cutoff).Scan(&n); err != nil {
		return 0, fmt.Errorf("count rows to purge: %w", err)
	}
	if n == 0 {
		return 0, nil
	}

	var setClause string
	if nullable {
		setClause = "input = NULL, output = NULL"
	} else {
		setClause = "input = '', output = ''"
	}

	// mutations_sync = 1 makes the ALTER TABLE UPDATE block until applied,
	// which is what lets this run's Report claim the purge actually
	// happened rather than merely having been scheduled (K5-3).
	updateQuery := fmt.Sprintf(
		"ALTER TABLE %s UPDATE %s WHERE %s < ? AND NOT (%s) SETTINGS mutations_sync = 1",
		table, setClause, timeColumn, emptyPayload,
	)
	if _, err := p.db.ExecContext(ctx, updateQuery, cutoff); err != nil {
		return 0, fmt.Errorf("blank payload: %w", err)
	}

	slog.Info("langfuseretention: purged payload", "table", table, "rows", n, "cutoff", cutoff.Format(time.RFC3339))
	return n, nil
}
