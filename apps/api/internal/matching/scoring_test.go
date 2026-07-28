package matching

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/domain"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/profile"
)

type fakeMatchRepo struct {
	activity.Store
	embedCalls    int
	lastHash      *string
	lastEmbedding []float32
}

func (f *fakeMatchRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (domain.Job, error) {
	return domain.Job{}, nil
}

func (f *fakeMatchRepo) UpdateJobEmbedding(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingParams) error {
	f.embedCalls++
	return nil
}

func (f *fakeMatchRepo) UpdateJobEmbeddingWithHash(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingWithHashParams) error {
	f.embedCalls++
	f.lastHash = arg.EmbeddingHash
	if arg.Embedding != nil {
		f.lastEmbedding = arg.Embedding.Slice()
	}
	return nil
}

func (f *fakeMatchRepo) UpsertMatchResult(ctx context.Context, arg sqlcgen.UpsertMatchResultParams) (sqlcgen.MatchResult, error) {
	return sqlcgen.MatchResult{}, nil
}

type fakeEmbedProvider struct {
	calls  int
	vector []float32
}

func (f *fakeEmbedProvider) ModelName() string { return "fake" }
func (f *fakeEmbedProvider) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeEmbedProvider) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeEmbedProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	f.calls++
	return f.vector, nil
}

type fakeProfileRepo struct {
	profile.Repository
	hasEmbedding bool
}

func (f *fakeProfileRepo) GetDefaultProfile(ctx context.Context) (domain.Profile, error) {
	return domain.Profile{}, nil
}
func (f *fakeProfileRepo) ProfileHasEmbedding(ctx context.Context, id pgtype.UUID) (interface{}, error) {
	return f.hasEmbedding, nil
}
func (f *fakeProfileRepo) ProfileSimilarity(ctx context.Context, arg sqlcgen.ProfileSimilarityParams) (float64, error) {
	return 0.5, nil
}

func newTestService(t *testing.T, matchRepo *fakeMatchRepo, embedProvider llm.Provider) *Service {
	t.Helper()
	profSvc := profile.NewService(&fakeProfileRepo{hasEmbedding: true}, embedProvider, "", "")
	return NewService(matchRepo, profSvc, nil, embedProvider, 0.35, "")
}

func TestRunEmbeddingPrefilter_UnchangedHashSkipsEmbed(t *testing.T) {
	repo := &fakeMatchRepo{}
	stored := []float32{0.1, 0.2, 0.3}
	job := domain.Job{ID: "11111111-1111-1111-1111-111111111111", Title: "Engineer", Company: "Acme", Description: "desc"}
	hash := contentHash("Engineer at Acme\ndesc")
	job.EmbeddingHash = &hash
	job.Embedding = stored

	embedProvider := &fakeEmbedProvider{vector: []float32{9, 9, 9}}
	svc := newTestService(t, repo, embedProvider)

	prof := domain.Profile{ID: "22222222-2222-2222-2222-222222222222"}
	sim, err := svc.runEmbeddingPrefilter(context.Background(), prof, job, nil)
	if err != nil {
		t.Fatalf("runEmbeddingPrefilter: %v", err)
	}
	if embedProvider.calls != 0 {
		t.Errorf("expected 0 Embed calls (hash unchanged), got %d", embedProvider.calls)
	}
	if repo.embedCalls != 0 {
		t.Errorf("expected no embedding write when hash unchanged, got %d", repo.embedCalls)
	}
	if sim != 0.5 {
		t.Errorf("expected similarity from stored vector, got %v", sim)
	}
}

func TestRunEmbeddingPrefilter_ChangedDescriptionRecomputes(t *testing.T) {
	repo := &fakeMatchRepo{}
	job := domain.Job{ID: "11111111-1111-1111-1111-111111111111", Title: "Engineer", Company: "Acme", Description: "new description"}
	staleHash := contentHash("Engineer at Acme\nold description")
	job.EmbeddingHash = &staleHash
	job.Embedding = []float32{0.1, 0.2, 0.3}

	embedProvider := &fakeEmbedProvider{vector: []float32{9, 9, 9}}
	svc := newTestService(t, repo, embedProvider)

	prof := domain.Profile{ID: "22222222-2222-2222-2222-222222222222"}
	if _, err := svc.runEmbeddingPrefilter(context.Background(), prof, job, nil); err != nil {
		t.Fatalf("runEmbeddingPrefilter: %v", err)
	}
	if embedProvider.calls != 1 {
		t.Errorf("expected 1 Embed call (hash changed), got %d", embedProvider.calls)
	}
	if repo.embedCalls != 1 {
		t.Errorf("expected embedding+hash write, got %d", repo.embedCalls)
	}
	wantHash := contentHash("Engineer at Acme\nnew description")
	if repo.lastHash == nil || *repo.lastHash != wantHash {
		t.Errorf("stored hash = %v, want %q", repo.lastHash, wantHash)
	}
}

func TestRunEmbeddingPrefilter_NullHashAlwaysRecomputes(t *testing.T) {
	repo := &fakeMatchRepo{}
	job := domain.Job{ID: "11111111-1111-1111-1111-111111111111", Title: "Engineer", Company: "Acme", Description: "desc"}
	// EmbeddingHash left nil — existing rows before this feature shipped.

	embedProvider := &fakeEmbedProvider{vector: []float32{9, 9, 9}}
	svc := newTestService(t, repo, embedProvider)

	prof := domain.Profile{ID: "22222222-2222-2222-2222-222222222222"}
	if _, err := svc.runEmbeddingPrefilter(context.Background(), prof, job, nil); err != nil {
		t.Fatalf("runEmbeddingPrefilter: %v", err)
	}
	if embedProvider.calls != 1 {
		t.Errorf("expected 1 Embed call (null hash), got %d", embedProvider.calls)
	}
}
