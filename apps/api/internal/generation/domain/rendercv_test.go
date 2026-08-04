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

// The spoken-languages group states a fact about the candidate, so a rewrite
// returned for its index is ignored and the master's details survive.
func TestMergeTailored_KeepsSpokenLanguagesGroupVerbatim(t *testing.T) {
	master := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"skills": []any{
			map[string]any{"label": "Backend", "details": "Go, Postgres"},
			map[string]any{"label": "Spoken Languages", "details": "Ukrainian (native), English (B2)"},
		},
	}}}
	payload := TailoredSections{Skills: []TailoredSkillGroup{
		{Index: 0, Details: "Go, Kubernetes"},
		{Index: 1, Details: "English (fluent), German (basic)"},
	}}

	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	skills := AsSliceOfMaps(CvSections(merged)["skills"])
	if got := StringField(skills[0], "details"); got != "Go, Kubernetes" {
		t.Errorf("skill[0].details = %q, want the tailored value", got)
	}
	if got := StringField(skills[1], "details"); got != "Ukrainian (native), English (B2)" {
		t.Errorf("spoken languages rewritten to %q, want the master's details", got)
	}
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
// Feature 028: US1 — block sequence is immutable
// ---------------------------------------------------------------------------

func TestMergeTailoredPreservesBlockOrder(t *testing.T) {
	master := RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"_order":     []any{"experience", "education", "skills", "summary", "projects"},
				"experience": []any{map[string]any{"company": "Acme Corp", "position": "Senior Engineer", "start_date": "2020-01", "end_date": "present", "highlights": []any{"Did a thing"}}},
				"education":  []any{map[string]any{"institution": "MIT", "area": "Computer Science"}},
				"skills":     []any{map[string]any{"label": "Backend", "details": "Go, Postgres, Docker"}},
				"summary":    []any{"Old summary"},
				"projects":   []any{map[string]any{"name": "SideProject", "highlights": []any{"Had fun"}}},
			},
		},
	}
	payload := TailoredSections{
		Summary: "New tailored summary.",
		Skills:  []TailoredSkillGroup{{Index: 0, Details: "Go, Kubernetes"}},
		Experience: []TailoredExperience{
			{Company: "Acme Corp", Highlights: []string{"Shipped feature X"}},
		},
	}
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	sections := CvSections(merged)
	wantOrder := []string{"experience", "education", "skills", "summary", "projects"}
	order := StringSliceField(sections, "_order")
	if len(order) != len(wantOrder) {
		t.Fatalf("merged _order changed: got %v, want %v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("merged _order changed at %d: got %v, want %v", i, order, wantOrder)
		}
	}

	// No section added, removed, renamed, or reordered: exact key set match.
	masterSections := CvSections(master)
	if len(sections) != len(masterSections) {
		t.Fatalf("section set changed: master %v vs merged %v", SectionKeys(masterSections), SectionKeys(sections))
	}
	for k := range masterSections {
		if _, ok := sections[k]; !ok {
			t.Fatalf("section %q missing from merged resume", k)
		}
	}
	// Non-protected, user-authored blocks survive.
	if _, ok := sections["projects"]; !ok {
		t.Fatal("projects section was removed by merge")
	}

	// Contents: summary replaced, highlights replaced, identity verbatim.
	if got := StringSliceField(sections, "summary"); len(got) != 1 || got[0] != "New tailored summary." {
		t.Fatalf("summary not replaced: %v", got)
	}
	exp := AsSliceOfMaps(sections["experience"])
	if len(exp) != 1 {
		t.Fatalf("experience entries changed: %v", exp)
	}
	if hl := StringSliceField(exp[0], "highlights"); len(hl) != 1 || hl[0] != "Shipped feature X" {
		t.Fatalf("highlights not replaced: %v", exp[0])
	}
	if StringField(exp[0], "company") != "Acme Corp" || StringField(exp[0], "position") != "Senior Engineer" || StringField(exp[0], "start_date") != "2020-01" {
		t.Fatalf("experience identity fields changed: %v", exp[0])
	}
}

