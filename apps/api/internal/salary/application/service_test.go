package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/platform/llm"
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

// fakeLLM plays the shape of a real tool exchange (037): the first round is
// sent with tool_choice "required" and answers with the two declared lookups,
// the second round asks for nothing more, and the terminal step — recognisable
// because it carries no tools — returns the band.
//
// It is written this way rather than as an always-answers stub because the loop
// treats prose on the required first round as proof that the serving tier
// cannot call tools. A stub that answered immediately would fail for that
// reason and look like a broken conversion.
type fakeLLM struct {
	err error
	// calls counts CompleteChat invocations, so a test can assert the exchange
	// took the shape it expected rather than only that it produced a band.
	calls int
	// toolCalls records the lookups the loop actually dispatched.
	toolCalls []string
	// noTools makes the fake answer prose on the required first round, which is
	// how a non-tool-capable tier behaves.
	noTools bool
}

func (f *fakeLLM) ModelName() string { return "test-model" }
func (f *fakeLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return `{"min":80000,"max":120000,"currency":"USD","confidence":0.4,"source":"llm"}`, nil
}

func (f *fakeLLM) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	f.calls++
	if f.err != nil {
		return llm.ChatResult{}, f.err
	}
	if opts != nil && opts.ToolChoice == "required" {
		if f.noTools {
			return llm.ChatResult{Content: "Probably around 100k."}, nil
		}
		jobID := ""
		for _, m := range msgs {
			if idx := strings.Index(m.Content, "The id of this posting is "); idx >= 0 {
				jobID = strings.TrimSuffix(m.Content[idx+len("The id of this posting is "):], ".")
			}
		}
		f.toolCalls = append(f.toolCalls, "lookup_comparable_bands", "get_posting_details")
		return llm.ChatResult{ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "lookup_comparable_bands", Arguments: json.RawMessage(`{"title":"Backend Engineer","location":"US"}`)},
			{ID: "c2", Name: "get_posting_details", Arguments: json.RawMessage(`{"job_id":"` + jobID + `"}`)},
		}}, nil
	}
	if opts != nil && len(opts.Tools) > 0 {
		// A later round, still tool-capable, but nothing more to look up.
		return llm.ChatResult{Content: "I have enough to answer."}, nil
	}
	// The terminal step: no tools declared.
	return llm.ChatResult{Content: `{"min":80000,"max":120000,"currency":"USD","confidence":0.4,"source":"llm"}`}, nil
}

func (f *fakeLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	return nil, nil
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
	svc := salary.NewService(repo, &fakeLLM{}, nil, "")

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
	svc := salary.NewService(repo, &fakeLLM{}, nil, "")

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
	svc := salary.NewService(repo, &fakeLLM{}, nil, "")

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
	svc := salary.NewService(repo, &fakeLLM{}, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceBlended) {
		t.Errorf("expected source blended, got %v", repo.updatedBand.SalarySource)
	}
}

func TestInfer_LLMFallback(t *testing.T) {
	repo := &fakeRepo{
		job:       makeJob("Backend Engineer", "Acme", "US", ""),
		cacheRows: nil,
	}
	svc := salary.NewService(repo, &fakeLLM{}, nil, "")

	err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceLLM) {
		t.Errorf("expected source llm, got %v", repo.updatedBand.SalarySource)
	}
}

func TestInfer_JobNotFound(t *testing.T) {
	repo := &fakeRepo{
		jobErr: errors.New("not found"),
	}
	svc := salary.NewService(repo, &fakeLLM{}, nil, "")

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

// 037 FR-021 / SC-008: a posting with no parseable salaryRaw and no cached
// bucket reaches the loop, performs **both** lookups, and returns a band that
// passes Validate().
func TestInfer_LLMPathPerformsBothLookupsAndProducesAValidBand(t *testing.T) {
	repo := &fakeRepo{job: makeJob("Backend Engineer", "Acme", "US", "")}
	llmc := &fakeLLM{}
	svc := salary.NewService(repo, llmc, nil, "")

	if err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001"); err != nil {
		t.Fatalf("Infer: %v", err)
	}

	if len(llmc.toolCalls) != 2 {
		t.Fatalf("the exchange dispatched %v, want both declared lookups", llmc.toolCalls)
	}
	// One round of lookups, one round that asks for nothing more, one terminal
	// step: three provider calls. Asserted so a conversion that quietly skips
	// the loop and calls the model once cannot pass.
	if llmc.calls != 3 {
		t.Errorf("provider called %d times, want 3 (lookup round, wrap-up round, terminal step)", llmc.calls)
	}

	band := salary.SalaryBand{
		Min:      int(*repo.updatedBand.SalaryMin),
		Max:      int(*repo.updatedBand.SalaryMax),
		Currency: *repo.updatedBand.SalaryCurrency,
	}
	if err := band.Validate(); err != nil {
		t.Errorf("persisted band does not validate: %v", err)
	}
	if repo.updatedBand.SalarySource == nil || *repo.updatedBand.SalarySource != string(salary.SourceLLM) {
		t.Errorf("source = %v, want llm", repo.updatedBand.SalarySource)
	}
}

// FR-022 / C7-3: when the exchange stops for any reason other than answered,
// Infer returns an error and persists **nothing**. A fallback to a
// low-confidence band here would write a fabrication to the database and label
// it an estimate.
func TestInfer_PersistsNothingWhenTheExchangeDoesNotAnswer(t *testing.T) {
	for name, llmc := range map[string]*fakeLLM{
		"provider failure": {err: errors.New("provider unavailable")},
		"not tool capable": {noTools: true},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{job: makeJob("Backend Engineer", "Acme", "US", "")}
			svc := salary.NewService(repo, llmc, nil, "")

			err := svc.Infer(context.Background(), "00000001-0000-0000-0000-000000000001")
			if err == nil {
				t.Fatal("Infer returned nil on an exchange that never answered")
			}
			if repo.updatedBand.SalaryMin != nil || repo.updatedBand.SalarySource != nil {
				t.Errorf("a band was persisted despite the failure: %+v", repo.updatedBand)
			}
			if repo.upserted.Bucket != "" {
				t.Errorf("the cache was written despite the failure: %+v", repo.upserted)
			}
		})
	}
}
