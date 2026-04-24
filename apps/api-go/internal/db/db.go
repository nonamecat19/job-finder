// Package db wires the pgx connection pool and exposes the sqlc-generated
// Queries alongside the raw pool (needed for ad-hoc dynamic SQL, e.g. the
// jobs list filters and pgvector cosine-similarity lookups).
package db

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/job-finder/api-go/internal/db/sqlcgen"

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

// Open connects to Postgres and verifies connectivity with a ping.
func Open(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
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
