package domain

import (
	"fmt"
	"strings"
	"testing"
)

func masterWithBullets(counts map[string]int, order ...string) RendercvMaster {
	experience := make([]any, 0, len(order))
	for _, company := range order {
		highlights := make([]any, 0, counts[company])
		for i := 0; i < counts[company]; i++ {
			highlights = append(highlights, fmt.Sprintf("%s bullet %d", company, i))
		}
		experience = append(experience, map[string]any{
			"company":    company,
			"highlights": highlights,
		})
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"experience": experience}}}
}

func highlightsOf(t *testing.T, m RendercvMaster, company string) []string {
	t.Helper()
	for _, e := range AsSliceOfMaps(CvSections(m)["experience"]) {
		if StringField(e, "company") == company {
			return StringSliceField(e, "highlights")
		}
	}
	t.Fatalf("no experience entry for %q", company)
	return nil
}

func TestApplyHardLimitsClampsExperienceBullets(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 12}, "Acme")
	cfg := DefaultShapeConfig()

	report := ApplyHardLimits(m, m, cfg)

	got := highlightsOf(t, m, "Acme")
	if len(got) != 10 {
		t.Fatalf("kept %d bullets, want 10 (the configured max)", len(got))
	}
	for i, h := range got {
		if want := fmt.Sprintf("Acme bullet %d", i); h != want {
			t.Errorf("bullet %d = %q, want %q", i, h, want)
		}
	}
	if len(report.Shortfalls) != 0 {
		t.Errorf("shortfalls = %+v, want none", report.Shortfalls)
	}
}

func TestApplyHardLimitsUnlimitedMaxLeavesContentUntouched(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 15}, "Acme")
	cfg := DefaultShapeConfig()
	cfg.ExperienceBulletsMax = 0
	cfg.ExperienceBulletsMin = 1

	ApplyHardLimits(m, m, cfg)

	if got := highlightsOf(t, m, "Acme"); len(got) != 15 {
		t.Errorf("kept %d bullets, want all 15 (max 0 = unlimited)", len(got))
	}
}

func TestApplyHardLimitsReportsShortfallWithoutPadding(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 3, "StartupX": 9}, "Acme", "StartupX")
	cfg := DefaultShapeConfig()

	report := ApplyHardLimits(m, m, cfg)

	if got := highlightsOf(t, m, "Acme"); len(got) != 3 {
		t.Errorf("Acme kept %d bullets, want the 3 that exist — a minimum is never padded", len(got))
	}
	if len(report.Shortfalls) != 1 {
		t.Fatalf("shortfalls = %+v, want exactly one (Acme)", report.Shortfalls)
	}
	sf := report.Shortfalls[0]
	if !strings.Contains(sf.Path, "Acme") {
		t.Errorf("shortfall path = %q, want it to name Acme", sf.Path)
	}
	if sf.Requested != 8 || sf.Available != 3 {
		t.Errorf("shortfall = requested %d / available %d, want 8 / 3", sf.Requested, sf.Available)
	}
}

func TestApplyHardLimitsPadsFromMasterWhenModelUnderselected(t *testing.T) {
	master := masterWithBullets(map[string]int{"Acme": 9}, "Acme")
	cfg := DefaultShapeConfig()

	merged := masterWithBullets(map[string]int{"Acme": 3}, "Acme")

	report := ApplyHardLimits(master, merged, cfg)

	got := highlightsOf(t, merged, "Acme")
	if len(got) != 8 {
		t.Fatalf("kept %d bullets, want 8 (padded up to the configured min)", len(got))
	}
	for i, h := range got {
		if want := fmt.Sprintf("Acme bullet %d", i); h != want {
			t.Errorf("bullet %d = %q, want %q (master order)", i, h, want)
		}
	}
	if len(report.Shortfalls) != 0 {
		t.Errorf("shortfalls = %+v, want none — the master had enough", report.Shortfalls)
	}
}

func TestApplyHardLimitsPadDoesNotDuplicateAlreadySelectedBullets(t *testing.T) {
	master := masterWithBullets(map[string]int{"Acme": 9}, "Acme")
	cfg := DefaultShapeConfig()

	merged := RendercvMaster{"cv": map[string]any{"sections": map[string]any{"experience": []any{
		map[string]any{"company": "Acme", "highlights": []any{"Acme bullet 2", "Acme bullet 0", "Acme bullet 5"}},
	}}}}

	ApplyHardLimits(master, merged, cfg)

	got := highlightsOf(t, merged, "Acme")
	if len(got) != 8 {
		t.Fatalf("kept %d bullets, want 8", len(got))
	}
	seen := map[string]bool{}
	for _, h := range got {
		if seen[h] {
			t.Fatalf("bullet %q duplicated in %v", h, got)
		}
		seen[h] = true
	}
}

