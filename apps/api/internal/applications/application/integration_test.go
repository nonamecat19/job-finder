//go:build integration

package application_test

import (
	"context"
	"os"
	"testing"

	"github.com/job-finder/api/internal/applications/application"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

var itDB *db.DB

func TestMain(m *testing.M) {
	// This suite owns its database (internal/dbtest), so seedApplication can
	// TRUNCATE between tests without disturbing packages running in parallel.
	database, release, err := dbtest.NewForMain("applications")
	if err != nil {
		panic("dbtest: " + err.Error())
	}
	itDB = database

	code := m.Run()
	release()
	os.Exit(code)
}

// seedApplication creates the job source / job / application chain the service
// needs and returns the application's id as a string.
func seedApplication(t *testing.T, key string) string {
	t.Helper()
	ctx := context.Background()
	for _, tbl := range []string{`"ApplicationOutcome"`, `"Application"`, `"Job"`, `"JobSource"`} {
		if _, err := itDB.Pool.Exec(ctx, "TRUNCATE TABLE "+tbl+" CASCADE"); err != nil {
			t.Fatalf("truncate %s: %v", tbl, err)
		}
	}
	if err := itDB.Queries.UpsertJobSource(ctx, sqlcgen.UpsertJobSourceParams{
		Key: key, Kind: "api", Config: []byte(`{}`),
	}); err != nil {
		t.Fatalf("upsert source: %v", err)
	}
	job, err := itDB.Queries.InsertJob(ctx, sqlcgen.InsertJobParams{
		DedupeKey: key + "-dedupe", SourceKey: key, Title: "Outcome Service Job",
		Company: "TestCo", Url: "https://example.com/" + key, Description: "d", Raw: []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	if err := itDB.Queries.UpsertApplicationStatus(ctx, sqlcgen.UpsertApplicationStatusParams{
		JobId: job.ID, Status: "shortlisted", Events: []byte(`[]`),
	}); err != nil {
		t.Fatalf("upsert application: %v", err)
	}
	app, err := itDB.Queries.GetApplicationByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	return dbutil.UUIDString(app.ID)
}

func statusOf(s dto.ApplicationStatus) *dto.ApplicationStatus { return &s }

// TestUpdateWritesOutcomeLogThroughTx drives the real service against Postgres
// with *db.DB as the TxRunner, proving the status write and the outcome event
// commit together and read back in order.
func TestUpdateWritesOutcomeLogThroughTx(t *testing.T) {
	ctx := context.Background()
	id := seedApplication(t, "svc-outcome")
	svc := application.NewService(itDB.Queries, itDB)

	for _, st := range []dto.ApplicationStatus{dto.StatusApplied, dto.StatusInterview, dto.StatusRejected} {
		if _, err := svc.Update(ctx, id, application.UpdateInput{Status: statusOf(st)}); err != nil {
			t.Fatalf("update to %s: %v", st, err)
		}
	}

	timeline, err := svc.Timeline(ctx, id)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	want := []dto.OutcomeEventType{dto.OutcomeApplied, dto.OutcomeScreen, dto.OutcomeRejected}
	if len(timeline) != len(want) {
		t.Fatalf("expected %d events, got %d", len(want), len(timeline))
	}
	for i, w := range want {
		if timeline[i].EventType != w {
			t.Fatalf("event %d = %s, want %s", i, timeline[i].EventType, w)
		}
	}

	// The log is additive: "Application"."status" still holds the latest state
	// and "appliedAt" is set from the applied transition.
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	app, err := itDB.Queries.GetApplicationByID(ctx, uid)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if app.Status != string(dto.StatusRejected) {
		t.Fatalf("status = %s, want rejected", app.Status)
	}
	if !app.AppliedAt.Valid {
		t.Fatal("expected appliedAt to be set by the applied transition")
	}
	// The post-age signal reads "appliedAt" and joins it against the applied
	// event, so the two timestamps must be the same instant.
	rows, err := itDB.Queries.ListApplicationOutcomes(ctx, uid)
	if err != nil {
		t.Fatalf("list outcomes: %v", err)
	}
	if !rows[0].OccurredAt.Time.Equal(app.AppliedAt.Time) {
		t.Fatalf("appliedAt %v != applied event occurredAt %v", app.AppliedAt.Time, rows[0].OccurredAt.Time)
	}
}

// TestUpdateAppliedIsIdempotentEndToEnd: re-submitting a transition that has
// already been recorded must not duplicate its terminal-once event.
func TestUpdateAppliedIsIdempotentEndToEnd(t *testing.T) {
	ctx := context.Background()
	id := seedApplication(t, "svc-idempotent")
	svc := application.NewService(itDB.Queries, itDB)

	// applied -> shortlisted -> applied: the second `applied` transition is a
	// real status change (so it reaches the insert) but the partial unique index
	// drops the duplicate event.
	for _, st := range []dto.ApplicationStatus{dto.StatusApplied, dto.StatusShortlisted, dto.StatusApplied} {
		if _, err := svc.Update(ctx, id, application.UpdateInput{Status: statusOf(st)}); err != nil {
			t.Fatalf("update to %s: %v", st, err)
		}
	}

	timeline, err := svc.Timeline(ctx, id)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(timeline) != 1 {
		t.Fatalf("expected 1 applied event after re-apply, got %d", len(timeline))
	}
	if timeline[0].EventType != dto.OutcomeApplied {
		t.Fatalf("event = %s, want applied", timeline[0].EventType)
	}
}
