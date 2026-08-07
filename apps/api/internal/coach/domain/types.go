package domain

import (
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword"
)

type FitGapAssessment struct {
	JobID           string    `json:"jobId"`
	TotalMustHaves  int       `json:"totalMustHaves"`
	FailedMustHaves int       `json:"failedMustHaves"`
	CoveragePct     float64   `json:"coveragePct"`
	Gaps            []GapItem `json:"gaps"`
}

type GapItem struct {
	Term               string         `json:"term"`
	Polarity           string         `json:"polarity"`
	AdjacentEvidence   []EvidenceItem `json:"adjacentEvidence"`
	NoAdjacentEvidence bool           `json:"noAdjacentEvidence"`
}

type EvidenceItem struct {
	SourceEntry  string            `json:"sourceEntry"`
	SourceBullet string            `json:"sourceBullet"`
	Proximity    keyword.Proximity `json:"proximity"`
	Rephrase     string            `json:"rephrase"`
}

type ProfileEntry struct {
	SourceLabel string
	Bullet      string
}

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
