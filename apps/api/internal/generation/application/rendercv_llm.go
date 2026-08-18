package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/strutil"
)

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

func analyzeVacancy(ctx context.Context, lc llm.Provider, model, vacancy string, hints *domain.VacancyHints) (domain.VacancyAnalysis, error) {
	ctx, cancel := context.WithTimeout(ctx, analyzeStageTimeout)
	defer cancel()
	prompt := buildAnalyzePrompt(vacancy, hints)
	maxT := analysisMaxTokens
	return llm.CompleteStructured[domain.VacancyAnalysis](ctx, lc, prompt, &llm.CompleteOptions{
		System:    "You are a job-market analyst who extracts structured requirements from vacancy descriptions. Be precise and concise.",
		Model:     model,
		MaxTokens: &maxT,
	})
}

func summarySelectRange(cfg domain.ShapeConfig) (int, int) {
	return atLeastOne(cfg.SummaryLines - 1), atLeastOne(cfg.SummaryLines)
}

func bulletsExpandRange(cfg domain.ShapeConfig) (int, int) {
	base := cfg.ExperienceBulletsMax
	if base == 0 {
		base = cfg.ExperienceBulletsMin
	}
	return atLeastOne(base), atLeastOne(base + 2)
}

func bulletsCondenseRange(cfg domain.ShapeConfig) (int, int) {
	max := cfg.ExperienceBulletsMax
	if max == 0 {
		max = cfg.ExperienceBulletsMin
	}
	return atLeastOne(scaleDown(cfg.ExperienceBulletsMin)), atLeastOne(scaleDown(max))
}

func scaleDown(n int) int { return (n*6 + 5) / 10 }

func atLeastOne(n int) int {
	if n < 1 {
		return 1
	}
	return n
}

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

const (
	selectMaxTokens = 16384

	analysisMaxTokens = 8192

	summaryMaxTokens = 2048
)

const generationMaxTokens = selectMaxTokens

const (
	analyzeStageTimeout = 90 * time.Second
	selectStageTimeout  = 240 * time.Second
	summaryStageTimeout = 120 * time.Second
)

func renderAnalysisLines(analysis domain.VacancyAnalysis) []string {
	var lines []string
	lines = append(lines, "REQUIRED SKILLS: "+strings.Join(analysis.RequiredSkills, ", "))
	if len(analysis.NiceToHaveSkills) > 0 {
		lines = append(lines, "NICE-TO-HAVE: "+strings.Join(analysis.NiceToHaveSkills, ", "))
	}
	lines = append(lines, "EXPERIENCE LEVEL: "+analysis.ExperienceLevel)
	if len(analysis.KeyResponsibilities) > 0 {
		lines = append(lines, "KEY RESPONSIBILITIES:")
		for _, r := range analysis.KeyResponsibilities {
			lines = append(lines, "  - "+r)
		}
	}
	if len(analysis.IndustryKeywords) > 0 {
		lines = append(lines, "INDUSTRY: "+strings.Join(analysis.IndustryKeywords, ", "))
	}
	if len(analysis.SeniorityKeywords) > 0 {
		lines = append(lines, "SENIORITY SIGNALS: "+strings.Join(analysis.SeniorityKeywords, ", "))
	}
	return lines
}

func renderSkillGroupLines(skills []map[string]any) []string {
	var lines []string
	for i, s := range skills {
		lines = append(lines, fmt.Sprintf("  [%d] %s: %s", i, domain.StringField(s, "label"), domain.StringField(s, "details")))
	}
	return lines
}

func renderExperienceEntryLines(e map[string]any) []string {
	var lines []string
	line := "  - company: " + domain.StringField(e, "company")
	if pos := domain.StringField(e, "position"); pos != "" {
		line += " (" + pos + ")"
	}
	if loc := domain.StringField(e, "location"); loc != "" {
		line += " | " + loc
	}
	lines = append(lines, line)
	for i, h := range domain.StringSliceField(e, "highlights") {
		lines = append(lines, fmt.Sprintf("      [%d] %s", i, h))
	}
	return lines
}

