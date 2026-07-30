package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/job-finder/api/internal/platform/llm"
)

// extractedContact is the raw LLM structured-output shape for posting-text
// extraction. Every field is a plain string ("" means "not found") so the
// model cannot express partial/nullable ambiguity that would complicate
// grounding below.
type extractedContact struct {
	Name        string `json:"name" jsonschema:"description=Full name of a real human contact person EXPLICITLY named in the text as owning this requisition. Empty string if no person is named."`
	Title       string `json:"title" jsonschema:"description=The named person's job title or role copied exactly as it appears in the text. Empty string if unknown or no person is named."`
	Email       string `json:"email" jsonschema:"description=A contact email address found in the text copied exactly as written. Empty string if none."`
	Phone       string `json:"phone" jsonschema:"description=A contact phone number found in the text copied exactly as written. Empty string if none."`
	LinkedInURL string `json:"linkedInUrl" jsonschema:"description=A LinkedIn profile URL found in the text copied exactly as written. Empty string if none."`
}

var (
	emailRe = regexp.MustCompile(`(?i)^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
	phoneRe = regexp.MustCompile(`[+\d][\d ()\-.]{6,}\d`)

	// explicitContactLineRe matches a posting that labels a person as the
	// point of contact ("Contact:", "Recruiter:", "Hiring Manager:", ...)
	// — the strongest on-page signal this source can see, so it earns the
	// top confidence tier.
	explicitContactLineRe = regexp.MustCompile(`(?i)\b(contact|recruiter|hiring manager|talent partner|talent acquisition)\s*[:\-]`)

	// genericMailboxLocal is the set of local-parts (before the @) that
	// name a role mailbox rather than a person (FR-007, SC-003). Matched
	// case-insensitively against the email's local-part only.
	genericMailboxLocal = map[string]bool{
		"jobs": true, "careers": true, "career": true, "hr": true,
		"recruiting": true, "recruitment": true, "apply": true,
		"info": true, "talent": true, "hello": true, "contact": true,
		"vacancies": true, "recruiter": true, "hiring": true,
	}
)

// ExtractPostingContact runs the local LLM over a job's description text
// and returns at most one grounded ResolvedContact, or (nil, nil) when no
// human contact could be grounded in the text (FR-016 — zero contacts is
// success, not an error). Every non-empty field the LLM returns is
// verified to actually occur in description before being trusted
// (Constitution Principle II) — a hallucinated field is silently dropped,
// never surfaced.
func ExtractPostingContact(ctx context.Context, llmc llm.Provider, model string, description string) (*ResolvedContact, error) {
	text := strings.TrimSpace(description)
	if text == "" {
		return nil, nil
	}

	truncated := text
	if len(truncated) > 4000 {
		truncated = truncated[:4000]
	}

	prompt := fmt.Sprintf(
		"Read this job posting and identify, if named, the specific human being who owns this requisition "+
			"(a recruiter, hiring manager, or similar) — someone a candidate could reach out to.\n\n"+
			"Only report a person, title, email, phone, or LinkedIn URL if it is EXPLICITLY present in the text "+
			"below. Never guess or invent a name. A generic mailbox like jobs@company.com or careers@company.com "+
			"with no named person is NOT a contact — leave name empty in that case.\n\n"+
			"POSTING TEXT:\n%s\n\n"+
			"Return a single JSON object with the fields below. Use \"\" for anything not explicitly present.",
		truncated,
	)

	out, err := llm.CompleteStructured[extractedContact](ctx, llmc, prompt, &llm.CompleteOptions{
		System: "You extract only what is explicitly written in the given text; you never fabricate names, titles, or contact details.",
		Model:  model,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiter: posting extraction: %w", err)
	}

	return groundContact(out, text, SourcePosting, postingConfidence)
}

// groundContact validates every field the LLM returned actually occurs
// (case-insensitively, Unicode-aware) in the source text, drops anything
// that doesn't, and applies the no-fabrication rules (FR-007, FR-008):
//   - a field not found verbatim in source is discarded;
//   - a contact without a grounded Name is not returned at all — a phone
//     or email with no named person is not a "contact" for this feature,
//     it may only ever be dropped, never stored under an invented name;
//   - a generic-mailbox email (jobs@, hr@, careers@, ...) never counts as
//     a personal channel, even alongside a grounded name.
//
// Shared by every extraction source (posting.go, companypage.go,
// linkedin.go) so grounding/no-fabrication logic lives in exactly one
// place; only the source label and confidence formula vary per source.
func groundContact(out extractedContact, source, sourceName string, confidenceFn func(source, email, phone string) float64) (*ResolvedContact, error) {
	lowerSource := strings.ToLower(source)

	ground := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		if !strings.Contains(lowerSource, strings.ToLower(v)) {
			return "" // not grounded in the source text — drop it
		}
		return v
	}

	name := ground(out.Name)
	if name == "" {
		// No grounded human name: per FR-007/FR-008 a channel with no name
		// is never surfaced as a contact, named or otherwise.
		return nil, nil
	}

	title := ground(out.Title)
	email := ground(out.Email)
	phone := ground(out.Phone)
	linkedIn := ground(out.LinkedInURL)

	if email != "" && !emailRe.MatchString(email) {
		email = ""
	}
	if email != "" && isGenericMailbox(email) {
		email = ""
	}
	if phone != "" && !phoneRe.MatchString(phone) {
		phone = ""
	}

	contact := &ResolvedContact{
		Name:       name,
		Source:     sourceName,
		Confidence: confidenceFn(source, email, phone),
	}
	if title != "" {
		contact.Title = &title
	}
	if email != "" {
		contact.Email = &email
	}
	if phone != "" {
		contact.Phone = &phone
	}
	if linkedIn != "" {
		contact.LinkedInURL = &linkedIn
	}
	return contact, nil
}

func isGenericMailbox(email string) bool {
	at := strings.Index(email, "@")
	if at <= 0 {
		return false
	}
	local := strings.ToLower(email[:at])
	return genericMailboxLocal[local]
}

// postingConfidence scores higher when the posting explicitly labels a
// contact line ("Contact:", "Recruiter:", ...) and when more channels were
// grounded — an explicit label is the strongest on-page signal this source
// can see (assumptions.md "Confidence is a producer-assigned float").
func postingConfidence(source, email, phone string) float64 {
	c := 0.55
	if explicitContactLineRe.MatchString(source) {
		c = 0.9
	}
	if email != "" {
		c += 0.05
	}
	if phone != "" {
		c += 0.03
	}
	if c > 0.99 {
		c = 0.99
	}
	return c
}
