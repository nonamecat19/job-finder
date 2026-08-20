package application

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/generation/domain"
)

const sampleMasterYAML = `
cv:
  name: Jane Doe
  sections:
    summary:
      - Old summary line.
    skills:
      - label: Backend
        details: Go, Postgres, Docker
      - label: Frontend
        details: React, TypeScript
    experience:
      - company: Acme Corp
        position: Senior Engineer
        start_date: 2020-01
        end_date: present
        highlights:
          - Did a thing
      - company: StartupX
        position: Junior Dev
        start_date: 2018-06
        end_date: 2020-01
        highlights:
          - Built a prototype
    education:
      - institution: MIT
        area: Computer Science
        studyType: Bachelor
        start_date: 2014-09
        end_date: 2018-05
design:
  theme: sb2nov
`

func loadSampleMaster(t *testing.T) domain.RendercvMaster {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal([]byte(sampleMasterYAML), &m); err != nil {
		t.Fatalf("unmarshal sample master: %v", err)
	}
	return domain.RendercvMaster(domain.NormalizeYAMLMap(m).(map[string]any))
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestBuildAnalyzePrompt_IncludesVacancyAndHints(t *testing.T) {
	prompt := buildAnalyzePrompt("", "", "Looking for a Go backend engineer with Docker experience.", nil)
	if !containsAll(prompt, "Go backend engineer", "Analyze this job vacancy") {
		t.Fatalf("basic prompt missing expected content:\n%s", prompt)
	}

	hints := &domain.VacancyHints{
		RequiredSkills:  []string{"Go", "Docker"},
		NiceToHave:      []string{"Kubernetes"},
		ExperienceLevel: "senior",
	}
	promptWithHints := buildAnalyzePrompt("", "", "Looking for a Go backend engineer with Docker experience.", hints)
	if !containsAll(promptWithHints, "Required skills (provided): Go, Docker", "Nice-to-have skills (provided): Kubernetes", "Experience level (provided): senior") {
		t.Fatalf("hint prompt missing expected content:\n%s", promptWithHints)
	}
}

func TestBuildSelectPrompt_IncludesSkillIndexesAndCompanies(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{
		RequiredSkills:   []string{"Go", "Postgres"},
		NiceToHaveSkills: []string{"Docker"},
		ExperienceLevel:  "senior",
	}
	prompt := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, domain.DefaultShapeConfig())
	if !containsAll(prompt, "[0] Backend", "[1] Frontend", "company: Acme Corp", "company: StartupX", "GROUNDING = MODERATE") {
		t.Fatalf("prompt missing expected content:\n%s", prompt)
	}
}

func TestBuildSelectPrompt_IncludesPreviousViolations(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "mid"}
	violations := []string{`skill "rust" not in master`, `company "FakeCo" not in master`}
	prompt := buildSelectPrompt(master, analysis, domain.GroundingStrict, violations, domain.DefaultShapeConfig())
	if !containsAll(prompt, `skill "rust" not in master`, `company "FakeCo" not in master`) {
		t.Fatalf("prompt should include previous violations:\n%s", prompt)
	}
}

func TestBuildSelectPromptUsesConfiguredTargets(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}
	cfg := domain.DefaultShapeConfig()
	cfg.SummaryLines = 2
	cfg.ExperienceBulletsMin = 4
	cfg.ExperienceBulletsMax = 5

	prompt := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, cfg)

	if !containsAll(prompt, "TOP 4-5 most relevant bullets") {
		t.Errorf("select prompt does not carry the configured bullet targets:\n%s", prompt)
	}
	for _, stale := range []string{"TOP 8-10"} {
		if strings.Contains(prompt, stale) {
			t.Errorf("select prompt still contains the hardcoded %q:\n%s", stale, prompt)
		}
	}

	if strings.Contains(prompt, "sentences") {
		t.Errorf("select prompt still asks for a summary:\n%s", prompt)
	}
	min, max := summarySelectRange(cfg)
	summaryPrompt := buildSummaryPrompt(domain.SummaryBrief{
		Analysis:    analysis,
		TotalYears:  7,
		SentenceMin: min,
		SentenceMax: max,
	})
	if !containsAll(summaryPrompt, "Write 1-2 sentences") {
		t.Errorf("summary prompt does not carry the configured sentence target:\n%s", summaryPrompt)
	}
	if strings.Contains(summaryPrompt, "3-4 sentences") {
		t.Errorf("summary prompt still contains the hardcoded range:\n%s", summaryPrompt)
	}
}

