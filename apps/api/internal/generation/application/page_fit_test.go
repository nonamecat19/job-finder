package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

func TestPageFitCannotAlterTheSummary(t *testing.T) {
	const premiumSummary = "8+ years of experience building payment systems."

	merged, err := domain.MergeTailored(stageMaster(), domain.TailoredSelection{
		Experience: []domain.TailoredExperience{{Company: "Acme", Highlights: []domain.HighlightRef{{SourceIndex: 0}}}},
	}, &domain.TailoredSummary{Summary: premiumSummary}, domain.GroundingModerate)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	f := &fakeRenderer{pages: []int{1, 2}}
	deps := f.deps()
	cfg := domain.DefaultShapeConfig()
	cfg.TargetPages = 2

	s := &Service{}
	if _, err := s.renderToPageTarget(context.Background(), deps, stageMaster(), merged, domain.VacancyAnalysis{}, domain.GroundingModerate, cfg, "base", nil); err != nil {
		t.Fatalf("renderToPageTarget: %v", err)
	}
	if f.expands == 0 {
		t.Fatal("expand never ran; the test is not exercising the page-fit path")
	}

	got := domain.CurrentSummary(merged)
	if got == nil || got.Summary != premiumSummary {
		t.Errorf("summary after page fitting = %v, want it unchanged at %q", got, premiumSummary)
	}
}

func TestStructureRepromptKeepsTheSummary(t *testing.T) {
	const premiumSummary = "8+ years of experience building payment systems."
	merged, err := domain.MergeTailored(stageMaster(), domain.TailoredSelection{
		Experience: []domain.TailoredExperience{{Company: "Acme", Highlights: []domain.HighlightRef{{SourceIndex: 0}}}},
	}, &domain.TailoredSummary{Summary: premiumSummary}, domain.GroundingModerate)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	reMerged, err := domain.MergeTailored(stageMaster(), domain.TailoredSelection{
		Experience: []domain.TailoredExperience{{Company: "Acme", Highlights: []domain.HighlightRef{{SourceIndex: 0, Rephrased: "Did a thing again"}}}},
	}, domain.CurrentSummary(merged), domain.GroundingModerate)
	if err != nil {
		t.Fatalf("re-merge: %v", err)
	}

	got := domain.CurrentSummary(reMerged)
	if got == nil || got.Summary != premiumSummary {
		t.Errorf("summary after a structure re-prompt = %v, want it carried through as %q", got, premiumSummary)
	}
}
