//go:build integration

package ghostjob_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}
	database, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return database
}

func insertTestJob(t *testing.T, q *sqlcgen.Queries, dedupeKey, company string) sqlcgen.Job {
	t.Helper()
	job, err := q.InsertJob(context.Background(), sqlcgen.InsertJobParams{
		DedupeKey:   dedupeKey,
		SourceKey:   "test",
		Title:       "Test Engineer",
		Company:     company,
		Url:         "https://example.com/" + dedupeKey,
		Description: "Test job for ghostjob integration coverage",
		Raw:         []byte("{}"),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	return job
}

// FR-002a / FR-003: repost count must be reachable above 1. This exercises
// the real path — RecordJobRepost (ingestion's duplicate-hit branch) bumping
// "seenCount" — rather than the trap of counting Job rows by dedupeKey,
// which can only ever return 1 because "dedupeKey" is UNIQUE.
func TestIntegration_CountRepostsByDedupeKey_ReachesAboveOne(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()
	q := database.Queries
	ctx := context.Background()

	dedupeKey := "ghostjob-repost-" + time.Now().Format("20060102150405.000000")
	job := insertTestJob(t, q, dedupeKey, "RepostCo")

	if _, err := q.RecordJobRepost(ctx, sqlcgen.RecordJobRepostParams{DedupeKey: dedupeKey}); err != nil {
		t.Fatalf("record repost 1: %v", err)
	}
	if _, err := q.RecordJobRepost(ctx, sqlcgen.RecordJobRepostParams{DedupeKey: dedupeKey}); err != nil {
		t.Fatalf("record repost 2: %v", err)
	}

	got, err := q.CountRepostsByDedupeKey(ctx, dedupeKey)
	if err != nil {
		t.Fatalf("count reposts: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected seenCount 3 (1 initial + 2 reposts), got %d", got)
	}
	_ = job
}

// FR-006: a rejected application counts as progression, not as
// unprogressed — the posting got a real answer, it wasn't ignored.
func TestIntegration_CountAlwaysHiringByCompany_RejectedCountsAsProgression(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()
	q := database.Queries
	ctx := context.Background()

	company := "AlwaysHiringCo-" + time.Now().Format("20060102150405.000000")

	unprogressed := insertTestJob(t, q, "ghostjob-ah-unprog-"+company, company)
	rejected := insertTestJob(t, q, "ghostjob-ah-rejected-"+company, company)

	events, _ := json.Marshal([]map[string]string{{"status": "rejected", "at": time.Now().UTC().Format(time.RFC3339)}})
	if err := q.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId: rejected.ID, Status: "rejected", Events: events,
	}); err != nil {
		t.Fatalf("upsert application status: %v", err)
	}

	got, err := q.CountAlwaysHiringByCompany(ctx, company)
	if err != nil {
		t.Fatalf("count always hiring: %v", err)
	}
	if got != 1 {
		t.Fatalf("expected 1 unprogressed job (rejected excluded), got %d", got)
	}
	_ = unprogressed
}

// SC-010: deleting a job removes its "JobSignal" row too — zero orphans.
func TestIntegration_JobSignalCascadesOnJobDelete(t *testing.T) {
	database := openTestDB(t)
	defer database.Close()
	q := database.Queries
	ctx := context.Background()

	job := insertTestJob(t, q, "ghostjob-cascade-"+time.Now().Format("20060102150405.000000"), "CascadeCo")
	model := "test-model"
	if _, err := q.UpsertJobSignal(ctx, sqlcgen.UpsertJobSignalParams{
		JobId: job.ID, Kind: "ghost", Score: 42, Signals: []byte(`{}`), Model: &model,
	}); err != nil {
		t.Fatalf("upsert job signal: %v", err)
	}

	if _, err := q.DeleteAllJobs(ctx); err != nil {
		t.Fatalf("delete all jobs: %v", err)
	}

	_, err := q.GetJobSignal(ctx, sqlcgen.GetJobSignalParams{JobId: job.ID, Kind: "ghost"})
	if err == nil {
		t.Fatal("expected the JobSignal row to be gone after cascading delete")
	}
	_ = dbutil.UUIDString(job.ID)
}