func TestPromptsWithDefaultConfigReproduceOriginalWording(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}
	cfg := domain.DefaultShapeConfig()

	selectPrompt := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, cfg)
	if !containsAll(selectPrompt,
		"select the TOP 8-10 most relevant bullets",

		"Do NOT return skills",
	) {
		t.Errorf("default select prompt drifted from the original wording:\n%s", selectPrompt)
	}

	summaryPrompt := buildSummaryPrompt(domain.SummaryBrief{Analysis: analysis, TotalYears: 7, SentenceMin: 3, SentenceMax: 4})
	if !containsAll(summaryPrompt, "Write 3-4 sentences", "years of experience") {
		t.Errorf("default summary prompt drifted from the original wording:\n%s", summaryPrompt)
	}

	expandPrompt := buildExpandPrompt(master, analysis, cfg)
	if !containsAll(expandPrompt,
		"Expand it to fill TWO pages",
		"Leave the summary alone",
		"aim for 10-12 per job",
	) {
		t.Errorf("default expand prompt drifted from the original wording:\n%s", expandPrompt)
	}

}

func TestExpandPromptDerivesFromConfig(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}
	cfg := domain.DefaultShapeConfig()
	cfg.SummaryLines = 6
	cfg.ExperienceBulletsMin = 4
	cfg.ExperienceBulletsMax = 6
	cfg.TargetPages = 1

	expandPrompt := buildExpandPrompt(master, analysis, cfg)

	if !containsAll(expandPrompt, "fill ONE page", "aim for 6-8 per job") {
		t.Errorf("expand prompt does not derive from the config:\n%s", expandPrompt)
	}

	if _, max := bulletsCondenseRange(cfg); max != 4 {
		t.Errorf("condense ceiling = %d, want 4 — 60%% of the configured maximum", max)
	}
}

func TestBuildSelectPromptNoReorderOrDrop(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}
	prompt := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, domain.DefaultShapeConfig())

	for _, forbidden := range []string{"Reorder experience", "drop: true", "Decide which sections to drop"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{
		"Keep experience entries in the EXACT order",
		"never set drop",
		"Do not drop, add, rename, or reorder any resume section",
	} {
		if !strings.Contains(prompt, required) {
			t.Errorf("prompt must contain %q:\n%s", required, prompt)
		}
	}
}

func TestBuildSelectPromptNoRemovedFieldReferences(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}
	prompt := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, domain.DefaultShapeConfig())

	for _, forbidden := range []string{"sectionsToDrop", "ExperienceOrder"} {
		if strings.Contains(prompt, forbidden) {
			t.Errorf("prompt must not reference removed field %q:\n%s", forbidden, prompt)
		}
	}
}

func TestBuildSelectPromptProjectsBlockOnlyWhenLimited(t *testing.T) {
	master := loadSampleMaster(t)
	domain.CvSections(master)["projects"] = []any{
		map[string]any{"name": "SideProject", "highlights": []any{"Had fun"}},
	}
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "senior"}

	defaults := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, domain.DefaultShapeConfig())
	if strings.Contains(defaults, "PROJECTS (master)") {
		t.Errorf("default prompt must not carry a projects block:\n%s", defaults)
	}

	cfg := domain.DefaultShapeConfig()
	cfg.ProjectsMax = 2
	limited := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, cfg)
	if !containsAll(limited, "PROJECTS (master)", "name: SideProject", "Return the 2 most vacancy-relevant projects",
		"copied EXACTLY", "that same project's own bullet list") {
		t.Errorf("limited prompt missing the projects block:\n%s", limited)
	}

	bulletsOnly := domain.DefaultShapeConfig()
	bulletsOnly.ProjectBulletsMax = 3
	withBulletCap := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil, bulletsOnly)
	if !containsAll(withBulletCap, "PROJECTS (master)", "keep at most 3 highlights") {
		t.Errorf("bullet-capped prompt missing the projects block:\n%s", withBulletCap)
	}
}
