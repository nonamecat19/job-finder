// Package adapters — Indeed, direct scrape of public search-results and job
// pages (no login/session). Search only supports an operator-pasted search
// URL (query.SubscriptionURL) — no keyword search, matching dou.go's stance.
// Selectors are defensive/best-effort: Indeed's markup is non-semantic and
// known to churn, and the site may intermittently block scraping (see
// specs/002-indeed-job-source/research.md R3/R4) — a missing field or a
// blocked request degrades to an empty/partial result, never a panic.
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
	// indeedMaxSubscriptionPages caps pagination so a redirect loop or an
	// unbounded feed can't run forever, mirroring douMaxSubscriptionPages.
	indeedMaxSubscriptionPages = 50
	// indeedRequestDelay is the floor between paginated requests (FR-010).
	indeedRequestDelay = 500 * time.Millisecond
	// indeedPageSize is Indeed's own per-page result count; the "start"
	// query param advances by this amount per page.
	indeedPageSize = 10
)

var indeedRemoteRe = regexp.MustCompile(`(?i)\bremote\b|work from home`)

// IndeedAdapter — indeed.com, public search-results pages, no credentials.
type IndeedAdapter struct {
	Scraping *scraping.Service
}

func (IndeedAdapter) Key() string          { return "indeed" }
func (IndeedAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

// NeedsDetail reports true: the results page carries a snippet only;
// FetchDetail fetches the posting body, so ingestion defers match/ghost
// scoring until enrichment has run.
func (IndeedAdapter) NeedsDetail() bool { return true }

func (d IndeedAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.Scraping.FetchHTML(ctx, "https://www.indeed.com/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(strings.ToLower(html), "indeed"), nil
}

// Search only supports the pasted-subscription-URL flow (FR-015); keyword
// search is out of scope, matching DouAdapter.Search's stance exactly.
func (d IndeedAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("indeed keyword search not implemented — use subscription URL instead")
	}
	jobs, err := d.scrapeSubscription(ctx, query.SubscriptionURL)
	if len(jobs) == 0 && err == nil {
		slog.Warn("indeed subscription returned 0 jobs — markup may have changed or search has no matches", "url", query.SubscriptionURL)
	}
	return jobs, err
}

// scrapeSubscription pages through a pasted Indeed search URL by
// incrementing its "start" query parameter (Indeed's own offset-based
// pagination — see research.md R3). Stops on an empty page, a hard page
// cap, or the page repeating the previous page's first card (loop guard).
// A later page failing ends pagination with whatever was collected; only
// page 1 failing is fatal (mirrors DjinniAdapter.scrapeSubscription).
func (d IndeedAdapter) scrapeSubscription(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("indeed: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstHref := ""
	for page := 0; page < indeedMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("start", strconv.Itoa(page*indeedPageSize))
		pageURL.RawQuery = q.Encode()

		if page > 0 {
			time.Sleep(indeedRequestDelay)
		}

		htmlStr, err := d.Scraping.FetchHTML(ctx, pageURL.String(), nil)
		if err != nil {
			if page == 0 {
				return nil, fmt.Errorf("indeed subscription fetch: %w", err)
			}
			slog.Warn("indeed subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
		if err != nil {
			if page == 0 {
				return nil, fmt.Errorf("parse indeed subscription page: %w", err)
			}
			slog.Warn("indeed subscription page parse failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		cards := parseIndeedCards(doc, pageURL.String())
		if len(cards) == 0 {
			break
		}
		if cards[0].URL == seenFirstHref {
			break
		}
		seenFirstHref = cards[0].URL

		jobs = append(jobs, cards...)
	}
	return jobs, nil
}

// parseIndeedCards extracts job cards from an Indeed search-results page.
// Selectors are defensive: title is required (card skipped without it),
// every other field degrades to empty/nil rather than erroring.
func parseIndeedCards(doc *goquery.Document, pageURL string) []dto.NormalizedJob {
	var results []dto.NormalizedJob

	base, _ := url.Parse(pageURL)
	if base == nil {
		base, _ = url.Parse("https://www.indeed.com")
	}

	cards := doc.Find(".job_seen_beacon")
	if cards.Length() == 0 {
		cards = doc.Find(`td.resultContent, [data-testid="slider_item"]`)
	}
	cards.Each(func(i int, card *goquery.Selection) {
		titleLink := card.Find(`h2.jobTitle a, a.jcs-JobTitle, a[data-jk]`).First()
		title := strings.TrimSpace(titleLink.Find("span").First().Text())
		if title == "" {
			title = strings.TrimSpace(titleLink.Text())
		}
		href, hasHref := titleLink.Attr("href")
		if title == "" || !hasHref || href == "" {
			return
		}

		absURL := href
		if full, err := url.Parse(href); err == nil {
			absURL = base.ResolveReference(full).String()
		}

		company := strings.TrimSpace(card.Find(`[data-testid="company-name"], .companyName`).First().Text())
		location := strings.TrimSpace(card.Find(`[data-testid="text-location"], .companyLocation`).First().Text())
		salary := strings.TrimSpace(card.Find(`[data-testid="attribute_snippet_testid"], .salary-snippet-container, .metadata.salary-snippet-container`).First().Text())
		description := jobsources.SelectionText(card.Find(`.job-snippet, [data-testid="job-snippet"]`).First())

		isRemote := indeedRemoteRe.MatchString(location) || indeedRemoteRe.MatchString(description)

		var externalID *string
		if jk := extractIndeedJK(href); jk != "" {
			externalID = jobsources.Ptr(jk)
		}

		results = append(results, dto.NormalizedJob{
			SourceKey:   "indeed",
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(location),
			Remote:      isRemote,
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         absURL,
			Description: description,
			Raw:         map[string]string{},
		})
	})

	return results
}

// extractIndeedJK pulls the "jk" query parameter (Indeed's stable per-
// listing job-key id) out of a card href, e.g. "/rc/clk?jk=abc123".
func extractIndeedJK(href string) string {
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("jk")
}

// IndeedDetailPatch is the parsed subset of an Indeed job-detail page used
// to fill in a shallow (list-only) Job row. Not part of the Adapter
// interface — Indeed-specific, called directly by the enrichment handler.
type IndeedDetailPatch struct {
	Description string
	SalaryRaw   *string
	Location    *string
	Remote      bool
	PostedAt    *string
	Raw         map[string]string
}

// FetchDetail fetches a single Indeed job page and parses the full
// description/location/remote flag/posted-date. Returns a non-nil error on
// fetch failure (e.g. the listing is gone) so the caller can leave the
// job's existing summary data untouched rather than overwriting it with
// nothing (FR-009 edge case).
func (d IndeedAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (IndeedDetailPatch, error) {
	htmlStr, err := d.Scraping.FetchHTML(ctx, jobURL, nil)
	if err != nil {
		return IndeedDetailPatch{}, fmt.Errorf("indeed detail fetch: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return IndeedDetailPatch{}, fmt.Errorf("parse indeed detail page: %w", err)
	}

	descSel := doc.Find(`#jobDescriptionText, .jobsearch-JobComponent-description`)
	if descSel.Length() == 0 {
		descSel = doc.Find("article, .jobsearch-JobComponent")
	}
	description := strings.TrimSpace(jobsources.SelectionText(descSel.First()))
	if description == "" {
		return IndeedDetailPatch{}, fmt.Errorf("indeed: detail page unavailable or unparseable: %s", jobURL)
	}

	location := strings.TrimSpace(doc.Find(`[data-testid="inlineHeader-companyLocation"], .jobsearch-JobInfoHeader-subtitle`).First().Text())

	isRemote := indeedRemoteRe.MatchString(description) || indeedRemoteRe.MatchString(location)

	var postedAt *string
	postedText := strings.TrimSpace(doc.Find(`[data-testid="myJobsStateDate"]`).First().Text())
	if postedText != "" {
		postedAt = &postedText
	}

	return IndeedDetailPatch{
		Description: description,
		Location:    jobsources.NilIfEmpty(location),
		Remote:      isRemote,
		PostedAt:    postedAt,
		Raw:         map[string]string{},
	}, nil
}
