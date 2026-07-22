package ghostjob_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/ghostjob"
)

// fakeRepo is a controllable stand-in for ghostjob.Repository. Only the
// methods MeasureSignals actually calls need real behavior; the rest exist
// to satisfy the interface.
type fakeRepo struct {
	repostCount     int32
	repostErr       error
	crossBoardRows  []sqlcgen.ListJobsForCrossBoardCheckRow
	crossBoardErr   error
	alwaysHiring    int32
	alwaysHiringErr error

	alwaysHiringCalls []string // records every company arg passed
}

func (f *fakeRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return sqlcgen.Job{}, errors.New("not implemented")
}

func (f *fakeRepo) CountRepostsByDedupeKey(ctx context.Context, dedupekey string) (int32, error) {
	return f.repostCount, f.repostErr
}

func (f *fakeRepo) ListJobsForCrossBoardCheck(ctx context.Context, id pgtype.UUID) ([]sqlcgen.ListJobsForCrossBoardCheckRow, error) {
	return f.crossBoardRows, f.crossBoardErr
}

func (f *fakeRepo) CountAlwaysHiringByCompany(ctx context.Context, lower string) (int32, error) {
	f.alwaysHiringCalls = append(f.alwaysHiringCalls, lower)
	return f.alwaysHiring, f.alwaysHiringErr
}

func (f *fakeRepo) UpsertJobSignal(ctx context.Context, arg sqlcgen.UpsertJobSignalParams) (sqlcgen.JobSignal, error) {
	return sqlcgen.JobSignal{}, errors.New("not implemented")
}

func (f *fakeRepo) GetJobSignal(ctx context.Context, arg sqlcgen.GetJobSignalParams) (sqlcgen.JobSignal, error) {
	return sqlcgen.JobSignal{}, errors.New("not implemented")
}

var _ ghostjob.Repository = (*fakeRepo)(nil)

func baseJob() sqlcgen.Job {
	return sqlcgen.Job{
		ID:          pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		DedupeKey:   "dedupe-1",
		SourceKey:   "adzuna",
		Company:     "Acme Corp",
		Title:       "Senior Backend Engineer",
		Description: longDescription,
	}
}

const longDescription = `We are looking for a Senior Backend Engineer to join our growing team.
You will design and build scalable services in Go, work closely with
product and design, and mentor junior engineers. Requirements: 5+ years
of backend experience, strong knowledge of distributed systems, and
excellent communication skills.`

// T008 trap: repost count must be able to exceed 1. A naive
// `count(*) FROM "Job" WHERE "dedupeKey" = $1` can only ever return 1
// because "dedupeKey" is UNIQUE — this asserts the measured value the
// service receives is NOT silently clamped to 1.
func TestMeasureSignals_RepostCountGreaterThanOneIsReachable(t *testing.T) {
	repo := &fakeRepo{repostCount: 3, alwaysHiring: 1}
	got := ghostjob.MeasureSignals(context.Background(), repo, baseJob())
	if got.RepostCount != 3 {
		t.Fatalf("expected RepostCount 3, got %d", got.RepostCount)
	}
	if got.Notes["repost"] != "measured" {
		t.Errorf("expected repost note 'measured', got %q", got.Notes["repost"])
	}
}

func TestMeasureSignals_RepostCountDefaultsToOneOnQueryError(t *testing.T) {
	repo := &fakeRepo{repostErr: errors.New("boom"), alwaysHiring: 1}
	got := ghostjob.MeasureSignals(context.Background(), repo, baseJob())
	if got.RepostCount != 1 {
		t.Fatalf("expected RepostCount 1 on query error, got %d", got.RepostCount)
	}
}

func TestMeasureSignals_DaysOpenNilOnNullPostedAt(t *testing.T) {
	repo := &fakeRepo{repostCount: 1, alwaysHiring: 1}
	job := baseJob()
	job.PostedAt = pgtype.Timestamp{Valid: false}

	got := ghostjob.MeasureSignals(context.Background(), repo, job)
	if got.DaysOpen != nil {
		t.Fatalf("expected DaysOpen nil, got %v", *got.DaysOpen)
	}
	if got.Notes["daysOpen"] != "unknown: no postedAt" {
		t.Errorf("unexpected note: %q", got.Notes["daysOpen"])
	}
}

func TestMeasureSignals_DaysOpenMeasuredWhenPostedAtPresent(t *testing.T) {
	repo := &fakeRepo{repostCount: 1, alwaysHiring: 1}
	job := baseJob()
	job.PostedAt = pgtype.Timestamp{Time: time.Now().Add(-72 * time.Hour), Valid: true}

	got := ghostjob.MeasureSignals(context.Background(), repo, job)
	if got.DaysOpen == nil {
		t.Fatal("expected DaysOpen to be measured")
	}
	if *got.DaysOpen < 2 || *got.DaysOpen > 4 {
		t.Errorf("expected DaysOpen ~3, got %d", *got.DaysOpen)
	}
}

func TestMeasureSignals_AlwaysHiringNilForUnparseableCompany(t *testing.T) {
	cases := []string{"", "   ", "---", "???", "Unknown", "unknown", "UNKNOWN"}
	for _, company := range cases {
		repo := &fakeRepo{repostCount: 1, alwaysHiring: 99}
		job := baseJob()
		job.Company = company

		got := ghostjob.MeasureSignals(context.Background(), repo, job)
		if got.AlwaysHiringCount != nil {
			t.Errorf("company %q: expected AlwaysHiringCount nil, got %d", company, *got.AlwaysHiringCount)
		}
		if len(repo.alwaysHiringCalls) != 0 {
			t.Errorf("company %q: expected no query call for an unparseable company", company)
		}
	}
}

