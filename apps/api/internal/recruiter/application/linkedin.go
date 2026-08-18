package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/db/sqlcgen"

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

			return s.extractLinkedInContacts(ctx, text)
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

func (s *Service) extractLinkedInContacts(ctx context.Context, pageText string) ([]domain.ResolvedContact, error) {
	text := strings.TrimSpace(pageText)
	if text == "" {
		return nil, nil
	}

	contacts, err := s.extractor.Extract(ctx, SourceLinkedIn, text)
	if err != nil {
		return nil, fmt.Errorf("recruiter: linkedin extraction: %w", err)
	}

	var results []domain.ResolvedContact
	for _, raw := range contacts {
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
