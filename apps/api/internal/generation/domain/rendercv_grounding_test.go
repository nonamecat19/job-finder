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

	violations := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{})

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

	violations := VerifyRendercvGrounding(master, merged, GroundingStrict, VacancyAnalysis{})

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

	if violations := VerifyRendercvGrounding(master, merged, GroundingStrict, VacancyAnalysis{}); len(violations) != 0 {
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

	if violations := VerifyRendercvGrounding(master, merged, GroundingStrict, VacancyAnalysis{}); len(violations) != 0 {
		t.Errorf("violations = %v, want none for removed sections", violations)
	}
}

// 033 FR-001: moderate grounding now rejects skill tokens not in the master
// (when not adjacency-allowed via the vacancy analysis).
func TestVerifyRendercvGroundingModerateRejectsUnlistedSkill(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	skills := AsSliceOfMaps(CvSections(merged)["skills"])
	skills[0]["details"] = "Go, Rust, Kubernetes"

	violations := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{})
	if !hasViolationContaining(violations, `skill "rust"`) {
		t.Errorf("moderate grounding should reject unlisted skill 'rust', got %v", violations)
	}
}

// 033 FR-001: moderate grounding allows a vacancy-required skill when the
// master already has a skill in the same group (adjacency).
func TestVerifyRendercvGroundingModerateAllowsAdjacentSkill(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	skills := AsSliceOfMaps(CvSections(merged)["skills"])
	// Add a vacancy-required skill to the first group (which already has skills).
	skills[0]["details"] = "Go, Terraform"

	analysis := VacancyAnalysis{RequiredSkills: []string{"Terraform"}}
	violations := VerifyRendercvGrounding(master, merged, GroundingModerate, analysis)
	if hasViolationContaining(violations, `skill "terraform"`) {
		t.Errorf("moderate grounding should allow adjacent skill 'Terraform', got %v", violations)
	}
}

// 033 FR-002: a highlight that drifts too far from the master's bullets is
// flagged at all grounding levels.
func TestVerifyRendercvGroundingFlagsDriftedHighlight(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	exp := AsSliceOfMaps(CvSections(merged)["experience"])
	// Replace a highlight with something that shares no words with any master bullet.
	exp[0]["highlights"] = []any{"Completely fabricated unrelated quantum banana"}

	for _, level := range []GroundingLevel{GroundingStrict, GroundingModerate, GroundingAggressive} {
		violations := VerifyRendercvGrounding(master, merged, level, VacancyAnalysis{})
		if !hasViolationContaining(violations, "highlight not grounded in master") {
			t.Errorf("level %s: expected highlight drift violation, got %v", level, violations)
		}
	}
}

// 033 FR-003: StripUngroundedHighlights replaces a drifted highlight with the
// closest master bullet.
func TestStripUngroundedHighlightsReplacesDriftedBullet(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	exp := AsSliceOfMaps(CvSections(merged)["experience"])
	company := StringField(exp[0], "company")
	masterBullets := StringSliceField(AsSliceOfMaps(CvSections(master)["experience"])[0], "highlights")
	if len(masterBullets) == 0 {
		t.Skip("master has no experience highlights for the first entry")
	}
	// Replace with a completely unrelated highlight.
	exp[0]["highlights"] = []any{"Completely fabricated unrelated quantum banana"}

	result := StripUngroundedHighlights(master, merged)
	resultExp := AsSliceOfMaps(CvSections(result)["experience"])
	hl := StringSliceField(resultExp[0], "highlights")
	if len(hl) != 1 {
		t.Fatalf("expected 1 highlight after strip, got %d", len(hl))
	}
	// The replacement should be one of the master's bullets, not the fabricated text.
	if hl[0] == "Completely fabricated unrelated quantum banana" {
		t.Fatal("drifted highlight was not replaced")
	}
	found := false
	for _, b := range masterBullets {
		if hl[0] == b {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("replacement %q is not a master bullet for %s, master bullets: %v", hl[0], company, masterBullets)
	}
}