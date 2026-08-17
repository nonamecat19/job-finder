package domain

import "strings"

// RewriteVariants is the LLM structured-output shape for a single-bullet
// rewrite request: 2-3 alternate phrasings of one existing achievement, no
// new facts. Consumed via llm.CompleteStructured, same pattern as
// VacancyAnalysis / TailoredSummary.
type RewriteVariants struct {
	Variants []string `json:"variants"`
}

// FilterGroundedRewordings keeps only the proposals that are actually
// grounded in source: word-overlap covered (lcsCovered) and asserting no
// numeric claim absent from source (ungroundedMetrics) — the same two checks
// VerifyTailoredSectionsGrounding runs on an LLM-proposed rewording, applied
// here to a single bullet instead of a whole payload. Empty proposals, a
// proposal identical to source, and duplicates of each other are dropped
// too, so the result is never a list of trivial variants.
func FilterGroundedRewordings(source string, proposals []string) []string {
	sourceNorm := norm(strings.TrimSpace(source))
	seen := map[string]bool{sourceNorm: true}
	var out []string
	for _, p := range proposals {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		key := norm(trimmed)
		if seen[key] {
			continue
		}
		if !lcsCovered(trimmed, []string{source}) {
			continue
		}
		if len(ungroundedMetrics(trimmed, []string{source})) > 0 {
			continue
		}
		seen[key] = true
		out = append(out, trimmed)
	}
	return out
}
