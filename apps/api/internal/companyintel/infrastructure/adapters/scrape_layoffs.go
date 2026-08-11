package adapters

import (
	"context"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/companyintel/domain"
	"github.com/nonamecat19/jobscraper/ports"
)

const (
	layoffsDomain = "layoffs.fyi"
	layoffsURL    = "https://layoffs.fyi/"
	noLayoffData  = "no layoff data found"
)

type LayoffsScraper struct {
	Scraping ports.Scraper
}

func (LayoffsScraper) Kind() string   { return domain.KindLayoffs }
func (LayoffsScraper) Domain() string { return layoffsDomain }

func (s LayoffsScraper) Scrape(ctx context.Context, in domain.Input) (*domain.SignalResult, error) {
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

func parseLayoffsRow(doc *goquery.Document, name, sourceURL string) *domain.SignalResult {
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
		return &domain.SignalResult{
			Kind:   domain.KindLayoffs,
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

	return &domain.SignalResult{
		Kind:   domain.KindLayoffs,
		Value:  value,
		Source: sourceURL,
		Raw:    rawHTML,
	}
}
