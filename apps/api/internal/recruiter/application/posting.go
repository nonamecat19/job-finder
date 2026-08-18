package application

import (
	"context"
	"regexp"
	"strings"

	"github.com/job-finder/api/internal/recruiter/domain"
)

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

func (s *Service) extractPostingContact(ctx context.Context, description string) (*domain.ResolvedContact, error) {
	text := strings.TrimSpace(description)
	if text == "" {
		return nil, nil
	}

	contacts, err := s.extractor.Extract(ctx, SourcePosting, text)
	if err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}

	return groundContact(contacts[0], text, domain.SourcePosting, postingConfidence)
}

func groundContact(out ExtractedContact, source, sourceName string, confidenceFn func(source, email, phone string) float64) (*domain.ResolvedContact, error) {
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
