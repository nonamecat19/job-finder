package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/outreach/domain"
	"github.com/job-finder/api/internal/platform/llm"
)

func (s *Service) generateGrounded(ctx context.Context, tone domain.Tone, contactName, companyName string, facts []domain.Fact) (string, []domain.GroundingTrace) {
	var lastViolation string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		prompt := domain.BuildPrompt(tone, contactName, companyName, facts, lastViolation)
		out, err := llm.CompleteStructured[domain.DraftOutput](ctx, s.llmc, prompt, &llm.CompleteOptions{
			System: "You write brief, honest outreach messages. You never state a specific fact about a " +
				"company or team that is not explicitly given to you as an allowed fact. Vagueness is always " +
				"preferred to invention.",
			Model: s.model,
		})
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
