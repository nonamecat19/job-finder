package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/generation/domain"
)

// 035 FR-010: page fitting adjusts selection content only. The premium-written
// summary is immutable once produced — a page-fit response has nowhere to put
// a summary (TailoredSelection has no such field), and the merge carries the
// existing one through rather than reverting to the master's.
func TestPageFitCannotAlterTheSummary(t *testing.T) {
	const premiumSummary = "8+ years of experience building payment systems."

	merged, err := domain.MergeTailored(stageMaster(), domain.TailoredSelection{
		Experience: []domain.TailoredExperience{{Company: "Acme", Highlights: []string{"Did a thing"}}},
	}, &domain.TailoredSummary{Summary: premiumSummary})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	// A page-fit stage that tries to rewrite the summary: the field is not in
	// the type, so whatever it says about the summary is discarded at unmarshal.
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

// The structure re-prompt rebuilds the selection from the master, which is
// exactly where a premium summary would be silently lost if the merge did not
// carry it through.
func TestStructureRepromptKeepsTheSummary(t *testing.T) {
	const premiumSummary = "8+ years of experience building payment systems."
	merged, err := domain.MergeTailored(stageMaster(), domain.TailoredSelection{
		Experience: []domain.TailoredExperience{{Company: "Acme", Highlights: []string{"Did a thing"}}},
	}, &domain.TailoredSummary{Summary: premiumSummary})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	reMerged, err := domain.MergeTailored(stageMaster(), domain.TailoredSelection{
		Experience: []domain.TailoredExperience{{Company: "Acme", Highlights: []string{"Did a thing again"}}},
	}, domain.CurrentSummary(merged))
	if err != nil {
		t.Fatalf("re-merge: %v", err)
	}

	got := domain.CurrentSummary(reMerged)
	if got == nil || got.Summary != premiumSummary {
		t.Errorf("summary after a structure re-prompt = %v, want it carried through as %q", got, premiumSummary)
	}
}
