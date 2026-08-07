package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/job-finder/api/internal/platform/llm"

	"github.com/job-finder/api/internal/recruiter/domain"
)

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

	explicitContactLineRe = regexp.MustCompile(`(?i)\b(contact|recruiter|hiring manager|talent partner|talent acquisition)\s*[:\-]`)

	genericMailboxLocal = map[string]bool{
		"jobs": true, "careers": true, "career": true, "hr": true,
		"recruiting": true, "recruitment": true, "apply": true,
		"info": true, "talent": true, "hello": true, "contact": true,
		"vacancies": true, "recruiter": true, "hiring": true,
	}
)

func ExtractPostingContact(ctx context.Context, llmc llm.Provider, model string, description string) (*domain.ResolvedContact, error) {
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

	return groundContact(out, text, domain.SourcePosting, postingConfidence)
}

func groundContact(out extractedContact, source, sourceName string, confidenceFn func(source, email, phone string) float64) (*domain.ResolvedContact, error) {
	lowerSource := strings.ToLower(source)

	ground := func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		if !strings.Contains(lowerSource, strings.ToLower(v)) {
			return ""
		}
		return v
	}

	name := ground(out.Name)
	if name == "" {
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

	contact := &domain.ResolvedContact{
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
