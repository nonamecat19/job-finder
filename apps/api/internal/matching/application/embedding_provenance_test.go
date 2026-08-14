package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/matching/domain"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/profile"
	profiledomain "github.com/job-finder/api/internal/profile/domain"
	"github.com/job-finder/api/internal/strutil"
)

const (
	currentModel = "openai/text-embedding-3-small"
	staleModel   = "nomic-embed-text"
)

func ptr[T any](v T) *T { return &v }

func vec(f ...float32) *pgvector.Vector {
	v := pgvector.NewVector(f)
	return &v
}

func TestReusableJobEmbedding(t *testing.T) {
	const hash = "abc123"
	stored := []float32{1, 0, 0}

	cases := []struct {
		name       string
		job        sqlcgen.Job
		embedModel string
		wantReuse  bool
	}{
		{
			name:       "current model, matching hash, present vector",
			job:        sqlcgen.Job{EmbedModel: ptr(currentModel), EmbeddingHash: ptr(hash), Embedding: vec(stored...)},
			embedModel: currentModel,
			wantReuse:  true,
		},
		{
			name:       "stale model is treated as unembedded",
			job:        sqlcgen.Job{EmbedModel: ptr(staleModel), EmbeddingHash: ptr(hash), Embedding: vec(stored...)},
			embedModel: currentModel,
			wantReuse:  false,
		},
		{
			name:       "absent provenance is treated as unembedded",
			job:        sqlcgen.Job{EmbedModel: nil, EmbeddingHash: ptr(hash), Embedding: vec(stored...)},
			embedModel: currentModel,
			wantReuse:  false,
		},
		{
			name:       "unconfigured model cannot certify any row",
			job:        sqlcgen.Job{EmbedModel: ptr(""), EmbeddingHash: ptr(hash), Embedding: vec(stored...)},
			embedModel: "",
			wantReuse:  false,
		},
		{
			name:       "changed content",
			job:        sqlcgen.Job{EmbedModel: ptr(currentModel), EmbeddingHash: ptr("other"), Embedding: vec(stored...)},
			embedModel: currentModel,
			wantReuse:  false,
		},
		{
			name:       "no hash",
			job:        sqlcgen.Job{EmbedModel: ptr(currentModel), Embedding: vec(stored...)},
			embedModel: currentModel,
			wantReuse:  false,
		},
		{
			name:       "provenance without a vector",
			job:        sqlcgen.Job{EmbedModel: ptr(currentModel), EmbeddingHash: ptr(hash)},
			embedModel: currentModel,
			wantReuse:  false,
		},
		{
			name:       "provenance with an empty vector",
			job:        sqlcgen.Job{EmbedModel: ptr(currentModel), EmbeddingHash: ptr(hash), Embedding: vec()},
			embedModel: currentModel,
			wantReuse:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reuse := reusableJobEmbedding(tc.job, hash, tc.embedModel)
			if reuse != tc.wantReuse {
				t.Fatalf("reuse = %v, want %v", reuse, tc.wantReuse)
			}
			if !reuse && got != nil {
				t.Fatalf("expected no vector when the row is not reusable, got %v", got)
			}
			if reuse && len(got) != len(stored) {
				t.Fatalf("reused vector = %v, want %v", got, stored)
			}
		})
	}
}