// ---------------------------------------------------------------------------
// Feature 028: US2 — experience order and identity are preserved
// ---------------------------------------------------------------------------

func TestMergeTailoredPreservesExperienceOrder(t *testing.T) {
	master := RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"experience": []any{
					map[string]any{"company": "Acme Corp", "position": "Engineer", "start_date": "2019-01", "end_date": "2020-12", "highlights": []any{"Acme bullet"}},
					map[string]any{"company": "Globex", "position": "Engineer", "start_date": "2021-01", "end_date": "2022-12", "highlights": []any{"Globex bullet"}},
					map[string]any{"company": "Initech", "position": "Engineer", "start_date": "2023-01", "end_date": "present", "highlights": []any{"Initech bullet"}},
				},
			},
		},
	}
	// LLM tries to reorder (Initech first) and drops Globex by omission.
	payload := TailoredSections{
		Summary: "Summary",
		Experience: []TailoredExperience{
			{Company: "Initech", Highlights: []string{"Initech new bullet"}},
			{Company: "Acme Corp", Highlights: []string{"Acme new bullet"}},
		},
	}
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	exp := AsSliceOfMaps(CvSections(merged)["experience"])
	if len(exp) != 3 {
		t.Fatalf("expected 3 experience entries (no drops), got %d", len(exp))
	}
	want := []string{"Acme Corp", "Globex", "Initech"}
	for i := range want {
		if StringField(exp[i], "company") != want[i] {
			t.Fatalf("experience order changed at %d: got %s, want %s", i, StringField(exp[i], "company"), want[i])
		}
	}
	// The entry the payload omitted (Globex) retains its master highlights.
	if hl := StringSliceField(exp[1], "highlights"); len(hl) != 1 || hl[0] != "Globex bullet" {
		t.Fatalf("omitted entry's highlights changed: %v", exp[1])
	}
	// Covered entries get the new highlights.
	if hl := StringSliceField(exp[0], "highlights"); len(hl) != 1 || hl[0] != "Acme new bullet" {
		t.Fatalf("covered entry highlights not replaced: %v", exp[0])
	}
	if hl := StringSliceField(exp[2], "highlights"); len(hl) != 1 || hl[0] != "Initech new bullet" {
		t.Fatalf("covered entry highlights not replaced: %v", exp[2])
	}
}

// ---------------------------------------------------------------------------
// Feature 028: US3 — dates are immutable, text-asserted years are flagged
// ---------------------------------------------------------------------------

func TestMergeTailoredPreservesDates(t *testing.T) {
	master := RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"experience": []any{
					map[string]any{"company": "Acme Corp", "position": "Engineer", "start_date": "2019-01", "end_date": "2023-06", "highlights": []any{"Old bullet"}},
				},
			},
		},
	}
	payload := TailoredSections{
		Summary:    "Summary",
		Experience: []TailoredExperience{{Company: "Acme Corp", Highlights: []string{"New bullet"}}},
	}
	merged, err := MergeTailored(master, payload)
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	exp := AsSliceOfMaps(CvSections(merged)["experience"])
	if got := StringField(exp[0], "start_date"); got != "2019-01" {
		t.Fatalf("start_date changed: %q", got)
	}
	if got := StringField(exp[0], "end_date"); got != "2023-06" {
		t.Fatalf("end_date changed: %q", got)
	}
}

// yearsMaster derives to exactly 5 years: 2019–2021 (2) + 2021–2024 (3).
func yearsMaster() RendercvMaster {
	return RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"experience": []any{
					map[string]any{"company": "Acme Corp", "start_date": "2019-01", "end_date": "2021-01"},
					map[string]any{"company": "Globex", "start_date": "2021-01", "end_date": "2024-01"},
				},
				"summary": []any{"Old summary"},
			},
		},
	}
}

