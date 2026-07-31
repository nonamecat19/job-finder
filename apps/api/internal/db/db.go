// Package db wires the pgx connection pool and exposes the sqlc-generated
// Queries alongside the raw pool (needed for ad-hoc dynamic SQL, e.g. the
// jobs list filters and pgvector cosine-similarity lookups).
package db

import (
	"context"
	"embed"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/job-finder/api/internal/db/sqlcgen"

	stdsql "database/sql"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB bundles the pgx pool with sqlc's generated query methods.
type DB struct {
	Pool    *pgxpool.Pool
	Queries *sqlcgen.Queries
}

// PoolConfig is the explicit connection-capacity policy (026-db-pool-capacity),
// built in cmd/server/platform.go from config.Config and validated there before
// pgx sees any of it.
type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// Option mutates the parsed pgx pool configuration before the pool is opened.
type Option func(*pgxpool.Config)

// WithPoolConfig applies an explicit capacity policy. Fields it does not set
// (HealthCheckPeriod, connect timeouts, anything supplied in the DSN) keep the
// values ParseConfig produced.
func WithPoolConfig(pc PoolConfig) Option {
	return func(cfg *pgxpool.Config) {
		cfg.MaxConns = pc.MaxConns
		cfg.MinConns = pc.MinConns
		cfg.MaxConnLifetime = pc.MaxConnLifetime
		cfg.MaxConnIdleTime = pc.MaxConnIdleTime
	}
}

// Open connects to Postgres and verifies connectivity with a ping. Without
// options the pool keeps pgx's own defaults; cmd/server always passes
// WithPoolConfig so capacity is a stated decision rather than a core count.
func Open(ctx context.Context, databaseURL string, opts ...Option) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	for _, opt := range opts {
		opt(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return &DB{Pool: pool, Queries: sqlcgen.New(pool)}, nil
}

// Close releases the pool.
func (d *DB) Close() {
	d.Pool.Close()
}

// WithinTx runs fn against a *sqlcgen.Queries bound to a single transaction,
// committing on success and rolling back on any error or panic. Use it when two
// writes must not diverge — e.g. an application status update and its
// "ApplicationOutcome" event insert (spec 010).
//
// The signature deliberately takes *sqlcgen.Queries rather than a domain port
// so use-case packages can declare their own structural interface over it
// without db importing them.
func (d *DB) WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	defer func() {
		// No-op once the tx is committed; guarantees rollback on panic.
		_ = tx.Rollback(ctx)
	}()
	if err := fn(sqlcgen.New(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit tx: %w", err)
	}
	return nil
}

// Migrate runs embedded goose migrations up to the latest version. It opens
// a separate database/sql connection (goose's requirement) over the same
// DSN pgx uses.
func Migrate(databaseURL string) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	sqlDB, err := stdsql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("db: migrate open: %w", err)
	}
	defer sqlDB.Close()
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	return nil
}
