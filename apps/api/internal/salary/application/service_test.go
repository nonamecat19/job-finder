package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/salary"
)

var _ salary.Repository = (*sqlcgen.Queries)(nil)

type fakeRepo struct {
	job          sqlcgen.Job
	jobErr       error
	cacheRows    []sqlcgen.SalaryCache
	cacheErr     error
	updatedJobID pgtype.UUID
	updatedBand  sqlcgen.UpdateJobSalaryParams
	upserted     sqlcgen.UpsertSalaryCacheParams
}

func (f *fakeRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return f.job, f.jobErr
}

func (f *fakeRepo) UpdateJobSalary(ctx context.Context, arg sqlcgen.UpdateJobSalaryParams) error {
	f.updatedJobID = arg.ID
	f.updatedBand = arg
	return nil
}

func (f *fakeRepo) UpsertSalaryCache(ctx context.Context, arg sqlcgen.UpsertSalaryCacheParams) error {
	f.upserted = arg
	return nil
}

func (f *fakeRepo) GetSalaryCacheByBucket(ctx context.Context, bucket string) ([]sqlcgen.SalaryCache, error) {
	return f.cacheRows, f.cacheErr
}

func makeJob(title, company, location, salaryRaw string) sqlcgen.Job {
	var sr *string
	if salaryRaw != "" {
		sr = &salaryRaw
	}
	var loc *string
	if location != "" {
		loc = &location
	}
	return sqlcgen.Job{
		ID:          pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		Title:       title,
		Company:     company,
		Location:    loc,
		SalaryRaw:   sr,
		Description: "A great job",
	}
}

func TestInfer_SalaryRawParses(t *testing.T) {
	repo := &fakeRepo{
		job: makeJob("Backend Engineer", "Acme", "US", "$120k-$150k"),
	}
	svc := salary.NewService(repo, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceIngestedCache) {
		t.Errorf("expected source ingested-cache, got %v", repo.updatedBand.SalarySource)
	}
	if repo.updatedBand.SalaryMin == nil || *repo.updatedBand.SalaryMin != 120000 {
		t.Errorf("expected min 120000, got %v", repo.updatedBand.SalaryMin)
	}
	if repo.updatedBand.SalaryMax == nil || *repo.updatedBand.SalaryMax != 150000 {
		t.Errorf("expected max 150000, got %v", repo.updatedBand.SalaryMax)
	}
}

func TestInfer_CacheHit(t *testing.T) {
	repo := &fakeRepo{
		job: makeJob("Backend Engineer", "Acme", "US", ""),
		cacheRows: []sqlcgen.SalaryCache{
			{
				Bucket:    "backend-engineer|us|unknown",
				SalaryMin: int32Ptr(100000),
				SalaryMax: int32Ptr(140000),
				Currency:  "USD",
				Source:    string(salary.SourceIngestedCache),
			},
		},
	}
	svc := salary.NewService(repo, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceIngestedCache) {
		t.Errorf("expected source ingested-cache, got %v", repo.updatedBand.SalarySource)
	}
}

func TestInfer_LevelsFyiHit(t *testing.T) {
	repo := &fakeRepo{
		job: makeJob("Backend Engineer", "Acme", "US", ""),
		cacheRows: []sqlcgen.SalaryCache{
			{
				Bucket:    "backend-engineer|us|unknown",
				SalaryMin: int32Ptr(110000),
				SalaryMax: int32Ptr(160000),
				Currency:  "USD",
				Source:    string(salary.SourceLevelsFyi),
			},
		},
	}
	svc := salary.NewService(repo, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceLevelsFyi) {
		t.Errorf("expected source levels-fyi, got %v", repo.updatedBand.SalarySource)
	}
}

func TestInfer_BlendCacheAndLevels(t *testing.T) {
	repo := &fakeRepo{
		job: makeJob("Backend Engineer", "Acme", "US", ""),
		cacheRows: []sqlcgen.SalaryCache{
			{
				Bucket:    "backend-engineer|us|unknown",
				SalaryMin: int32Ptr(100000),
				SalaryMax: int32Ptr(140000),
				Currency:  "USD",
				Source:    string(salary.SourceIngestedCache),
			},
			{
				Bucket:    "backend-engineer|us|unknown",
				SalaryMin: int32Ptr(120000),
				SalaryMax: int32Ptr(160000),
				Currency:  "USD",
				Source:    string(salary.SourceLevelsFyi),
			},
		},
	}
	svc := salary.NewService(repo, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceBlended) {
		t.Errorf("expected source blended, got %v", repo.updatedBand.SalarySource)
	}
}

// TestInfer_NoLLMFallback guards T103: once neither salaryRaw, cache, nor
// levels.fyi produces a band, Infer errors and persists nothing rather than
// falling back to a Go tool loop — that loop is deleted, and estimating from
// scratch is apps/ai's job now (AI_CAPABILITY_ROUTING).
func TestInfer_NoLLMFallback(t *testing.T) {
	repo := &fakeRepo{
		job:       makeJob("Backend Engineer", "Acme", "US", ""),
		cacheRows: nil,
	}
	svc := salary.NewService(repo, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err == nil {
		t.Fatal("Infer: want an error with no cache/levels.fyi band and no Go LLM path left, got nil")
	}
	if repo.updatedBand.SalarySource != nil {
		t.Errorf("a band was persisted despite no source producing one: %+v", repo.updatedBand)
	}
}

func TestInfer_JobNotFound(t *testing.T) {
	repo := &fakeRepo{
		jobErr: errors.New("not found"),
	}
	svc := salary.NewService(repo, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err == nil {
		t.Fatal("expected error for missing job")
	}
}

func int32Ptr(v int32) *int32 { return &v }

//nolint:unused
func strPtr(v string) *string { return &v }

//nolint:unused
func float64Ptr(v float64) *float64 { return &v }

