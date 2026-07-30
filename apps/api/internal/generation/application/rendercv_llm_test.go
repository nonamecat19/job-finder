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
	prompt := buildAnalyzePrompt("Looking for a Go backend engineer with Docker experience.", nil)
	if !containsAll(prompt, "Go backend engineer", "Analyze this job vacancy") {
		t.Fatalf("basic prompt missing expected content:\n%s", prompt)
	}

	hints := &domain.VacancyHints{
		RequiredSkills:  []string{"Go", "Docker"},
		NiceToHave:      []string{"Kubernetes"},
		ExperienceLevel: "senior",
	}
	promptWithHints := buildAnalyzePrompt("Looking for a Go backend engineer with Docker experience.", hints)
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
	prompt := buildSelectPrompt(master, analysis, domain.GroundingModerate, nil)
	if !containsAll(prompt, "[0] Backend", "[1] Frontend", "company: Acme Corp", "company: StartupX", "GROUNDING = MODERATE") {
		t.Fatalf("prompt missing expected content:\n%s", prompt)
	}
}

func TestBuildSelectPrompt_IncludesPreviousViolations(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := domain.VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "mid"}
	violations := []string{`skill "rust" not in master`, `company "FakeCo" not in master`}
	prompt := buildSelectPrompt(master, analysis, domain.GroundingStrict, violations)
	if !containsAll(prompt, `skill "rust" not in master`, `company "FakeCo" not in master`) {
		t.Fatalf("prompt should include previous violations:\n%s", prompt)
	}
}
