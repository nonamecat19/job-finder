package domain

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
	return RendercvMaster(NormalizeYAMLMap(m).(map[string]any))
}

// ---------------------------------------------------------------------------
// MergeTailored basic tests
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
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	design, _ := merged["design"].(map[string]any)
	if design["theme"] != "sb2nov" {
		t.Fatalf("expected design.theme preserved, got %v", design["theme"])
	}

	sections := CvSections(merged)
	summary := StringSliceField(sections, "summary")
	if len(summary) != 1 || summary[0] != "New tailored summary." {
		t.Fatalf("summary not replaced correctly: %v", summary)
	}

	skills := AsSliceOfMaps(sections["skills"])
	if StringField(skills[0], "details") != "Go, Kubernetes" {
		t.Fatalf("skill[0].details not replaced: %v", skills[0])
	}
	if StringField(skills[1], "label") != "Frontend" || StringField(skills[1], "details") != "React, TypeScript" {
		t.Fatalf("skill[1] should be untouched: %v", skills[1])
	}

	exp := AsSliceOfMaps(sections["experience"])
	if StringField(exp[0], "start_date") != "2020-01" || StringField(exp[0], "end_date") != "present" {
		t.Fatalf("experience dates must be preserved verbatim: %v", exp[0])
	}
	highlights := StringSliceField(exp[0], "highlights")
	if len(highlights) != 2 || highlights[0] != "Shipped feature X" {
		t.Fatalf("highlights not replaced correctly: %v", highlights)
	}

	origSections := CvSections(master)
	origSummary := StringSliceField(origSections, "summary")
	if origSummary[0] != "Old summary line." {
		t.Fatalf("MergeTailored must not mutate the master profile, got %v", origSummary)
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
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	sections := CvSections(merged)
	exp := AsSliceOfMaps(sections["experience"])
	if len(exp) != 2 {
		t.Fatalf("expected 2 experience entries, got %d", len(exp))
	}
	if StringField(exp[0], "company") != "StartupX" {
		t.Fatalf("expected StartupX first after reorder, got %s", StringField(exp[0], "company"))
	}
	if StringField(exp[1], "company") != "Acme Corp" {
		t.Fatalf("expected Acme Corp second after reorder, got %s", StringField(exp[1], "company"))
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
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	sections := CvSections(merged)
	exp := AsSliceOfMaps(sections["experience"])
	if len(exp) != 1 {
		t.Fatalf("expected 1 experience entry after drop, got %d", len(exp))
	}
	if StringField(exp[0], "company") != "Acme Corp" {
		t.Fatalf("expected Acme Corp to remain, got %s", StringField(exp[0], "company"))
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
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	sections := CvSections(merged)
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
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	sections := CvSections(merged)
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
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := CvSections(merged)
	exp := AsSliceOfMaps(sections["experience"])
	exp[0]["company"] = "Fabricated Co"

	violations := VerifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violations) == 0 {
		t.Fatal("expected a violation for a fabricated company")
	}
}

func TestVerifyRendercvGrounding_StrictRejectsUnlistedSkill(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := CvSections(merged)
	skills := AsSliceOfMaps(sections["skills"])
	skills[0]["details"] = "Go, Rust, Kubernetes"

	violations := VerifyRendercvGrounding(master, merged, GroundingStrict)
	if len(violations) == 0 {
		t.Fatal("expected strict grounding to reject a skill token not in the master")
	}

	violationsModerate := VerifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violationsModerate) != 0 {
		t.Fatalf("moderate grounding should not check skill tokens, got %v", violationsModerate)
	}
}

func TestVerifyRendercvGrounding_RejectsAddedSection(t *testing.T) {
	master := loadSampleMaster(t)
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	sections := CvSections(merged)
	sections["custom_section"] = []any{"some content"}

	violations := VerifyRendercvGrounding(master, merged, GroundingModerate)
	if len(violations) == 0 {
		t.Fatal("expected a violation for an added section not in master")
	}
}

// ---------------------------------------------------------------------------
// RendercvToText
// ---------------------------------------------------------------------------

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestRendercvToText_ExtractsExperienceHighlights(t *testing.T) {
	master := loadSampleMaster(t)
	text := RendercvToText(master)
	expectedTexts := []string{
		"Jane Doe",
		"Backend Go, Postgres, Docker",
		"Frontend React, TypeScript",
		"Senior Engineer at Acme Corp",
		"Did a thing",
		"Junior Dev at StartupX",
		"Built a prototype",
	}
	if !containsAll(text, expectedTexts...) {
		t.Fatalf("RendercvToText missing expected elements. Got:\n%s", text)
	}
}