func TestVerifyStructureIntegrityFlagsYearsAssertion(t *testing.T) {
	master := yearsMaster()
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	CvSections(merged)["summary"] = []any{"Senior engineer with over 12 years of experience."}

	violations := VerifyStructureIntegrity(master, merged)
	if len(violations) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Kind != StructureTotalExperienceYears {
		t.Fatalf("unexpected kind %q", v.Kind)
	}
	if v.Path != "cv.sections.summary[0]" {
		t.Fatalf("unexpected path %q", v.Path)
	}
	if !strings.Contains(v.Message, "12") || !strings.Contains(v.Message, "5") {
		t.Fatalf("message should cite both asserted 12 and derived 5: %q", v.Message)
	}
}

func TestVerifyStructureIntegrityNoYearsAssertion(t *testing.T) {
	master := yearsMaster()
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	CvSections(merged)["summary"] = []any{"Senior backend engineer specializing in distributed systems."}

	if v := VerifyStructureIntegrity(master, merged); len(v) != 0 {
		t.Fatalf("expected no violations, got %+v", v)
	}
}

func TestVerifyStructureIntegrityFlagsAssertionInHighlights(t *testing.T) {
	master := yearsMaster()
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	exp := AsSliceOfMaps(CvSections(merged)["experience"])
	exp[0]["highlights"] = []any{"Led teams for 8+ years of experience"}

	violations := VerifyStructureIntegrity(master, merged)
	if len(violations) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Path != "cv.sections.experience[Acme Corp].highlights[0]" {
		t.Fatalf("unexpected path %q", v.Path)
	}
}

func TestStripStructureViolationsRemovesYearsClaims(t *testing.T) {
	master := yearsMaster()
	merged, err := DeepCloneYAML(master)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	CvSections(merged)["summary"] = []any{"Senior engineer with over 12 years of experience. Plus more."}
	exp := AsSliceOfMaps(CvSections(merged)["experience"])
	exp[1]["highlights"] = []any{"Built systems for 8+ years of experience and led the team."}

	violations := VerifyStructureIntegrity(master, merged)
	if len(violations) == 0 {
		t.Fatal("expected violations before strip")
	}
	StripStructureViolations(merged, violations)

	if got := StringSliceField(CvSections(merged), "summary"); len(got) != 1 || strings.Contains(got[0], "12") {
		t.Fatalf("summary years claim not stripped: %v", got)
	}
	exp2 := AsSliceOfMaps(CvSections(merged)["experience"])
	hl := StringSliceField(exp2[1], "highlights")
	if len(hl) != 1 || strings.Contains(hl[0], "8+") {
		t.Fatalf("highlight years claim not stripped: %v", hl)
	}
	if !strings.Contains(hl[0], "led the team") {
		t.Fatalf("highlight content beyond the claim should be preserved: %q", hl[0])
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

// projectMaster builds a master whose projects carry identity fields (url,
// dates) alongside their bullets, so a merge can be checked for corrupting
// them.
func projectMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"summary": []any{"Old summary."},
		"projects": []any{
			map[string]any{
				"name":       "Orbit",
				"url":        "https://example.com/orbit",
				"start_date": "2021-01",
				"end_date":   "2021-09",
				"highlights": []any{"Built the scheduler", "Cut latency"},
			},
			map[string]any{
				"name":       "Beacon",
				"url":        "https://example.com/beacon",
				"start_date": "2022-03",
				"end_date":   "present",
				"highlights": []any{"Shipped alerting"},
			},
		},
	}}}
}

func projectEntry(t *testing.T, m RendercvMaster, name string) map[string]any {
	t.Helper()
	for _, p := range AsSliceOfMaps(CvSections(m)["projects"]) {
		if StringField(p, "name") == name {
			return p
		}
	}
	t.Fatalf("no project named %q", name)
	return nil
}

