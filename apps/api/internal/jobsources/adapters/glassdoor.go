// Package adapters — Glassdoor, direct scrape of public search-results and
// job pages (no login/session). Search only supports an operator-pasted
// search URL (query.SubscriptionURL) — no keyword search, matching
// indeed.go's stance. Selectors target Glassdoor's `data-test` attributes,
// which are more stable than Indeed's hashed CSS classes (confirmed live —
// see specs/004-glassdoor-job-provider/research.md R3). Glassdoor is
// empirically harder to reach than Indeed: a plain unauthenticated HTTP
// request is blocked 100% of the time in live testing during this feature's
// implementation (research.md R3), so a blocked/challenge response is
// treated as an expected, distinctly-reported outcome (FR-011, FR-018)
// rather than an edge case — never a panic, and never retried aggressively
// or bypassed.
package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/scraping"
)

const (
	// glassdoorMaxSubscriptionPages caps pagination so a redirect loop or an
	// unbounded feed can't run forever, mirroring indeedMaxSubscriptionPages.
	glassdoorMaxSubscriptionPages = 50
	// glassdoorRequestDelay is the floor between paginated requests (FR-010).
	glassdoorRequestDelay = 500 * time.Millisecond
)

var glassdoorRemoteRe = regexp.MustCompile(`(?i)\bremote\b|work from home`)

// GlassdoorAdapter — glassdoor.com, public search-results pages, no
// credentials (FR-013).
type GlassdoorAdapter struct {
	Scraping *scraping.Service
}

func (GlassdoorAdapter) Key() string          { return "glassdoor" }
func (GlassdoorAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

// NeedsDetail reports true: the results page carries a snippet only;
// FetchDetail fetches the posting body, so ingestion defers match/ghost
// scoring until enrichment has run.
func (GlassdoorAdapter) NeedsDetail() bool { return true }

func (d GlassdoorAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.Scraping.FetchHTML(ctx, "https://www.glassdoor.com/", nil)
	if err != nil {
		return false, nil
	}
	if glassdoorIsBlockedPage(html) {
		return false, nil
	}
	return strings.Contains(strings.ToLower(html), "glassdoor"), nil
}

// glassdoorIsBlockedPage reports whether an HTML response looks like
// Glassdoor's bot-challenge/security-interstitial page rather than real
// content, per the response actually captured during research.md R3's live
// check: a "Security | Glassdoor" title combined with a noindex/nofollow
// robots directive.
func glassdoorIsBlockedPage(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "security | glassdoor") &&
		strings.Contains(lower, "noindex") && strings.Contains(lower, "nofollow")
}

// Search only supports the pasted-subscription-URL flow (FR-014); keyword
// search is out of scope, matching IndeedAdapter.Search's stance exactly.
func (d GlassdoorAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("glassdoor keyword search not implemented — use subscription URL instead")
	}
	jobs, err := d.scrapeSubscription(ctx, query.SubscriptionURL)
	if len(jobs) == 0 && err == nil {
		slog.Warn("glassdoor subscription returned 0 jobs — markup may have changed or search has no matches", "url", query.SubscriptionURL)
	}
	return jobs, err
}

