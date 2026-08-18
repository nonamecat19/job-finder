package application

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/db/sqlcgen"

	"github.com/job-finder/api/internal/recruiter/domain"
)

var companyPagePaths = []string{"/about", "/team", "/about-us", "/company"}

func (s *Service) companyPageSource(job sqlcgen.Job) resolutionSource {
	return resolutionSource{
		name: domain.SourceCompanyPage,
		run: func(ctx context.Context) ([]domain.ResolvedContact, error) {
			if s.scraping == nil {
				return nil, nil
			}
			website, err := s.companyWebsite(ctx, job)
			if err != nil {
				return nil, err
			}
			if website == "" {
				return nil, nil
			}

			for _, path := range companyPagePaths {
				pageURL, err := joinURL(website, path)
				if err != nil {
					continue
				}
				html, err := s.scraping.FetchHTML(ctx, pageURL, nil)
				if err != nil {
					continue
				}
				text := extractPageText(html)
				if text == "" {
					continue
				}
				contacts, err := s.extractCompanyPageContacts(ctx, text)
				if err != nil {
					return nil, err
				}
				if len(contacts) > 0 {
					return contacts, nil
				}
			}
			return nil, nil
		},
	}
}

func (s *Service) companyWebsite(ctx context.Context, job sqlcgen.Job) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(job.Company))
	if normalized == "" {
		return "", nil
	}
	company, err := s.q.GetCompanyByNormalizedName(ctx, normalized)
	if err != nil {
		return "", nil //nolint:nilerr // "company never probed" is not a source failure
	}
	if company.Website == nil {
		return "", nil
	}
	return *company.Website, nil
}

func joinURL(website, path string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(website))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("companypage: invalid website %q", website)
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

var whitespaceRe = regexp.MustCompile(`\s+`)

func extractPageText(html string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(doc.Text(), " "))
}

func (s *Service) extractCompanyPageContacts(ctx context.Context, pageText string) ([]domain.ResolvedContact, error) {
	text := strings.TrimSpace(pageText)
	if text == "" {
		return nil, nil
	}

	contacts, err := s.extractor.Extract(ctx, SourceCompanyPage, text)
	if err != nil {
		return nil, fmt.Errorf("recruiter: company-page extraction: %w", err)
	}

	var results []domain.ResolvedContact
	for _, raw := range contacts {
		contact, err := groundContact(raw, text, domain.SourceCompanyPage, companyPageConfidence)
		if err != nil {
			return nil, err
		}
		if contact != nil {
			results = append(results, *contact)
		}
	}
	return results, nil
}

func companyPageConfidence(_, email, phone string) float64 {
	c := 0.35
	if email != "" {
		c += 0.05
	}
	if phone != "" {
		c += 0.03
	}
	if c > 0.5 {
		c = 0.5
	}
	return c
}
