package generation

import (
	"os"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/generation/domain"
)

func loadFixtureMaster(t *testing.T) RendercvMaster {
	t.Helper()
	data, err := os.ReadFile("testdata/sample_rendercv.yaml")
	if err != nil {
		t.Fatalf("read sample_rendercv.yaml: %v", err)
	}
	master, err := ParseRendercv(string(data))
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}
	return master
}

func TestMasterToResume_AllEntryTypes(t *testing.T) {
	master := loadFixtureMaster(t)
	resume, err := MasterToResume(master)
	if err != nil {
		t.Fatalf("MasterToResume: %v", err)
	}

	if resume.Name != "Jane Doe" {
		t.Fatalf("name = %q, want Jane Doe", resume.Name)
	}

	byName := map[string]dto.Section{}
	for _, s := range resume.Sections {
		byName[s.Name] = s
	}

	cases := []struct {
		section   string
		entryType dto.EntryType
	}{
		{"Welcome to RenderCV", dto.EntryText},
		{"education", dto.EntryEducation},
		{"experience", dto.EntryExperience},
		{"projects", dto.EntryNormal},
		{"publications", dto.EntryPublication},
		{"selected_honors", dto.EntryBullet},
		{"skills", dto.EntryOneLine},
		{"patents", dto.EntryNumbered},
		{"invited_talks", dto.EntryReversedNumbered},
	}
	for _, c := range cases {
		sec, ok := byName[c.section]
		if !ok {
			t.Errorf("missing section %q", c.section)
			continue
		}
		if sec.EntryType != c.entryType {
			t.Errorf("section %q entryType = %q, want %q", c.section, sec.EntryType, c.entryType)
		}
		if len(sec.Entries) == 0 {
			t.Errorf("section %q has zero entries", c.section)
		}
	}

	edu := byName["education"].Entries[0]
	if edu.Institution == nil || *edu.Institution != "Princeton University" {
		t.Errorf("education[0].institution = %v, want Princeton University", edu.Institution)
	}
	if len(edu.Highlights) != 3 {
		t.Errorf("education[0].highlights len = %d, want 3", len(edu.Highlights))
	}

	pub := byName["publications"].Entries[0]
	if len(pub.Authors) != 3 {
		t.Errorf("publications[0].authors len = %d, want 3", len(pub.Authors))
	}

	skill := byName["skills"].Entries[0]
	if skill.Label == nil || *skill.Label != "Languages" {
		t.Errorf("skills[0].label = %v, want Languages", skill.Label)
	}

	talk := byName["invited_talks"].Entries[0]
	if talk.ReversedNumber == nil {
		t.Fatalf("invited_talks[0].reversedNumber is nil")
	}
}

func TestResumeRoundTrip_PreservesSectionOrder(t *testing.T) {
	master := loadFixtureMaster(t)
	resume, err := MasterToResume(master)
	if err != nil {
		t.Fatalf("MasterToResume: %v", err)
	}

	rebuilt, err := ResumeToMaster(resume, master)
	if err != nil {
		t.Fatalf("ResumeToMaster: %v", err)
	}

	prepared, err := PrepareMasterForMarshal(rebuilt)
	if err != nil {
		t.Fatalf("PrepareMasterForMarshal: %v", err)
	}
	sections := CvSections(prepared)
	ordered, ok := sections["_order"]
	_ = ordered
	_ = ok

	cv, _ := prepared["cv"].(map[string]any)
	orderedMap, ok := cv["sections"].(domain.OrderedYAMLMap)
	if !ok {
		t.Fatalf("cv.sections is not a domain.OrderedYAMLMap after PrepareMasterForMarshal")
	}

	originalOrder := orderedSectionKeys(CvSections(master))
	if len(orderedMap.Keys) != len(originalOrder) {
		t.Fatalf("section count = %d, want %d", len(orderedMap.Keys), len(originalOrder))
	}
	for i, k := range originalOrder {
		if orderedMap.Keys[i] != k {
			t.Errorf("section order[%d] = %q, want %q", i, orderedMap.Keys[i], k)
		}
	}

	resume2, err := MasterToResume(rebuilt)
	if err != nil {
		t.Fatalf("MasterToResume (second pass): %v", err)
	}
	if len(resume2.Sections) != len(resume.Sections) {
		t.Fatalf("section count after round trip = %d, want %d", len(resume2.Sections), len(resume.Sections))
	}
	for i, s := range resume.Sections {
		if resume2.Sections[i].Name != s.Name || len(resume2.Sections[i].Entries) != len(s.Entries) {
			t.Errorf("section %d = %+v, want %+v", i, resume2.Sections[i], s)
		}
	}
}

func TestMasterToResume_UnrecognizedDataPreserved(t *testing.T) {
	yamlText := `
cv:
  name: Test User
  sections:
    experience:
      - company: Acme
        position: Engineer
        weird_custom_field: keep me
`
	master, err := ParseRendercv(yamlText)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}
	resume, err := MasterToResume(master)
	if err != nil {
		t.Fatalf("MasterToResume: %v", err)
	}
	entry := resume.Sections[0].Entries[0]
	if entry.Unrecognized["weird_custom_field"] != "keep me" {
		t.Fatalf("unrecognized field dropped, got %+v", entry.Unrecognized)
	}

	rebuilt, err := ResumeToMaster(resume, master)
	if err != nil {
		t.Fatalf("ResumeToMaster: %v", err)
	}
	sections := CvSections(rebuilt)
	entries, _ := sections["experience"].([]any)
	entryMap, _ := entries[0].(map[string]any)
	if entryMap["weird_custom_field"] != "keep me" {
		t.Fatalf("unrecognized field lost on write-back, got %+v", entryMap)
	}
}

