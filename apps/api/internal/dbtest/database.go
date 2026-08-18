//go:build integration

package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/testinfra"
)

const templateLockKey = 0x104B_F1DE

const setupTimeout = 2 * time.Minute

var (
	maintenanceOnce sync.Once
	maintenancePool *pgxpool.Pool
	maintenanceErr  error

	templateOnce sync.Once
	templateErr  error
)

func New(t *testing.T) *db.DB {
	t.Helper()
	database, _ := NewWithDSN(t)
	return database
}

func NewWithDSN(t *testing.T) (*db.DB, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	database, dsn, release, err := newDatabase(ctx, t.Name())
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	t.Cleanup(release)
	return database, dsn
}

func NewForMain(label string) (*db.DB, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	database, _, release, err := newDatabase(ctx, label)
	if err != nil {
		return nil, nil, err
	}
	return database, release, nil
}

func newDatabase(ctx context.Context, label string) (*db.DB, string, func(), error) {
	if err := ensureTemplate(ctx); err != nil {
		return nil, "", nil, err
	}

	name, err := databaseName(label)
	if err != nil {
		return nil, "", nil, err
	}
	if err := cloneTemplate(ctx, name); err != nil {
		return nil, "", nil, fmt.Errorf("clone template into %s: %w", name, err)
	}

	dsn, err := dsnFor(name)
	if err != nil {
		return nil, "", nil, err
	}
	database, err := db.Open(ctx, dsn)
	if err != nil {
		dropDatabase(name)
		return nil, "", nil, fmt.Errorf("open %s: %w", name, err)
	}

	return database, dsn, func() {
		database.Close()
		dropDatabase(name)
	}, nil
}

func ensureTemplate(ctx context.Context) error {
	templateOnce.Do(func() {
		pool, err := maintenance()
		if err != nil {
			templateErr = err
			return
		}
		conn, err := pool.Acquire(ctx)
		if err != nil {
			templateErr = err
			return
		}
		defer conn.Release()

		if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", templateLockKey); err != nil {
			templateErr = fmt.Errorf("lock template: %w", err)
			return
		}
		defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", templateLockKey) }()

		name, err := templateName()
		if err != nil {
			templateErr = err
			return
		}
		if _, err := conn.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil && !isDuplicateDatabase(err) {
			templateErr = fmt.Errorf("create template %s: %w", name, err)
			return
		}

		dsn, err := dsnFor(name)
		if err != nil {
			templateErr = err
			return
		}
		if err := db.Migrate(dsn); err != nil {
			templateErr = fmt.Errorf("migrate template %s: %w", name, err)
			return
		}
	})
	return templateErr
}

func cloneTemplate(ctx context.Context, name string) error {
	template, err := templateName()
	if err != nil {
		return err
	}
	pool, err := maintenance()
	if err != nil {
		return err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", templateLockKey); err != nil {
		return fmt.Errorf("lock template: %w", err)
	}
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", templateLockKey) }()

	_, err = conn.Exec(ctx, `CREATE DATABASE "`+name+`" TEMPLATE "`+template+`"`)
	return err
}

func dropDatabase(name string) {
	pool, err := maintenance()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = pool.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
}

func maintenance() (*pgxpool.Pool, error) {
	maintenanceOnce.Do(func() {
		base, err := baseDSN()
		if err != nil {
			maintenanceErr = err
			return
		}
		cfg, err := pgxpool.ParseConfig(base)
		if err != nil {
			maintenanceErr = fmt.Errorf("parse container DSN: %w", err)
			return
		}
		cfg.MaxConns = 4
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		maintenancePool, maintenanceErr = pgxpool.NewWithConfig(ctx, cfg)
	})
	return maintenancePool, maintenanceErr
}

func baseDSN() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	return testinfra.PostgresDSN(ctx)
}

func templateName() (string, error) {
	base, err := baseDSN()
	if err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse container DSN: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("container DSN has no database name")
	}
	return truncateName(name + "_tmpl"), nil
}

func dsnFor(name string) (string, error) {
	base, err := baseDSN()
	if err != nil {
		return "", err
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse container DSN: %w", err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

func databaseName(label string) (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '_'
		}
	}, label)
	return truncateName("it_" + slug + "_" + hex.EncodeToString(suffix[:])), nil
}

func truncateName(name string) string {
	const maxIdentifier = 63
	if len(name) <= maxIdentifier {
		return name
	}
	return name[len(name)-maxIdentifier:]
}

func isDuplicateDatabase(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "42P04"
}
