package rephraseadapter

import (
	"context"

	"github.com/job-finder/api/internal/keyword/application"
	"github.com/job-finder/api/internal/platform/llm"
)

type ProviderRephraseModel struct {
	p     llm.Provider
	model string
}

var _ application.RephraseModel = (*ProviderRephraseModel)(nil)

func NewProviderRephraseModel(p llm.Provider, model string) *ProviderRephraseModel {
	return &ProviderRephraseModel{p: p, model: model}
}

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
