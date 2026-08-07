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
	"github.com/job-finder/api/internal/platform/scraping"
	"github.com/job-finder/api/internal/retrieval"
)

const (
	wellfoundMaxSubscriptionPages = 50
	wellfoundRequestDelay         = 500 * time.Millisecond
)

var wellfoundRemoteRe = regexp.MustCompile(`(?i)\bremote\b|work from home`)

var wellfoundAgeRe = regexp.MustCompile(`(?i)^(\d+)\s*([hd])\b`)

type WellfoundAdapter struct {
	Scraping  *scraping.Service
	Retrieval retrieval.Service
}

func (WellfoundAdapter) Key() string          { return "wellfound" }
func (WellfoundAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (WellfoundAdapter) NeedsDetail() bool { return true }

func (d WellfoundAdapter) HealthCheck(ctx context.Context, _ map[string]any) (bool, error) {
	html, err := d.fetchPage(ctx, "https://wellfound.com/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(strings.ToLower(html), "wellfound"), nil
}

func (d WellfoundAdapter) fetchPage(ctx context.Context, url string, headers map[string]string) (string, error) {
	if d.Retrieval != nil {
		result, err := d.Retrieval.Fetch(ctx, retrieval.FetchRequest{URL: url, Headers: headers})
		if err != nil {
			return "", err
		}
		if result.Outcome.Status == retrieval.PageChallenged {
			return "", fmt.Errorf("wellfound: challenged: %s", result.Outcome.Reason)
		}
		if result.Outcome.Status == retrieval.PageRefused {
			return "", fmt.Errorf("wellfound: refused: %s", result.Outcome.Reason)
		}
		if result.Outcome.Status == retrieval.PageDeferred {
			return "", fmt.Errorf("wellfound: deferred: %s", result.Outcome.Reason)
		}
		return result.Body, nil
	}
	html, err := d.Scraping.FetchHTML(ctx, url, headers)
	if err != nil {
		return "", err
	}
	if retrieval.IsChallenged(html, 0) {
		return "", fmt.Errorf("wellfound: request blocked by bot-challenge/rate-limit interstitial: %s", url)
	}
	return html, nil
}

func (d WellfoundAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("wellfound keyword search not implemented — use subscription URL instead")
	}
	jobs, err := d.scrapeSubscription(ctx, query.SubscriptionURL)
	if len(jobs) == 0 && err == nil {
		slog.Warn("wellfound subscription returned 0 jobs — markup may have changed or search has no matches", "url", query.SubscriptionURL)
	}
	return jobs, err
}

func (d WellfoundAdapter) scrapeSubscription(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("wellfound: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstID := ""
	for page := 1; page <= wellfoundMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("page", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		if page > 1 {
			time.Sleep(wellfoundRequestDelay)
		}

		htmlStr, err := d.fetchPage(ctx, pageURL.String(), nil)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("wellfound subscription fetch: %w", err)
			}
			slog.Warn("wellfound subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("parse wellfound subscription page: %w", err)
			}
			slog.Warn("wellfound subscription page parse failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		cards := parseWellfoundCards(doc, pageURL.String())
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

func parseWellfoundCards(doc *goquery.Document, pageURL string) []dto.NormalizedJob {
	var results []dto.NormalizedJob

	base, _ := url.Parse(pageURL)
	if base == nil {
		base, _ = url.Parse("https://wellfound.com")
	}

	doc.Find(`[data-test="JobSearchResult"]`).Each(func(i int, card *goquery.Selection) {
		titleLink := card.Find(`[data-test="JobSearchResult-title"]`).First()
		title := strings.TrimSpace(titleLink.Text())
		href, hasHref := titleLink.Attr("href")
		if title == "" || !hasHref || href == "" {
			return
		}

		absURL := href
		if full, err := url.Parse(href); err == nil {
			absURL = base.ResolveReference(full).String()
		}

		company := strings.TrimSpace(card.Find(`[data-test="JobSearchResult-company"]`).First().Text())
		location := strings.TrimSpace(card.Find(`[data-test="JobSearchResult-location"]`).First().Text())
		comp := strings.TrimSpace(card.Find(`[data-test="JobSearchResult-compensation"]`).First().Text())
		description := strings.TrimSpace(card.Find(`[data-test="JobSearchResult-description"]`).First().Text())
		age := strings.TrimSpace(card.Find(`[data-test="JobSearchResult-postedAt"]`).First().Text())

		isRemote := wellfoundRemoteRe.MatchString(location) || wellfoundRemoteRe.MatchString(title)

		var externalID *string
		if id, ok := card.Attr("data-job-id"); ok && id != "" {
			externalID = jobsources.Ptr(id)
		}

		results = append(results, dto.NormalizedJob{
			SourceKey:   "wellfound",
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(location),
			Remote:      isRemote,
			SalaryRaw:   jobsources.NilIfEmpty(comp),
			URL:         absURL,
			Description: description,
			PostedAt:    wellfoundPostedAtFromAge(age),
			Raw:         map[string]any{"hasEquity": wellfoundTextHasEquity(comp)},
		})
	})

	return results
}

func wellfoundTextHasEquity(comp string) bool {
	return strings.Contains(comp, "%")
}

func wellfoundPostedAtFromAge(age string) *string {
	m := wellfoundAgeRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(age)))
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	var d time.Duration
	switch m[2] {
	case "h":
		d = time.Duration(n) * time.Hour
	case "d":
		d = time.Duration(n) * 24 * time.Hour
	default:
		return nil
	}
	t := time.Now().Add(-d).Format(time.RFC3339)
	return &t
}

type WellfoundDetailPatch struct {
	Description string
	SalaryRaw   *string
	PostedAt    *string
	Available   bool
	Raw         map[string]any
}

func (d WellfoundAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (WellfoundDetailPatch, error) {
	htmlStr, err := d.fetchPage(ctx, jobURL, nil)
	if err != nil {
		return WellfoundDetailPatch{}, fmt.Errorf("wellfound detail fetch: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlStr))
	if err != nil {
		return WellfoundDetailPatch{}, fmt.Errorf("parse wellfound detail page: %w", err)
	}

	descSel := doc.Find(`[data-test="JobDetails-description"]`)
	description := strings.TrimSpace(jobsources.SelectionText(descSel.First()))
	if description == "" {
		return WellfoundDetailPatch{Available: false}, nil
	}

	comp := strings.TrimSpace(doc.Find(`[data-test="JobDetails-compensation"]`).First().Text())

	postedSel := doc.Find(`[data-test="JobDetails-postedAt"]`).First()
	var postedAt *string
	if dt, ok := postedSel.Attr("datetime"); ok && dt != "" {
		if parsed, err := time.Parse("2006-01-02", dt); err == nil {
			s := parsed.Format(time.RFC3339)
			postedAt = &s
		}
	}
	if postedAt == nil {
		postedAt = wellfoundPostedAtFromAge(strings.TrimSpace(postedSel.Text()))
	}

	return WellfoundDetailPatch{
		Description: description,
		SalaryRaw:   jobsources.NilIfEmpty(comp),
		PostedAt:    postedAt,
		Available:   true,
		Raw:         map[string]any{"hasEquity": wellfoundTextHasEquity(comp)},
	}, nil
}
