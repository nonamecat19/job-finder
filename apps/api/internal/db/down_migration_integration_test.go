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
