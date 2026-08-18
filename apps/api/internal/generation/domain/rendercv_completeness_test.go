package domain

import (
	"strings"
	"testing"
)

func completenessMaster(groups map[string]string, order []string, bullets map[string]int, companies []string) RendercvMaster {
	skills := make([]any, 0, len(order))
	for _, label := range order {
		skills = append(skills, map[string]any{"label": label, "details": groups[label]})
	}
	experience := make([]any, 0, len(companies))
	for _, company := range companies {
		highlights := make([]any, 0, bullets[company])
		for i := 0; i < bullets[company]; i++ {
			highlights = append(highlights, "did a thing")
		}
		experience = append(experience, map[string]any{"company": company, "highlights": highlights})
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills":     skills,
		"experience": experience,
	}}}
}

func completenessFixture() RendercvMaster {
	return completenessMaster(
		map[string]string{
			"Languages": "Go, TypeScript, Python",
			"Frontend":  "React Native, Redux, Tailwind, Storybook, Vite",
		},
		[]string{"Languages", "Frontend"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)
}

func completenessCfg() ShapeConfig {
	cfg := DefaultShapeConfig()
	cfg.ExperienceBulletsMin = 4
	return cfg
}

func TestVerifyCompletenessFlagsMissingRequiredSkill(t *testing.T) {
	master := completenessFixture()
	merged := completenessMaster(
		map[string]string{
			"Languages": "TypeScript, Python",
			"Frontend":  "React Native, Redux, Tailwind, Storybook, Vite",
		},
		[]string{"Languages", "Frontend"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go", "React Native"}}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if !report.Shortfall {
		t.Fatalf("Shortfall = false, want true when a required skill is dropped")
	}
	if len(report.RequiredMissing) != 1 || report.RequiredMissing[0] != "go" {
		t.Errorf("RequiredMissing = %v, want [go]", report.RequiredMissing)
	}
	if !strings.Contains(report.Reason(), "go") {
		t.Errorf("Reason() = %q, want it to name the missing skill", report.Reason())
	}
}

func TestVerifyCompletenessFlagsNiceToHaveBelowFloor(t *testing.T) {

	master := completenessFixture()
	merged := completenessMaster(
		map[string]string{
			"Languages": "Go, TypeScript, Python",
			"Frontend":  "React Native, Redux, Tailwind",
		},
		[]string{"Languages", "Frontend"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)
	analysis := VacancyAnalysis{
		RequiredSkills:   []string{"Go"},
		NiceToHaveSkills: []string{"React Native", "Redux", "Tailwind", "Storybook", "Vite"},
	}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if !report.Shortfall {
		t.Fatalf("Shortfall = false, want true at 60%% nice-to-have retention")
	}
	if report.NiceToHaveRetained != 0.6 {
		t.Errorf("NiceToHaveRetained = %v, want 0.6", report.NiceToHaveRetained)
	}
	if len(report.NiceToHaveMissing) != 2 {
		t.Errorf("NiceToHaveMissing = %v, want two entries", report.NiceToHaveMissing)
	}
}

func TestVerifyCompletenessAcceptsNiceToHaveAtFloor(t *testing.T) {
	master := completenessFixture()
	merged := completenessMaster(
		map[string]string{
			"Languages": "Go, TypeScript, Python",
			"Frontend":  "React Native, Redux, Tailwind, Storybook",
		},
		[]string{"Languages", "Frontend"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)
	analysis := VacancyAnalysis{
		RequiredSkills:   []string{"Go"},
		NiceToHaveSkills: []string{"React Native", "Redux", "Tailwind", "Storybook", "Vite"},
	}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if report.NiceToHaveRetained != 0.8 {
		t.Fatalf("NiceToHaveRetained = %v, want 0.8", report.NiceToHaveRetained)
	}
	if report.Shortfall {
		t.Errorf("Shortfall = true at exactly 80%% retention, want false: %s", report.Reason())
	}
}

func TestVerifyCompletenessFlagsCompanyBelowBulletMinimum(t *testing.T) {
	master := completenessFixture()
	merged := completenessMaster(
		map[string]string{
			"Languages": "Go, TypeScript, Python",
			"Frontend":  "React Native, Redux, Tailwind, Storybook, Vite",
		},
		[]string{"Languages", "Frontend"},
		map[string]int{"Acme": 2},
		[]string{"Acme"},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if !report.Shortfall {
		t.Fatalf("Shortfall = false, want true when a company falls below the bullet minimum")
	}
	if got, ok := report.BulletShortfalls["Acme"]; !ok || got != 2 {
		t.Errorf("BulletShortfalls = %v, want Acme -> 2", report.BulletShortfalls)
	}
	if !strings.Contains(report.Reason(), "Acme") {
		t.Errorf("Reason() = %q, want it to name the company", report.Reason())
	}
}

func TestVerifyCompletenessIgnoresMasterLimitedCompany(t *testing.T) {

	master := completenessMaster(
		map[string]string{"Languages": "Go"},
		[]string{"Languages"},
		map[string]int{"Acme": 2},
		[]string{"Acme"},
	)
	merged := completenessMaster(
		map[string]string{"Languages": "Go"},
		[]string{"Languages"},
		map[string]int{"Acme": 2},
		[]string{"Acme"},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if len(report.BulletShortfalls) != 0 {
		t.Errorf("BulletShortfalls = %v, want none for a master-limited company", report.BulletShortfalls)
	}
	if report.Shortfall {
		t.Errorf("Shortfall = true, want false: %s", report.Reason())
	}
}

func TestVerifyCompletenessFallsBackToStructuralCheck(t *testing.T) {
	master := completenessFixture()
	merged := completenessMaster(
		map[string]string{"Languages": "Go, TypeScript, Python"},
		[]string{"Languages"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)

	report := VerifyCompleteness(master, merged, VacancyAnalysis{}, completenessCfg())

	if !report.StructuralFallback {
		t.Fatalf("StructuralFallback = false, want true when the analysis has no required skills")
	}
	if !report.Shortfall {
		t.Errorf("Shortfall = false, want true — merged dropped a whole skill group")
	}
	if len(report.RequiredMissing) != 0 {
		t.Errorf("RequiredMissing = %v, want none under the structural fallback", report.RequiredMissing)
	}
	if !strings.Contains(report.Reason(), "skill groups") {
		t.Errorf("Reason() = %q, want it to explain the structural check", report.Reason())
	}
}

func TestVerifyCompletenessStructuralFallbackStillChecksBullets(t *testing.T) {
	master := completenessFixture()
	merged := completenessMaster(
		map[string]string{
			"Languages": "Go, TypeScript, Python",
			"Frontend":  "React Native, Redux, Tailwind, Storybook, Vite",
		},
		[]string{"Languages", "Frontend"},
		map[string]int{"Acme": 1},
		[]string{"Acme"},
	)

	report := VerifyCompleteness(master, merged, VacancyAnalysis{}, completenessCfg())

	if !report.StructuralFallback {
		t.Fatalf("StructuralFallback = false, want true")
	}
	if got, ok := report.BulletShortfalls["Acme"]; !ok || got != 1 {
		t.Errorf("BulletShortfalls = %v, want Acme -> 1 even under the fallback", report.BulletShortfalls)
	}
	if !report.Shortfall {
		t.Errorf("Shortfall = false, want true")
	}
}

func TestVerifyCompletenessAcceptsCompleteOutput(t *testing.T) {
	master := completenessFixture()
	merged := completenessFixture()
	analysis := VacancyAnalysis{
		RequiredSkills:   []string{"Go", "React Native"},
		NiceToHaveSkills: []string{"Redux", "Vite"},
	}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if report.Shortfall {
		t.Fatalf("Shortfall = true for a complete selection: %s", report.Reason())
	}
	if len(report.RequiredMissing) != 0 || len(report.NiceToHaveMissing) != 0 || len(report.BulletShortfalls) != 0 {
		t.Errorf("report = %+v, want no missing entries", report)
	}
	if report.NiceToHaveRetained != 1 || report.StructuralFallback {
		t.Errorf("NiceToHaveRetained = %v, StructuralFallback = %v, want 1 and false", report.NiceToHaveRetained, report.StructuralFallback)
	}
	if report.Reason() != "" {
		t.Errorf("Reason() = %q, want empty when nothing fell short", report.Reason())
	}
}

func TestVerifyCompletenessRetainsSkillWithExtraQualifier(t *testing.T) {

	master := completenessMaster(
		map[string]string{"Frontend": "React Native, Redux"},
		[]string{"Frontend"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)
	merged := completenessMaster(
		map[string]string{"Frontend": "React Native (Expo), Redux"},
		[]string{"Frontend"},
		map[string]int{"Acme": 10},
		[]string{"Acme"},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"React Native"}}

	report := VerifyCompleteness(master, merged, analysis, completenessCfg())

	if report.Shortfall {
		t.Errorf("Shortfall = true, want false: %s", report.Reason())
	}
}