func TestApplyHardLimitsCarriesConfigAndPageTarget(t *testing.T) {
	m := masterWithBullets(map[string]int{"Acme": 9}, "Acme")
	cfg := DefaultShapeConfig()
	cfg.TargetPages = 3

	report := ApplyHardLimits(m, m, cfg)

	if report.Config != cfg {
		t.Errorf("report.Config = %+v, want the config the run used", report.Config)
	}
	if report.PageTarget != 3 {
		t.Errorf("report.PageTarget = %d, want 3", report.PageTarget)
	}
}

func TestDefaultShapeConfig(t *testing.T) {
	c := DefaultShapeConfig()
	if c.SummaryLines != 4 {
		t.Errorf("SummaryLines = %d, want 4", c.SummaryLines)
	}
	if !c.SkillsEnabled {
		t.Error("SkillsEnabled = false, want true")
	}
	if c.SkillsMaxGroups != 0 {
		t.Errorf("SkillsMaxGroups = %d, want 0", c.SkillsMaxGroups)
	}
	if c.ExperienceBulletsMin != 8 {
		t.Errorf("ExperienceBulletsMin = %d, want 8", c.ExperienceBulletsMin)
	}
	if c.ExperienceBulletsMax != 10 {
		t.Errorf("ExperienceBulletsMax = %d, want 10", c.ExperienceBulletsMax)
	}
	if c.TargetPages != 2 {
		t.Errorf("TargetPages = %d, want 2", c.TargetPages)
	}
	if !c.ProjectsEnabled {
		t.Error("ProjectsEnabled = false, want true")
	}
	if c.ProjectsMin != 0 {
		t.Errorf("ProjectsMin = %d, want 0", c.ProjectsMin)
	}
	if c.ProjectsMax != 0 {
		t.Errorf("ProjectsMax = %d, want 0", c.ProjectsMax)
	}
	if c.ProjectBulletsMax != 0 {
		t.Errorf("ProjectBulletsMax = %d, want 0", c.ProjectBulletsMax)
	}
	if !c.CertificationsEnabled {
		t.Error("CertificationsEnabled = false, want true")
	}
	if c.CertificationsMin != 0 {
		t.Errorf("CertificationsMin = %d, want 0", c.CertificationsMin)
	}
	if c.CertificationsMax != 0 {
		t.Errorf("CertificationsMax = %d, want 0", c.CertificationsMax)
	}
}

