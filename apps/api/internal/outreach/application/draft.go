package application

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/aiclient"
	"github.com/job-finder/api/internal/outreach/domain"
)

// DraftAttempt is one call's result: the draft text and the specific claims
// it makes, ungrounded — generateGrounded (generate.go) still runs
// domain.GroundClaims on the result, retries on violation and falls back to
// a generic opener; none of that lives in a Drafter (T107 territory).
type DraftAttempt struct {
	Text           string
	SpecificClaims []string
}

// Drafter runs one outreach-draft attempt for the given tone/contact/
// company/facts, optionally told about the previous attempt's grounding
// violation so a retry can address it.
type Drafter interface {
	Draft(ctx context.Context, tone domain.Tone, contactName, companyName string, facts []domain.Fact, lastViolation string) (DraftAttempt, error)
}

// AIDrafter calls the `outreach` capability over aiclient (contracts/http.md
// H1-1) — the capability builds its own prompt server-side
// (prompts/outreach.py); this sends the structured fields and parses the
// result.
type AIDrafter struct {
	Client *aiclient.Client
}

var _ Drafter = (*AIDrafter)(nil)

type outreachFact struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type outreachInput struct {
	Tone          string         `json:"tone"`
	ContactName   string         `json:"contact_name"`
	CompanyName   string         `json:"company_name"`
	Facts         []outreachFact `json:"facts"`
	LastViolation string         `json:"last_violation"`
}

type outreachResult struct {
	Text           string   `json:"text"`
	SpecificClaims []string `json:"specific_claims"`
}

func (d *AIDrafter) Draft(ctx context.Context, tone domain.Tone, contactName, companyName string, facts []domain.Fact, lastViolation string) (DraftAttempt, error) {
	wireFacts := make([]outreachFact, 0, len(facts))
	for _, f := range facts {
		wireFacts = append(wireFacts, outreachFact{Kind: f.Kind, Value: f.Value})
	}
	input := outreachInput{
		Tone:          string(tone),
		ContactName:   contactName,
		CompanyName:   companyName,
		Facts:         wireFacts,
		LastViolation: lastViolation,
	}

	resp, err := d.Client.Invoke(ctx, "outreach", input, aiclient.RequestContext{})
	if err != nil {
		return DraftAttempt{}, err
	}
	if resp.Failure != nil {
		return DraftAttempt{}, fmt.Errorf("aiclient: outreach failed: %s", resp.Failure.Message)
	}

	var out outreachResult
	if err := json.Unmarshal(resp.Result, &out); err != nil {
		return DraftAttempt{}, fmt.Errorf("aiclient: outreach: unmarshal result: %w", err)
	}
	return DraftAttempt(out), nil
}
