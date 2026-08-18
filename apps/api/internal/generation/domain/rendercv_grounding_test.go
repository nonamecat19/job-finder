package domain

import (
	"strings"
	"testing"
)

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

func TestVerifyRendercvGroundingModerateAllowsAdjacentSkill(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	skills := AsSliceOfMaps(CvSections(merged)["skills"])

	skills[0]["details"] = "Go, Terraform"

	analysis := VacancyAnalysis{RequiredSkills: []string{"Terraform"}}
	violations := VerifyRendercvGrounding(master, merged, GroundingModerate, analysis)
	if hasViolationContaining(violations, `skill "terraform"`) {
		t.Errorf("moderate grounding should allow adjacent skill 'Terraform', got %v", violations)
	}
}

func TestVerifyRendercvGroundingFlagsDriftedHighlight(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	exp := AsSliceOfMaps(CvSections(merged)["experience"])

	exp[0]["highlights"] = []any{"Completely fabricated unrelated quantum banana"}

	for _, level := range []GroundingLevel{GroundingStrict, GroundingModerate, GroundingAggressive} {
		violations := VerifyRendercvGrounding(master, merged, level, VacancyAnalysis{})
		if !hasViolationContaining(violations, "highlight not grounded in master") {
			t.Errorf("level %s: expected highlight drift violation, got %v", level, violations)
		}
	}
}

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

	exp[0]["highlights"] = []any{"Completely fabricated unrelated quantum banana"}

	result := StripUngroundedHighlights(master, merged)
	resultExp := AsSliceOfMaps(CvSections(result)["experience"])
	hl := StringSliceField(resultExp[0], "highlights")
	if len(hl) != 1 {
		t.Fatalf("expected 1 highlight after strip, got %d", len(hl))
	}

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
func summaryGroundingMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{
			map[string]any{
				"company": "Northwind Systems", "position": "Backend Engineer",
				"start_date": "2015-01", "end_date": "2021-01",
				"highlights": []any{
					"Cut checkout latency by 40% across the payments path",
					"Led a team of 12 engineers through the storage migration",
				},
			},
		},
		"skills": []any{
			map[string]any{"label": "Languages", "details": "Go, Python, SQL"},
			map[string]any{"label": "Infrastructure", "details": "Docker, Kubernetes, PostgreSQL"},
		},
	}}}
}

func summaryGroundingBrief() SummaryBrief {
	return SummaryBrief{
		Analysis:         VacancyAnalysis{RequiredSkills: []string{"Rust", "Kafka"}},
		TotalYears:       6,
		Highlights:       []string{"Cut checkout latency by 40% across the payments path", "Led a team of 12 engineers through the storage migration"},
		SkillGroupLabels: []string{"Languages", "Infrastructure"},
	}
}

func TestVerifySummaryGroundingAcceptsGroundedSummary(t *testing.T) {
	summary := TailoredSummary{Summary: "Backend engineer with 6 years of experience building payment systems in Go and Python. " +
		"Cut checkout latency by 40% and led a team of 12 through a Kubernetes migration."}

	if violations := VerifySummaryGrounding(summaryGroundingMaster(), summary, summaryGroundingBrief()); len(violations) != 0 {
		t.Errorf("violations = %v, want none — every claim traces to the master", violations)
	}
}

func TestVerifySummaryGroundingRejectsUngroundedSkillToken(t *testing.T) {
	summary := TailoredSummary{Summary: "Backend engineer with 6 years of experience building services in Go, with deep expertise in Rust."}

	violations := VerifySummaryGrounding(summaryGroundingMaster(), summary, summaryGroundingBrief())

	if !hasViolationContaining(violations, `summary skill "Rust" not in master skill tokens`) {
		t.Errorf("violations = %v, want one naming the fabricated skill", violations)
	}
	if hasViolationContaining(violations, `"Go"`) {
		t.Errorf("violations = %v, master-backed skill Go must not be flagged", violations)
	}
}

func TestVerifySummaryGroundingRejectsUnsupportedMetric(t *testing.T) {
	summary := TailoredSummary{Summary: "Backend engineer with 6 years of experience who reduced infrastructure spend by 73% in a single quarter."}

	violations := VerifySummaryGrounding(summaryGroundingMaster(), summary, summaryGroundingBrief())

	if !hasViolationContaining(violations, `summary metric "73%" not supported by the selected highlights`) {
		t.Errorf("violations = %v, want one naming the unsupported metric", violations)
	}
}

func TestVerifySummaryGroundingRejectsContradictingYearsFigure(t *testing.T) {
	summary := TailoredSummary{Summary: "Backend engineer with 9+ years of experience shipping services in Go and Python."}

	violations := VerifySummaryGrounding(summaryGroundingMaster(), summary, summaryGroundingBrief())

	if !hasViolationContaining(violations, "master's experience spans 6 years") {
		t.Errorf("violations = %v, want one contradicting the derived years figure", violations)
	}
}

func TestVerifySummaryGroundingIgnoresOrdinaryEnglish(t *testing.T) {
	summary := TailoredSummary{Summary: "Pragmatic engineer who ships reliable services, mentors peers and keeps systems boring. " +
		"Comfortable across Go, Docker and PostgreSQL, with a bias toward simple designs that survive contact with production."}

	if violations := VerifySummaryGrounding(summaryGroundingMaster(), summary, summaryGroundingBrief()); len(violations) != 0 {
		t.Errorf("violations = %v, want none — ordinary English is not a skill claim", violations)
	}
}

func metricMaster(bullets ...string) RendercvMaster {
	entries := make([]any, 0, len(bullets))
	for _, b := range bullets {
		entries = append(entries, b)
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{map[string]any{"company": "Acme", "highlights": entries}},
	}}}
}