func TestShapeConfigValidate(t *testing.T) {
	with := func(mutate func(*ShapeConfig)) ShapeConfig {
		c := DefaultShapeConfig()
		mutate(&c)
		return c
	}

	tests := []struct {
		name    string
		cfg     ShapeConfig
		wantErr string
	}{
		{"defaults", DefaultShapeConfig(), ""},
		{"summaryLines too low", with(func(c *ShapeConfig) { c.SummaryLines = 0 }), "summaryLines must be between 1 and 12"},
		{"summaryLines too high", with(func(c *ShapeConfig) { c.SummaryLines = 13 }), "summaryLines must be between 1 and 12"},
		{"skillsMaxGroups too low", with(func(c *ShapeConfig) { c.SkillsMaxGroups = -1 }), "skillsMaxGroups must be between 0 and 20"},
		{"skillsMaxGroups too high", with(func(c *ShapeConfig) { c.SkillsMaxGroups = 21 }), "skillsMaxGroups must be between 0 and 20"},
		{"experienceBulletsMin too low", with(func(c *ShapeConfig) { c.ExperienceBulletsMin = 0 }), "experienceBulletsMin must be between 1 and 20"},
		{"experienceBulletsMin too high", with(func(c *ShapeConfig) {
			c.ExperienceBulletsMin = 21
			c.ExperienceBulletsMax = 20
		}), "experienceBulletsMin must be between 1 and 20"},
		{"experienceBulletsMax too low", with(func(c *ShapeConfig) {
			c.ExperienceBulletsMin = 1
			c.ExperienceBulletsMax = 0
		}), "experienceBulletsMax must be between 1 and 20"},
		{"experienceBulletsMax too high", with(func(c *ShapeConfig) { c.ExperienceBulletsMax = 21 }), "experienceBulletsMax must be between 1 and 20"},
		{"targetPages too low", with(func(c *ShapeConfig) { c.TargetPages = 0 }), "targetPages must be between 1 and 3"},
		{"targetPages too high", with(func(c *ShapeConfig) { c.TargetPages = 4 }), "targetPages must be between 1 and 3"},
		{"projectsMin too low", with(func(c *ShapeConfig) { c.ProjectsMin = -1 }), "projectsMin must be between 0 and 20"},
		{"projectsMin too high", with(func(c *ShapeConfig) { c.ProjectsMin = 21 }), "projectsMin must be between 0 and 20"},
		{"projectsMax too low", with(func(c *ShapeConfig) { c.ProjectsMax = -1 }), "projectsMax must be between 0 and 20"},
		{"projectsMax too high", with(func(c *ShapeConfig) { c.ProjectsMax = 21 }), "projectsMax must be between 0 and 20"},
		{"projectBulletsMax too low", with(func(c *ShapeConfig) { c.ProjectBulletsMax = -1 }), "projectBulletsMax must be between 0 and 10"},
		{"projectBulletsMax too high", with(func(c *ShapeConfig) { c.ProjectBulletsMax = 11 }), "projectBulletsMax must be between 0 and 10"},
		{"bullets min above max", with(func(c *ShapeConfig) {
			c.ExperienceBulletsMin = 8
			c.ExperienceBulletsMax = 5
		}), "experienceBulletsMin must be <= experienceBulletsMax"},
		{"projects min above max", with(func(c *ShapeConfig) {
			c.ProjectsMin = 5
			c.ProjectsMax = 3
		}), "projectsMin must be <= projectsMax"},
		{"projects min with unlimited max is fine", with(func(c *ShapeConfig) {
			c.ProjectsMin = 5
			c.ProjectsMax = 0
		}), ""},
		{"projects min without projects enabled", with(func(c *ShapeConfig) {
			c.ProjectsMin = 2
			c.ProjectsEnabled = false
		}), "projectsMin > 0 requires projectsEnabled"},
		{"projects disabled with no minimum is fine", with(func(c *ShapeConfig) { c.ProjectsEnabled = false }), ""},
		{"certificationsMin too low", with(func(c *ShapeConfig) { c.CertificationsMin = -1 }), "certificationsMin must be between 0 and 20"},
		{"certificationsMin too high", with(func(c *ShapeConfig) { c.CertificationsMin = 21 }), "certificationsMin must be between 0 and 20"},
		{"certificationsMax too low", with(func(c *ShapeConfig) { c.CertificationsMax = -1 }), "certificationsMax must be between 0 and 20"},
		{"certificationsMax too high", with(func(c *ShapeConfig) { c.CertificationsMax = 21 }), "certificationsMax must be between 0 and 20"},
		{"certifications min above max", with(func(c *ShapeConfig) {
			c.CertificationsMin = 5
			c.CertificationsMax = 3
		}), "certificationsMin must be <= certificationsMax"},
		{"certifications min with unlimited max is fine", with(func(c *ShapeConfig) {
			c.CertificationsMin = 5
			c.CertificationsMax = 0
		}), ""},
		{"certifications min without certifications enabled", with(func(c *ShapeConfig) {
			c.CertificationsMin = 2
			c.CertificationsEnabled = false
		}), "certificationsMin > 0 requires certificationsEnabled"},
		{"certifications disabled with no minimum is fine", with(func(c *ShapeConfig) { c.CertificationsEnabled = false }), ""},
		{"fontSize too low", with(func(c *ShapeConfig) { c.FontSize = 7 }), "fontSize must be between 8 and 14"},
		{"fontSize too high", with(func(c *ShapeConfig) { c.FontSize = 15 }), "fontSize must be between 8 and 14"},
		{"in-range non-defaults", ShapeConfig{
			SummaryLines: 2, SkillsEnabled: false, SkillsMaxGroups: 3,
			ExperienceBulletsMin: 4, ExperienceBulletsMax: 5, TargetPages: 1,
			ProjectsEnabled: true, ProjectsMin: 1, ProjectsMax: 2, ProjectBulletsMax: 3,
			CertificationsEnabled: true, CertificationsMin: 1, CertificationsMax: 2,
			FontSize: 11,
		}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestShapeConfigProjectsLimited(t *testing.T) {
	tests := []struct {
		name string
		cfg  ShapeConfig
		want bool
	}{
		{"defaults are unlimited", DefaultShapeConfig(), false},
		{"all zero", ShapeConfig{}, false},
		{"project count limited", ShapeConfig{ProjectsMax: 3}, true},
		{"project bullets limited", ShapeConfig{ProjectBulletsMax: 2}, true},
		{"both limited", ShapeConfig{ProjectsMax: 3, ProjectBulletsMax: 2}, true},
		{"minimum alone does not limit", ShapeConfig{ProjectsMin: 2}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.ProjectsLimited(); got != tt.want {
				t.Errorf("ProjectsLimited() = %v, want %v", got, tt.want)
			}
		})
	}
}

func masterWithProjects(bullets map[string]int, order ...string) RendercvMaster {
	projects := make([]any, 0, len(order))
	for _, name := range order {
		highlights := make([]any, 0, bullets[name])
		for i := 0; i < bullets[name]; i++ {
			highlights = append(highlights, fmt.Sprintf("%s bullet %d", name, i))
		}
		projects = append(projects, map[string]any{"name": name, "highlights": highlights})
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"projects": projects}}}
}