func buildSelectPrompt(master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string, cfg domain.ShapeConfig) string {
	sections := domain.CvSections(master)
	skills := domain.AsSliceOfMaps(sections["skills"])
	experience := domain.AsSliceOfMaps(sections["experience"])

	analysisLines := renderAnalysisLines(analysis)
	skillLines := renderSkillGroupLines(skills)

	var expLines []string
	for _, e := range experience {
		expLines = append(expLines, renderExperienceEntryLines(e)...)
	}

	var b strings.Builder
	b.WriteString("Given this vacancy analysis, tailor the candidate's resume content.\n\n")
	b.WriteString("VACANCY ANALYSIS:\n")
	b.WriteString(strings.Join(analysisLines, "\n"))
	b.WriteString("\n\n")
	b.WriteString(domain.LevelRules[level])
	b.WriteString("\n\nHARD RULES (all levels):\n")
	b.WriteString("- Return experience keyed by the EXACT company name shown below; do not add companies.\n")
	fmt.Fprintf(&b, "- For each experience entry, select the TOP %d-%d most relevant bullets by their [index], in the order they should appear.\n",
		cfg.ExperienceBulletsMin, atLeastOne(cfg.ExperienceBulletsMax))
	b.WriteString("- A highlight is {sourceIndex, rephrased}. sourceIndex is the [index] of the bullet as shown. rephrased is optional: set it only to reword THAT bullet for this vacancy, and omit it to keep the master's wording.\n")
	b.WriteString("- A rewording never merges two bullets, never borrows from another entry, and never changes a number. A rewording that does is discarded and the original bullet is used.\n")
	b.WriteString("- Keep every experience entry; never set drop to true. Do not omit any job.\n")
	b.WriteString("- Keep experience entries in the EXACT order shown in the master; do not reorder.\n")
	b.WriteString("- Do NOT return skills. Skill selection and ordering are computed from the analysis above, not chosen here.\n")
	b.WriteString("- Keep highlights concise, one achievement each.\n")
	b.WriteString("- Do not drop, add, rename, or reorder any resume section. Keep the master's section set and order exactly as given.\n")
	b.WriteString("- Do NOT write a summary. A separate step writes it.\n\n")

	if cfg.SkillsEnabled {
		b.WriteString("SKILL GROUPS (master, reference only):\n")
		b.WriteString(strings.Join(skillLines, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("\nEXPERIENCE (master):\n")
	b.WriteString(strings.Join(expLines, "\n"))

	if cfg.ProjectsLimited() {
		projects := domain.AsSliceOfMaps(sections["projects"])
		if len(projects) > 0 {
			b.WriteString("\n\nPROJECTS (master):\n")
			for _, p := range projects {
				b.WriteString("  - name: " + domain.StringField(p, "name") + "\n")
				for i, h := range domain.StringSliceField(p, "highlights") {
					fmt.Fprintf(&b, "      [%d] %s\n", i, h)
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
			b.WriteString("Project highlights are indices into that same project's own bullet list above; an index cannot reach another project's bullets.\n")
		}
	}

	if len(prevViolations) > 0 {
		b.WriteString("\n\nYour previous attempt violated grounding rules:\n- ")
		b.WriteString(strings.Join(prevViolations, "\n- "))
		b.WriteString("\nRegenerate without these violations.")
	}

	return b.String()
}

func selectContent(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string, cfg domain.ShapeConfig) (domain.TailoredSelection, error) {
	ctx, cancel := context.WithTimeout(ctx, selectStageTimeout)
	defer cancel()
	prompt := buildSelectPrompt(master, analysis, level, prevViolations, cfg)
	maxT := selectMaxTokens
	return llm.CompleteStructured[domain.TailoredSelection](ctx, lc, prompt, &llm.CompleteOptions{
		System:       "You are an expert resume writer who never fabricates information. " + "You select, reorder and rephrase existing content to match a specific vacancy.",
		Model:        model,
		MaxTokens:    &maxT,
		ResponseMode: llm.ResponseModeStrict,
	})
}

func buildSummaryPrompt(brief domain.SummaryBrief) string {
	var b strings.Builder
	b.WriteString("Write a professional summary about the candidate.\n\n")
	if len(brief.SkillGroupLabels) > 0 {
		b.WriteString("CANDIDATE SKILL AREAS: " + strings.Join(brief.SkillGroupLabels, ", ") + "\n")
	}
	if len(brief.Highlights) > 0 {
		b.WriteString("\nCANDIDATE ACHIEVEMENTS (the only achievements you may reference):\n")
		for _, h := range brief.Highlights {
			b.WriteString("  - " + h + "\n")
		}
	}
	fmt.Fprintf(&b, "\nWrite %d-%d sentences that:\n", brief.SentenceMin, brief.SentenceMax)
	fmt.Fprintf(&b, "- Open with \"%d+ years of experience\" (derived from the candidate's dates; use it verbatim) and domain expertise\n", brief.TotalYears)
	b.WriteString("- Summarize the candidate's background and strengths, drawing from the skill areas and achievements above\n")
	b.WriteString("- Never use a seniority label (e.g. 'mid-level', 'senior') in place of the years figure\n")
	b.WriteString("- Introduce no skill, employer, credential or metric that does not appear above\n")
	if len(brief.PreviousViolations) > 0 {
		b.WriteString("\nYour previous attempt violated these grounding rules:\n- ")
		b.WriteString(strings.Join(brief.PreviousViolations, "\n- "))
		b.WriteString("\nRewrite without them.")
	}
	return b.String()
}

func writeSummary(ctx context.Context, lc llm.Provider, model string, brief domain.SummaryBrief) (domain.TailoredSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, summaryStageTimeout)
	defer cancel()
	maxT := summaryMaxTokens
	return llm.CompleteStructured[domain.TailoredSummary](ctx, lc, buildSummaryPrompt(brief), &llm.CompleteOptions{
		System:       "You are an expert resume writer who never fabricates information. You write a concise professional summary using only the facts you are given.",
		Model:        model,
		MaxTokens:    &maxT,
		ResponseMode: llm.ResponseModeStrict,
	})
}

func retailorForStructure(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, violations []domain.StructureViolation, cfg domain.ShapeConfig) (domain.TailoredSelection, error) {
	var b strings.Builder
	b.WriteString(buildSelectPrompt(master, analysis, level, nil, cfg))
	b.WriteString("\n\nSTRUCTURAL INTEGRITY VIOLATIONS (must fix):\n")
	for _, v := range violations {
		fmt.Fprintf(&b, "- %s: %s Use exactly %d+ years; never substitute a seniority label.\n", v.Path, v.Message, domain.DeriveTotalExperienceYears(master))
	}
	maxT := generationMaxTokens
	return llm.CompleteStructured[domain.TailoredSelection](ctx, lc, b.String(), &llm.CompleteOptions{
		System:       "You are an expert resume writer who never fabricates information. " + "You select, reorder and rephrase existing content to match a specific vacancy.",
		Model:        model,
		MaxTokens:    &maxT,
		ResponseMode: llm.ResponseModeStrict,
	})
}

func expandContent(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, cfg domain.ShapeConfig) (domain.TailoredSelection, error) {
	maxT := generationMaxTokens
	return llm.CompleteStructured[domain.TailoredSelection](ctx, lc, buildExpandPrompt(master, analysis, cfg), &llm.CompleteOptions{
		System:       "You are an expert resume writer who adds relevant detail without fabricating information. " + "Use only content from the master profile.",
		Model:        model,
		MaxTokens:    &maxT,
		ResponseMode: llm.ResponseModeStrict,
	})
}

func skillGroupLines(sections map[string]any) []string {
	groups := domain.AsSliceOfMaps(sections["skills"])
	lines := make([]string, 0, len(groups))
	for i, s := range groups {
		lines = append(lines, fmt.Sprintf("  [%d] %s: %s", i, domain.StringField(s, "label"), domain.StringField(s, "details")))
	}
	return lines
}

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
		for i, h := range domain.StringSliceField(e, "highlights") {
			expLines = append(expLines, fmt.Sprintf("      [%d] %s", i, h))
		}
	}

	var b strings.Builder
	bulletsMin, bulletsMax := bulletsExpandRange(cfg)
	fmt.Fprintf(&b, "The resume below is too short (only 1 page). Expand it to fill %s with more detail.\n\n", pageTargetPhrase(cfg))
	b.WriteString("RULES:\n")
	b.WriteString("- Leave the summary alone. It is written by a separate step and is not yours to change.\n")
	fmt.Fprintf(&b, "- Experience highlights: return 2-3 more bullet [index]es per job than are shown (aim for %d-%d per job).\n", bulletsMin, bulletsMax)
	b.WriteString("- A highlight is {sourceIndex, rephrased}, where sourceIndex is the [index] shown for that entry and rephrased is an optional rewording of that one bullet.\n")
	b.WriteString("- Leave skills alone. They are ordered by the pipeline, not returned here.\n")
	b.WriteString("- Do NOT drop any job entry.\n")
	b.WriteString("- Do NOT invent new content or change company names — only use what's in the master.\n")
	b.WriteString("- Keep the same structure: same company keys.\n\n")
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
