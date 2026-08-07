package adapters

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/companyintel/domain"
	"github.com/job-finder/api/internal/platform/scraping"
)

const glassdoorDomain = "glassdoor.com"

type GlassdoorScraper struct {
	Scraping scraping.Scraper
}

func (GlassdoorScraper) Kind() string   { return domain.KindGlassdoorRating }
func (GlassdoorScraper) Domain() string { return glassdoorDomain }

func (s GlassdoorScraper) Scrape(ctx context.Context, in domain.Input) (*domain.SignalResult, error) {
	slug := crunchbaseSlug(in.CompanyName)
	if slug == "" {
		return nil, nil
	}
	url := fmt.Sprintf("https://www.glassdoor.com/Reviews/%s-reviews-SRCH_KE0,%d.htm", slug, len(slug))

	html, err := s.Scraping.FetchHTML(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("glassdoor: fetch %s: %w", url, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("glassdoor: parse %s: %w", url, err)
	}

	return parseGlassdoorRating(doc, url)
}

func parseGlassdoorRating(doc *goquery.Document, sourceURL string) (*domain.SignalResult, error) {
	text := strings.TrimSpace(doc.Find(".rating-summary .rating-value").First().Text())
	if text == "" {
		return nil, fmt.Errorf("glassdoor: no rating found at %s", sourceURL)
	}
	rating, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("glassdoor: unparsable rating %q at %s: %w", text, sourceURL, err)
	}

	return &domain.SignalResult{
		Kind:   domain.KindGlassdoorRating,
		Value:  rating,
		Source: sourceURL,
		Raw:    text,
	}, nil
}
