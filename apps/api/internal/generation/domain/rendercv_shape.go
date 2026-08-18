package domain

import "fmt"

type Shortfall struct {
	Path      string
	Requested int
	Available int
}

type ShapeReport struct {
	Config        ShapeConfig
	Shortfalls    []Shortfall
	PageTarget    int
	PagesAchieved int
	ConflictNoted bool
}

type ShapeConfig struct {
	SummaryLines          int
	SummaryEnabled        bool
	SkillsEnabled         bool
	SkillsMaxGroups       int
	ExperienceEnabled     bool
	ExperienceBulletsMin  int
	ExperienceBulletsMax  int
	TargetPages           int
	ProjectsEnabled       bool
	ProjectsMin           int
	ProjectsMax           int
	ProjectBulletsMax     int
	CertificationsEnabled bool
	CertificationsMin     int
	CertificationsMax     int
	EducationEnabled      bool
	FontSize              int
}

func DefaultShapeConfig() ShapeConfig {
	return ShapeConfig{
		SummaryLines:          4,
		SummaryEnabled:        true,
		SkillsEnabled:         true,
		SkillsMaxGroups:       0,
		ExperienceEnabled:     true,
		ExperienceBulletsMin:  8,
		ExperienceBulletsMax:  10,
		TargetPages:           2,
		ProjectsEnabled:       true,
		ProjectsMin:           0,
		ProjectsMax:           0,
		ProjectBulletsMax:     0,
		CertificationsEnabled: true,
		CertificationsMin:     0,
		CertificationsMax:     0,
		EducationEnabled:      true,
		FontSize:              10,
	}
}

func (c ShapeConfig) ProjectsLimited() bool {
	return c.ProjectsMax > 0 || c.ProjectBulletsMax > 0
}

func (c ShapeConfig) Validate() error {
	ranges := []struct {
		name     string
		value    int
		min, max int
	}{
		{"summaryLines", c.SummaryLines, 1, 12},
		{"skillsMaxGroups", c.SkillsMaxGroups, 0, 20},
		{"experienceBulletsMin", c.ExperienceBulletsMin, 1, 20},
		{"experienceBulletsMax", c.ExperienceBulletsMax, 1, 20},
		{"targetPages", c.TargetPages, 1, 3},
		{"projectsMin", c.ProjectsMin, 0, 20},
		{"projectsMax", c.ProjectsMax, 0, 20},
		{"projectBulletsMax", c.ProjectBulletsMax, 0, 10},
		{"certificationsMin", c.CertificationsMin, 0, 20},
		{"certificationsMax", c.CertificationsMax, 0, 20},
		{"fontSize", c.FontSize, 8, 14},
	}
	for _, r := range ranges {
		if r.value < r.min || r.value > r.max {
			return fmt.Errorf("%s must be between %d and %d", r.name, r.min, r.max)
		}
	}

	if c.ExperienceBulletsMin > c.ExperienceBulletsMax {
		return fmt.Errorf("experienceBulletsMin must be <= experienceBulletsMax")
	}
	if c.ProjectsMax > 0 && c.ProjectsMin > c.ProjectsMax {
		return fmt.Errorf("projectsMin must be <= projectsMax")
	}
	if c.ProjectsMin > 0 && !c.ProjectsEnabled {
		return fmt.Errorf("projectsMin > 0 requires projectsEnabled")
	}
	if c.CertificationsMax > 0 && c.CertificationsMin > c.CertificationsMax {
		return fmt.Errorf("certificationsMin must be <= certificationsMax")
	}
	if c.CertificationsMin > 0 && !c.CertificationsEnabled {
		return fmt.Errorf("certificationsMin > 0 requires certificationsEnabled")
	}
	return nil
}

func ApplySectionToggles(master RendercvMaster, cfg ShapeConfig) {
	sections := CvSections(master)
	if sections == nil {
		return
	}
	if !cfg.SkillsEnabled {
		RemoveSection(sections, "skills")
	}
	if !cfg.ProjectsEnabled {
		RemoveSection(sections, "projects")
	}
	if !cfg.CertificationsEnabled {
		RemoveSection(sections, "certifications")
	}
}

func ApplyFontSize(master RendercvMaster, cfg ShapeConfig) {
	if cfg.FontSize <= 0 {
		return
	}
	design, _ := master["design"].(map[string]any)
	if design == nil {
		design = map[string]any{}
		master["design"] = design
	}
	typography, _ := design["typography"].(map[string]any)
	if typography == nil {
		typography = map[string]any{}
		design["typography"] = typography
	}
	fontSize, _ := typography["font_size"].(map[string]any)
	if fontSize == nil {
		fontSize = map[string]any{}
		typography["font_size"] = fontSize
	}
	body := fmt.Sprintf("%dpt", cfg.FontSize)
	fontSize["body"] = body
	fontSize["name"] = fmt.Sprintf("%dpt", cfg.FontSize*3)
	fontSize["headline"] = body
	fontSize["connections"] = body
}