func TestResumeSkillLevelRoundTrip(t *testing.T) {
	yamlText := `
cv:
  name: Test User
  sections:
    skills:
      - label: Backend
        details: Go, Node.js
        skills_level: medium
      - label: Spoken Languages
        details: English, Ukrainian
        skills_level: relevant
`
	master, err := ParseRendercv(yamlText)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}
	resume, err := MasterToResume(master)
	if err != nil {
		t.Fatalf("MasterToResume: %v", err)
	}
	var skills dto.Section
	for _, s := range resume.Sections {
		if s.Name == "skills" {
			skills = s
		}
	}
	if len(skills.Entries) != 2 {
		t.Fatalf("skills entries = %d, want 2", len(skills.Entries))
	}
	if got := skills.Entries[0].SkillLevel; got == nil || *got != "medium" {
		t.Errorf("Backend skillLevel = %v, want medium", got)
	}

	rebuilt, err := ResumeToMaster(resume, master)
	if err != nil {
		t.Fatalf("ResumeToMaster: %v", err)
	}
	groups := domain.AsSliceOfMaps(domain.CvSections(rebuilt)["skills"])
	if got := domain.StringField(groups[0], "skills_level"); got != "medium" {
		t.Errorf("skills_level after write-back = %q, want medium", got)
	}
	if domain.StringField(groups[1], "skills_level") != "relevant" {
		t.Errorf("pinned group skills_level after write-back = %q, want relevant", domain.StringField(groups[1], "skills_level"))
	}
}

func TestResumeSkillLevel_OnlyMappedForSkillsSection(t *testing.T) {
	yamlText := `
cv:
  name: Test User
  sections:
    certifications:
      - label: AWS
        details: 'Amazon Web Services, 2024'
        skills_level: medium
`
	master, err := ParseRendercv(yamlText)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}
	resume, err := MasterToResume(master)
	if err != nil {
		t.Fatalf("MasterToResume: %v", err)
	}
	entry := resume.Sections[0].Entries[0]
	if entry.SkillLevel != nil {
		t.Errorf("certifications entry exposed skillLevel = %v, want nil", *entry.SkillLevel)
	}
	if entry.Unrecognized["skills_level"] != "medium" {
		t.Errorf("skills_level should stay unrecognized outside the skills section, got %+v", entry.Unrecognized)
	}
}

func TestValidateResume(t *testing.T) {
	badEndDate := dto.Resume{
		Name: "Jane",
		Sections: []dto.Section{{
			Name:      "experience",
			EntryType: dto.EntryExperience,
			Entries: []dto.Entry{{
				Company:   strp("Acme"),
				StartDate: strp("2022-01"),
				EndDate:   strp("2020-01"),
			}},
		}},
	}
	if verr := ValidateResume(badEndDate); verr == nil {
		t.Fatal("expected validation error for end date before start date")
	}

	missingName := dto.Resume{}
	if verr := ValidateResume(missingName); verr == nil || verr.Path != "name" {
		t.Fatalf("expected name-required error, got %v", verr)
	}
}

func TestMasterToResume_SocialNetworksAndCustomConnections(t *testing.T) {
	yamlText := `
cv:
  name: Test User
  social_networks:
    - network: LinkedIn
      username: testuser
    - network: GitHub
      username: testuser
    - network: Telegram
      username: testuser
  custom_connections:
    - url: "https://t.me/testuser"
      placeholder: Telegram
      fontawesome_icon: telegram
`
	master, err := ParseRendercv(yamlText)
	if err != nil {
		t.Fatalf("ParseRendercv: %v", err)
	}
	resume, err := MasterToResume(master)
	if err != nil {
		t.Fatalf("MasterToResume: %v", err)
	}

	if len(resume.SocialNetworks) != 3 {
		t.Fatalf("social networks = %+v, want 3 entries", resume.SocialNetworks)
	}
	want := map[string]string{"LinkedIn": "testuser", "GitHub": "testuser", "Telegram": "testuser"}
	for _, sn := range resume.SocialNetworks {
		if want[sn.Network] != sn.Username {
			t.Errorf("social network %q username = %q, want %q", sn.Network, sn.Username, want[sn.Network])
		}
	}
	if len(resume.CustomConnections) != 1 || resume.CustomConnections[0].Placeholder != "Telegram" ||
		resume.CustomConnections[0].URL != "https://t.me/testuser" || resume.CustomConnections[0].Icon != "telegram" {
		t.Fatalf("custom connections = %+v, want 1 Telegram entry", resume.CustomConnections)
	}

	rebuilt, err := ResumeToMaster(resume, master)
	if err != nil {
		t.Fatalf("ResumeToMaster: %v", err)
	}
	resume2, err := MasterToResume(rebuilt)
	if err != nil {
		t.Fatalf("MasterToResume (round trip): %v", err)
	}
	if len(resume2.SocialNetworks) != 3 {
		t.Fatalf("social networks after round trip = %+v, want 3 entries", resume2.SocialNetworks)
	}
}

func strp(s string) *string { return &s }