// TestMatchJob_StaleEmbedModelIsReEmbedded is the end of the FR-020 rule that
// matters: not that the predicate returns false, but that the vector reaching
// the similarity comparison is the freshly embedded one. A stale row compares
// against a current-model profile vector without erroring, so nothing but this
// assertion separates a correct score from a plausible wrong one.
func TestMatchJob_StaleEmbedModelIsReEmbedded(t *testing.T) {
	staleVector := []float32{1, 0, 0}
	freshVector := []float32{0, 1, 0}

	for _, tc := range []struct {
		name       string
		rowModel   *string
		wantEmbeds int
		wantScored []float32
	}{
		{name: "stale provenance", rowModel: ptr(staleModel), wantEmbeds: 1, wantScored: freshVector},
		{name: "current provenance", rowModel: ptr(currentModel), wantEmbeds: 0, wantScored: staleVector},
	} {
		t.Run(tc.name, func(t *testing.T) {
			jobID := mustUUID(t, "11111111-1111-1111-1111-111111111111")
			profileID := mustUUID(t, "22222222-2222-2222-2222-222222222222")

			row := sqlcgen.Job{
				ID:          jobID,
				Title:       "Backend Engineer",
				Company:     "TestCo",
				Description: "Go services.",
				Embedding:   vec(staleVector...),
				EmbedModel:  tc.rowModel,
			}
			row.EmbeddingHash = ptr(contentHash(jobTextOf(row)))

			repo := &fakeMatchRepo{job: row}
			profileRepo := &fakeProfileRepo{
				profile: sqlcgen.Profile{
					ID:             profileID,
					RendercvConfig: []byte(`{}`),
					EmbedModel:     ptr(currentModel),
				},
				similarity: 0.1,
			}
			profiles := profile.NewService(profileRepo, stubLLM{})

			llmc := &countingLLM{vector: freshVector, servedModel: currentModel}
			svc := NewService(repo, profiles, llmc, 0.9, "")

			if _, err := svc.MatchJob(context.Background(), dbutil.UUIDString(jobID), nil); err != nil {
				t.Fatalf("MatchJob: %v", err)
			}

			if llmc.embeds != tc.wantEmbeds {
				t.Errorf("embed calls = %d, want %d", llmc.embeds, tc.wantEmbeds)
			}
			if !equalVectors(profileRepo.scored, tc.wantScored) {
				t.Errorf("vector compared against the profile = %v, want %v", profileRepo.scored, tc.wantScored)
			}
			if tc.wantEmbeds > 0 {
				if repo.wroteModel == nil || *repo.wroteModel != currentModel {
					t.Errorf("re-embed wrote provenance %v, want %q", repo.wroteModel, currentModel)
				}
			}
		})
	}
}

func jobTextOf(j sqlcgen.Job) string {
	return strutil.Truncate(fmt.Sprintf("%s at %s\n%s", j.Title, j.Company, j.Description), 8000)
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	u, err := dbutil.ParseUUID(s)
	if err != nil {
		t.Fatalf("parse uuid: %v", err)
	}
	return u
}

func equalVectors(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type fakeMatchRepo struct {
	domain.Repository
	job        sqlcgen.Job
	wroteModel *string
}

func (r *fakeMatchRepo) GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error) {
	return r.job, nil
}

func (r *fakeMatchRepo) UpdateJobEmbeddingWithHash(ctx context.Context, arg sqlcgen.UpdateJobEmbeddingWithHashParams) error {
	r.wroteModel = arg.EmbedModel
	return nil
}

func (r *fakeMatchRepo) UpsertMatchResult(ctx context.Context, arg sqlcgen.UpsertMatchResultParams) (sqlcgen.MatchResult, error) {
	return sqlcgen.MatchResult{JobId: arg.JobId, Similarity: arg.Similarity}, nil
}

type fakeProfileRepo struct {
	profiledomain.Repository
	profile    sqlcgen.Profile
	similarity float64
	scored     []float32
}

func (r *fakeProfileRepo) GetDefaultProfile(ctx context.Context) (sqlcgen.Profile, error) {
	return r.profile, nil
}

func (r *fakeProfileRepo) ProfileHasEmbedding(ctx context.Context, id pgtype.UUID) (interface{}, error) {
	return true, nil
}

func (r *fakeProfileRepo) ProfileSimilarity(ctx context.Context, arg sqlcgen.ProfileSimilarityParams) (float64, error) {
	r.scored = arg.Embedding.Slice()
	return r.similarity, nil
}

type countingLLM struct {
	stubLLM
	vector      []float32
	servedModel string
	embeds      int
}

func (l *countingLLM) Embed(ctx context.Context, text string) ([]float32, error) {
	l.embeds++
	llm.ReportServedModel(ctx, l.servedModel)
	return l.vector, nil
}

type stubLLM struct{}

func (stubLLM) ModelName() string { return "stub" }
func (stubLLM) Complete(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (stubLLM) CompleteJSON(ctx context.Context, prompt string, opts *llm.CompleteOptions) (string, error) {
	return "", nil
}
func (stubLLM) CompleteChat(ctx context.Context, msgs []llm.Message, opts *llm.CompleteOptions) (llm.ChatResult, error) {
	return llm.ChatResult{}, nil
}
func (stubLLM) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }
