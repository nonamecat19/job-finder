package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/strutil"
)

// ---------------------------------------------------------------------------
// Step 1: Vacancy Analysis
// ---------------------------------------------------------------------------

// buildAnalyzePrompt constructs the prompt for Step 1. When VacancyHints are
// provided, they are included so the LLM can validate/refine them rather than
// starting from scratch.
func buildAnalyzePrompt(vacancy string, hints *domain.VacancyHints) string {
	vac := strutil.Truncate(vacancy, 6000)

	var b strings.Builder
	b.WriteString("Analyze this job vacancy and extract structured requirements.\n\n")
	b.WriteString("VACANCY TEXT:\n")
	b.WriteString(vac)
	b.WriteString("\n")

	if hints != nil {
		b.WriteString("\nPROVIDED HINTS (validate and refine these):\n")
		if len(hints.RequiredSkills) > 0 {
			b.WriteString("  Required skills (provided): ")
			b.WriteString(strings.Join(hints.RequiredSkills, ", "))
			b.WriteString("\n")
		}
		if len(hints.NiceToHave) > 0 {
			b.WriteString("  Nice-to-have skills (provided): ")
			b.WriteString(strings.Join(hints.NiceToHave, ", "))
			b.WriteString("\n")
		}
		if hints.ExperienceLevel != "" {
			b.WriteString("  Experience level (provided): ")
			b.WriteString(hints.ExperienceLevel)
			b.WriteString("\n")
		}
		b.WriteString("\nVerify the hints against the vacancy text. Add any missing required/nice-to-have skills. Correct the experience level if the hints seem wrong.\n")
	}

	b.WriteString("\nReturn a VacancyAnalysis with:\n")
	b.WriteString("- requiredSkills: skills explicitly listed as required/mandatory\n")
	b.WriteString("- niceToHaveSkills: preferred but not required\n")
	b.WriteString("- experienceLevel: one of junior|mid|senior|lead|staff|principal\n")
	b.WriteString("- keyResponsibilities: top 8-10 responsibilities\n")
	b.WriteString("- industryKeywords: domain terms (e.g. fintech, healthcare, SaaS, e-commerce)\n")
	b.WriteString("- seniorityKeywords: leadership indicators (e.g. mentor, lead team, architecture decisions)")

	return b.String()
}

// analyzeVacancy calls the LLM to produce a VacancyAnalysis from the raw
// vacancy text (optionally enriched with caller-provided hints).
func analyzeVacancy(ctx context.Context, lc llm.Provider, model, vacancy string, hints *domain.VacancyHints) (domain.VacancyAnalysis, error) {
	prompt := buildAnalyzePrompt(vacancy, hints)
	return llm.CompleteStructured[domain.VacancyAnalysis](ctx, lc, prompt, &llm.CompleteOptions{
		System: "You are a job-market analyst who extracts structured requirements from vacancy descriptions. Be precise and concise.",
		Model:  model,
	})
}

// ---------------------------------------------------------------------------
// Shape targets
//
// The prompts state approximate targets — sentence counts, bullets per job —
// derived from the run's ShapeConfig instead of the literals this pipeline
// used to hardcode. The default config reproduces those literals exactly, so
// a user who changes nothing gets the prompts (and the output) they had
// before the config existed.
// ---------------------------------------------------------------------------

// summarySelectRange is the tailoring target, e.g. "3-4 sentences" for the
// default SummaryLines of 4: the configured length is the ceiling.
func summarySelectRange(cfg domain.ShapeConfig) (int, int) {
	return atLeastOne(cfg.SummaryLines - 1), atLeastOne(cfg.SummaryLines)
}

// summaryExpandRange is one step above the target — the expand path runs when
// the resume came out short, so it asks for a little more than configured.
func summaryExpandRange(cfg domain.ShapeConfig) (int, int) {
	return atLeastOne(cfg.SummaryLines), atLeastOne(cfg.SummaryLines + 1)
}

// summaryCondenseRange is one step below the target. Per FR-016 the page
// target outranks the configured section lengths, so on the condense path the
// summary is asked to come in under its configured length rather than at it.
func summaryCondenseRange(cfg domain.ShapeConfig) (int, int) {
	return atLeastOne(cfg.SummaryLines - 2), atLeastOne(cfg.SummaryLines - 1)
}

