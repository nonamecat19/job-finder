// Package adapters — Jobgether, direct scrape of public search-results and
// job pages (no login/session). Search only supports an operator-pasted
// search URL (query.SubscriptionURL) — no keyword search, mirroring
// glassdoor.go's stance exactly. Jobgether is treated as anonymously
// accessible (research.md R1): a plain unauthenticated HTTP request is
// expected to work, but a rate-limit/challenge response is treated as an
// expected, distinctly-reported outcome (FR-011) rather than an edge case —
// never a panic, and never retried aggressively or bypassed. Jobgether
// surfaces its own AI-generated match-percentage score on both list and
// detail pages; that score is descriptive metadata only and MUST stay in
// Raw["jobgetherMatchScore"], never a first-class field feeding this
// product's own matching/scoring (FR-012).
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
	"github.com/job-finder/api/internal/retrieval"
	"github.com/job-finder/api/internal/platform/scraping"
)

const (
	// jobgetherMaxSubscriptionPages caps pagination so a redirect loop or an
	// unbounded feed can't run forever, mirroring glassdoorMaxSubscriptionPages.
	jobgetherMaxSubscriptionPages = 50
	// jobgetherRequestDelay is the floor between paginated requests (FR-010).
	jobgetherRequestDelay = 500 * time.Millisecond
)

var (
	jobgetherRemoteRe    = regexp.MustCompile(`(?i)\bremote\b|work from home`)
	jobgetherPostedAgoRe = regexp.MustCompile(`(?i)(\d+)\s*(day|hour)s?\s*ago`)
)

// JobgetherAdapter — jobgether.com, public search-results pages, no
// credentials (FR-013).
type JobgetherAdapter struct {
	Scraping  *scraping.Service
	Retrieval retrieval.Service
}

func (JobgetherAdapter) Key() string          { return "jobgether" }
func (JobgetherAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

// NeedsDetail reports true: the results page carries a snippet only;
// FetchDetail fills in the full description.
func (JobgetherAdapter) NeedsDetail() bool { return true }

func (d JobgetherAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.fetchPage(ctx, "https://jobgether.com/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(strings.ToLower(html), "jobgether"), nil
}

func (d JobgetherAdapter) fetchPage(ctx context.Context, url string, headers map[string]string) (string, error) {
	if d.Retrieval != nil {
		result, err := d.Retrieval.Fetch(ctx, retrieval.FetchRequest{URL: url, Headers: headers})
		if err != nil {
			return "", err
		}
		if result.Outcome.Status == retrieval.PageChallenged {
			return "", fmt.Errorf("jobgether: challenged: %s", result.Outcome.Reason)
		}
		if result.Outcome.Status == retrieval.PageRefused {
			return "", fmt.Errorf("jobgether: refused: %s", result.Outcome.Reason)
		}
		if result.Outcome.Status == retrieval.PageDeferred {
			return "", fmt.Errorf("jobgether: deferred: %s", result.Outcome.Reason)
		}
		return result.Body, nil
	}
	html, err := d.Scraping.FetchHTML(ctx, url, headers)
	if err != nil {
		return "", err
	}
	if jobgetherIsBlockedPage(html) {
		return "", fmt.Errorf("jobgether: request blocked by rate-limit/challenge interstitial: %s", url)
	}
	return html, nil
}

// jobgetherIsBlockedPage reports whether an HTML response looks like
// Jobgether's rate-limit/interstitial page rather than real content
// (research.md R3): a "Rate Limit Exceeded" title/heading combined with body
// text indicating too many requests. Used as a fallback when Retrieval is
// nil (tests).
func jobgetherIsBlockedPage(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "rate limit exceeded") && strings.Contains(lower, "too many requests")
}

// Search only supports the pasted-subscription-URL flow (FR-014); keyword
// search is out of scope, matching GlassdoorAdapter.Search's stance exactly.
func (d JobgetherAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("jobgether keyword search not implemented — use subscription URL instead")
	}
	jobs, err := d.scrapeSubscription(ctx, query.SubscriptionURL)
	if len(jobs) == 0 && err == nil {
		slog.Warn("jobgether subscription returned 0 jobs — markup may have changed or search has no matches", "url", query.SubscriptionURL)
	}
	return jobs, err
}

