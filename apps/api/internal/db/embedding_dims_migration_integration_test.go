//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

func vectorLiteral(n int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("0.1")
	}
	b.WriteByte(']')
	return b.String()
}

func columnType(t *testing.T, table, column string) string {
	t.Helper()
	var typ string
	err := testDB.Pool.QueryRow(context.Background(),
		`SELECT format_type(a.atttypid, a.atttypmod)
		 FROM pg_attribute a
		 WHERE a.attrelid = $1::regclass AND a.attname = $2 AND NOT a.attisdropped`,
		`"`+table+`"`, column).Scan(&typ)
	if err != nil {
		t.Fatalf("read %s.%s type: %v", table, column, err)
	}
	return typ
}

func TestIntegration_Migration00044_EmbeddingColumnsAre1024(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	for _, table := range []string{"Job", "Profile"} {
		if got := columnType(t, table, "embedding"); got != "vector(1024)" {
			t.Errorf(`%q."embedding" is %s, want vector(1024)`, table, got)
		}
	}

	source := mustInsertJobSource(t, "js-044-dims", "api")
	job := mustInsertJob(t, source.Key, "dedupe-044-dims", "Backend Engineer")

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE "Job" SET "embedding" = $2::vector WHERE "id" = $1`, job.ID, vectorLiteral(1024)); err != nil {
		t.Fatalf("store a 1024-dimension job vector: %v", err)
	}
	var dims int
	if err := testDB.Pool.QueryRow(ctx,
		`SELECT vector_dims("embedding") FROM "Job" WHERE "id" = $1`, job.ID).Scan(&dims); err != nil {
		t.Fatalf("read job vector dims: %v", err)
	}
	if dims != 1024 {
		t.Errorf("stored job vector has %d dimensions, want 1024", dims)
	}

	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE "Job" SET "embedding" = $2::vector WHERE "id" = $1`, job.ID, vectorLiteral(768)); err == nil {
		t.Error("a 768-dimension vector was accepted — the column no longer enforces its width")
	}

	prof, err := testDB.Queries.CreateProfile(ctx, sqlcgen.CreateProfileParams{Name: "Dims Profile"})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := testDB.Pool.Exec(ctx,
		`UPDATE "Profile" SET "embedding" = $2::vector WHERE "id" = $1`, prof.ID, vectorLiteral(768)); err == nil {
		t.Error(`a 768-dimension vector was accepted into "Profile"."embedding"`)
	}
}

func TestIntegration_Migration00044_DiscardsPreMigrationEmbeddings(t *testing.T) {
	truncateAll(t)
	ctx := context.Background()

	source := mustInsertJobSource(t, "js-044-discard", "api")
	job := mustInsertJob(t, source.Key, "dedupe-044-discard", "Backend Engineer")
	prof, err := testDB.Queries.CreateProfile(ctx, sqlcgen.CreateProfileParams{Name: "Legacy Profile"})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}

	tx, err := testDB.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE "Job" SET "embedding" = $2::vector, "embeddingHash" = 'stale-hash', "embedModel" = 'nomic-embed-text' WHERE "id" = $1`,
		job.ID, vectorLiteral(1024)); err != nil {
		t.Fatalf("populate legacy job embedding: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE "Profile" SET "embedding" = $2::vector, "embedModel" = 'nomic-embed-text' WHERE "id" = $1`,
		prof.ID, vectorLiteral(1024)); err != nil {
		t.Fatalf("populate legacy profile embedding: %v", err)
	}

	for _, stmt := range []string{
		`ALTER TABLE "Job"     ALTER COLUMN "embedding" TYPE vector(1024) USING NULL`,
		`ALTER TABLE "Profile" ALTER COLUMN "embedding" TYPE vector(1024) USING NULL`,
		`UPDATE "Job"     SET "embeddingHash" = NULL`,
		`UPDATE "Profile" SET "embedModel" = NULL`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			t.Fatalf("replay %q: %v", stmt, err)
		}
	}

	var jobEmbedding, jobHash, jobModel *string
	if err := tx.QueryRow(ctx,
		`SELECT "embedding"::text, "embeddingHash", "embedModel" FROM "Job" WHERE "id" = $1`, job.ID).
		Scan(&jobEmbedding, &jobHash, &jobModel); err != nil {
		t.Fatalf("read job after replay: %v", err)
	}
	if jobEmbedding != nil {
		t.Error(`"Job"."embedding" survived the retype — it must be discarded, not converted`)
	}
	if jobHash != nil {
		t.Errorf(`"Job"."embeddingHash" = %q, want null so the row re-embeds on next use`, *jobHash)
	}

	var profEmbedding, profModel *string
	if err := tx.QueryRow(ctx,
		`SELECT "embedding"::text, "embedModel" FROM "Profile" WHERE "id" = $1`, prof.ID).
		Scan(&profEmbedding, &profModel); err != nil {
		t.Fatalf("read profile after replay: %v", err)
	}
	if profEmbedding != nil {
		t.Error(`"Profile"."embedding" survived the retype — it must be discarded, not converted`)
	}
	if profModel != nil {
		t.Errorf(`"Profile"."embedModel" = %q, want null`, *profModel)
	}
}
