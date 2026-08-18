package domain

import "strings"

type RewriteVariants struct {
	Variants []string `json:"variants"`
}

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
