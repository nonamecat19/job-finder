package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/generation/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

// buildRankPrompt constructs the prompt for the ranking stage
// (contracts/llm-contracts.md §1, T041). It reuses buildSelectPrompt's
// numbered-bullet rendering verbatim (renderExperienceEntryLines,
// renderSkillGroupLines) so there is exactly one index space —
// HighlightRef.SourceIndex today, RankedExperience.Ranking here — and prints
// K = min(2N, A) after each entry's bullet list (research.md R2).
func buildRankPrompt(master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig, prevViolations []string) string {
	sections := domain.CvSections(master)
	skills := domain.AsSliceOfMaps(sections["skills"])
	experience := domain.AsSliceOfMaps(sections["experience"])

	analysisLines := renderAnalysisLines(analysis)
	skillLines := renderSkillGroupLines(skills)

	var expLines []string
	for _, e := range experience {
		expLines = append(expLines, renderExperienceEntryLines(e)...)
		available := len(domain.StringSliceField(e, "highlights"))
		k := domain.RankingK(available, cfg.ExperienceBulletsMin)
		expLines = append(expLines, fmt.Sprintf("      K = %d", k))
	}

	var b strings.Builder
	b.WriteString("Given this vacancy analysis, rank the candidate's EXISTING resume content by relevance. " +
		"You are ordering the candidate's own material, not writing anything new.\n\n")
	b.WriteString("VACANCY ANALYSIS:\n")
	b.WriteString(strings.Join(analysisLines, "\n"))
	b.WriteString("\n\nRULES:\n")
	b.WriteString("- Return experience keyed by the EXACT company name shown below; do not add companies.\n")
	b.WriteString("- For each experience entry, return the K most relevant bullet [index] values, most relevant first.\n")
	b.WriteString("- K is stated per entry below. Return exactly K distinct indices; do not return fewer.\n")
	b.WriteString("- Return indices ONLY. There is no field for bullet text: you are ordering the candidate's own\n")
	b.WriteString("  wording, not writing it.\n")
	b.WriteString("- Do NOT write a summary and do NOT suggest new bullets. Separate steps do both.\n")
	b.WriteString("- Keep every experience entry, in the EXACT order shown; do not reorder, drop, add or rename.\n")
	b.WriteString("- Return groupOrder: the [index] of every skill group below, most relevant first, each index exactly once.\n\n")

	if cfg.SkillsEnabled {
		b.WriteString("SKILL GROUPS (master):\n")
		b.WriteString(strings.Join(skillLines, "\n"))
		b.WriteString("\n")
	}
	b.WriteString("\nEXPERIENCE (master):\n")
	b.WriteString(strings.Join(expLines, "\n"))

	if len(prevViolations) > 0 {
		b.WriteString("\n\nYour previous attempt violated the rules above:\n- ")
		b.WriteString(strings.Join(prevViolations, "\n- "))
		b.WriteString("\nRegenerate without these violations.")
	}

	return b.String()
}

// rankContent calls the LLM to produce a RankedSelection from the master
// resume content and the vacancy analysis. It is routed through the existing
// `generation-select` router — the same task key selectContent uses — with
// the same per-stage timeout and token cap (T042, contracts/llm-contracts.md
// §1).
func rankContent(ctx context.Context, lc llm.Provider, model string, master domain.RendercvMaster, analysis domain.VacancyAnalysis, cfg domain.ShapeConfig, prevViolations []string) (domain.RankedSelection, error) {
	ctx, cancel := context.WithTimeout(ctx, selectStageTimeout)
	defer cancel()
	prompt := buildRankPrompt(master, analysis, cfg, prevViolations)
	maxT := selectMaxTokens
	return llm.CompleteStructured[domain.RankedSelection](ctx, lc, prompt, &llm.CompleteOptions{
		System: "You are ranking a candidate's existing resume content by relevance to a vacancy. " +
			"You never write, reword or invent content — you return indices only.",
		Model:        model,
		MaxTokens:    &maxT,
		ResponseMode: llm.ResponseModeStrict,
	})
}
