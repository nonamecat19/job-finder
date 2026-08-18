package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/outreach/domain"
)

func (s *Service) generateGrounded(ctx context.Context, tone domain.Tone, contactName, companyName string, facts []domain.Fact) (string, []domain.GroundingTrace) {
	var lastViolation string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, err := s.drafter.Draft(ctx, tone, contactName, companyName, facts, lastViolation)
		if err != nil {
			lastViolation = "generation failed: " + err.Error()
			continue
		}

		text := strings.TrimSpace(out.Text)
		if text == "" {
			lastViolation = "text was empty"
			continue
		}
		if len(text) > domain.MaxDraftChars {
			lastViolation = fmt.Sprintf("text was %d characters, over the %d character limit", len(text), domain.MaxDraftChars)
			continue
		}

		traces, ok := domain.GroundClaims(out.SpecificClaims, text, facts)
		if !ok {
			lastViolation = "one or more specificClaims could not be verified verbatim against ALLOWED FACTS or the message text"
			continue
		}

		return text, traces
	}

	return domain.GenericOpener(tone, contactName, companyName), []domain.GroundingTrace{}
}
