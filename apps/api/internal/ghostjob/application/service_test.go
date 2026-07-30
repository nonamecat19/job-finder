package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/ghostjob"
	"github.com/job-finder/api/internal/platform/llm"
)

// fakeServiceRepo is a controllable stand-in for ghostjob.Repository at the
// service level: it tracks every UpsertJobSignal call and simulates the
// (jobId, kind) upsert-replace semantics the real "JobSignal" unique
// constraint enforces (FR-009).
type fakeServiceRepo struct {
	job sqlcgen.Job

	repostCount    int32
	crossBoardRows []sqlcgen.ListJobsForCrossBoardCheckRow
	alwaysHiring   int32

	rows        map[string]sqlcgen.JobSignal // key: jobId+kind
	upsertCalls int
}

func newFakeServiceRepo(job sqlcgen.Job) *fakeServiceRepo {
	return &fakeServiceRepo{job: job, alwaysHiring: 1, rows: map[string]sqlcgen.JobSignal{}}
}

func (f *fakeServiceRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return f.job, nil
}

func (f *fakeServiceRepo) CountRepostsByDedupeKey(ctx context.Context, dedupekey string) (int32, error) {
	return f.repostCount, nil
}

func (f *fakeServiceRepo) ListJobsForCrossBoardCheck(ctx context.Context, id pgtype.UUID) ([]sqlcgen.ListJobsForCrossBoardCheckRow, error) {
	return f.crossBoardRows, nil
}

func (f *fakeServiceRepo) CountAlwaysHiringByCompany(ctx context.Context, lower string) (int32, error) {
	return f.alwaysHiring, nil
}

func (f *fakeServiceRepo) UpsertJobSignal(ctx context.Context, arg sqlcgen.UpsertJobSignalParams) (sqlcgen.JobSignal, error) {
	f.upsertCalls++
	key := dbKey(arg.JobId, arg.Kind)
	row := sqlcgen.JobSignal{
		ID:        pgtype.UUID{Bytes: [16]byte{byte(f.upsertCalls)}, Valid: true},
		JobId:     arg.JobId,
		Kind:      arg.Kind,
		Score:     arg.Score,
		Signals:   arg.Signals,
		Model:     arg.Model,
		CreatedAt: pgtype.Timestamp{Valid: true},
	}
	f.rows[key] = row // replaces any prior row for this (jobId, kind) — no history accumulates
	return row, nil
}

func (f *fakeServiceRepo) GetJobSignal(ctx context.Context, arg sqlcgen.GetJobSignalParams) (sqlcgen.JobSignal, error) {
	row, ok := f.rows[dbKey(arg.JobId, arg.Kind)]
	if !ok {
		return sqlcgen.JobSignal{}, errors.New("not found")
	}
	return row, nil
}

func dbKey(id pgtype.UUID, kind string) string {
	return string(id.Bytes[:]) + "|" + kind
}

var _ ghostjob.Repository = (*fakeServiceRepo)(nil)

// fakeLLM is a scripted llm.Provider: each call to CompleteJSON returns the
// next canned response in order, so a test can simulate retry-then-succeed,
// exhaust-the-retry-budget, or a clean one-shot answer.
type fakeLLM struct {
	model     string
	responses []string
	calls     int
}

func (f *fakeLLM) ModelName() string { return f.model }

func (f *fakeLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	if f.calls >= len(f.responses) {
		return "", errors.New("fakeLLM: no more scripted responses")
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

func (f *fakeLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, errors.New("not implemented")
}

var _ llm.Provider = (*fakeLLM)(nil)

func scoredJob() sqlcgen.Job {
	return sqlcgen.Job{
		ID:          pgtype.UUID{Bytes: [16]byte{9}, Valid: true},
		DedupeKey:   "dedupe-9",
		SourceKey:   "adzuna",
		Company:     "Acme Corp",
		Title:       "Senior Backend Engineer",
		Description: longDescription,
	}
}

func TestScoreJob_PersistsAValidResult(t *testing.T) {
	repo := newFakeServiceRepo(scoredJob())
	repo.repostCount = 3
	llmc := &fakeLLM{model: "qwen2.5:14b", responses: []string{
		`{"score": 82, "confidence": 0.8, "explanation": "reposted 3 times", "topSignals": ["repostCount"]}`,
	}}

	svc := ghostjob.NewService(repo, llmc, "")
	out, err := svc.ScoreJob(context.Background(), "09000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Score != 82 {
		t.Errorf("expected score 82, got %d", out.Score)
	}
	if out.Kind != ghostjob.Kind {
		t.Errorf("expected kind %q, got %q", ghostjob.Kind, out.Kind)
	}
	// SC-003: every persisted row carries score/model/confidence and a
	// value-or-explicit-unknown for all four signals.
	if out.Model == "" {
		t.Error("expected model to be set")
	}
	if out.Signals.RepostCount != 3 {
		t.Errorf("expected RepostCount 3, got %d", out.Signals.RepostCount)
	}
	if out.Signals.Notes["daysOpen"] == "" || out.Signals.Notes["crossBoard"] == "" || out.Signals.Notes["alwaysHiring"] == "" || out.Signals.Notes["repost"] == "" {
		t.Errorf("expected every signal to carry a provenance note, got %+v", out.Signals.Notes)
	}
	if repo.upsertCalls != 1 {
		t.Errorf("expected exactly 1 upsert call, got %d", repo.upsertCalls)
	}
}

// FR-010: an out-of-range score persists nothing and preserves whatever was
// there before (here: nothing was there before, and nothing must appear
// after either).
func TestScoreJob_OutOfRangeScorePersistsNothing(t *testing.T) {
	repo := newFakeServiceRepo(scoredJob())
	repo.repostCount = 2
	invalid := `{"score": 500, "confidence": 0.5, "explanation": "bad", "topSignals": []}`
	llmc := &fakeLLM{model: "qwen2.5:14b", responses: []string{invalid, invalid, invalid}}

	svc := ghostjob.NewService(repo, llmc, "")
	_, err := svc.ScoreJob(context.Background(), "09000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected an error for an out-of-range score")
	}
	if repo.upsertCalls != 0 {
		t.Errorf("expected no upsert on validation failure, got %d calls", repo.upsertCalls)
	}
}

// FR-009: two successive score runs leave exactly one row, with the later
// values — never accumulated history.
func TestScoreJob_SecondUpsertReplacesFirst(t *testing.T) {
	repo := newFakeServiceRepo(scoredJob())
	repo.repostCount = 2
	llmc := &fakeLLM{model: "qwen2.5:14b", responses: []string{
		`{"score": 40, "confidence": 0.6, "explanation": "first pass", "topSignals": []}`,
		`{"score": 85, "confidence": 0.9, "explanation": "reposted again", "topSignals": ["repostCount"]}`,
	}}
	svc := ghostjob.NewService(repo, llmc, "")

	jobID := "09000000-0000-0000-0000-000000000000"
	if _, err := svc.ScoreJob(context.Background(), jobID); err != nil {
		t.Fatalf("first score: %v", err)
	}
	repo.repostCount = 4 // signals age between runs
	second, err := svc.ScoreJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("second score: %v", err)
	}

	if len(repo.rows) != 1 {
		t.Fatalf("expected exactly 1 stored row after two score runs, got %d", len(repo.rows))
	}
	if second.Score != 85 {
		t.Errorf("expected the stored row to carry the later score 85, got %d", second.Score)
	}
}

