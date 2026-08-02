package domain

import (
	"strings"
	"testing"
)

// groundingProjectMaster has two projects with disjoint vocabulary, so a
// bullet borrowed from the wrong project is detectable.
func groundingProjectMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"projects": []any{
			map[string]any{"name": "Orbit", "highlights": []any{"Built the scheduler service"}},
			map[string]any{"name": "Beacon", "highlights": []any{"Shipped alerting pipelines"}},
		},
	}}}
}

func mergedWithProjects(projects ...map[string]any) RendercvMaster {
	entries := make([]any, 0, len(projects))
	for _, p := range projects {
		entries = append(entries, p)
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"projects": entries}}}
}

func hasViolationContaining(violations []string, want string) bool {
	for _, v := range violations {
		if strings.Contains(v, want) {
			return true
		}
	}
	return false
}

func TestVerifyRendercvGroundingRejectsFabricatedProject(t *testing.T) {
	master := groundingProjectMaster()
	merged := mergedWithProjects(
		map[string]any{"name": "Orbit", "highlights": []any{"Built the scheduler service"}},
		map[string]any{"name": "Phantom", "highlights": []any{"Invented entirely"}},
	)

	violations := VerifyRendercvGrounding(master, merged, GroundingModerate)

	if !hasViolationContaining(violations, `project "Phantom" not in master profile`) {
		t.Errorf("violations = %v, want one naming the fabricated project", violations)
	}
}

func TestVerifyRendercvGroundingStrictRejectsBorrowedProjectHighlight(t *testing.T) {
	master := groundingProjectMaster()
	// Orbit's bullet was taken from Beacon's material.
	merged := mergedWithProjects(
		map[string]any{"name": "Orbit", "highlights": []any{"Shipped alerting pipelines"}},
	)

	violations := VerifyRendercvGrounding(master, merged, GroundingStrict)

	if !hasViolationContaining(violations, "(Orbit) not in master profile (strict grounding)") {
		t.Errorf("violations = %v, want a strict-grounding violation for Orbit", violations)
	}
}

func TestVerifyRendercvGroundingStrictAcceptsOwnProjectHighlights(t *testing.T) {
	master := groundingProjectMaster()
	merged := mergedWithProjects(
		map[string]any{"name": "Orbit", "highlights": []any{"Built the scheduler"}},
		map[string]any{"name": "Beacon", "highlights": []any{"Shipped alerting"}},
	)

	if violations := VerifyRendercvGrounding(master, merged, GroundingStrict); len(violations) != 0 {
		t.Errorf("violations = %v, want none — each project kept its own material", violations)
	}
}

// FR-020: a disabled section is a deletion, and the section-subset check
// already permits deletions — no carve-out needed.
func TestVerifyRendercvGroundingAllowsRemovedSections(t *testing.T) {
	master := loadSampleMaster(t)
	merged := loadSampleMaster(t)
	sections := CvSections(merged)
	delete(sections, "skills")
	delete(sections, "projects")

	if violations := VerifyRendercvGrounding(master, merged, GroundingStrict); len(violations) != 0 {
		t.Errorf("violations = %v, want none for removed sections", violations)
	}
}
