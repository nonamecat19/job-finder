package adapters

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/companyintel/domain"
	"github.com/job-finder/api/internal/platform/scraping"
)

const crunchbaseDomain = "crunchbase.com"

var crunchbaseSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

type CrunchbaseScraper struct {
	Scraping scraping.Scraper
}

func (CrunchbaseScraper) Kind() string   { return domain.KindFunding }
func (CrunchbaseScraper) Domain() string { return crunchbaseDomain }

func (s CrunchbaseScraper) Scrape(ctx context.Context, in domain.Input) (*domain.SignalResult, error) {
	slug := crunchbaseSlug(in.CompanyName)
	if slug == "" {
		return nil, nil
	}
	url := fmt.Sprintf("https://www.crunchbase.com/organization/%s", slug)

	html, err := s.Scraping.FetchHTML(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("crunchbase: fetch %s: %w", url, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("crunchbase: parse %s: %w", url, err)
	}

	result, err := parseCrunchbaseFunding(doc, url)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func crunchbaseSlug(name string) string {
	slug := crunchbaseSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	return strings.Trim(slug, "-")
}

func parseCrunchbaseFunding(doc *goquery.Document, sourceURL string) (*domain.SignalResult, error) {
	row := doc.Find(".funding-round").First()
	if row.Length() == 0 {
		return nil, fmt.Errorf("crunchbase: no funding rounds found at %s", sourceURL)
	}

	roundName := strings.TrimSpace(row.Find(".round-name").First().Text())
	if roundName == "" {
		return nil, fmt.Errorf("crunchbase: funding round row missing a name at %s", sourceURL)
	}
	amount := strings.TrimSpace(row.Find(".round-amount").First().Text())
	date := strings.TrimSpace(row.Find(".round-date").First().Text())

	value := roundName
	if amount != "" {
		value += " — " + amount
	}
	if date != "" {
		value += " (" + date + ")"
	}

	rawHTML, _ := row.Html()

	return &domain.SignalResult{
		Kind:   domain.KindFunding,
		Value:  value,
		Source: sourceURL,
		Raw:    rawHTML,
	}, nil
}
