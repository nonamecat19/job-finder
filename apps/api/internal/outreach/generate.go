package outreach

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/llm"
)

// draftOutput is the LLM structured-output shape for the grounded-generation
// path. specificClaims must be copied verbatim from the allowed facts given
// in the prompt — that verbatim requirement is what groundClaims below
// checks (Constitution Principle II, mirroring recruiter/posting.go's
// extractedContact + groundContact pattern).
type draftOutput struct {
	Text           string   `json:"text" jsonschema:"description=The outreach message body, addressed to the named contact, written in the requested tone. Must contain ONLY facts explicitly listed in ALLOWED FACTS below — never invent a technology, funding round, headcount figure, or any other specific detail."`
	SpecificClaims []string `json:"specificClaims" jsonschema:"description=Every specific factual claim the message text makes about the team, company, technology, funding, or size, copied VERBATIM from the ALLOWED FACTS list. Empty array if the message makes no specific claim."`
}

// toneInstruction is the per-tone register instruction folded into the
// prompt. Facts (specificClaims) are identical across tones for the same
// job — only this wording instruction differs (FR-010, SC-006).
var toneInstruction = map[Tone]string{
	ToneWarm:   "warm, friendly, and enthusiastic, while staying professional",
	ToneDirect: "direct and concise — get to the point in as few words as possible, minimal pleasantries",
	ToneFormal: "formal and polished, traditional business register",
}

// generateGrounded asks the local LLM for a tone-appropriate draft that
// only cites the given facts, verifies every declared specific claim
// actually traces to one of them, and retries on violation. After
// maxAttempts it falls back to the deterministic generic opener — a
// generation that cannot be grounded is never presented (FR-006).
func (s *Service) generateGrounded(ctx context.Context, tone Tone, contactName, companyName string, facts []Fact) (string, []GroundingTrace) {
	var lastViolation string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		prompt := buildPrompt(tone, contactName, companyName, facts, lastViolation)
		out, err := llm.CompleteStructured[draftOutput](ctx, s.llmc, prompt, &llm.CompleteOptions{
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
		if len(text) > maxDraftChars {
			lastViolation = fmt.Sprintf("text was %d characters, over the %d character limit", len(text), maxDraftChars)
			continue
		}

		traces, ok := groundClaims(out.SpecificClaims, text, facts)
		if !ok {
			lastViolation = "one or more specificClaims could not be verified verbatim against ALLOWED FACTS or the message text"
			continue
		}

		return text, traces
	}

	// Every attempt either fabricated a claim or violated a hard
	// constraint: fall back to the guaranteed-safe generic opener rather
	// than ever presenting an ungrounded or over-length draft.
	return genericOpener(tone, contactName, companyName), []GroundingTrace{}
}

// groundClaims verifies every claim is (a) actually present in text and
// (b) a verbatim substring of some fact's value — the same "field must
// occur in source" check recruiter/posting.go's groundContact applies to
// LLM-extracted fields. Any claim failing either check fails the whole
// batch (ok=false), which triggers a retry rather than silently keeping
// the other, valid claims — a draft that needed one retry to become fully
// grounded is preferable to ever guessing which of several claims to trust.
func groundClaims(claims []string, text string, facts []Fact) ([]GroundingTrace, bool) {
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

// matchFact finds the first Fact whose Value contains claim as a
// case-insensitive substring.
func matchFact(claim string, facts []Fact) (Fact, bool) {
	lowerClaim := strings.ToLower(claim)
	for _, f := range facts {
		if strings.Contains(strings.ToLower(f.Value), lowerClaim) {
			return f, true
		}
	}
	return Fact{}, false
}

// buildPrompt renders the full generation prompt, appending the previous
// violation (if any) so a retry can self-correct, mirroring
// CompleteStructured's own retry-with-error-appended shape but at the
// domain-validation level rather than the JSON-parsing level.
func buildPrompt(tone Tone, contactName, companyName string, facts []Fact, lastViolation string) string {
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
		"just the message body.\n", maxDraftChars)

	if lastViolation != "" {
		fmt.Fprintf(&b, "\nYour previous attempt was rejected: %s. Fix this and answer again.\n", lastViolation)
	}

	return b.String()
}

// genericOpener is the deterministic, zero-signal-dependency fallback used
// whenever no company-intel signal exists, or grounded generation could not
// be produced within maxAttempts. It contains no specific claim about the
// team or company (FR-005, FR-012) — vagueness over invention.
func genericOpener(tone Tone, contactName, companyName string) string {
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
	default: // ToneWarm
		return fmt.Sprintf(
			"%s! I just applied for %s and wanted to say hello directly — I'd love to learn more about the team "+
				"and see if there's a fit. Would you be open to a quick chat?",
			greeting, role,
		)
	}
}

// enforceLength is the final safety net (FR-009): truncates at the nearest
// word boundary within the limit and drops any trace whose claim text was
// truncated away, so a trace is never presented for text that no longer
// contains it.
func enforceLength(text string, traces []GroundingTrace, max int) (string, []GroundingTrace) {
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