// bulletsExpandRange asks for the configured cap plus a couple, matching the
// old "aim for 10-12 per job" against a default max of 10.
func bulletsExpandRange(cfg domain.ShapeConfig) (int, int) {
	base := cfg.ExperienceBulletsMax
	if base == 0 {
		base = cfg.ExperienceBulletsMin
	}
	return atLeastOne(base), atLeastOne(base + 2)
}

// bulletsCondenseRange is ~60% of the configured range, reproducing the old
// "TOP 5-6" against the default 8-10 while scaling with any other setting.
func bulletsCondenseRange(cfg domain.ShapeConfig) (int, int) {
	max := cfg.ExperienceBulletsMax
	if max == 0 {
		max = cfg.ExperienceBulletsMin
	}
	return atLeastOne(scaleDown(cfg.ExperienceBulletsMin)), atLeastOne(scaleDown(max))
}

// scaleDown returns 60% of n, rounded to nearest.
func scaleDown(n int) int { return (n*6 + 5) / 10 }

func atLeastOne(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

// pageTargetPhrase renders the configured page target the way the prompts
// have always phrased it ("TWO pages"), so a default config leaves the
// wording byte-identical.
func pageTargetPhrase(cfg domain.ShapeConfig) string {
	switch cfg.TargetPages {
	case 1:
		return "ONE page"
	case 3:
		return "THREE pages"
	default:
		return "TWO pages"
	}
}

// ---------------------------------------------------------------------------
// Step 2: Content Selection & Tailoring
// ---------------------------------------------------------------------------

// buildSelectPrompt constructs the prompt for Step 2. It receives the vacancy
// analysis from Step 1 and the full master resume content, and asks the LLM
// to select, reorder, rephrase and optionally drop content.
func buildSelectPrompt(master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string, cfg domain.ShapeConfig) string {
	sections := domain.CvSections(master)
	skills := domain.AsSliceOfMaps(sections["skills"])
	experience := domain.AsSliceOfMaps(sections["experience"])

	// Format vacancy analysis
	var analysisLines []string
	analysisLines = append(analysisLines, "REQUIRED SKILLS: "+strings.Join(analysis.RequiredSkills, ", "))
	if len(analysis.NiceToHaveSkills) > 0 {
		analysisLines = append(analysisLines, "NICE-TO-HAVE: "+strings.Join(analysis.NiceToHaveSkills, ", "))
	}
	analysisLines = append(analysisLines, "EXPERIENCE LEVEL: "+analysis.ExperienceLevel)
	if len(analysis.KeyResponsibilities) > 0 {
		analysisLines = append(analysisLines, "KEY RESPONSIBILITIES:")
		for _, r := range analysis.KeyResponsibilities {
			analysisLines = append(analysisLines, "  - "+r)
		}
	}
	if len(analysis.IndustryKeywords) > 0 {
		analysisLines = append(analysisLines, "INDUSTRY: "+strings.Join(analysis.IndustryKeywords, ", "))
	}
	if len(analysis.SeniorityKeywords) > 0 {
		analysisLines = append(analysisLines, "SENIORITY SIGNALS: "+strings.Join(analysis.SeniorityKeywords, ", "))
	}

	// Format skill groups
	var skillLines []string
	for i, s := range skills {
		skillLines = append(skillLines, fmt.Sprintf("  [%d] %s: %s", i, domain.StringField(s, "label"), domain.StringField(s, "details")))
	}

	// Format experience
	var expLines []string
	for _, e := range experience {
		line := "  - company: " + domain.StringField(e, "company")
		if pos := domain.StringField(e, "position"); pos != "" {
			line += " (" + pos + ")"
		}
		if loc := domain.StringField(e, "location"); loc != "" {
			line += " | " + loc
		}
		expLines = append(expLines, line)
		for _, h := range domain.StringSliceField(e, "highlights") {
			expLines = append(expLines, "      • "+h)
		}
	}

	var b strings.Builder
	b.WriteString("Given this vacancy analysis, tailor the candidate's resume content.\n\n")
	b.WriteString("VACANCY ANALYSIS:\n")
	b.WriteString(strings.Join(analysisLines, "\n"))
	b.WriteString("\n\n")
	b.WriteString(domain.LevelRules[level])
	b.WriteString("\n\nHARD RULES (all levels):\n")
	b.WriteString("- Return skills as one entry per group, using the SAME [index] shown below.\n")
	b.WriteString("- Return experience keyed by the EXACT company name shown below; do not add companies.\n")
	fmt.Fprintf(&b, "- For each experience entry, select the TOP %d-%d most relevant highlights and rephrase them to emphasize what the vacancy asks for.\n",
		cfg.ExperienceBulletsMin, atLeastOne(cfg.ExperienceBulletsMax))
	b.WriteString("- Keep every experience entry; never set drop to true. Do not omit any job.\n")
	b.WriteString("- Keep experience entries in the EXACT order shown in the master; do not reorder.\n")
	if cfg.SkillsMaxGroups > 0 {
		fmt.Fprintf(&b, "- Reorder skills within each group: vacancy-required skills first, and return only the %d most vacancy-relevant skill groups.\n", cfg.SkillsMaxGroups)
	} else {
		b.WriteString("- Reorder skills within each group: vacancy-required skills first, keep all relevant keywords (do not trim).\n")
	}
	b.WriteString("- Return the \"Spoken Languages\" group exactly as given: same details, same order, no rewording. It is a fact about the candidate, not a tailoring target.\n")
	b.WriteString("- Keep highlights concise, one achievement each, no fabricated numbers.\n")
	b.WriteString("- Do not drop, add, rename, or reorder any resume section. Keep the master's section set and order exactly as given. Do not populate sectionsToDrop.\n\n")
	summaryMin, summaryMax := summarySelectRange(cfg)
	fmt.Fprintf(&b, "Generate a tailored summary (%d-%d sentences) that:\n", summaryMin, summaryMax)
	b.WriteString("- Opens with the candidate's seniority level and domain expertise\n")
	b.WriteString("- References 2-3 key skills from the vacancy's required skills\n")
	b.WriteString("- Mentions one quantified achievement from the selected experience\n")
	b.WriteString("- Do not state a total number of years of experience (e.g. 'over 8 years'); describe seniority descriptively without a numeric claim\n\n")
	// A disabled skills section is removed from the document after the merge,
	// so there is no reason to spend tokens tailoring it.
	if cfg.SkillsEnabled {
		b.WriteString("SKILL GROUPS (master):\n")
		b.WriteString(strings.Join(skillLines, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("\nEXPERIENCE (master):\n")
	b.WriteString(strings.Join(expLines, "\n"))

	// Projects only enter the prompt when a project limit is configured. With
	// the default (unlimited) config this block is absent entirely, so the
	// default path costs exactly the tokens it always did.
	if cfg.ProjectsLimited() {
		projects := domain.AsSliceOfMaps(sections["projects"])
		if len(projects) > 0 {
			b.WriteString("\n\nPROJECTS (master):\n")
			for _, p := range projects {
				b.WriteString("  - name: " + domain.StringField(p, "name") + "\n")
				for _, h := range domain.StringSliceField(p, "highlights") {
					b.WriteString("      • " + h + "\n")
				}
			}
			if cfg.ProjectsMax > 0 {
				fmt.Fprintf(&b, "Return the %d most vacancy-relevant projects", cfg.ProjectsMax)
			} else {
				b.WriteString("Return the most vacancy-relevant projects")
			}
			b.WriteString(", with each name copied EXACTLY as shown above.\n")
			if cfg.ProjectBulletsMax > 0 {
				fmt.Fprintf(&b, "For each returned project, keep at most %d highlights.\n", cfg.ProjectBulletsMax)
			}
			b.WriteString("Draw every project highlight ONLY from that same project's own bullets above; never move a bullet between projects and never invent one.\n")
		}
	}

	if len(prevViolations) > 0 {
		b.WriteString("\n\nYour previous attempt violated grounding rules:\n- ")
		b.WriteString(strings.Join(prevViolations, "\n- "))
		b.WriteString("\nRegenerate without these violations.")
	}

	return b.String()
}

// selectAndTailor calls the LLM to produce a TailoredSections from the master
// resume content and the vacancy analysis.
func selectAndTailor(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string, cfg domain.ShapeConfig) (domain.TailoredSections, error) {
	prompt := buildSelectPrompt(master, analysis, level, prevViolations, cfg)
	return llm.CompleteStructured[domain.TailoredSections](ctx, lc, prompt, &llm.CompleteOptions{
		System: "You are an expert resume writer who never fabricates information. " +
			"You select, reorder and rephrase existing content to match a specific vacancy.",
		Model: model,
	})
}

// retailorForStructure is the single targeted re-prompt for feature 028's
// text-asserted-years invariant: the initial generation asserted a numeric
// total years figure that contradicts the master's derivable total, so the
// violation is fed back and the LLM asked to regenerate without it.
func retailorForStructure(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, violations []domain.StructureViolation, cfg domain.ShapeConfig) (domain.TailoredSections, error) {
	var b strings.Builder
	b.WriteString(buildSelectPrompt(master, analysis, level, nil, cfg))
	b.WriteString("\n\nSTRUCTURAL INTEGRITY VIOLATIONS (must fix):\n")
	for _, v := range violations {
		b.WriteString("- " + v.Path + ": " + v.Message + " Remove any numeric total years-of-experience claim; describe seniority descriptively instead.\n")
	}
	return llm.CompleteStructured[domain.TailoredSections](ctx, lc, b.String(), &llm.CompleteOptions{
		System: "You are an expert resume writer who never fabricates information. " +
			"You select, reorder and rephrase existing content to match a specific vacancy.",
		Model: model,
	})
}

// expandContent asks the LLM to add more detail to the already-tailored
// content so the resume fills two pages instead of one. It takes the current
// merged master and returns a new TailoredSections with a longer summary,
// more highlights per job, and richer skill details.
func expandContent(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSections, error) {
	return llm.CompleteStructured[domain.TailoredSections](ctx, lc, buildExpandPrompt(master, analysis, cfg), &llm.CompleteOptions{
		System: "You are an expert resume writer who adds relevant detail without fabricating information. " +
			"Use only content from the master profile.",
		Model: model,
	})
}

// skillGroupLines renders the current skill groups the way the select prompt
// does — "[index] label: details" — so the expand/condense passes, which are
// asked to return one entry per group by index, can see what each group
// actually holds instead of guessing from the summary and the bullets.
func skillGroupLines(sections map[string]any) []string {
	groups := domain.AsSliceOfMaps(sections["skills"])
	lines := make([]string, 0, len(groups))
	for i, s := range groups {
		lines = append(lines, fmt.Sprintf("  [%d] %s: %s", i, domain.StringField(s, "label"), domain.StringField(s, "details")))
	}
	return lines
}

// writeSkillGroups appends the current skill groups plus the rules that keep a
// group's contents matching its label. Without them the model returns details
// by index that have nothing to do with the group's category — a "Languages"
// group coming back full of tools and frameworks.
func writeSkillGroups(b *strings.Builder, sections map[string]any) {
	lines := skillGroupLines(sections)
	if len(lines) == 0 {
		return
	}
	b.WriteString("\n\nSKILL GROUPS (current):\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n- Return one entry per group using the SAME [index], and keep each group's label and category: every token must belong under that label (only programming/spoken languages under a Languages group, only databases under a Databases group, and so on).\n")
	b.WriteString("- Never move a token from one group into another, and never add a token that is not already somewhere in the groups above.\n")
	b.WriteString("- Return the \"Spoken Languages\" group exactly as given: same details, same order, no rewording.\n")
}

func buildExpandPrompt(master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig) string {
	sections := domain.CvSections(master)
	experience := domain.AsSliceOfMaps(sections["experience"])

	var expLines []string
	for _, e := range experience {
		line := "  - company: " + domain.StringField(e, "company")
		if pos := domain.StringField(e, "position"); pos != "" {
			line += " (" + pos + ")"
		}
		expLines = append(expLines, line)
		for _, h := range domain.StringSliceField(e, "highlights") {
			expLines = append(expLines, "      • "+h)
		}
	}

	var b strings.Builder
	summaryMin, summaryMax := summaryExpandRange(cfg)
	bulletsMin, bulletsMax := bulletsExpandRange(cfg)
	fmt.Fprintf(&b, "The resume below is too short (only 1 page). Expand it to fill %s with more detail.\n\n", pageTargetPhrase(cfg))
	b.WriteString("RULES:\n")
	fmt.Fprintf(&b, "- Summary: expand to %d-%d sentences with more domain context and achievements.\n", summaryMin, summaryMax)
	fmt.Fprintf(&b, "- Experience highlights: add 2-3 more relevant bullets per job (aim for %d-%d per job) from the master's available highlights.\n", bulletsMin, bulletsMax)
	b.WriteString("- Skill details: keep every keyword already in the group and reorder the vacancy-relevant ones first; don't fabricate.\n")
	b.WriteString("- Do NOT drop any job entry or skill group.\n")
	b.WriteString("- Do NOT invent new content or change company names — only use what's in the master.\n")
	b.WriteString("- Keep the same structure: same skill group indexes, same company keys.\n\n")
	b.WriteString("CURRENT CONTENT:\n")
	b.WriteString("Summary: ")
	if summaryRaw, ok := sections["summary"]; ok {
		if items, ok := summaryRaw.([]any); ok && len(items) > 0 {
			if s, ok := items[0].(string); ok {
				b.WriteString(s)
			}
		}
	}
	b.WriteString("\n\nExperience:\n")
	b.WriteString(strings.Join(expLines, "\n"))
	if cfg.SkillsEnabled {
		writeSkillGroups(&b, sections)
	}
	b.WriteString("\n\nVACANCY ANALYSIS:\n")
	b.WriteString("Required: " + strings.Join(analysis.RequiredSkills, ", ") + "\n")
	b.WriteString("Level: " + analysis.ExperienceLevel + "\n")

	return b.String()
}

// condenseContent asks the LLM to shorten the already-tailored content so
// the resume fits on a single page. It takes the current merged master and
// returns a new TailoredSections with shorter summary, fewer highlights per
// job, and more compact skill details.
func condenseContent(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSections, error) {
	return llm.CompleteStructured[domain.TailoredSections](ctx, lc, buildCondensePrompt(master, analysis, cfg), &llm.CompleteOptions{
		System: "You are an expert resume writer who makes content concise without losing impact. " +
			"Never fabricate information.",
		Model: model,
	})
}

func buildCondensePrompt(master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig) string {
	sections := domain.CvSections(master)
	experience := domain.AsSliceOfMaps(sections["experience"])

	var expLines []string
	for _, e := range experience {
		line := "  - company: " + domain.StringField(e, "company")
		if pos := domain.StringField(e, "position"); pos != "" {
			line += " (" + pos + ")"
		}
		expLines = append(expLines, line)
		for _, h := range domain.StringSliceField(e, "highlights") {
			expLines = append(expLines, "      • "+h)
		}
	}

	var b strings.Builder
	// Per FR-016 the page target outranks the configured section lengths, so
	// this path deliberately asks for less than the configured range.
	summaryMin, summaryMax := summaryCondenseRange(cfg)
	bulletsMin, bulletsMax := bulletsCondenseRange(cfg)
	fmt.Fprintf(&b, "The resume below is too long and overflows past %s. Shorten it to fit on %s.\n\n",
		strings.ToLower(pageTargetPhrase(cfg)), pageTargetPhrase(cfg))
	b.WriteString("RULES:\n")
	fmt.Fprintf(&b, "- Summary: reduce to %d-%d tight sentences.\n", summaryMin, summaryMax)
	fmt.Fprintf(&b, "- Experience highlights: keep only the TOP %d-%d most relevant per job; make each bullet shorter (one line).\n", bulletsMin, bulletsMax)
	b.WriteString("- Skill details: trim to the most vacancy-relevant keywords; remove generic filler.\n")
	b.WriteString("- Do NOT drop any job entry or skill group — just make each one shorter.\n")
	b.WriteString("- Do NOT invent new content or change company names.\n")
	b.WriteString("- Keep the same structure: same skill group indexes, same company keys.\n\n")
	b.WriteString("CURRENT CONTENT:\n")
	b.WriteString("Summary: ")
	if summaryRaw, ok := sections["summary"]; ok {
		if items, ok := summaryRaw.([]any); ok && len(items) > 0 {
			if s, ok := items[0].(string); ok {
				b.WriteString(s)
			}
		}
	}
	b.WriteString("\n\nExperience:\n")
	b.WriteString(strings.Join(expLines, "\n"))
	if cfg.SkillsEnabled {
		writeSkillGroups(&b, sections)
	}
	b.WriteString("\n\nVACANCY ANALYSIS:\n")
	b.WriteString("Required: " + strings.Join(analysis.RequiredSkills, ", ") + "\n")
	b.WriteString("Level: " + analysis.ExperienceLevel + "\n")

	return b.String()
}
