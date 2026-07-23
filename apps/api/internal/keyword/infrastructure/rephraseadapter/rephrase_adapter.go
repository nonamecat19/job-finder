// Package rephraseadapter adapts an llm.Provider to the keyword bounded
// context's application.RephraseModel port used by application.Suggester.
package rephraseadapter

import (
	"context"

	"github.com/job-finder/api/internal/keyword/application"
	"github.com/job-finder/api/internal/llm"
)

// ProviderRephraseModel adapts an llm.Provider to the RephraseModel port used
// by Suggester. It is the production wiring; the suggester's algorithm and its
// grounding guards are exercised in tests with a deterministic fake, so this
// adapter stays deliberately thin.
type ProviderRephraseModel struct {
	p     llm.Provider
	model string
}

var _ application.RephraseModel = (*ProviderRephraseModel)(nil)

// NewProviderRephraseModel wraps p; model overrides the provider default when
// non-empty (per-task model selection, mirroring generation.Service.genModel).
func NewProviderRephraseModel(p llm.Provider, model string) *ProviderRephraseModel {
	return &ProviderRephraseModel{p: p, model: model}
}

// Rephrase asks the provider for a single reframed bullet. A low temperature
// keeps the output close to the source bullet, which the grounding post-check
// then verifies regardless.
func (m *ProviderRephraseModel) Rephrase(ctx context.Context, prompt string) (string, error) {
	temp := 0.2
	return m.p.Complete(ctx, prompt, &llm.CompleteOptions{
		System: "You reframe existing resume bullets truthfully. You never invent skills, " +
			"technologies, employers, job titles, dates, or metrics. If the source does not " +
			"support the target term, you return the bullet unchanged.",
		Temperature: &temp,
		Model:       m.model,
	})
}