func projectNames(m RendercvMaster) []string {
	var out []string
	for _, p := range AsSliceOfMaps(CvSections(m)["projects"]) {
		out = append(out, StringField(p, "name"))
	}
	return out
}

func masterWithSkillGroups(labels ...string) RendercvMaster {
	skills := make([]any, 0, len(labels))
	for _, label := range labels {
		skills = append(skills, map[string]any{"label": label, "details": label + " details"})
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"skills": skills}}}
}

func masterWithCertifications(labels ...string) RendercvMaster {
	certifications := make([]any, 0, len(labels))
	for _, label := range labels {
		certifications = append(certifications, map[string]any{"label": label, "details": label + " details"})
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"certifications": certifications}}}
}

func certificationLabels(m RendercvMaster) []string {
	var out []string
	for _, c := range AsSliceOfMaps(CvSections(m)["certifications"]) {
		out = append(out, StringField(c, "label"))
	}
	return out
}

func certificationsFixtureMaster() RendercvMaster {
	return masterWithCertifications(
		"AWS Certified Solutions Architect",
		"Google Cloud Professional ML Engineer",
		"Certified Kubernetes Administrator",
		"TensorFlow Developer Certificate",
		"Deep Learning Specialization",
		"Certified Scrum Master",
		"Azure AI Engineer Associate",
		"NVIDIA Deep Learning Institute Certificate",
	)
}

func TestCertificationsFixtureMasterHasEightEntries(t *testing.T) {
	labels := certificationLabels(certificationsFixtureMaster())
	if len(labels) < 8 {
		t.Fatalf("certificationsFixtureMaster entries = %d, want at least 8", len(labels))
	}
	if labels[0] != "AWS Certified Solutions Architect" {
		t.Errorf("first entry = %q, want authored order preserved", labels[0])
	}
}

func skillLabels(m RendercvMaster) []string {
	var out []string
	for _, s := range AsSliceOfMaps(CvSections(m)["skills"]) {
		out = append(out, StringField(s, "label"))
	}
	return out
}

func TestApplyHardLimitsClampsSkillGroupsInMasterOrder(t *testing.T) {
	m := masterWithSkillGroups("Languages", "Backend", "Frontend", "Databases")
	cfg := DefaultShapeConfig()
	cfg.SkillsMaxGroups = 2

	ApplyHardLimits(m, m, cfg)

	got := skillLabels(m)
	if len(got) != 2 || got[0] != "Languages" || got[1] != "Backend" {
		t.Errorf("skill groups = %v, want the first two in master order [Languages Backend]", got)
	}
}

func TestApplyHardLimitsKeepsSpokenLanguagesGroupUnderCap(t *testing.T) {
	m := masterWithSkillGroups("Backend", "Frontend", "Databases", "Spoken Languages")
	cfg := DefaultShapeConfig()
	cfg.SkillsMaxGroups = 2

	ApplyHardLimits(m, m, cfg)

	got := skillLabels(m)
	if len(got) != 2 || got[0] != "Backend" || got[1] != "Spoken Languages" {
		t.Errorf("skill groups = %v, want [Backend Spoken Languages]", got)
	}
}

