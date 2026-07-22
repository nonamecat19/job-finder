// Package ghostjob implements the ghost-job detector (spec 005): it measures
// four deterministic signals from data already held (repost count, days
// open, cross-board duplicates, always-hiring count), hands them to the
// local LLM to blend into a 0-100 score plus a plain-English explanation,
// and persists the result to the generic "JobSignal" table under
// kind="ghost". Principle I: this package only ever informs — nothing here
// hides, filters, reorders, or auto-rejects a job.
package ghostjob

import "fmt"

// GhostJobResult is the structured LLM output schema, mirroring
// matching.FitResult field-for-field in shape and discipline: a plain
// struct with json + jsonschema tags plus a Validate() the retry loop in
// llm.CompleteStructured enforces (FR-010).
type GhostJobResult struct {
	Score float64 `json:"score" jsonschema:"minimum=0,maximum=100"`
	// Confidence in the score, lowered by the prompt whenever a signal is
	// unknown (FR-011); also clamped defensively by the service so a model
	// that ignores the instruction can't report false certainty (SC-005).
	Confidence float64 `json:"confidence" jsonschema:"minimum=0,maximum=1"`
	// Explanation is plain English, grounded only in the measured signals
	// handed to the prompt — never an invented fact about the employer, the
	// role, or hiring intent (FR-019).
	Explanation string `json:"explanation"`
	// TopSignals is the model's own ranking of which measured signals drove
	// the score (spec: "each contributing signal... a plain-English
	// explanation of why").
	TopSignals []string `json:"topSignals"`
}

// Validate reproduces the semantic range check llm.CompleteStructured's
// retry loop enforces via the Validator interface, exactly as
// matching.FitResult.Validate does. FR-010: a result failing this after the
// retry budget persists nothing; any prior row survives untouched.
func (g *GhostJobResult) Validate() error {
	if g.Score < 0 || g.Score > 100 {
		return fmt.Errorf("score must be between 0 and 100, got %v", g.Score)
	}
	if g.Confidence < 0 || g.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1, got %v", g.Confidence)
	}
	return nil
}