func ApplyHardLimits(master, merged RendercvMaster, cfg ShapeConfig) ShapeReport {
	report := ShapeReport{Config: cfg, PageTarget: cfg.TargetPages}
	sections := CvSections(merged)
	if sections == nil {
		return report
	}

	masterHighlights := map[string][]any{}
	if masterSections := CvSections(master); masterSections != nil {
		for _, e := range AsSliceOfMaps(masterSections["experience"]) {
			highlights, _ := e["highlights"].([]any)
			masterHighlights[norm(StringField(e, "company"))] = highlights
		}
	}

	for _, e := range AsSliceOfMaps(sections["experience"]) {
		highlights, _ := e["highlights"].([]any)
		available := len(highlights)
		if cfg.ExperienceBulletsMax > 0 && available > cfg.ExperienceBulletsMax {
			e["highlights"] = highlights[:cfg.ExperienceBulletsMax]
		} else if available < cfg.ExperienceBulletsMin {
			pool := masterHighlights[norm(StringField(e, "company"))]
			if len(pool) >= cfg.ExperienceBulletsMin {
				e["highlights"] = padHighlights(highlights, pool, cfg.ExperienceBulletsMin, cfg.ExperienceBulletsMax)
			} else {
				report.Shortfalls = append(report.Shortfalls, Shortfall{
					Path:      fmt.Sprintf("cv.sections.experience[%s].highlights", StringField(e, "company")),
					Requested: cfg.ExperienceBulletsMin,
					Available: available,
				})
			}
		}
	}

	if cfg.SkillsMaxGroups > 0 {
		if skills, ok := sections["skills"].([]any); ok && len(skills) > cfg.SkillsMaxGroups {
			sections["skills"] = capSkillGroups(skills, cfg.SkillsMaxGroups)
		}
	}

	if projects, ok := sections["projects"].([]any); ok {
		if cfg.ProjectsMax > 0 && len(projects) > cfg.ProjectsMax {
			projects = projects[:cfg.ProjectsMax]
			sections["projects"] = projects
		}
		if cfg.ProjectsMin > 0 && len(projects) < cfg.ProjectsMin {
			report.Shortfalls = append(report.Shortfalls, Shortfall{
				Path:      "cv.sections.projects",
				Requested: cfg.ProjectsMin,
				Available: len(projects),
			})
		}
		if cfg.ProjectBulletsMax > 0 {
			for _, p := range AsSliceOfMaps(projects) {
				if highlights, _ := p["highlights"].([]any); len(highlights) > cfg.ProjectBulletsMax {
					p["highlights"] = highlights[:cfg.ProjectBulletsMax]
				}
			}
		}
	}

	if certs, ok := sections["certifications"].([]any); ok {
		if cfg.CertificationsMax > 0 && len(certs) > cfg.CertificationsMax {
			certs = certs[:cfg.CertificationsMax]
			sections["certifications"] = certs
		}
		if cfg.CertificationsMin > 0 && len(certs) < cfg.CertificationsMin {
			report.Shortfalls = append(report.Shortfalls, Shortfall{
				Path:      "cv.sections.certifications",
				Requested: cfg.CertificationsMin,
				Available: len(certs),
			})
		}
	}

	return report
}

func capSkillGroups(groups []any, max int) []any {
	kept := make([]any, 0, max)
	pinned := make([]bool, len(groups))
	slots := max
	for i, g := range groups {
		m, ok := g.(map[string]any)
		if ok && IsPinnedSkillGroup(StringField(m, "label")) {
			pinned[i] = true
			slots--
		}
	}
	for i, g := range groups {
		if pinned[i] {
			kept = append(kept, g)
			continue
		}
		if slots > 0 {
			kept = append(kept, g)
			slots--
		}
	}
	return kept
}

func padHighlights(current, pool []any, min, max int) []any {
	have := map[string]bool{}
	for _, h := range current {
		if s, ok := h.(string); ok {
			have[s] = true
		}
	}
	padded := append([]any{}, current...)
	for _, h := range pool {
		if len(padded) >= min {
			break
		}
		if max > 0 && len(padded) >= max {
			break
		}
		s, ok := h.(string)
		if !ok || have[s] {
			continue
		}
		have[s] = true
		padded = append(padded, h)
	}
	return padded
}

// TrimHighlights shortens every experience and project entry to at most
// maxPerEntry bullets, dropping from the end, and reports whether it changed
// anything.
//
// This is the page-fitting condense pass. It used to be an LLM call asking for
// "the TOP 5-6, each shorter", which put a third model turn between the master
// profile and the page — one more chance to reword a bullet the user never
// approved, to fit a target that is a layout problem, not a judgement call.
// The selection stage already ordered these bullets by relevance, so dropping
// from the end drops the least relevant, and the wording that survives is the
// wording that was already verified.
func TrimHighlights(doc RendercvMaster, maxPerEntry int) bool {
	if maxPerEntry < 1 {
		return false
	}
	changed := false
	sections := CvSections(doc)
	for _, key := range []string{"experience", "projects"} {
		for _, e := range AsSliceOfMaps(sections[key]) {
			highlights, _ := e["highlights"].([]any)
			if len(highlights) > maxPerEntry {
				e["highlights"] = highlights[:maxPerEntry]
				changed = true
			}
		}
	}
	return changed
}
