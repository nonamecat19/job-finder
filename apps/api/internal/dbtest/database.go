//go:build integration

// Package dbtest holds shared helpers for the integration test suites. It is
// compiled only under the `integration` build tag and is never linked into the
// production binary.
//
// Every suite gets its own physical database, cloned from a migrated template.
// That is what lets `go test ./...` keep running packages in parallel: suites
// no longer share tables, so one package's TRUNCATE or fixture cannot disturb
// another's, and no cross-package locking is needed.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db"
)

// defaultDSN matches the compose development database, so a suite run without
// DATABASE_URL behaves the way the rest of the repo's tooling does.
const defaultDSN = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"

// templateLockKey serialises template creation and cloning across the parallel
// package binaries `go test ./...` starts. It is held for milliseconds, not for
// the duration of a suite.
const templateLockKey = 0x104B_F1DE

// setupTimeout bounds template creation and the clone, which are local
// operations on a small schema.
const setupTimeout = 2 * time.Minute

var (
	maintenanceOnce sync.Once
	maintenancePool *pgxpool.Pool
	maintenanceErr  error

	templateOnce sync.Once
	templateErr  error
)

// New returns a *db.DB backed by a fresh database of its own, migrated to the
// current schema and empty of rows. The database is dropped when the test ends.
//
// Suites should call this instead of db.Open: an isolated database removes the
// need to TRUNCATE shared tables, which is what used to make parallel packages
// clobber each other.
func New(t *testing.T) *db.DB {
	t.Helper()
	database, _ := NewWithDSN(t)
	return database
}

// NewWithDSN is New for the few suites that also need the connection string,
// for example to hand it to code that opens its own pool.
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

// NewForMain is New for suites whose fixtures are shared by every test in the
// package and are therefore built in TestMain, where no *testing.T exists. The
// returned release function closes the pool and drops the database; callers
// that end in os.Exit must run it before exiting, since os.Exit skips defers.
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

// ensureTemplate creates the migrated template database once per process, and
// once per database across processes: parallel package binaries race here, so
// the work happens under an advisory lock and every loser of the race finds the
// template already present.
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
		// Migrate opens and closes its own connection, so the template is left
		// with no sessions attached — CREATE DATABASE ... TEMPLATE requires that.
		if err := db.Migrate(dsn); err != nil {
			templateErr = fmt.Errorf("migrate template %s: %w", name, err)
			return
		}
	})
	return templateErr
}

// cloneTemplate copies the template into a new database. Postgres rejects a
// clone while another session is connected to the source, and a concurrent
// clone counts, so the copies take turns on the same advisory lock the template
// bootstrap uses.
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

// dropDatabase removes a suite's database. FORCE terminates any connection the
// suite leaked, so a stray session cannot keep the database (and its disk)
// around for the rest of the run.
func dropDatabase(name string) {
	pool, err := maintenance()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = pool.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
}

// maintenance returns the pool used for CREATE/DROP DATABASE, which cannot run
// against the database being created. It points at the DATABASE_URL database,
// which exists for the whole run and is never a clone target.
func maintenance() (*pgxpool.Pool, error) {
	maintenanceOnce.Do(func() {
		cfg, err := pgxpool.ParseConfig(baseDSN())
		if err != nil {
			maintenanceErr = fmt.Errorf("parse DATABASE_URL: %w", err)
			return
		}
		cfg.MaxConns = 4
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		maintenancePool, maintenanceErr = pgxpool.NewWithConfig(ctx, cfg)
	})
	return maintenancePool, maintenanceErr
}

func baseDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return defaultDSN
}

// templateName derives the template's name from the base database, so parallel
// runs against different databases (a developer's and CI's, say) do not share
// one template.
func templateName() (string, error) {
	u, err := url.Parse(baseDSN())
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	base := strings.TrimPrefix(u.Path, "/")
	if base == "" {
		return "", fmt.Errorf("DATABASE_URL has no database name")
	}
	return truncateName(base + "_tmpl"), nil
}

func dsnFor(name string) (string, error) {
	u, err := url.Parse(baseDSN())
	if err != nil {
		return "", fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	u.Path = "/" + name
	return u.String(), nil
}

// databaseName builds a unique, legal identifier from the caller's label. The
// label is only there to make a leftover database traceable to its suite.
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

// truncateName keeps identifiers inside Postgres's 63-byte limit, trimming the
// front so the random suffix — the part that makes the name unique — survives.
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
