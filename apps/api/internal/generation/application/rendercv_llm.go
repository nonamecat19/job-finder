package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/llm"
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
	b.WriteString("- keyResponsibilities: top 3-5 responsibilities\n")
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
// Step 2: Content Selection & Tailoring
// ---------------------------------------------------------------------------

// buildSelectPrompt constructs the prompt for Step 2. It receives the vacancy
// analysis from Step 1 and the full master resume content, and asks the LLM
// to select, reorder, rephrase and optionally drop content.
func buildSelectPrompt(master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string) string {
	sections := domain.CvSections(master)
	skills := domain.AsSliceOfMaps(sections["skills"])
	experience := domain.AsSliceOfMaps(sections["experience"])
	sectionKeys := domain.SectionKeys(sections)

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
	b.WriteString("- For each experience entry, select the TOP 3-5 most relevant highlights and rephrase them to emphasize what the vacancy asks for.\n")
	b.WriteString("- Set drop: true only for entries with score below 3 (completely irrelevant to the role).\n")
	b.WriteString("- Reorder experience: most relevant company first.\n")
	b.WriteString("- Reorder skills within each group: vacancy-required skills first.\n")
	b.WriteString("- Keep highlights concise, one achievement each, no fabricated numbers.\n")
	b.WriteString("- Decide which sections to drop. Section keys available: ")
	b.WriteString(strings.Join(sectionKeys, ", "))
	b.WriteString("\n- NEVER drop: summary, experience, education, skills.\n")
	b.WriteString("- Drop academic sections (patents, invited_talks, publications) for non-academic roles.\n")
	b.WriteString("- Drop projects if they are irrelevant to the vacancy.\n\n")
	b.WriteString("Generate a tailored summary (2-3 sentences) that:\n")
	b.WriteString("- Opens with the candidate's seniority level and domain expertise\n")
	b.WriteString("- References 2-3 key skills from the vacancy's required skills\n")
	b.WriteString("- Mentions one quantified achievement from the selected experience\n\n")
	b.WriteString("SKILL GROUPS (master):\n")
	b.WriteString(strings.Join(skillLines, "\n"))
	b.WriteString("\n\nEXPERIENCE (master):\n")
	b.WriteString(strings.Join(expLines, "\n"))

	if len(prevViolations) > 0 {
		b.WriteString("\n\nYour previous attempt violated grounding rules:\n- ")
		b.WriteString(strings.Join(prevViolations, "\n- "))
		b.WriteString("\nRegenerate without these violations.")
	}

	return b.String()
}

// selectAndTailor calls the LLM to produce a TailoredSections from the master
// resume content and the vacancy analysis.
func selectAndTailor(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, level domain.GroundingLevel, prevViolations []string) (domain.TailoredSections, error) {
	prompt := buildSelectPrompt(master, analysis, level, prevViolations)
	return llm.CompleteStructured[domain.TailoredSections](ctx, lc, prompt, &llm.CompleteOptions{
		System: "You are an expert resume writer who never fabricates information. " +
			"You select, reorder and rephrase existing content to match a specific vacancy.",
		Model: model,
	})
}
