//go:build integration

package application_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	return dbtest.New(t)
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

func TestIntegration_CountRepostsByDedupeKey_ReachesAboveOne(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	q := database.Queries

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

func TestIntegration_CountAlwaysHiringByCompany_RejectedCountsAsProgression(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	q := database.Queries

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

func TestIntegration_JobSignalCascadesOnJobDelete(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()

	q := database.Queries

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
