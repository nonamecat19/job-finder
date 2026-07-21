package coach

// Package coach — fit-gap assessment for missing must-haves (spec 009 §1).
// Produces the coach payload: 'you fail N of M must-haves', and per failed
// must-have up to 3 concrete items from the profile that count as adjacent.
// Every adjacent claim cites its source profile entry. If nothing adjacent
// exists, noAdjacentEvidence is set true — the honest empty result.

import "github.com/job-finder/api/internal/keyword"

// FitGapAssessment is the coach output: failure summary + per-gap adjacent
// evidence (spec 009 §1).
type FitGapAssessment struct {
	JobID            string    `json:"jobId"`
	TotalMustHaves   int       `json:"totalMustHaves"`
	FailedMustHaves  int       `json:"failedMustHaves"`
	CoveragePct      float64   `json:"coveragePct"`
	Gaps             []GapItem `json:"gaps"`
}

// GapItem is one missing must-have with up to 3 adjacent evidence items.
type GapItem struct {
	Term               string         `json:"term"`
	Polarity           string         `json:"polarity"` // always "required"
	AdjacentEvidence   []EvidenceItem `json:"adjacentEvidence"`
	NoAdjacentEvidence bool           `json:"noAdjacentEvidence"`
}

// EvidenceItem is one concrete profile item adjacent to the missing term.
type EvidenceItem struct {
	SourceEntry  string            `json:"sourceEntry"`
	SourceBullet string            `json:"sourceBullet"`
	Proximity    keyword.Proximity `json:"proximity"`
	Rephrase     string            `json:"rephrase"`
}

// ProfileEntry groups a bullet with its source metadata (employer + dates).
type ProfileEntry struct {
	// SourceLabel is the human-readable entry header, e.g.
	// "DevOps Engineer, Acme Corp (2022–2024)"
	SourceLabel string
	// Bullet is the verbatim profile bullet text
	Bullet string
}