// scrapeSubscription pages through a pasted Glassdoor search URL by
// incrementing its "p" query parameter (Glassdoor's own page-number
// pagination — research.md R3). Stops on an empty page, a hard page cap, or
// the page repeating the previous page's first card (loop guard). A later
// page failing ends pagination with whatever was collected; only page 1
// failing is fatal (mirrors IndeedAdapter.scrapeSubscription). A blocked
// response on page 1 is a distinct, reported failure (FR-011, FR-018).
func (d GlassdoorAdapter) scrapeSubscription(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("glassdoor: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstID := ""
	for page := 1; page <= glassdoorMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("p", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		if page > 1 {
			time.Sleep(glassdoorRequestDelay)
		}

		htmlStr, err := d.Scraping.FetchHTML(ctx, pageURL.String(), nil)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("glassdoor subscription fetch: %w", err)
			}
			slog.Warn("glassdoor subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		if glassdoorIsBlockedPage(htmlStr) {
			if page == 1 {
				return nil, fmt.Errorf("glassdoor: request blocked by bot-challenge/security interstitial: %s", pageURL.String())
			}
			slog.Warn("glassdoor subscription page blocked mid-pagination, stopping pagination", "url", pageURL.String())
			break
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("parse glassdoor subscription page: %w", err)
			}
			slog.Warn("glassdoor subscription page parse failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		cards := parseGlassdoorCards(doc, pageURL.String())
		if len(cards) == 0 {
			break
		}
		firstID := ""
		if cards[0].ExternalID != nil {
			firstID = *cards[0].ExternalID
		}
		if firstID != "" && firstID == seenFirstID {
			break
		}
		seenFirstID = firstID

		jobs = append(jobs, cards...)
	}
	return jobs, nil
}

// parseGlassdoorCards extracts job cards from a Glassdoor search-results
// page using its `data-test` attributes (research.md R3). Title and URL are
// required (card skipped without them); every other field degrades to
// empty/nil rather than erroring.
func parseGlassdoorCards(doc *goquery.Document, pageURL string) []dto.NormalizedJob {
	var results []dto.NormalizedJob

	base, _ := url.Parse(pageURL)
	if base == nil {
		base, _ = url.Parse("https://www.glassdoor.com")
	}

	doc.Find(`[data-test="jobListing"]`).Each(func(i int, card *goquery.Selection) {
		titleLink := card.Find(`[data-test="job-title"]`).First()
		title := strings.TrimSpace(titleLink.Text())
		href, hasHref := titleLink.Attr("href")
		if title == "" || !hasHref || href == "" {
			return
		}

		absURL := href
		if full, err := url.Parse(href); err == nil {
			absURL = base.ResolveReference(full).String()
		}

		company := strings.TrimSpace(card.Find(`[class*="EmployerName"]`).First().Text())
		location := strings.TrimSpace(card.Find(`[data-test="emp-location"]`).First().Text())
		salary := strings.TrimSpace(card.Find(`[data-test="detailSalary"]`).First().Text())
		age := strings.TrimSpace(card.Find(`[data-test="job-age"]`).First().Text())
		rating := strings.TrimSpace(card.Find(`[class*="RatingText"]`).First().Text())

		isRemote := glassdoorRemoteRe.MatchString(location) || glassdoorRemoteRe.MatchString(title)

		var externalID *string
		if id, ok := card.Attr("data-jobid"); ok && id != "" {
			externalID = jobsources.Ptr(id)
		}

		raw := map[string]any{"salaryEstimated": glassdoorSalaryIsEstimate(salary)}
		if rating != "" {
			raw["employerRating"] = rating
		}

		results = append(results, dto.NormalizedJob{
			SourceKey:   "glassdoor",
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(location),
			Remote:      isRemote,
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         absURL,
			Description: "",
			PostedAt:    glassdoorPostedAtFromAge(age),
			Raw:         raw,
		})
	})

	return results
}

// glassdoorSalaryIsEstimate reports whether Glassdoor labeled a salary
// string as its own estimate ("Glassdoor est.") rather than
// employer-provided (research.md R4).
func glassdoorSalaryIsEstimate(salary string) bool {
	lower := strings.ToLower(salary)
	return strings.Contains(lower, "glassdoor est") || strings.Contains(lower, "estimate")
}

// glassdoorPostedAtFromAge resolves Glassdoor's relative "job-age" text
// (e.g. "3d", "24h") into an approximate RFC3339 timestamp, or nil when it
// doesn't match a recognizable shape.
func glassdoorPostedAtFromAge(age string) *string {
	age = strings.ToLower(strings.TrimSpace(age))
	if age == "" {
		return nil
	}
	var n int
	var unit byte
	if _, err := fmt.Sscanf(age, "%d%c", &n, &unit); err != nil {
		return nil
	}
	var d time.Duration
	switch unit {
	case 'h':
		d = time.Duration(n) * time.Hour
	case 'd':
		d = time.Duration(n) * 24 * time.Hour
	default:
		return nil
	}
	t := time.Now().Add(-d).Format(time.RFC3339)
	return &t
}

// GlassdoorDetailPatch is the parsed subset of a Glassdoor job-detail page
// used to fill in a shallow (list-only) Job row. Not part of the Adapter
// interface — Glassdoor-specific, called directly by the enrichment
// handler.
type GlassdoorDetailPatch struct {
	Description string
	SalaryRaw   *string
	PostedAt    *string
	Available   bool
	Raw         map[string]any
}

// FetchDetail fetches a single Glassdoor job-detail page and parses the full
// description, salary estimate, and posted-date. Returns Available: false
// with a nil error when the detail page has no recognizable description
// (the listing has rotated out / is gone — FR-009 edge case), so the caller
// leaves the job's existing summary data untouched rather than overwriting
// it with nothing. Returns a non-nil error only on fetch failure or a
// blocked/challenge response.
func (d GlassdoorAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (GlassdoorDetailPatch, error) {
	htmlStr, err := d.Scraping.FetchHTML(ctx, jobURL, nil)
	if err != nil {
		return GlassdoorDetailPatch{}, fmt.Errorf("glassdoor detail fetch: %w", err)
	}
	if glassdoorIsBlockedPage(htmlStr) {
		return GlassdoorDetailPatch{}, fmt.Errorf("glassdoor: detail request blocked by bot-challenge/security interstitial: %s", jobURL)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return GlassdoorDetailPatch{}, fmt.Errorf("parse glassdoor detail page: %w", err)
	}

	descSel := doc.Find(`[class*="jobDescription"]`)
	description := strings.TrimSpace(jobsources.SelectionText(descSel.First()))
	if description == "" {
		return GlassdoorDetailPatch{Available: false}, nil
	}

	salary := strings.TrimSpace(doc.Find(`[data-test="detailSalary"]`).First().Text())
	age := strings.TrimSpace(doc.Find(`[data-test="job-age"]`).First().Text())

	return GlassdoorDetailPatch{
		Description: description,
		SalaryRaw:   jobsources.NilIfEmpty(salary),
		PostedAt:    glassdoorPostedAtFromAge(age),
		Available:   true,
		Raw:         map[string]any{"salaryEstimated": glassdoorSalaryIsEstimate(salary)},
	}, nil
}
