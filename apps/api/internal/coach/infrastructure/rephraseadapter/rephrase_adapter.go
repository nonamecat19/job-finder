package rephraseadapter

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/job-finder/api/internal/aiclient"
	"github.com/job-finder/api/internal/coach/application"
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

func (m *ProviderRephraseModel) Rephrase(ctx context.Context, req application.RephraseRequest) (string, error) {
	temp := 0.2
	return m.p.Complete(ctx, buildPrompt(req), &llm.CompleteOptions{
		System: "You reframe existing resume bullets truthfully. You never invent skills, " +
			"technologies, employers, job titles, dates, or metrics. If the source does not " +
			"support the target term, you return the bullet unchanged.",
		Temperature: &temp,
		Model:       m.model,
	})
}

func buildPrompt(req application.RephraseRequest) string {
	want := req.Term
	if req.Canonical != "" {
		want = req.Canonical
	}
	var b strings.Builder
	b.WriteString("Reframe the candidate's EXISTING resume bullet so that it honestly surfaces the target skill/term the job wants.\n\n")
	b.WriteString("STRICT RULES:\n")
	b.WriteString("- Rephrase ONLY the experience shown in the source bullet below.\n")
	b.WriteString("- Do NOT add any skill, technology, employer, job title, date, or metric that is not already in the source bullet.\n")
	b.WriteString("- Do NOT inflate seniority. If the source label says 'Junior', do not call it 'Senior'.\n")
	b.WriteString("- Do NOT inflate duration. If the source label says '2022–2024', do not claim '10+ years'.\n")
	b.WriteString("- Do NOT borrow technologies from other entries. Only use what is in the source bullet.\n")
	b.WriteString("- If the source bullet does not genuinely support the target term, do not stretch it — return the bullet unchanged.\n")
	b.WriteString("- Output the single reframed bullet as plain text. No preamble, no quotes, no bullet marker.\n\n")
	b.WriteString("TARGET TERM: " + want + "\n\n")
	b.WriteString("SOURCE LABEL (seniority, employer, dates):\n" + req.SourceLabel + "\n\n")
	b.WriteString("SOURCE BULLET:\n" + req.SourceBullet + "\n")
	if len(req.PriorViolations) > 0 {
		b.WriteString("\nYour previous attempt violated the no-invention rule:\n- ")
		b.WriteString(strings.Join(req.PriorViolations, "\n- "))
		b.WriteString("\nRegenerate using only what the source bullet and label contain.\n")
	}
	return b.String()
}

type AIClientRephraseModel struct {
	Client *aiclient.Client
}

var _ application.RephraseModel = (*AIClientRephraseModel)(nil)

type rephraseInput struct {
	Term            string   `json:"term"`
	Canonical       string   `json:"canonical"`
	SourceBullet    string   `json:"source_bullet"`
	SourceLabel     *string  `json:"source_label,omitempty"`
	PriorViolations []string `json:"prior_violations"`
}

type rephraseResult struct {
	Rephrase string `json:"rephrase"`
}

func (m *AIClientRephraseModel) Rephrase(ctx context.Context, req application.RephraseRequest) (string, error) {
	input := rephraseInput{
		Term:            req.Term,
		Canonical:       req.Canonical,
		SourceBullet:    req.SourceBullet,
		PriorViolations: req.PriorViolations,
	}
	if req.SourceLabel != "" {
		input.SourceLabel = &req.SourceLabel
	}

	resp, err := m.Client.Invoke(ctx, "rephrase", input, aiclient.RequestContext{})
	if err != nil {
		return "", err
	}
	if resp.Failure != nil {
		return "", &aiclientFailureError{msg: resp.Failure.Message}
	}

	var out rephraseResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return "", err
	}
	return out.Rephrase, nil
}

type aiclientFailureError struct{ msg string }

func (e *aiclientFailureError) Error() string { return "aiclient: rephrase failed: " + e.msg }
