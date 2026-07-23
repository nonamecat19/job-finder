package domain

import (
	"strings"
	"unicode"
)

// GhostSignals is the deterministic measurement bundle computed by SQL (plus
// simhash) before any model call, handed to the prompt verbatim so the model
// blends rather than invents (FR-019). Every unmeasurable signal is nil
// *plus* an entry in Notes — never 0, never a guess (FR-011).
type GhostSignals struct {
	// RepostCount is always measurable (>=1: the job's own appearance).
	RepostCount int
	// DaysOpen is nil when Job.postedAt is null (edge case: no posting date).
	DaysOpen *int
	// CrossBoardCount is nil when the description is empty/a teaser.
	CrossBoardCount *int
	// AlwaysHiringCount is nil when the company name is unparseable.
	AlwaysHiringCount *int
	// Notes carries per-signal provenance: "measured" or "unknown: <reason>".
	Notes map[string]string
}

// AllOptionalSignalsUnknown reports whether every signal beyond the job's
// own repost count of 1 is unmeasurable — the "every signal is unknown"
// edge case where the service declines to score entirely (no LLM call, no
// row written; keeps SC-003 true).
func (g GhostSignals) AllOptionalSignalsUnknown() bool {
	return g.RepostCount <= 1 && g.DaysOpen == nil && g.CrossBoardCount == nil && g.AlwaysHiringCount == nil
}

// HasUnknownSignal reports whether any of the three optional signals could
// not be measured, used to cap an overconfident model result (FR-011).
func (g GhostSignals) HasUnknownSignal() bool {
	return g.DaysOpen == nil || g.CrossBoardCount == nil || g.AlwaysHiringCount == nil
}

// IsUsableCompanyName reports whether company is a real, groupable company
// name: not empty/whitespace-only, not punctuation-only, and not the
// ingestion placeholder "Unknown" (case-insensitive). A placeholder value
// must never cause every unnamed company's jobs to be grouped as one
// employer (spec edge case).
func IsUsableCompanyName(company string) bool {
	if company == "" {
		return false
	}
	if strings.EqualFold(company, "unknown") {
		return false
	}
	for _, r := range company {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false // punctuation-only
}
