package domain

import (
	"fmt"
	"strings"
)

type DraftOutput struct {
	Text           string   `json:"text" jsonschema:"description=The outreach message body, addressed to the named contact, written in the requested tone. Must contain ONLY facts explicitly listed in ALLOWED FACTS below — never invent a technology, funding round, headcount figure, or any other specific detail."`
	SpecificClaims []string `json:"specificClaims" jsonschema:"description=Every specific factual claim the message text makes about the team, company, technology, funding, or size, copied VERBATIM from the ALLOWED FACTS list. Empty array if the message makes no specific claim."`
}

const MaxDraftChars = 500

var toneInstruction = map[Tone]string{
	ToneWarm:   "warm, friendly, and enthusiastic, while staying professional",
	ToneDirect: "direct and concise — get to the point in as few words as possible, minimal pleasantries",
	ToneFormal: "formal and polished, traditional business register",
}

func BuildPrompt(tone Tone, contactName, companyName string, facts []Fact, lastViolation string) string {
	var b strings.Builder
	b.WriteString("Write a single short outreach message to a hiring contact after the sender has just applied " +
		"to a job at their company.\n\n")

	if contactName != "" {
		fmt.Fprintf(&b, "Address it to: %s\n", contactName)
	} else {
		b.WriteString("No named contact is known — use a neutral salutation such as \"Hi there\" and never invent a name.\n")
	}
	if companyName != "" {
		fmt.Fprintf(&b, "Company: %s\n", companyName)
	}
	fmt.Fprintf(&b, "Tone: %s\n\n", toneInstruction[tone])

	b.WriteString("ALLOWED FACTS (the ONLY specific things you may state about the team, company, or role — " +
		"copy any you use into specificClaims VERBATIM, unaltered):\n")
	for _, f := range facts {
		fmt.Fprintf(&b, "- %s: %s\n", f.Kind, f.Value)
	}
	fmt.Fprintf(&b, "\nUse at most one or two of these facts, only if they fit naturally. Keep the whole message "+
		"under %d characters. Never state a specific technology, funding figure, headcount, rating, or any other "+
		"detail that is not one of the ALLOWED FACTS above — if you are not sure something is allowed, leave it "+
		"out. This is a draft the user will read and send themselves, so it must contain no send/apply action, "+
		"just the message body.\n", MaxDraftChars)

	if lastViolation != "" {
		fmt.Fprintf(&b, "\nYour previous attempt was rejected: %s. Fix this and answer again.\n", lastViolation)
	}

	return b.String()
}

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