// Two placeholder-company jobs must not be grouped as one employer: each is
// independently skipped (nil), never counted against each other.
func TestMeasureSignals_TwoUnknownCompanyJobsNotGrouped(t *testing.T) {
	repo := &fakeRepo{repostCount: 1, alwaysHiring: 99}
	job1 := baseJob()
	job1.Company = "Unknown"
	job2 := baseJob()
	job2.Company = "Unknown"

	got1 := ghostjob.MeasureSignals(context.Background(), repo, job1)
	got2 := ghostjob.MeasureSignals(context.Background(), repo, job2)

	if got1.AlwaysHiringCount != nil || got2.AlwaysHiringCount != nil {
		t.Fatal("expected both placeholder-company jobs to have AlwaysHiringCount nil")
	}
	if len(repo.alwaysHiringCalls) != 0 {
		t.Errorf("expected CountAlwaysHiringByCompany never called for either job, got %d calls", len(repo.alwaysHiringCalls))
	}
}

// SC-004: a company with exactly one posting must not be treated as evidence
// of ghosting by the measurement layer — it measures 1 and lets the
// prompt/service layer treat that as "no evidence", it does not skip it.
func TestMeasureSignals_AlwaysHiringCountOneForOneOffCompany(t *testing.T) {
	repo := &fakeRepo{repostCount: 1, alwaysHiring: 1}
	got := ghostjob.MeasureSignals(context.Background(), repo, baseJob())
	if got.AlwaysHiringCount == nil || *got.AlwaysHiringCount != 1 {
		t.Fatalf("expected AlwaysHiringCount 1, got %v", got.AlwaysHiringCount)
	}
}

func TestMeasureSignals_CrossBoardNilOnTeaserDescription(t *testing.T) {
	repo := &fakeRepo{repostCount: 1, alwaysHiring: 1}
	job := baseJob()
	job.Description = "Great opportunity!" // short teaser

	got := ghostjob.MeasureSignals(context.Background(), repo, job)
	if got.CrossBoardCount != nil {
		t.Fatalf("expected CrossBoardCount nil for a teaser description, got %d", *got.CrossBoardCount)
	}
	if len(repo.crossBoardRows) != 0 {
		t.Fatal("test setup: expected no candidate rows configured")
	}
}

func TestMeasureSignals_CrossBoardCountsDistinctSourcesOnly(t *testing.T) {
	job := baseJob()
	repo := &fakeRepo{
		repostCount:  1,
		alwaysHiring: 1,
		crossBoardRows: []sqlcgen.ListJobsForCrossBoardCheckRow{
			{ID: pgtype.UUID{Valid: true}, SourceKey: "djinni", Description: longDescription},
			// Second appearance on the SAME other source must not double-count.
			{ID: pgtype.UUID{Valid: true}, SourceKey: "djinni", Description: longDescription},
			{ID: pgtype.UUID{Valid: true}, SourceKey: "dou", Description: longDescription},
			// Own source must be excluded even if the description matches.
			{ID: pgtype.UUID{Valid: true}, SourceKey: job.SourceKey, Description: longDescription},
			// Unrelated JD must not count.
			{ID: pgtype.UUID{Valid: true}, SourceKey: "robota", Description: "Totally unrelated marketing coordinator role with different requirements and duties entirely."},
		},
	}

	got := ghostjob.MeasureSignals(context.Background(), repo, job)
	if got.CrossBoardCount == nil {
		t.Fatal("expected CrossBoardCount to be measured")
	}
	if *got.CrossBoardCount != 2 {
		t.Fatalf("expected 2 distinct cross-board sources (djinni, dou), got %d", *got.CrossBoardCount)
	}
}

// SC-006: re-measuring the same job over unchanged fixtures yields
// byte-identical results — the measurements are deterministic even though
// the model's prose is not.
func TestMeasureSignals_DeterministicAcrossRuns(t *testing.T) {
	job := baseJob()
	job.PostedAt = pgtype.Timestamp{Time: time.Now().Add(-100 * time.Hour), Valid: true}
	repo := &fakeRepo{
		repostCount:  4,
		alwaysHiring: 7,
		crossBoardRows: []sqlcgen.ListJobsForCrossBoardCheckRow{
			{SourceKey: "dou", Description: longDescription},
		},
	}

	run1 := ghostjob.MeasureSignals(context.Background(), repo, job)
	run2 := ghostjob.MeasureSignals(context.Background(), repo, job)

	if run1.RepostCount != run2.RepostCount ||
		*run1.DaysOpen != *run2.DaysOpen ||
		*run1.CrossBoardCount != *run2.CrossBoardCount ||
		*run1.AlwaysHiringCount != *run2.AlwaysHiringCount {
		t.Fatalf("expected deterministic measurements across runs: %+v vs %+v", run1, run2)
	}
}

func TestGhostSignals_AllOptionalSignalsUnknown(t *testing.T) {
	repo := &fakeRepo{repostCount: 1}
	job := baseJob()
	job.PostedAt = pgtype.Timestamp{Valid: false}
	job.Description = ""
	job.Company = "Unknown"

	got := ghostjob.MeasureSignals(context.Background(), repo, job)
	if !got.AllOptionalSignalsUnknown() {
		t.Fatal("expected AllOptionalSignalsUnknown to be true")
	}
}

func TestGhostSignals_NotAllUnknownWhenOneMeasured(t *testing.T) {
	repo := &fakeRepo{repostCount: 1, alwaysHiring: 1}
	got := ghostjob.MeasureSignals(context.Background(), repo, baseJob())
	if got.AllOptionalSignalsUnknown() {
		t.Fatal("expected AllOptionalSignalsUnknown to be false when always-hiring is measured")
	}
}
