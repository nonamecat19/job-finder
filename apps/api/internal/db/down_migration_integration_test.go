//go:build integration

package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/testinfra"
)

// Every migration in internal/db/migrations declares a `-- +goose Down`
// block, and until this file nothing ever ran one: `goose up` is the only
// path exercised by the application, by CI and by dbtest's template. A Down
// that drops the wrong table, forgets a column it added, or is simply invalid
// SQL therefore only fails when someone is rolling back a bad deploy, which
// is the worst possible moment to discover it.
//
// These tests run against a database of their own on the shared Postgres
// container, created and dropped here rather than through internal/dbtest:
// dbtest hands out clones of an already-migrated template, and what is under
// test is migration itself.

func migrationScratchDB(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	base, err := testinfra.PostgresDSN(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	defer admin.Close()

	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("random name: %v", err)
	}
	name := "migrate_" + hex.EncodeToString(suffix[:])
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pool, err := pgxpool.New(cleanup, base)
		if err != nil {
			return
		}
		defer pool.Close()
		_, _ = pool.Exec(cleanup, `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

func tableNames(t *testing.T, dsn string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		// goose's own bookkeeping table is not part of the schema under
		// test and legitimately survives a full rollback.
		if name == "goose_db_version" {
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	sort.Strings(names)
	return names
}

func columnSignature(t *testing.T, dsn string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `
		SELECT table_name || '.' || column_name || ':' || data_type || ':' || is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name <> 'goose_db_version'
		ORDER BY 1`)
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	defer rows.Close()

	var signature []string
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		signature = append(signature, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns: %v", err)
	}
	return signature
}

// TestMigrationsRollBackCompletely proves every Down block runs, in order,
// against the schema its Up produced, and that the result is an empty schema
// — no table, view or column left behind by a Down that forgot half of what
// its Up created.
func TestMigrationsRollBackCompletely(t *testing.T) {
	dsn := migrationScratchDB(t)

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if len(tableNames(t, dsn)) == 0 {
		t.Fatal("migrating up produced no tables")
	}

	if err := db.MigrateDownTo(dsn, 0); err != nil {
		t.Fatalf("migrate down to 0: %v", err)
	}

	// 00027_drop_djinni_dashboard_subs.sql keeps "DjinniLegacySubAudit" on
	// purpose: its Up deletes rows irreversibly and the audit table is the
	// only record of what went, so its Down says so and drops nothing. That
	// is the one artifact a full rollback may leave.
	retained := map[string]bool{"DjinniLegacySubAudit": true}
	var unexpected []string
	for _, name := range tableNames(t, dsn) {
		if !retained[name] {
			unexpected = append(unexpected, name)
		}
	}
	if len(unexpected) != 0 {
		t.Fatalf("rolling every migration back left %d tables behind: %s", len(unexpected), strings.Join(unexpected, ", "))
	}
}

// TestMigrationsAreReapplicableAfterRollback proves a rollback leaves the
// database in a state the same migrations can be applied to again, and that
// what comes back is the identical schema — the property an operator relies
// on when rolling back a bad deploy and rolling forward again after the fix.
func TestMigrationsAreReapplicableAfterRollback(t *testing.T) {
	dsn := migrationScratchDB(t)

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("first migrate up: %v", err)
	}
	before := columnSignature(t, dsn)

	if err := db.MigrateDownTo(dsn, 0); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("second migrate up: %v", err)
	}
	after := columnSignature(t, dsn)

	if len(before) != len(after) {
		t.Fatalf("schema after down+up has %d columns, first pass had %d", len(after), len(before))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("column %d differs after down+up: %q then %q", i, before[i], after[i])
		}
	}
}

// TestMigrateIsIdempotent proves running the migrator against an
// already-current database is a no-op rather than an error — what happens on
// every API container start, and on every dbtest template reuse.
func TestMigrateIsIdempotent(t *testing.T) {
	dsn := migrationScratchDB(t)

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	first := columnSignature(t, dsn)
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	second := columnSignature(t, dsn)

	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatal("running the migrator twice changed the schema")
	}
}
