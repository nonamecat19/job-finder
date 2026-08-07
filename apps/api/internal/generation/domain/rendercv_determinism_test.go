package domain

import (
	"reflect"
	"testing"
)

// These tests guard a property the eval harness (038) depends on and that
// nothing else in the suite covers: VerifyRendercvGrounding's output must be
// byte-identical across runs on identical input.
//
// It is not an aesthetic concern. Violations returned here are carried back
// into the next attempt's prompt (application/service.go tailorRendercvResume
// -> prevViolations -> buildSelectPrompt), so map-iteration order in the
// verifier makes the *retry request text* differ between runs. Replay fixtures
// keyed by a hash of the request would then miss at random.
//
// Both loops below previously ranged directly over a map. A single offending
// item cannot expose the bug — Go randomises map order per iteration, so the
// cases need several items each to fail reliably.

func multiSectionMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{},
	}}}
}

func multiSectionMerged() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience":     []any{},
		"publications":   []any{},
		"awards":         []any{},
		"certifications": []any{},
		"volunteering":   []any{},
		"languages":      []any{},
		"interests":      []any{},
	}}}
}

func TestVerifyRendercvGroundingIsDeterministic_AddedSections(t *testing.T) {
	master := multiSectionMaster()
	merged := multiSectionMerged()

	first := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{})
	if len(first) < 2 {
		t.Fatalf("fixture is not exercising the added-section path: got %d violations, want >= 2", len(first))
	}

	for i := 0; i < 50; i++ {
		got := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{})
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("violation order is not stable across runs\niteration %d\nfirst: %v\ngot:   %v", i, first, got)
		}
	}
}

func TestVerifyRendercvGroundingIsDeterministic_StrictProjectTokens(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"projects": []any{
			map[string]any{"name": "Orbit", "highlights": []any{"scheduler"}},
		},
	}}}
	merged := mergedWithProjects(
		map[string]any{"name": "Orbit", "highlights": []any{
			"scheduler kubernetes terraform postgres kafka grafana prometheus elasticsearch",
		}},
	)

	first := VerifyRendercvGrounding(master, merged, GroundingStrict, VacancyAnalysis{})
	if len(first) < 2 {
		t.Fatalf("fixture is not exercising the strict-token path: got %d violations, want >= 2", len(first))
	}

	for i := 0; i < 50; i++ {
		got := VerifyRendercvGrounding(master, merged, GroundingStrict, VacancyAnalysis{})
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("violation order is not stable across runs\niteration %d\nfirst: %v\ngot:   %v", i, first, got)
		}
	}
}

// DeriveTotalExperienceYears resolves "present" against the wall clock, so a
// profile with an ongoing role yields a different figure on either side of
// 1 January. That value reaches the summary prompt via SummaryBrief.TotalYears.
// The AsOf variant exists so a recorded request stays reproducible; this test
// pins the relationship between the two.
func TestDeriveTotalExperienceYearsAsOfIsPinned(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{
			map[string]any{"company": "Acme", "start_date": "2015-01", "end_date": "present"},
		},
	}}}

	if got := DeriveTotalExperienceYearsAsOf(master, 2030); got != 15 {
		t.Fatalf("as-of 2030: got %d years, want 15", got)
	}
	// The same input a year later must move — proving the figure really is
	// clock-dependent, which is why the AsOf variant is required for replay.
	if got := DeriveTotalExperienceYearsAsOf(master, 2031); got != 16 {
		t.Fatalf("as-of 2031: got %d years, want 16", got)
	}
}