// Spec edge case: when every signal is unknown, the service declines to
// score — no LLM call, no row written.
func TestScoreJob_DeclinesWhenAllSignalsUnknown(t *testing.T) {
	job := scoredJob()
	job.Description = ""
	job.Company = "Unknown"
	job.PostedAt = pgtype.Timestamp{Valid: false}

	repo := newFakeServiceRepo(job)
	repo.repostCount = 1 // only its own appearance
	repo.alwaysHiring = 0

	llmc := &fakeLLM{model: "qwen2.5:14b", responses: []string{
		`{"score": 90, "confidence": 0.9, "explanation": "should never be called", "topSignals": []}`,
	}}
	svc := ghostjob.NewService(repo, llmc, "")

	_, err := svc.ScoreJob(context.Background(), "09000000-0000-0000-0000-000000000000")
	if !errors.Is(err, ghostjob.ErrDeclinedToScore) {
		t.Fatalf("expected ErrDeclinedToScore, got %v", err)
	}
	if llmc.calls != 0 {
		t.Errorf("expected no LLM call when every signal is unknown, got %d calls", llmc.calls)
	}
	if repo.upsertCalls != 0 {
		t.Errorf("expected no row written when every signal is unknown, got %d upserts", repo.upsertCalls)
	}
}

// FR-011 / SC-005: confidence is capped when an optional signal is unknown,
// even if the model reports high confidence anyway.
func TestScoreJob_CapsConfidenceWhenSignalUnknown(t *testing.T) {
	job := scoredJob()
	job.PostedAt = pgtype.Timestamp{Valid: false} // daysOpen unknown

	repo := newFakeServiceRepo(job)
	repo.repostCount = 3
	llmc := &fakeLLM{model: "qwen2.5:14b", responses: []string{
		`{"score": 70, "confidence": 0.95, "explanation": "overconfident", "topSignals": []}`,
	}}
	svc := ghostjob.NewService(repo, llmc, "")

	out, err := svc.ScoreJob(context.Background(), "09000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Signals.Confidence > 0.6 {
		t.Errorf("expected confidence capped at 0.6 when a signal is unknown, got %v", out.Signals.Confidence)
	}
}

// FR-018 / SC-009: a scoring failure for one job must not corrupt or block
// scoring for another — verified here by scoring two independent jobs
// against two independent service instances (the real isolation guarantee
// is "no shared mutable state between calls", exercised at the handler
// level too).
func TestScoreJob_FailureIsIsolatedPerJob(t *testing.T) {
	failing := newFakeServiceRepo(scoredJob())
	failing.repostCount = 2
	failingLLM := &fakeLLM{model: "m", responses: []string{"not json", "not json", "not json"}}
	failingSvc := ghostjob.NewService(failing, failingLLM, "")
	if _, err := failingSvc.ScoreJob(context.Background(), "09000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("expected the malformed-response job to error")
	}

	okJob := scoredJob()
	okJob.ID = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	ok := newFakeServiceRepo(okJob)
	ok.repostCount = 2
	okLLM := &fakeLLM{model: "m", responses: []string{
		`{"score": 55, "confidence": 0.7, "explanation": "fine", "topSignals": []}`,
	}}
	okSvc := ghostjob.NewService(ok, okLLM, "")
	if _, err := okSvc.ScoreJob(context.Background(), "09000000-0000-0000-0000-000000000000"); err != nil {
		t.Fatalf("expected the other job to score successfully, got %v", err)
	}
	if ok.upsertCalls != 1 {
		t.Errorf("expected the healthy job to persist its result, got %d upserts", ok.upsertCalls)
	}
}
