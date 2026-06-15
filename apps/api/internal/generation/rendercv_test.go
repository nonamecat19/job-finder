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
    projects:
      - name: SideProject
        description: A fun thing
        highlights:
          - Had fun
    patents:
      - title: Some Patent
    invited_talks:
      - event: Conference talk
    publications:
      - title: Some Paper
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

// ---------------------------------------------------------------------------
// mergeTailored basic tests
// ---------------------------------------------------------------------------

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

	origSections := cvSections(master)
	origSummary := stringSliceField(origSections, "summary")
	if origSummary[0] != "Old summary line." {
		t.Fatalf("mergeTailored must not mutate the master profile, got %v", origSummary)
	}
}

// ---------------------------------------------------------------------------
// Experience reordering
// ---------------------------------------------------------------------------

func TestMergeTailored_ReordersExperience(t *testing.T) {
	master := loadSampleMaster(t)
	payload := TailoredSections{
		Summary: "Summary",
		Skills:  []TailoredSkillGroup{{Index: 0, Details: "Go"}},
		Experience: []TailoredExperience{
			{Company: "Acme Corp", Highlights: []string{"Relevant thing"}},
			{Company: "StartupX", Highlights: []string{"Less relevant thing"}},
		},
		ExperienceOrder: []string{"StartupX", "Acme Corp"},
	}
	merged, err := mergeTailored(master, payload)
	if err != nil {
		t.Fatalf("mergeTailored: %v", err)
	}

	sections := cvSections(merged)
	exp := asSliceOfMaps(sections["experience"])
	if len(exp) != 2 {
		t.Fatalf("expected 2 experience entries, got %d", len(exp))
	}
	if stringField(exp[0], "company") != "StartupX" {
		t.Fatalf("expected StartupX first after reorder, got %s", stringField(exp[0], "company"))
	}
	if stringField(exp[1], "company") != "Acme Corp" {
		t.Fatalf("expected Acme Corp second after reorder, got %s", stringField(exp[1], "company"))
	}
}

// ---------------------------------------------------------------------------
// Experience drop
// ---------------------------------------------------------------------------

func TestMergeTailored_DropsExperience(t *testing.T) {
	master := loadSampleMaster(t)
	payload := TailoredSections{
		Summary: "Summary",
		Skills:  []TailoredSkillGroup{{Index: 0, Details: "Go"}},
		Experience: []TailoredExperience{
			{Company: "Acme Corp", Highlights: []string{"Relevant thing"}},
			{Company: "StartupX", Highlights: []string{"Not relevant"}, Drop: true},
		},
	}
	merged, err := mergeTailored(master, payload)
	if err != nil {
		t.Fatalf("mergeTailored: %v", err)
	}

	sections := cvSections(merged)
	exp := asSliceOfMaps(sections["experience"])
	if len(exp) != 1 {
		t.Fatalf("expected 1 experience entry after drop, got %d", len(exp))
	}
	if stringField(exp[0], "company") != "Acme Corp" {
		t.Fatalf("expected Acme Corp to remain, got %s", stringField(exp[0], "company"))
	}
}

// ---------------------------------------------------------------------------
// Section dropping
// ---------------------------------------------------------------------------

func TestMergeTailored_DropsSections(t *testing.T) {
	master := loadSampleMaster(t)
	payload := TailoredSections{
		Summary: "Summary",
		Skills:  []TailoredSkillGroup{{Index: 0, Details: "Go"}},
		Experience: []TailoredExperience{
			{Company: "Acme Corp", Highlights: []string{"Thing"}},
		},
		SectionsToDrop: []string{"patents", "invited_talks", "publications", "projects"},
	}
	merged, err := mergeTailored(master, payload)
	if err != nil {
		t.Fatalf("mergeTailored: %v", err)
	}

	sections := cvSections(merged)
	for _, key := range []string{"patents", "invited_talks", "publications", "projects"} {
		if _, ok := sections[key]; ok {
			t.Errorf("expected section %q to be dropped", key)
		}
	}
	// Protected sections must survive
	for _, key := range []string{"summary", "experience", "education", "skills"} {
		if _, ok := sections[key]; !ok {
			t.Errorf("expected protected section %q to survive", key)
		}
	}
}

func TestMergeTailored_ProtectedSectionsNeverDropped(t *testing.T) {
	master := loadSampleMaster(t)
	payload := TailoredSections{
		Summary: "Summary",
		Skills:  []TailoredSkillGroup{{Index: 0, Details: "Go"}},
		Experience: []TailoredExperience{
			{Company: "Acme Corp", Highlights: []string{"Thing"}},
		},
		SectionsToDrop: []string{"summary", "experience", "education", "skills"},
	}
	merged, err := mergeTailored(master, payload)
	if err != nil {
		t.Fatalf("mergeTailored: %v", err)
	}

	sections := cvSections(merged)
	for _, key := range []string{"summary", "experience", "education", "skills"} {
		if _, ok := sections[key]; !ok {
			t.Errorf("protected section %q was incorrectly dropped", key)
		}
	}
}

// ---------------------------------------------------------------------------
// Grounding checks
// ---------------------------------------------------------------------------

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
	skills[0]["details"] = "Go, Rust, Kubernetes"

	violations := verifyRendercvGrounding(master, merged, GroundingStrict)
	if len(violations) == 0 {
		t.Fatal("expected strict grounding to reject a skill token not in the master")
	}

	violationsModerate := verifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violationsModerate) != 0 {
		t.Fatalf("moderate grounding should not check skill tokens, got %v", violationsModerate)
	}
}

func TestVerifyRendercvGrounding_RejectsAddedSection(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := deepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := cvSections(merged)
	sections["custom_section"] = []any{"some content"}

	violations := verifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violations) == 0 {
		t.Fatal("expected a violation for an added section not in master")
	}
}

// ---------------------------------------------------------------------------
// Prompt builders
// ---------------------------------------------------------------------------

func TestBuildAnalyzePrompt_IncludesVacancyAndHints(t *testing.T) {
	prompt := buildAnalyzePrompt("Looking for a Go backend engineer with Docker experience.", nil)
	if !containsAll(prompt, "Go backend engineer", "Analyze this job vacancy") {
		t.Fatalf("basic prompt missing expected content:\n%s", prompt)
	}

	hints := &VacancyHints{
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
	analysis := VacancyAnalysis{
		RequiredSkills:   []string{"Go", "Postgres"},
		NiceToHaveSkills: []string{"Docker"},
		ExperienceLevel:  "senior",
	}
	prompt := buildSelectPrompt(master, analysis, GroundingModerate, nil)
	if !containsAll(prompt, "[0] Backend", "[1] Frontend", "company: Acme Corp", "company: StartupX", "GROUNDING = MODERATE") {
		t.Fatalf("prompt missing expected content:\n%s", prompt)
	}
}

func TestBuildSelectPrompt_IncludesPreviousViolations(t *testing.T) {
	master := loadSampleMaster(t)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}, ExperienceLevel: "mid"}
	violations := []string{`skill "rust" not in master`, `company "FakeCo" not in master`}
	prompt := buildSelectPrompt(master, analysis, GroundingStrict, violations)
	if !containsAll(prompt, `skill "rust" not in master`, `company "FakeCo" not in master`) {
		t.Fatalf("prompt should include previous violations:\n%s", prompt)
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
