package companyintel

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/scraping"
)

const (
	layoffsDomain = "layoffs.fyi"
	layoffsURL    = "https://layoffs.fyi/"
	noLayoffData  = "no layoff data found"
)

// LayoffsScraper reads the public layoffs.fyi tracker table and looks up
// the most recent row matching the target company. Per spec.md edge cases,
// a page-layout change or a company that simply has no entry both resolve
// to the "no layoff data found" value — a successful, zero-result probe,
// never an error (mirrors the workua zero-results convention).
type LayoffsScraper struct {
	Scraping *scraping.Service
}

func (LayoffsScraper) Kind() string   { return KindLayoffs }
func (LayoffsScraper) Domain() string { return layoffsDomain }

func (s LayoffsScraper) Scrape(ctx context.Context, in Input) (*SignalResult, error) {
	if strings.TrimSpace(in.CompanyName) == "" {
		return nil, nil
	}

	html, err := s.Scraping.FetchHTML(ctx, layoffsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("layoffs.fyi: fetch %s: %w", layoffsURL, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("layoffs.fyi: parse %s: %w", layoffsURL, err)
	}

	return parseLayoffsRow(doc, in.CompanyName, layoffsURL), nil
}

// parseLayoffsRow scans `.layoff-row` rows for a `.company` cell matching
// name case-insensitively, returning the most recently-listed match (rows
// are assumed most-recent-first, matching the live site's default sort).
// Never returns an error — a missing/unmatched row is a successful
// zero-result probe.
func parseLayoffsRow(doc *goquery.Document, name, sourceURL string) *SignalResult {
	target := strings.ToLower(strings.TrimSpace(name))

	var matched *goquery.Selection
	doc.Find(".layoff-row").EachWithBreak(func(_ int, row *goquery.Selection) bool {
		company := strings.ToLower(strings.TrimSpace(row.Find(".company").First().Text()))
		if company == target {
			sel := row
			matched = sel
			return false
		}
		return true
	})

	if matched == nil {
		return &SignalResult{
			Kind:   KindLayoffs,
			Value:  noLayoffData,
			Source: sourceURL,
		}
	}

	date := strings.TrimSpace(matched.Find(".date").First().Text())
	laidOff := strings.TrimSpace(matched.Find(".laid-off").First().Text())
	percentage := strings.TrimSpace(matched.Find(".percentage").First().Text())

	var parts []string
	if laidOff != "" {
		parts = append(parts, laidOff+" laid off")
	}
	if percentage != "" {
		parts = append(parts, "("+percentage+")")
	}
	if date != "" {
		parts = append(parts, "on "+date)
	}

	value := strings.Join(parts, " ")
	if value == "" {
		value = noLayoffData
	}

	rawHTML, _ := matched.Html()

	return &SignalResult{
		Kind:   KindLayoffs,
		Value:  value,
		Source: sourceURL,
		Raw:    rawHTML,
	}
}
