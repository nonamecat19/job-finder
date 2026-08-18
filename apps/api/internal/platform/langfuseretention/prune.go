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

const DefaultRetentionDays = 30

var payloadTables = []struct {
	name       string
	timeColumn string
	nullable   bool
}{
	{name: "traces", timeColumn: "timestamp", nullable: true},
	{name: "observations", timeColumn: "start_time", nullable: true},
	{name: "events_full", timeColumn: "start_time", nullable: false},
	{name: "events_core", timeColumn: "start_time", nullable: false},
}

type Config struct {
	URL string

	User     string
	Password string

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

type Report struct {
	Skipped bool

	Cutoff time.Time

	TablesPurged map[string]int64
}

type Pruner struct {
	cfg Config
	db  *sql.DB
}

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
