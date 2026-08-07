package application

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/keyword/domain"
)

type fakeReader struct {
	row sqlcgen.KeywordDiff
	err error
}

func (f fakeReader) GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error) {
	return f.row, f.err
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

const validJobID = "11111111-1111-1111-1111-111111111111"

func TestDiffServiceReadsCacheAndRecomputesMetadata(t *testing.T) {
	pct := 50.0
	row := sqlcgen.KeywordDiff{
		Matched: mustJSON(t, []domain.DiffTerm{
			{Term: "kubernetes", Canonical: "Kubernetes", Polarity: domain.PolarityRequired, Stemmed: "kubernet", MatchType: domain.MatchExact},
			{Term: "typescript", Canonical: "TypeScript", Polarity: domain.PolarityPreferred, Stemmed: "typescript"},
		}),
		MissingRequired:  mustJSON(t, []domain.DiffTerm{{Term: "docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"}}),
		MissingPreferred: mustJSON(t, []domain.DiffTerm{{Term: "grpc", Canonical: "gRPC", Polarity: domain.PolarityPreferred, Stemmed: "grpc"}}),
		CoveragePct:      &pct,
	}
	svc := NewDiffService(fakeReader{row: row})

	out, err := svc.KeywordDiff(context.Background(), validJobID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if out.JobID != validJobID {
		t.Errorf("jobId = %q", out.JobID)
	}
	m := out.Metadata
	if m.TotalRequired != 2 || m.TotalPreferred != 2 || m.MatchedRequired != 1 || m.MatchedPreferred != 1 {
		t.Errorf("metadata = %+v", m)
	}
	if m.CoveragePct != 50.0 {
		t.Errorf("coveragePct = %v, want persisted 50.0", m.CoveragePct)
	}
	if len(out.Suggestions) != 0 {
		t.Errorf("expected no suggestions without a rephraser, got %d", len(out.Suggestions))
	}
}

func TestDiffServiceNotFound(t *testing.T) {
	svc := NewDiffService(fakeReader{err: pgx.ErrNoRows})
	if _, err := svc.KeywordDiff(context.Background(), validJobID); err != ErrDiffNotFound {
		t.Fatalf("expected ErrDiffNotFound, got %v", err)
	}
}

func TestDiffServiceBadUUID(t *testing.T) {
	svc := NewDiffService(fakeReader{})
	if _, err := svc.KeywordDiff(context.Background(), "not-a-uuid"); err != ErrDiffNotFound {
		t.Fatalf("expected ErrDiffNotFound for bad id, got %v", err)
	}
}

type fakeRephraser struct{}

func (fakeRephraser) SuggestAll(ctx context.Context, missing []domain.DiffTerm, bullets []string) []RephraseSuggestion {
	out := make([]RephraseSuggestion, 0, len(missing))
	for _, t := range missing {
		r := "Reframed " + t.Canonical
		out = append(out, RephraseSuggestion{Term: t.Term, Canonical: t.Canonical, Rephrase: &r, SourceBullet: bullets[0]})
	}
	return out
}

type fakeBullets struct{}

func (fakeBullets) ProfileBullets(ctx context.Context) ([]string, error) {
	return []string{"Built container images in CI"}, nil
}

func TestDiffServiceWithRephraser(t *testing.T) {
	row := sqlcgen.KeywordDiff{
		Matched:          mustJSON(t, []domain.DiffTerm{}),
		MissingRequired:  mustJSON(t, []domain.DiffTerm{{Term: "docker", Canonical: "Docker", Polarity: domain.PolarityRequired, Stemmed: "docker"}}),
		MissingPreferred: mustJSON(t, []domain.DiffTerm{}),
	}
	svc := NewDiffService(fakeReader{row: row}).WithRephraser(fakeRephraser{}, fakeBullets{})

	out, err := svc.KeywordDiff(context.Background(), validJobID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out.Suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(out.Suggestions))
	}
	if out.Suggestions[0].Rephrase == nil || *out.Suggestions[0].Rephrase != "Reframed Docker" {
		t.Errorf("suggestion = %+v", out.Suggestions[0])
	}
	if out.Metadata.CoveragePct != 0 {
		t.Errorf("coveragePct = %v, want 0", out.Metadata.CoveragePct)
	}
}
