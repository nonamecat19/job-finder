package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/coach/domain"
	"github.com/job-finder/api/internal/db/sqlcgen"
	kwdomain "github.com/job-finder/api/internal/keyword/domain"
)

const validJobID = "11111111-1111-1111-1111-111111111111"

type fakeDiffReader struct {
	row sqlcgen.KeywordDiff
	err error
}

func (f fakeDiffReader) GetKeywordDiffByJobID(ctx context.Context, jobID pgtype.UUID) (sqlcgen.KeywordDiff, error) {
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

func rowWithOneMissingRequired(t *testing.T) sqlcgen.KeywordDiff {
	pct := 50.0
	return sqlcgen.KeywordDiff{
		Matched: mustJSON(t, []kwdomain.DiffTerm{
			{Term: "Go", Canonical: "Go", Polarity: kwdomain.PolarityRequired},
		}),
		MissingRequired: mustJSON(t, []kwdomain.DiffTerm{
			{Term: "Kubernetes", Canonical: "Kubernetes", Polarity: kwdomain.PolarityRequired},
		}),
		MissingPreferred: mustJSON(t, []kwdomain.DiffTerm{}),
		CoveragePct:      &pct,
	}
}

func TestAssessmentServiceAssess_NoDiff(t *testing.T) {
	svc := NewAssessmentService(NewService(&fakeRephraseModel{}), fakeDiffReader{err: pgx.ErrNoRows}, nil)
	_, err := svc.Assess(context.Background(), validJobID)
	if !errors.Is(err, ErrNoDiff) {
		t.Fatalf("expected ErrNoDiff, got %v", err)
	}
}

func TestAssessmentServiceAssess_InvalidJobID(t *testing.T) {
	svc := NewAssessmentService(NewService(&fakeRephraseModel{}), fakeDiffReader{}, nil)
	_, err := svc.Assess(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrNoDiff) {
		t.Fatalf("expected ErrNoDiff for malformed id, got %v", err)
	}
}

func TestAssessmentServiceAssess_ComputesAndCaches(t *testing.T) {
	model := &fakeRephraseModel{responses: map[string]string{
		"Kubernetes": "Ran containerized workloads adjacent to Kubernetes",
	}}
	reader := fakeDiffReader{row: rowWithOneMissingRequired(t)}
	entries := func(ctx context.Context) ([]domain.ProfileEntry, error) {
		return []domain.ProfileEntry{{SourceLabel: "DevOps Engineer, Acme (2022-2024)", Bullet: "Ran Docker Swarm clusters in production"}}, nil
	}
	svc := NewAssessmentService(NewService(model), reader, entries)

	if _, err := svc.CachedAssessment(context.Background(), validJobID); !errors.Is(err, ErrNotAssessed) {
		t.Fatalf("expected ErrNotAssessed before Assess, got %v", err)
	}

	out, err := svc.Assess(context.Background(), validJobID)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if out.JobID != validJobID {
		t.Errorf("JobID = %q, want %q", out.JobID, validJobID)
	}
	if out.TotalMustHaves != 2 || out.FailedMustHaves != 1 {
		t.Errorf("unexpected counts: total=%d failed=%d", out.TotalMustHaves, out.FailedMustHaves)
	}
	if len(out.Gaps) != 1 || out.Gaps[0].Term != "Kubernetes" {
		t.Fatalf("unexpected gaps: %+v", out.Gaps)
	}

	cached, err := svc.CachedAssessment(context.Background(), validJobID)
	if err != nil {
		t.Fatalf("CachedAssessment: %v", err)
	}
	if cached.JobID != out.JobID || len(cached.Gaps) != len(out.Gaps) {
		t.Errorf("cached result does not match assessed result: %+v vs %+v", cached, out)
	}
}

func TestAssessmentServiceAssess_NoProfileEntriesFunc(t *testing.T) {
	reader := fakeDiffReader{row: rowWithOneMissingRequired(t)}
	svc := NewAssessmentService(NewService(&fakeRephraseModel{}), reader, nil)

	out, err := svc.Assess(context.Background(), validJobID)
	if err != nil {
		t.Fatalf("Assess: %v", err)
	}
	if len(out.Gaps) != 1 || !out.Gaps[0].NoAdjacentEvidence {
		t.Fatalf("expected the gap to have no adjacent evidence, got %+v", out.Gaps)
	}
}
