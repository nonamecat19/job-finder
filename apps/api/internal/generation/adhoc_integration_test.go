//go:build integration

package generation_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

// TestAdHocDocuments_NullJobId covers the migration + query contract that
// generation.Service.GenerateAdHoc/ListAdHocDocuments rely on: a
// GeneratedDocument row with no Job (ad-hoc, pasted-vacancy generation) must
// persist with jobId NULL and be listable separately from job-tied rows.
func TestAdHocDocuments_NullJobId(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	testDB, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer testDB.Close()

	for _, tbl := range []string{"GeneratedDocument", "Job", "JobSource"} {
		if _, err := testDB.Pool.Exec(ctx, `TRUNCATE TABLE "`+tbl+`" CASCADE`); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}

	company, title, vacancy := "Acme Inc", "Senior Engineer", "We are looking for a senior engineer..."
	adhoc, err := testDB.Queries.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		Type: string(dto.DocumentTypeResume), Version: 1, Content: []byte(`{}`), Model: "test-model",
		Company: &company, Title: &title, Vacancy: &vacancy,
	})
	if err != nil {
		t.Fatalf("insert ad-hoc document: %v", err)
	}
	if adhoc.JobId.Valid {
		t.Fatalf("expected ad-hoc document to have NULL jobId, got %v", adhoc.JobId)
	}

	// A job-tied document must not show up in the ad-hoc listing.
	if err := testDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key: "adhoc-src", Kind: "api", Config: []byte(`{}`),
	}); err != nil {
		t.Fatalf("upsert job source: %v", err)
	}
	job, err := testDB.Queries.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey: "adhoc-dedupe-1", SourceKey: "adhoc-src", Title: "Engineer", Company: "OtherCo",
		Url: "https://example.com/job/adhoc-dedupe-1", Description: "desc", Raw: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if _, err := testDB.Queries.InsertGeneratedDocument(ctx, sqlcgen.InsertGeneratedDocumentParams{
		JobId: job.ID, Type: string(dto.DocumentTypeResume), Version: 1, Content: []byte(`{}`), Model: "test-model",
	}); err != nil {
		t.Fatalf("insert job-tied document: %v", err)
	}

	rows, err := testDB.Queries.ListAdHocDocuments(ctx)
	if err != nil {
		t.Fatalf("list ad-hoc documents: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 ad-hoc document, got %d", len(rows))
	}
	if dbutil.UUIDString(rows[0].ID) != dbutil.UUIDString(adhoc.ID) {
		t.Fatalf("expected listed document to be the ad-hoc one")
	}
	if rows[0].Company == nil || *rows[0].Company != company {
		t.Fatalf("expected company %q, got %v", company, rows[0].Company)
	}
}
