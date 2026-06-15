// Package matching ports modules/matching/*: embedding prefilter (cosine via
// pgvector) followed by an LLM fit-score call, mirroring matching.service.ts.
package matching

import "fmt"

// FitResult is the structured LLM output schema, equivalent to the zod
// `fitSchema` in matching.service.ts:
//
//	z.object({
//	  score: z.number().min(0).max(100),
//	  matchedSkills: z.array(z.string()),
//	  missingSkills: z.array(z.string()),
//	  summary: z.string(),
//	  redFlags: z.array(z.string()),
//	})
type FitResult struct {
	Score         float64  `json:"score" jsonschema:"minimum=0,maximum=100"`
	MatchedSkills []string `json:"matchedSkills"`
	MissingSkills []string `json:"missingSkills"`
	Summary       string   `json:"summary"`
	RedFlags      []string `json:"redFlags"`
}

// Validate reproduces the zod .min(0).max(100) semantic check that
// completeStructured's retry loop enforces via schema.safeParse.
func (f *FitResult) Validate() error {
	if f.Score < 0 || f.Score > 100 {
		return fmt.Errorf("score must be between 0 and 100, got %v", f.Score)
	}
	return nil
}
