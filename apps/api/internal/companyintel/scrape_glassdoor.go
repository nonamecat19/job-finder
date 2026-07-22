package companyintel

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/scraping"
)

const glassdoorDomain = "glassdoor.com"

// GlassdoorScraper reads the public Glassdoor company review summary page
// for its overall rating. Best-effort, fail-closed (spec.md "Glassdoor is
// best-effort"): any fetch/parse/challenge failure returns an error, which
// the caller logs at Warn and simply omits the signal — never a card error.
type GlassdoorScraper struct {
	Scraping *scraping.Service
}

func (GlassdoorScraper) Kind() string   { return KindGlassdoorRating }
func (GlassdoorScraper) Domain() string { return glassdoorDomain }

func (s GlassdoorScraper) Scrape(ctx context.Context, in Input) (*SignalResult, error) {
	slug := crunchbaseSlug(in.CompanyName) // same "lowercase, hyphenate" shape Glassdoor URLs use
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

// parseGlassdoorRating extracts the overall rating from
// `.rating-summary .rating-value`. A missing/unparsable rating (blocked
// page, login wall, layout change) is a hard failure — the signal is
// omitted from the card, matching the "fails closed" contract.
func parseGlassdoorRating(doc *goquery.Document, sourceURL string) (*SignalResult, error) {
	text := strings.TrimSpace(doc.Find(".rating-summary .rating-value").First().Text())
	if text == "" {
		return nil, fmt.Errorf("glassdoor: no rating found at %s", sourceURL)
	}
	rating, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("glassdoor: unparsable rating %q at %s: %w", text, sourceURL, err)
	}

	return &SignalResult{
		Kind:   KindGlassdoorRating,
		Value:  rating,
		Source: sourceURL,
		Raw:    text,
	}, nil
}
