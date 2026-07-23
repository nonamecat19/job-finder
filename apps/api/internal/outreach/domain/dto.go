package domain

import (
	"time"

	"github.com/job-finder/api/internal/dto"
)

// ToDto flattens an OutreachDraft into its wire shape.
func (d *OutreachDraft) ToDto() dto.OutreachDraftDto {
	traces := make([]dto.GroundingTraceDto, 0, len(d.GroundingTraces))
	for _, tr := range d.GroundingTraces {
		traces = append(traces, dto.GroundingTraceDto{
			Claim:       tr.Claim,
			SignalKind:  tr.SignalKind,
			SignalValue: tr.SignalValue,
		})
	}
	out := dto.OutreachDraftDto{
		JobID:           d.JobID,
		Tone:            string(d.Tone),
		Text:            d.Text,
		GroundingTraces: traces,
		GeneratedAt:     d.GeneratedAt.Format(time.RFC3339),
	}
	if d.ContactID != "" {
		id := d.ContactID
		out.ContactID = &id
	}
	if d.ContactName != "" {
		name := d.ContactName
		out.ContactName = &name
	}
	return out
}