func TestApplyHardLimitsUnlimitedSkillGroupsKeepsAll(t *testing.T) {
	m := masterWithSkillGroups("Languages", "Backend", "Frontend")

	ApplyHardLimits(m, m, DefaultShapeConfig())

	if got := skillLabels(m); len(got) != 3 {
		t.Errorf("skill groups = %v, want all three kept", got)
	}
}

func TestApplyHardLimitsTruncatesProjectsInMasterOrder(t *testing.T) {
	m := masterWithProjects(map[string]int{"Orbit": 2, "Beacon": 2, "Comet": 2}, "Orbit", "Beacon", "Comet")
	cfg := DefaultShapeConfig()
	cfg.ProjectsMax = 2

	ApplyHardLimits(m, m, cfg)

	got := projectNames(m)
	if len(got) != 2 || got[0] != "Orbit" || got[1] != "Beacon" {
		t.Errorf("projects = %v, want the first two in master order [Orbit Beacon]", got)
	}
}

func TestApplyHardLimitsUnlimitedProjectsKeepsAll(t *testing.T) {
	m := masterWithProjects(map[string]int{"Orbit": 2, "Beacon": 2, "Comet": 2}, "Orbit", "Beacon", "Comet")

	ApplyHardLimits(m, m, DefaultShapeConfig())

	if got := projectNames(m); len(got) != 3 {
		t.Errorf("projects = %v, want all three kept", got)
	}
}

func TestApplyHardLimitsTruncatesProjectBullets(t *testing.T) {
	m := masterWithProjects(map[string]int{"Orbit": 5, "Beacon": 1}, "Orbit", "Beacon")
	cfg := DefaultShapeConfig()
	cfg.ProjectBulletsMax = 2

	ApplyHardLimits(m, m, cfg)

	orbit := StringSliceField(AsSliceOfMaps(CvSections(m)["projects"])[0], "highlights")
	if len(orbit) != 2 {
		t.Errorf("Orbit bullets = %d, want 2", len(orbit))
	}
	if orbit[0] != "Orbit bullet 0" || orbit[1] != "Orbit bullet 1" {
		t.Errorf("Orbit bullets = %v, want the first two in order", orbit)
	}
	beacon := StringSliceField(AsSliceOfMaps(CvSections(m)["projects"])[1], "highlights")
	if len(beacon) != 1 {
		t.Errorf("Beacon bullets = %d, want its single bullet untouched", len(beacon))
	}
}

func TestApplyHardLimitsReportsProjectShortfallWithoutInventing(t *testing.T) {
	m := masterWithProjects(map[string]int{"Orbit": 2}, "Orbit")
	cfg := DefaultShapeConfig()
	cfg.ProjectsMin = 3

	report := ApplyHardLimits(m, m, cfg)

	if got := projectNames(m); len(got) != 1 {
		t.Errorf("projects = %v, want the one that exists — nothing is invented to meet a minimum", got)
	}
	if len(report.Shortfalls) != 1 {
		t.Fatalf("shortfalls = %+v, want exactly one", report.Shortfalls)
	}
	sf := report.Shortfalls[0]
	if sf.Path != "cv.sections.projects" || sf.Requested != 3 || sf.Available != 1 {
		t.Errorf("shortfall = %+v, want cv.sections.projects requested 3 / available 1", sf)
	}
}

func TestApplyHardLimitsTruncatesCertificationsInMasterOrder(t *testing.T) {
	m := certificationsFixtureMaster()
	cfg := DefaultShapeConfig()
	cfg.CertificationsMax = 3

	ApplyHardLimits(m, m, cfg)

	got := certificationLabels(m)
	want := []string{
		"AWS Certified Solutions Architect",
		"Google Cloud Professional ML Engineer",
		"Certified Kubernetes Administrator",
	}
	if len(got) != len(want) {
		t.Fatalf("certifications = %v, want exactly the first 3 in authored order %v", got, want)
	}
	for i, label := range want {
		if got[i] != label {
			t.Errorf("certifications[%d] = %q, want %q (authored order preserved)", i, got[i], label)
		}
	}
}

func TestApplyHardLimitsUnlimitedCertificationsKeepsAll(t *testing.T) {
	m := certificationsFixtureMaster()
	want := len(certificationLabels(m))

	ApplyHardLimits(m, m, DefaultShapeConfig())

	if got := certificationLabels(m); len(got) != want {
		t.Errorf("certifications = %v, want all %d kept", got, want)
	}
}

