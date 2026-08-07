package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/platform/llm"

	"github.com/job-finder/api/internal/recruiter/domain"
)

const linkedInPeoplePath = "/people/"

func (s *Service) linkedInSource(job sqlcgen.Job) resolutionSource {
	return resolutionSource{
		name: domain.SourceLinkedIn,
		run: func(ctx context.Context) ([]domain.ResolvedContact, error) {
			if s.scraping == nil {
				return nil, nil
			}
			slug := companySlug(job.Company)
			if slug == "" {
				return nil, nil
			}
			pageURL := "https://www.linkedin.com/company/" + slug + linkedInPeoplePath

			html, err := s.scraping.FetchHTML(ctx, pageURL, nil)
			if err != nil {
				return nil, fmt.Errorf("recruiter: linkedin fetch %s: %w", pageURL, err)
			}
			text := extractPageText(html)
			if text == "" {
				return nil, nil
			}

			return ExtractLinkedInContacts(ctx, s.llmc, s.postingModel, text)
		},
	}
}

func companySlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	name = whitespaceRe.ReplaceAllString(name, "-")
	return name
}

func ExtractLinkedInContacts(ctx context.Context, llmc llm.Provider, model string, pageText string) ([]domain.ResolvedContact, error) {
	text := strings.TrimSpace(pageText)
	if text == "" {
		return nil, nil
	}

	truncated := text
	if len(truncated) > 4000 {
		truncated = truncated[:4000]
	}

	prompt := fmt.Sprintf(
		"Read this LinkedIn company People-section page and list every named human team member who could "+
			"plausibly own hiring for a role — a recruiter, talent acquisition specialist, HR/People team "+
			"member, or hiring manager.\n\n"+
			"Only report a person, title, email, phone, or LinkedIn URL if it is EXPLICITLY present in the text "+
			"below. Never guess or invent a name.\n\n"+
			"PAGE TEXT:\n%s\n\n"+
			"Return a single JSON object with a \"contacts\" array (empty if none).",
		truncated,
	)

	out, err := llm.CompleteStructured[extractedContactList](ctx, llmc, prompt, &llm.CompleteOptions{
		System: "You extract only what is explicitly written in the given text; you never fabricate names, titles, or contact details.",
		Model:  model,
	})
	if err != nil {
		return nil, fmt.Errorf("recruiter: linkedin extraction: %w", err)
	}

	var results []domain.ResolvedContact
	for _, raw := range out.Contacts {
		contact, err := groundContact(raw, text, domain.SourceLinkedIn, linkedInConfidence)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			results = append(results, *contact)
		}
	}
	return results, nil
}

func linkedInConfidence(_, email, phone string) float64 {
	c := 0.45
	if email != "" {
		c += 0.05
	}
	if phone != "" {
		c += 0.03
	}
	if c > 0.6 {
		c = 0.6
	}
	return c
}
