// Package domain holds the fit-gap coach's core model (spec 009 §1):
// FitGapAssessment/GapItem/EvidenceItem/ProfileEntry, and the pure
// grounding-verification helpers the application layer's LLM rephrase call
// relies on.
package domain

import (
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword"
)

// FitGapAssessment is the coach output: failure summary + per-gap adjacent
// evidence (spec 009 §1).
type FitGapAssessment struct {
	JobID           string    `json:"jobId"`
	TotalMustHaves  int       `json:"totalMustHaves"`
	FailedMustHaves int       `json:"failedMustHaves"`
	CoveragePct     float64   `json:"coveragePct"`
	Gaps            []GapItem `json:"gaps"`
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

// ToDto flattens a FitGapAssessment into its wire shape.
func (a *FitGapAssessment) ToDto() dto.FitGapAssessmentDto {
	gaps := make([]dto.FitGapItemDto, 0, len(a.Gaps))
	for _, g := range a.Gaps {
		evidence := make([]dto.FitGapEvidenceDto, 0, len(g.AdjacentEvidence))
		for _, e := range g.AdjacentEvidence {
			evidence = append(evidence, dto.FitGapEvidenceDto{
				SourceEntry:  e.SourceEntry,
				SourceBullet: e.SourceBullet,
				Proximity:    string(e.Proximity),
				Rephrase:     e.Rephrase,
			})
		}
		gaps = append(gaps, dto.FitGapItemDto{
			Term:               g.Term,
			Polarity:           g.Polarity,
			AdjacentEvidence:   evidence,
			NoAdjacentEvidence: g.NoAdjacentEvidence,
		})
	}
	return dto.FitGapAssessmentDto{
		JobID:           a.JobID,
		TotalMustHaves:  a.TotalMustHaves,
		FailedMustHaves: a.FailedMustHaves,
		CoveragePct:     a.CoveragePct,
		Gaps:            gaps,
	}
}