func TestApplyHardLimitsCertificationsCapAboveAvailableKeepsAllInventsNothing(t *testing.T) {
	m := certificationsFixtureMaster()
	available := len(certificationLabels(m))
	cfg := DefaultShapeConfig()
	cfg.CertificationsMax = available + 5

	ApplyHardLimits(m, m, cfg)

	if got := certificationLabels(m); len(got) != available {
		t.Errorf("certifications = %v, want all %d kept, nothing invented", got, available)
	}
}

func TestApplyHardLimitsReportsCertificationsShortfallWithoutPadding(t *testing.T) {
	m := masterWithCertifications("AWS Certified Solutions Architect", "Certified Kubernetes Administrator")
	cfg := DefaultShapeConfig()
	cfg.CertificationsMin = 4

	report := ApplyHardLimits(m, m, cfg)

	if got := certificationLabels(m); len(got) != 2 {
		t.Errorf("certifications = %v, want the two that exist — nothing is invented to meet a minimum", got)
	}
	if len(report.Shortfalls) != 1 {
		t.Fatalf("shortfalls = %+v, want exactly one", report.Shortfalls)
	}
	sf := report.Shortfalls[0]
	if sf.Path != "cv.sections.certifications" || sf.Requested != 4 || sf.Available != 2 {
		t.Errorf("shortfall = %+v, want cv.sections.certifications requested 4 / available 2", sf)
	}
}

func toggleMaster() RendercvMaster {
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		SectionOrderKey: []any{"summary", "skills", "experience", "projects", "education"},
		"summary":       []any{"A summary."},
		"skills":        []any{map[string]any{"label": "Backend", "details": "Go"}},
		"experience":    []any{map[string]any{"company": "Acme", "highlights": []any{"Did a thing"}}},
		"projects":      []any{map[string]any{"name": "Orbit", "highlights": []any{"Built it"}}},
		"education":     []any{map[string]any{"institution": "MIT"}},
	}}}
}

