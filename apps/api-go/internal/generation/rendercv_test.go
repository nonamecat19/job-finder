package generation

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
design:
  theme: sb2nov
`

func loadSampleMaster(t *testing.T) RendercvMaster {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal([]byte(sampleMasterYAML), &m); err != nil {
		t.Fatalf("unmarshal sample master: %v", err)
	}
	return RendercvMaster(normalizeYAMLMap(m).(map[string]any))
}

func TestMergeTailored_PreservesDesignAndDates(t *testing.T) {
	master := loadSampleMaster(t)
	payload := TailoredSections{
		Summary: "New tailored summary.",
		Skills: []TailoredSkillGroup{
			{Index: 0, Details: "Go, Kubernetes"},
		},
		Experience: []TailoredExperience{
			{Company: "Acme Corp", Highlights: []string{"Shipped feature X", "Led migration Y"}},
		},
	}
	merged, err := mergeTailored(master, payload)
	if err != nil {
		t.Fatalf("mergeTailored: %v", err)
	}

	// design untouched
	design, _ := merged["design"].(map[string]any)
	if design["theme"] != "sb2nov" {
		t.Fatalf("expected design.theme preserved, got %v", design["theme"])
	}

	sections := cvSections(merged)
	summary := stringSliceField(sections, "summary")
	if len(summary) != 1 || summary[0] != "New tailored summary." {
		t.Fatalf("summary not replaced correctly: %v", summary)
	}

	skills := asSliceOfMaps(sections["skills"])
	if stringField(skills[0], "details") != "Go, Kubernetes" {
		t.Fatalf("skill[0].details not replaced: %v", skills[0])
	}
	// untouched skill group must survive unchanged
	if stringField(skills[1], "label") != "Frontend" || stringField(skills[1], "details") != "React, TypeScript" {
		t.Fatalf("skill[1] should be untouched: %v", skills[1])
	}

	exp := asSliceOfMaps(sections["experience"])
	if stringField(exp[0], "start_date") != "2020-01" || stringField(exp[0], "end_date") != "present" {
		t.Fatalf("experience dates must be preserved verbatim: %v", exp[0])
	}
	highlights := stringSliceField(exp[0], "highlights")
	if len(highlights) != 2 || highlights[0] != "Shipped feature X" {
		t.Fatalf("highlights not replaced correctly: %v", highlights)
	}

	// original master must be untouched (deep clone, not mutation)
	origSections := cvSections(master)
	origSummary := stringSliceField(origSections, "summary")
	if origSummary[0] != "Old summary line." {
		t.Fatalf("mergeTailored must not mutate the master profile, got %v", origSummary)
	}
}

func TestVerifyRendercvGrounding_RejectsFabricatedCompany(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := deepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := cvSections(merged)
	exp := asSliceOfMaps(sections["experience"])
	exp[0]["company"] = "Fabricated Co"

	violations := verifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violations) == 0 {
		t.Fatal("expected a violation for a fabricated company")
	}
}

func TestVerifyRendercvGrounding_StrictRejectsUnlistedSkill(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := deepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := cvSections(merged)
	skills := asSliceOfMaps(sections["skills"])
	skills[0]["details"] = "Go, Rust, Kubernetes" // "rust" and "kubernetes" not in master

	violations := verifyRendercvGrounding(master, merged, GroundingStrict)
	if len(violations) == 0 {
		t.Fatal("expected strict grounding to reject a skill token not in the master")
	}

	// moderate/aggressive don't enforce the skill-token constraint
	violationsModerate := verifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violationsModerate) != 0 {
		t.Fatalf("moderate grounding should not check skill tokens, got %v", violationsModerate)
	}
}

func TestBuildTailorPrompt_IncludesSkillIndexesAndCompanies(t *testing.T) {
	master := loadSampleMaster(t)
	prompt := buildTailorPrompt(master, "We need a Go backend engineer.", GroundingModerate)
	if !containsAll(prompt, "[0] Backend", "[1] Frontend", "company: Acme Corp", "GROUNDING = MODERATE") {
		t.Fatalf("prompt missing expected content:\n%s", prompt)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
