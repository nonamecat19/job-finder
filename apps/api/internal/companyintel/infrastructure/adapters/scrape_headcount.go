package adapters

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/companyintel/domain"
	"github.com/job-finder/api/internal/platform/scraping"
)

var headcountRe = regexp.MustCompile(`(?i)([\d][\d,]*)\+?\s*employees`)

// HeadcountScraper reads the company's own About/Team page and looks for an
// employee-count figure. Per spec.md "Headcount trend requires patience":
// the first probe records a baseline; a second probe (a later manual
// refresh) computes a delta against domain.Input.PreviousHeadcount.
type HeadcountScraper struct {
	Scraping scraping.Scraper
}

func (HeadcountScraper) Kind() string { return domain.KindHeadcount }

// Domain is intentionally the generic label "company-site" — the actual
// host varies per company, so it cannot be paced/logged as one shared
// domain the way the other (fixed-host) scrapers can.
func (HeadcountScraper) Domain() string { return "company-site" }

func (s HeadcountScraper) Scrape(ctx context.Context, in domain.Input) (*domain.SignalResult, error) {
	if strings.TrimSpace(in.Website) == "" {
		// No known website yet — silently skipped, per spec.md edge case
		// "A company has no website in the job posting".
		return nil, nil
	}

	aboutURL, err := aboutPageURL(in.Website)
	if err != nil {
		return nil, fmt.Errorf("headcount: invalid website %q: %w", in.Website, err)
	}

	html, err := s.Scraping.FetchHTML(ctx, aboutURL, nil)
	if err != nil {
		return nil, fmt.Errorf("headcount: fetch %s: %w", aboutURL, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("headcount: parse %s: %w", aboutURL, err)
	}

	result, err := parseHeadcount(doc, in.PreviousHeadcount, aboutURL)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func aboutPageURL(website string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(website))
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("not an absolute URL")
	}
	u.Path = "/about"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// parseHeadcount finds the first "N employees" figure on the page and
// renders either a baseline message (no previous snapshot) or a
// current-vs-previous trend line. A page with no recognizable figure is a
// hard failure — headcount extraction has no standard layout (research.md
// risk), so a miss is logged and the signal simply stays unset.
func parseHeadcount(doc *goquery.Document, previous *int, sourceURL string) (*domain.SignalResult, error) {
	match := headcountRe.FindStringSubmatch(doc.Text())
	if match == nil {
		return nil, fmt.Errorf("headcount: no employee count found at %s", sourceURL)
	}
	current, err := strconv.Atoi(strings.ReplaceAll(match[1], ",", ""))
	if err != nil {
		return nil, fmt.Errorf("headcount: unparsable count %q at %s: %w", match[1], sourceURL, err)
	}

	var value string
	if previous == nil {
		value = fmt.Sprintf("%d employees (baseline captured)", current)
	} else {
		delta := current - *previous
		trend := "flat"
		switch {
		case delta > 0:
			trend = fmt.Sprintf("up %d", delta)
		case delta < 0:
			trend = fmt.Sprintf("down %d", -delta)
		}
		value = fmt.Sprintf("%d employees (was %d, %s)", current, *previous, trend)
	}

	return &domain.SignalResult{
		Kind:   domain.KindHeadcount,
		Value:  value,
		Source: sourceURL,
		Raw:    match[0],
	}, nil
}