func sectionOrder(m RendercvMaster) []string {
	raw, _ := CvSections(m)[SectionOrderKey].([]any)
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func TestApplyFontSizeSetsBodyNameHeadlineConnections(t *testing.T) {
	m := RendercvMaster{}
	cfg := DefaultShapeConfig()
	cfg.FontSize = 12

	ApplyFontSize(m, cfg)

	design := m["design"].(map[string]any)
	typography := design["typography"].(map[string]any)
	fontSize := typography["font_size"].(map[string]any)
	if fontSize["body"] != "12pt" {
		t.Errorf("body = %v, want 12pt", fontSize["body"])
	}
	if fontSize["name"] != "36pt" {
		t.Errorf("name = %v, want 36pt", fontSize["name"])
	}
	if fontSize["headline"] != "12pt" {
		t.Errorf("headline = %v, want 12pt", fontSize["headline"])
	}
	if fontSize["connections"] != "12pt" {
		t.Errorf("connections = %v, want 12pt", fontSize["connections"])
	}
}

func TestApplyFontSizeOverridesExistingValue(t *testing.T) {
	m := RendercvMaster{"design": map[string]any{"typography": map[string]any{
		"font_size": map[string]any{"body": "8pt", "name": "24pt"},
	}}}
	cfg := DefaultShapeConfig()
	cfg.FontSize = 10

	ApplyFontSize(m, cfg)

	fontSize := m["design"].(map[string]any)["typography"].(map[string]any)["font_size"].(map[string]any)
	if fontSize["body"] != "10pt" {
		t.Errorf("body = %v, want 10pt (setting overrides, unlike CompactDesign's defaults)", fontSize["body"])
	}
}

func TestApplySectionTogglesRemovesDisabledSkills(t *testing.T) {
	m := toggleMaster()
	cfg := DefaultShapeConfig()
	cfg.SkillsEnabled = false

	ApplySectionToggles(m, cfg)

	if _, present := CvSections(m)["skills"]; present {
		t.Error("skills section still present, want it removed")
	}
	want := []string{"summary", "experience", "projects", "education"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v — no stale entry may name a removed section", got, want)
	}
}

func TestApplySectionTogglesRemovesDisabledProjects(t *testing.T) {
	m := toggleMaster()
	cfg := DefaultShapeConfig()
	cfg.ProjectsEnabled = false

	ApplySectionToggles(m, cfg)

	if _, present := CvSections(m)["projects"]; present {
		t.Error("projects section still present, want it removed")
	}
	want := []string{"summary", "skills", "experience", "education"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v", got, want)
	}
}

func TestApplySectionTogglesAllEnabledIsANoOp(t *testing.T) {
	m := toggleMaster()

	ApplySectionToggles(m, DefaultShapeConfig())

	for _, key := range []string{"summary", "skills", "experience", "projects", "education"} {
		if _, present := CvSections(m)[key]; !present {
			t.Errorf("%s section missing, want every section kept", key)
		}
	}
	want := []string{"summary", "skills", "experience", "projects", "education"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v unchanged", got, want)
	}
}

func TestApplySectionTogglesKeepsMasterOrderForRemainingSections(t *testing.T) {
	m := toggleMaster()
	cfg := DefaultShapeConfig()
	cfg.SkillsEnabled = false
	cfg.ProjectsEnabled = false

	ApplySectionToggles(m, cfg)

	want := []string{"summary", "experience", "education"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v in the master's order", got, want)
	}
}

func toggleMasterWithCertifications() RendercvMaster {
	m := toggleMaster()
	sections := CvSections(m)
	sections["certifications"] = []any{map[string]any{"label": "AWS Certified Solutions Architect", "details": "AWS details"}}
	order, _ := sections[SectionOrderKey].([]any)
	sections[SectionOrderKey] = append(order, "certifications")
	return m
}

func TestApplySectionTogglesRemovesDisabledCertifications(t *testing.T) {
	m := toggleMasterWithCertifications()
	cfg := DefaultShapeConfig()
	cfg.CertificationsEnabled = false

	ApplySectionToggles(m, cfg)

	if _, present := CvSections(m)["certifications"]; present {
		t.Error("certifications section still present, want it removed")
	}
	want := []string{"summary", "skills", "experience", "projects", "education"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v — no stale entry may name a removed section", got, want)
	}
}

func TestApplySectionTogglesCertificationsEnabledIsANoOp(t *testing.T) {
	m := toggleMasterWithCertifications()

	ApplySectionToggles(m, DefaultShapeConfig())

	if _, present := CvSections(m)["certifications"]; !present {
		t.Error("certifications section missing, want it kept when enabled")
	}
	want := []string{"summary", "skills", "experience", "projects", "education", "certifications"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v unchanged", got, want)
	}
}

func TestApplySectionTogglesDisablingCertificationsWithoutSectionIsNoop(t *testing.T) {
	m := toggleMaster()
	cfg := DefaultShapeConfig()
	cfg.CertificationsEnabled = false

	ApplySectionToggles(m, cfg)

	if _, present := CvSections(m)["certifications"]; present {
		t.Error("certifications section present, want it absent as it always was")
	}
	want := []string{"summary", "skills", "experience", "projects", "education"}
	if got := sectionOrder(m); !equalStringSlices(got, want) {
		t.Errorf("_order = %v, want %v unaffected by disabling an absent section", got, want)
	}
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestTrimHighlightsDropsTheLeastRelevantBullets(t *testing.T) {
	doc := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{map[string]any{"company": "Acme", "highlights": []any{"first", "second", "third"}}},
		"projects":   []any{map[string]any{"name": "Orbit", "highlights": []any{"a", "b"}}},
	}}}

	if !TrimHighlights(doc, 2) {
		t.Fatal("TrimHighlights reported no change on a document that overflows")
	}
	sections := CvSections(doc)
	if got := StringSliceField(AsSliceOfMaps(sections["experience"])[0], "highlights"); len(got) != 2 || got[0] != "first" {
		t.Errorf("experience highlights = %v, want the first two kept", got)
	}
	if got := StringSliceField(AsSliceOfMaps(sections["projects"])[0], "highlights"); len(got) != 2 {
		t.Errorf("project highlights = %v, want them left alone at the cap", got)
	}
}

func TestTrimHighlightsReportsNoChangeAtTheFloor(t *testing.T) {
	doc := RendercvMaster{"cv": map[string]any{"sections": map[string]any{
		"experience": []any{map[string]any{"company": "Acme", "highlights": []any{"only"}}},
	}}}

	if TrimHighlights(doc, 2) {
		t.Error("TrimHighlights reported a change on a document already under the cap")
	}
}
