package domain

import (
	"fmt"
	"strings"
)

const MaxDraftChars = 500

func GroundClaims(claims []string, text string, facts []Fact) ([]GroundingTrace, bool) {
	lowerText := strings.ToLower(text)
	traces := make([]GroundingTrace, 0, len(claims))
	for _, raw := range claims {
		claim := strings.TrimSpace(raw)
		if claim == "" {
			continue
		}
		if !strings.Contains(lowerText, strings.ToLower(claim)) {
			return nil, false
		}
		fact, ok := matchFact(claim, facts)
		if !ok {
			return nil, false
		}
		traces = append(traces, GroundingTrace{Claim: claim, SignalKind: fact.Kind, SignalValue: fact.Value})
	}
	return traces, true
}

func matchFact(claim string, facts []Fact) (Fact, bool) {
	lowerClaim := strings.ToLower(claim)
	for _, f := range facts {
		if strings.Contains(strings.ToLower(f.Value), lowerClaim) {
			return f, true
		}
	}
	return Fact{}, false
}

func GenericOpener(tone Tone, contactName, companyName string) string {
	greeting := "Hi there"
	if contactName != "" {
		greeting = "Hi " + contactName
	}
	if tone == ToneFormal {
		greeting = "Dear " + contactName
		if contactName == "" {
			greeting = "Dear Hiring Team"
		}
	}

	role := "the role"
	if companyName != "" {
		role = "the role at " + companyName
	}

	switch tone {
	case ToneDirect:
		return fmt.Sprintf(
			"%s — I just applied for %s and wanted to reach out directly. "+
				"I think my background could be a strong fit and would appreciate a short conversation if you're open to it.",
			greeting, role,
		)
	case ToneFormal:
		return fmt.Sprintf(
			"%s,\n\nI am writing to follow up on my recent application for %s. "+
				"I would welcome the opportunity to discuss how my experience might align with your team's needs.\n\n"+
				"Thank you for your consideration.",
			greeting, role,
		)
	default:
		return fmt.Sprintf(
			"%s! I just applied for %s and wanted to say hello directly — I'd love to learn more about the team "+
				"and see if there's a fit. Would you be open to a quick chat?",
			greeting, role,
		)
	}
}

func EnforceLength(text string, traces []GroundingTrace, max int) (string, []GroundingTrace) {
	text = strings.TrimSpace(text)
	if len(text) > max {
		cut := text[:max]
		if idx := strings.LastIndexAny(cut, " \n\t"); idx > 0 {
			cut = cut[:idx]
		}
		text = strings.TrimSpace(cut)
	}

	lowerText := strings.ToLower(text)
	kept := make([]GroundingTrace, 0, len(traces))
	for _, tr := range traces {
		if strings.Contains(lowerText, strings.ToLower(tr.Claim)) {
			kept = append(kept, tr)
		}
	}
	return text, kept
}
