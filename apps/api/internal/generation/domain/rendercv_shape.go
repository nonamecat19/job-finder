package domain

import "fmt"

// Shortfall records a configured minimum the master could not satisfy. It is
// the honest alternative to padding: when a role has fewer bullets than the
// configured floor, the resume keeps what exists and the run reports the gap.
// Nothing is ever invented to close it.
type Shortfall struct {
	Path      string // e.g. cv.sections.experience[Acme Corp].highlights
	Requested int
	Available int
}

// ShapeReport is what a run's shape enforcement produced: the config it used,
// the minima it could not meet, and how the page target came out. It is
// written to the activity record so a past document can be explained.
type ShapeReport struct {
	Config        ShapeConfig
	Shortfalls    []Shortfall
	PageTarget    int
	PagesAchieved int
	ConflictNoted bool // the page target overrode the configured section lengths
}

// ShapeConfig is the user-configurable shape of a generated resume: how long
// the summary is, how many bullets each experience entry keeps, whether the
// optional sections render at all, how many projects and project bullets
// survive, and how many pages the render loop aims for.
//
// It is a plain value type with no DB or HTTP awareness, so the generation
// pipeline depends on nothing outside its own domain package. The settings
// service maps a persisted row onto it and the generation service resolves it
// once per run, threading it through tailoring, merging and rendering.
//
// Defaults reproduce the literals the pipeline hardcoded before this type
// existed, so a default config generates the same documents it always did.
type ShapeConfig struct {
	// SummaryLines is the approximate sentence budget for the summary.
	SummaryLines int
	// SkillsEnabled renders the skills section; false removes it entirely.
	SkillsEnabled bool
	// SkillsMaxGroups caps how many skill groups are kept. 0 = unlimited.
	SkillsMaxGroups int
	// ExperienceBulletsMin is the hard floor for bullets per experience entry.
	// When the master's own entry has at least this many bullets, a
	// model-trimmed selection is padded back up from the master's remaining
	// highlights. Only a master with fewer bullets available than the floor
	// reports a shortfall instead — the floor is never met by inventing one.
	ExperienceBulletsMin int
	// ExperienceBulletsMax is the hard cap on bullets per experience entry.
	// 0 = unlimited.
	ExperienceBulletsMax int
	// TargetPages is the page count the render loop aims for.
	TargetPages int
	// ProjectsEnabled renders the projects section; false removes it entirely.
	ProjectsEnabled bool
	// ProjectsMin is the target floor for how many projects are kept.
	// 0 = no minimum. Never satisfied by inventing projects.
	ProjectsMin int
	// ProjectsMax is the hard cap on how many projects are kept. 0 = unlimited.
	ProjectsMax int
	// ProjectBulletsMax is the hard cap on bullets per project. 0 = unlimited.
	ProjectBulletsMax int
	// CertificationsEnabled renders the certifications section; false removes
	// it entirely.
	CertificationsEnabled bool
	// CertificationsMin is the target floor for how many certifications are
	// kept. 0 = no minimum. Never satisfied by inventing certifications.
	CertificationsMin int
	// CertificationsMax is the hard cap on how many certifications are kept.
	// 0 = unlimited.
	CertificationsMax int
	// FontSize is the body text size in points. Name scales to 3x body,
	// headline and connections match body — the classic rendercv theme's own
	// ratios, just parameterized on body instead of hardcoded at 10pt.
	FontSize int
}

// DefaultShapeConfig is the single source of truth for the defaults, matching
// the column defaults of the ResumeShapeSetting table and reproducing the
// pipeline's pre-feature behaviour exactly: "3-4 sentences" summary,
// "TOP 8-10" highlights per job, untrimmed skills, a two-page target and
// projects passing through verbatim.
func DefaultShapeConfig() ShapeConfig {
	return ShapeConfig{
		SummaryLines:          4,
		SkillsEnabled:         true,
		SkillsMaxGroups:       0,
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
		FontSize:              10,
	}
}

// ProjectsLimited reports whether any project limit is configured. When it is
// false (the default), projects never enter the tailoring prompt and no
// project truncation runs, so they pass through exactly as authored and token
// usage is unchanged.
func (c ShapeConfig) ProjectsLimited() bool {
	return c.ProjectsMax > 0 || c.ProjectBulletsMax > 0
}

// Validate checks every range and cross-field rule. Each message names the
// offending field and its valid range so a rejected settings update tells the
// caller exactly what to fix. Validation runs before any write, so a rejected
// config leaves the stored one untouched.
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

// ApplySectionToggles removes the optional sections the user turned off, from
// both cv.sections and the authored order. It runs on the merged document
// before verification, so grounding and structure checks see exactly the
// document that will be rendered. Deleting a section is already legal under
// the grounding subset check, so no carve-out is needed there.
//
// Skills, projects and certifications can be disabled; summary and
// experience are never offered, which keeps the structure verifier's
// coverage intact.
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

// ApplyFontSize sets the master's design.typography.font_size block from
// cfg.FontSize, in points. It overrides rather than defaults (unlike
// CompactDesign) because the setting is the explicit, user-chosen size — name
// scales to 3x body, headline and connections match body, mirroring the
// classic rendercv theme's own default ratios (30pt/10pt/10pt at body=10pt).
// section_titles is left alone: it is authored as "1.4em", already relative
// to body.
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

// ApplyHardLimits enforces the count limits the model cannot be trusted with,
// on the already-merged document: bullets per experience entry are clamped to
// ExperienceBulletsMax, skill groups to SkillsMaxGroups, projects to
// ProjectsMax and their bullets to ProjectBulletsMax. Truncation keeps the
// first N in order, which is the most relevant N: the model returns
// highlights in relevance order and the merge preserves it.
//
// ExperienceBulletsMin is a hard floor when the master can actually meet it:
// if the model under-selected and the master's own entry for that company has
// at least Min bullets, the entry is padded back up from the master's
// remaining highlights (in the master's authored order) — never an invented
// bullet, just one the model left out. Only when the master itself has fewer
// than Min bullets is the gap reported as a Shortfall instead.
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

	// Skill groups are clamped the same way: the model is asked for a shorter
	// list, but only this truncation makes the configured cap binding. Keeps
	// the first N in the master's authored order, which is its own priority
	// order.
	// Pinned groups (spoken languages) survive the cap: they are kept in place
	// and count against it, so the cap still bounds the total.
	if cfg.SkillsMaxGroups > 0 {
		if skills, ok := sections["skills"].([]any); ok && len(skills) > cfg.SkillsMaxGroups {
			sections["skills"] = capSkillGroups(skills, cfg.SkillsMaxGroups)
		}
	}

	if projects, ok := sections["projects"].([]any); ok {
		// Keep the first N in the master's authored order, matching the
		// experience rule: the retained subset is never reordered.
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
		// Keep the first N in the master's authored order, matching the
		// projects rule: the retained subset is never reordered. Unlike
		// projects there is no per-entry bullet loop — certifications are
		// one_line entries with no highlights to clamp (research D2).
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

// padHighlights fills a model-trimmed highlight list back up to min bullets
// using the master's own remaining highlights, in the master's authored
// order, skipping any already present. It never exceeds max (when capped),
// so a model that under-selected on a short entry never ends up over the
// configured ceiling.
// capSkillGroups trims skill groups to max while keeping every pinned group.
// Pinned groups are kept regardless of position; the remaining slots go to the
// other groups in their authored order, and the result keeps that order too.
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
