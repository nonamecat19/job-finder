//go:build integration

package application_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/salary/domain"
)

func TestIntegration_SalaryCacheUpsert(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}

	database, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	q := database.Queries

	bucket := "test-title|test-location|unknown"
	band := domain.SalaryBand{
		Min:        100000,
		Max:        150000,
		Currency:   "USD",
		Confidence: 0.5,
		Source:     domain.SourceIngestedCache,
	}

	err = q.UpsertSalaryCache(ctx, sqlcgen.UpsertSalaryCacheParams{
		Bucket:     bucket,
		SalaryMin:  int32Ptr(int32(band.Min)),
		SalaryMax:  int32Ptr(int32(band.Max)),
		Currency:   band.Currency,
		Source:     string(band.Source),
		SampleSize: 1,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	rows, err := q.GetSalaryCacheByBucket(ctx, bucket)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Currency != "USD" {
		t.Errorf("expected USD, got %s", rows[0].Currency)
	}
	if rows[0].Source != string(domain.SourceIngestedCache) {
		t.Errorf("expected ingested-cache, got %s", rows[0].Source)
	}

	err = q.UpsertSalaryCache(ctx, sqlcgen.UpsertSalaryCacheParams{
		Bucket:     bucket,
		SalaryMin:  int32Ptr(110000),
		SalaryMax:  int32Ptr(160000),
		Currency:   "USD",
		Source:     string(domain.SourceIngestedCache),
		SampleSize: 1,
	})
	if err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	rows, err = q.GetSalaryCacheByBucket(ctx, bucket)
	if err != nil {
		t.Fatalf("get 2: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after upsert, got %d", len(rows))
	}
	if rows[0].SampleSize != 2 {
		t.Errorf("expected sampleSize 2, got %d", rows[0].SampleSize)
	}
}

func TestIntegration_JobSalaryPersistence(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}

	database, err := db.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	q := database.Queries

	dedupeKey := "test-salary-persist-" + t.Name() + time.Now().Format("20060102150405")
	job, err := q.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey:   dedupeKey,
		SourceKey:   "test",
		Title:       "Test Engineer",
		Company:     "TestCo",
		Url:         "https://example.com",
		Description: "Test job",
		Raw:         []byte("{}"),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}

	err = q.UpdateJobSalary(ctx, sqlcgen.UpdateJobSalaryParams{
		ID:               job.ID,
		SalaryMin:        int32Ptr(80000),
		SalaryMax:        int32Ptr(120000),
		SalaryCurrency:   strPtr("USD"),
		SalaryConfidence: float64Ptr(0.5),
		SalarySource:     strPtr(string(domain.SourceIngestedCache)),
	})
	if err != nil {
		t.Fatalf("update salary: %v", err)
	}

	updated, err := q.GetJobByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updated.SalaryMin == nil || *updated.SalaryMin != 80000 {
		t.Errorf("expected min 80000, got %v", updated.SalaryMin)
	}
	if updated.SalaryMax == nil || *updated.SalaryMax != 120000 {
		t.Errorf("expected max 120000, got %v", updated.SalaryMax)
	}
	if updated.SalaryCurrency == nil || *updated.SalaryCurrency != "USD" {
		t.Errorf("expected USD, got %v", updated.SalaryCurrency)
	}
	if updated.SalarySource == nil || *updated.SalarySource != string(domain.SourceIngestedCache) {
		t.Errorf("expected ingested-cache, got %v", updated.SalarySource)
	}
}
