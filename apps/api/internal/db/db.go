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

type DB struct {
	Pool    *pgxpool.Pool
	Queries *sqlcgen.Queries
}

type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

type Option func(*pgxpool.Config)

func WithPoolConfig(pc PoolConfig) Option {
	return func(cfg *pgxpool.Config) {
		cfg.MaxConns = pc.MaxConns
		cfg.MinConns = pc.MinConns
		cfg.MaxConnLifetime = pc.MaxConnLifetime
		cfg.MaxConnIdleTime = pc.MaxConnIdleTime
	}
}

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

func (d *DB) Close() {
	d.Pool.Close()
}

func (d *DB) WithinTx(ctx context.Context, fn func(*sqlcgen.Queries) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}
	defer func() {
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