// scrapeSubscription pages through a pasted Jobgether search URL by
// incrementing its "page" query parameter. Stops on an empty page, a hard
// page cap, or the page repeating the previous page's first card (loop
// guard). A later page failing (fetch error, blocked, or unparsable) ends
// pagination with whatever was collected; only page 1 failing is fatal
// (mirrors GlassdoorAdapter.scrapeSubscription). A blocked response on page 1
// is a distinct, reported failure (FR-011).
func (d JobgetherAdapter) scrapeSubscription(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("jobgether: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstID := ""
	for page := 1; page <= jobgetherMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("page", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		if page > 1 {
			time.Sleep(jobgetherRequestDelay)
		}

		htmlStr, err := d.fetchPage(ctx, pageURL.String(), nil)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("jobgether subscription fetch: %w", err)
			}
			slog.Warn("jobgether subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("parse jobgether subscription page: %w", err)
			}
			slog.Warn("jobgether subscription page parse failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		cards := parseJobgetherCards(doc, pageURL.String())
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

// parseJobgetherCards extracts job cards from a Jobgether search-results
// page. Title and URL are required (card skipped without them); every other
// field degrades to empty/nil rather than erroring. Jobgether's own
// match-percentage score, when present, is captured into
// Raw["jobgetherMatchScore"] only — never a first-class field (FR-012).
func parseJobgetherCards(doc *goquery.Document, pageURL string) []dto.NormalizedJob {
	var results []dto.NormalizedJob

	base, _ := url.Parse(pageURL)
	if base == nil {
		base, _ = url.Parse("https://jobgether.com")
	}

	doc.Find(`.job-card`).Each(func(i int, card *goquery.Selection) {
		titleLink := card.Find(`.job-card__title`).First()
		title := strings.TrimSpace(titleLink.Text())
		href, hasHref := titleLink.Attr("href")
		if title == "" || !hasHref || href == "" {
			return
		}

		absURL := href
		if full, err := url.Parse(href); err == nil {
			absURL = base.ResolveReference(full).String()
		}

		company := strings.TrimSpace(card.Find(`.job-card__company`).First().Text())
		location := strings.TrimSpace(card.Find(`.job-card__location`).First().Text())
		salary := strings.TrimSpace(card.Find(`.job-card__salary`).First().Text())
		posted := strings.TrimSpace(card.Find(`.job-card__posted`).First().Text())
		matchScore := strings.TrimSpace(card.Find(`.job-card__match-score`).First().Text())
		summary := strings.TrimSpace(jobsources.SelectionText(card.Find(`.job-card__summary`).First()))

		isRemote := jobgetherRemoteRe.MatchString(location) || jobgetherRemoteRe.MatchString(title)

		var externalID *string
		if id, ok := card.Attr("data-job-id"); ok && id != "" {
			externalID = jobsources.Ptr(id)
		}

		raw := map[string]any{}
		if matchScore != "" {
			raw["jobgetherMatchScore"] = matchScore
		}

		results = append(results, dto.NormalizedJob{
			SourceKey:   "jobgether",
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(location),
			Remote:      isRemote,
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         absURL,
			Description: summary,
			PostedAt:    jobgetherPostedAtFromText(posted),
			Raw:         raw,
		})
	})

	return results
}

// jobgetherPostedAtFromText resolves Jobgether's relative "posted N days/
// hours ago" text into an approximate RFC3339 timestamp, or nil when it
// doesn't match a recognizable shape.
func jobgetherPostedAtFromText(text string) *string {
	m := jobgetherPostedAgoRe.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	var d time.Duration
	switch strings.ToLower(m[2]) {
	case "hour":
		d = time.Duration(n) * time.Hour
	case "day":
		d = time.Duration(n) * 24 * time.Hour
	default:
		return nil
	}
	t := time.Now().Add(-d).Format(time.RFC3339)
	return &t
}

// JobgetherDetailPatch is the parsed subset of a Jobgether job-detail page
// used to fill in a shallow (list-only) Job row. Not part of the Adapter
// interface — Jobgether-specific, called directly by the enrichment handler.
type JobgetherDetailPatch struct {
	Description string
	SalaryRaw   *string
	PostedAt    *string
	Available   bool
	Raw         map[string]any
}

// FetchDetail fetches a single Jobgether job-detail page and parses the full
// description, salary, and posted-date. Returns Available: false with a nil
// error when the detail page has no recognizable description (the listing
// has rotated out / is gone — FR-009 edge case), so the caller leaves the
// job's existing summary data untouched rather than overwriting it with
// nothing. Returns a non-nil error only on fetch failure or a
// blocked/challenge response. Jobgether's match-percentage score, if present
// on the detail page, is captured into Raw["jobgetherMatchScore"] (FR-012).
func (d JobgetherAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (JobgetherDetailPatch, error) {
	htmlStr, err := d.fetchPage(ctx, jobURL, nil)
	if err != nil {
		return JobgetherDetailPatch{}, fmt.Errorf("jobgether detail fetch: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return JobgetherDetailPatch{}, fmt.Errorf("parse jobgether detail page: %w", err)
	}

	descSel := doc.Find(`.job-detail__description`)
	description := strings.TrimSpace(jobsources.SelectionText(descSel.First()))
	if description == "" {
		return JobgetherDetailPatch{Available: false}, nil
	}

	salary := strings.TrimSpace(doc.Find(`.job-detail__salary`).First().Text())
	posted := strings.TrimSpace(doc.Find(`.job-detail__posted`).First().Text())
	matchScore := strings.TrimSpace(doc.Find(`.job-detail__match-score`).First().Text())

	raw := map[string]any{}
	if matchScore != "" {
		raw["jobgetherMatchScore"] = matchScore
	}

	return JobgetherDetailPatch{
		Description: description,
		SalaryRaw:   jobsources.NilIfEmpty(salary),
		PostedAt:    jobgetherPostedAtFromText(posted),
		Available:   true,
		Raw:         raw,
	}, nil
}