func TestMergeTailoredReplacesProjectHighlightsByName(t *testing.T) {
	merged, err := MergeTailored(projectMaster(), TailoredSections{
		Projects: []TailoredProject{{Name: "Orbit", Highlights: []string{"Rewrote the scheduler in Go"}}},
	})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	if got := StringSliceField(projectEntry(t, merged, "Orbit"), "highlights"); len(got) != 1 || got[0] != "Rewrote the scheduler in Go" {
		t.Errorf("Orbit highlights = %v, want the tailored bullet", got)
	}
	// Untouched projects keep their master bullets.
	if got := StringSliceField(projectEntry(t, merged, "Beacon"), "highlights"); len(got) != 1 || got[0] != "Shipped alerting" {
		t.Errorf("Beacon highlights = %v, want the master's", got)
	}
}

func TestMergeTailoredKeepsProjectIdentityFieldsFromMaster(t *testing.T) {
	// The payload carries wrong values for every identity field; none of them
	// may reach the merged document.
	merged, err := MergeTailored(projectMaster(), TailoredSections{
		Projects: []TailoredProject{{Name: "orbit", Highlights: []string{"Rewrote the scheduler"}}},
	})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	orbit := projectEntry(t, merged, "Orbit")
	for field, want := range map[string]string{
		"name":       "Orbit",
		"url":        "https://example.com/orbit",
		"start_date": "2021-01",
		"end_date":   "2021-09",
	} {
		if got := StringField(orbit, field); got != want {
			t.Errorf("%s = %q, want %q byte-identical to the master", field, got, want)
		}
	}
}

func TestMergeTailoredIgnoresUnknownProjectNames(t *testing.T) {
	merged, err := MergeTailored(projectMaster(), TailoredSections{
		Projects: []TailoredProject{{Name: "Ghost Project", Highlights: []string{"Invented"}}},
	})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	projects := AsSliceOfMaps(CvSections(merged)["projects"])
	if len(projects) != 2 {
		t.Fatalf("projects = %d, want the master's 2 — an unknown name adds nothing", len(projects))
	}
	if got := StringSliceField(projectEntry(t, merged, "Orbit"), "highlights"); len(got) != 2 {
		t.Errorf("Orbit highlights = %v, want the master's two untouched", got)
	}
}

// The default path: no project limit configured, so the model returns no
// projects and the master's survive the merge exactly as authored (FR-003).
func TestMergeTailoredEmptyProjectPayloadLeavesMasterUntouched(t *testing.T) {
	merged, err := MergeTailored(projectMaster(), TailoredSections{Summary: "New summary."})
	if err != nil {
		t.Fatalf("MergeTailored: %v", err)
	}

	for name, want := range map[string][]string{
		"Orbit":  {"Built the scheduler", "Cut latency"},
		"Beacon": {"Shipped alerting"},
	} {
		got := StringSliceField(projectEntry(t, merged, name), "highlights")
		if len(got) != len(want) {
			t.Fatalf("%s highlights = %v, want %v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s highlight %d = %q, want %q", name, i, got[i], want[i])
			}
		}
	}
}

func TestDropUngroundedSkillTokens(t *testing.T) {
	pool := RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"skills": []any{
					map[string]any{"label": "Languages", "details": "Go, TypeScript, Python"},
					map[string]any{"label": "Frontend", "details": "React, SSR/SSG"},
				},
			},
		},
	}
	doc := RendercvMaster{
		"cv": map[string]any{
			"sections": map[string]any{
				"skills": []any{
					map[string]any{"label": "Languages", "details": "Go, Claude Code, TypeScript"},
					map[string]any{"label": "Frontend", "details": "SSR/SSG, React, Bolt"},
				},
			},
		},
	}

	DropUngroundedSkillTokens(pool, doc)

	groups := AsSliceOfMaps(CvSections(doc)["skills"])
	if got := StringField(groups[0], "details"); got != "Go, TypeScript" {
		t.Fatalf("group 0 details = %q, want %q", got, "Go, TypeScript")
	}
	if got := StringField(groups[1], "details"); got != "SSR/SSG, React" {
		t.Fatalf("group 1 details = %q, want %q", got, "SSR/SSG, React")
	}
}