func TestVerifyRendercvGroundingRejectsInventedMetric(t *testing.T) {
	master := metricMaster("Cut checkout latency on the payments service")
	merged := metricMaster("Cut checkout latency on the payments service by 40%")

	violations := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{})
	if !hasViolationContaining(violations, `asserts metric "40%"`) {
		t.Errorf("invented metric not reported; violations = %v", violations)
	}
}

func TestVerifyRendercvGroundingAllowsMetricsTheMasterClaims(t *testing.T) {
	master := metricMaster("Cut checkout latency by 40% across 12 services")
	merged := metricMaster("Reduced checkout latency 40% over 12 services")

	if violations := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{}); len(violations) != 0 {
		t.Errorf("rephrasing that keeps the master's own numbers must pass; got %v", violations)
	}
}

func TestVerifyRendercvGroundingRejectsAlteredMetric(t *testing.T) {
	master := metricMaster("Cut checkout latency by 40%")
	merged := metricMaster("Cut checkout latency by 60%")

	if !hasViolationContaining(VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{}), `asserts metric "60%"`) {
		t.Error("a rounded-up metric must be reported; it is a claim the candidate never made")
	}
}

func TestVerifyRendercvGroundingIgnoresDigitsInsideIdentifiers(t *testing.T) {
	master := metricMaster("Tuned p95 latency", "Migrated storage to S3")
	merged := metricMaster("Tuned p95 latency after migrating storage to S3")

	if violations := VerifyRendercvGrounding(master, merged, GroundingModerate, VacancyAnalysis{}); len(violations) != 0 {
		t.Errorf("p95/S3 are technology names, not metrics; got %v", violations)
	}
}

func TestVerifyTailoredSectionsGroundingRejectsInventedMetricBeforeMerge(t *testing.T) {
	master := metricMaster("Led the payments migration")
	payload := TailoredSections{TailoredSelection: TailoredSelection{
		Experience: []TailoredExperience{{Company: "Acme", Highlights: []HighlightRef{{SourceIndex: 0, Rephrased: "Led the payments migration for 3M users"}}}},
	}}

	if !hasViolationContaining(VerifyTailoredSectionsGrounding(master, payload), `asserts metric "3m"`) {
		t.Error("an invented metric must be caught before the merge, not only after it")
	}
}

func refMaster(bullets ...string) RendercvMaster { return metricMaster(bullets...) }

func mergedHighlights(t *testing.T, master RendercvMaster, refs []HighlightRef, level GroundingLevel) []string {
	t.Helper()
	merged, err := MergeTailored(master, TailoredSelection{
		Experience: []TailoredExperience{{Company: "Acme", Highlights: refs}},
	}, nil, level)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}
	return StringSliceField(AsSliceOfMaps(CvSections(merged)["experience"])[0], "highlights")
}

func TestMergeResolvesReferencesToMasterBullets(t *testing.T) {
	master := refMaster("Shipped the payments service", "Cut checkout latency by 40%")
	got := mergedHighlights(t, master, []HighlightRef{{SourceIndex: 1}, {SourceIndex: 0}}, GroundingModerate)

	want := []string{"Cut checkout latency by 40%", "Shipped the payments service"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("highlights = %v, want the master's own bullets in the selected order %v", got, want)
	}
}

func TestMergeFallsBackToTheMasterBulletWhenARewordingDrifts(t *testing.T) {
	master := refMaster("Shipped the payments service")
	got := mergedHighlights(t, master, []HighlightRef{
		{SourceIndex: 0, Rephrased: "Architected a distributed trading platform from scratch"},
	}, GroundingModerate)

	if len(got) != 1 || got[0] != "Shipped the payments service" {
		t.Errorf("highlights = %v, want the master's wording after the rewording was rejected", got)
	}
}

func TestMergeFallsBackWhenARewordingAddsAMetric(t *testing.T) {
	master := refMaster("Cut checkout latency on the payments service")
	got := mergedHighlights(t, master, []HighlightRef{
		{SourceIndex: 0, Rephrased: "Cut checkout latency on the payments service by 40%"},
	}, GroundingModerate)

	if len(got) != 1 || got[0] != "Cut checkout latency on the payments service" {
		t.Errorf("highlights = %v, want the metric-free master wording", got)
	}
}

func TestMergeKeepsAGroundedRewording(t *testing.T) {
	master := refMaster("Cut checkout latency by 40%")
	got := mergedHighlights(t, master, []HighlightRef{
		{SourceIndex: 0, Rephrased: "Cut checkout latency 40%"},
	}, GroundingModerate)

	if len(got) != 1 || got[0] != "Cut checkout latency 40%" {
		t.Errorf("highlights = %v, want the accepted rewording", got)
	}
}

func TestMergeIgnoresRewordingsUnderStrictGrounding(t *testing.T) {
	master := refMaster("Cut checkout latency by 40%")
	got := mergedHighlights(t, master, []HighlightRef{
		{SourceIndex: 0, Rephrased: "Cut checkout latency 40%"},
	}, GroundingStrict)

	if len(got) != 1 || got[0] != "Cut checkout latency by 40%" {
		t.Errorf("highlights = %v, want the master's wording untouched under strict grounding", got)
	}
}

func TestMergeDropsUnusableReferences(t *testing.T) {
	master := refMaster("Shipped the payments service", "Cut checkout latency")
	got := mergedHighlights(t, master, []HighlightRef{
		{SourceIndex: 0}, {SourceIndex: 0}, {SourceIndex: 9}, {SourceIndex: -1},
	}, GroundingModerate)

	if len(got) != 1 || got[0] != "Shipped the payments service" {
		t.Errorf("highlights = %v, want the repeat and the out-of-range indices dropped", got)
	}
}
