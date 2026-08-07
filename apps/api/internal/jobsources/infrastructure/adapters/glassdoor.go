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
	glassdoorMaxSubscriptionPages = 50
	glassdoorRequestDelay         = 500 * time.Millisecond
)

var glassdoorRemoteRe = regexp.MustCompile(`(?i)\bremote\b|work from home`)

type GlassdoorAdapter struct {
	Scraping  *scraping.Service
	Retrieval retrieval.Service
}

func (GlassdoorAdapter) Key() string          { return "glassdoor" }
func (GlassdoorAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (GlassdoorAdapter) NeedsDetail() bool { return true }

func (d GlassdoorAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.fetchPage(ctx, "https://www.glassdoor.com/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(strings.ToLower(html), "glassdoor"), nil
}

func (d GlassdoorAdapter) fetchPage(ctx context.Context, url string, headers map[string]string) (string, error) {
	if d.Retrieval != nil {
		result, err := d.Retrieval.Fetch(ctx, retrieval.FetchRequest{URL: url, Headers: headers})
		if err != nil {
			return "", err
		}
		if result.Outcome.Status == retrieval.PageChallenged {
			return "", fmt.Errorf("glassdoor: challenged: %s", result.Outcome.Reason)
		}
		if result.Outcome.Status == retrieval.PageRefused {
			return "", fmt.Errorf("glassdoor: refused: %s", result.Outcome.Reason)
		}
		if result.Outcome.Status == retrieval.PageDeferred {
			return "", fmt.Errorf("glassdoor: deferred: %s", result.Outcome.Reason)
		}
		return result.Body, nil
	}
	html, err := d.Scraping.FetchHTML(ctx, url, headers)
	if err != nil {
		return "", err
	}
	if glassdoorIsBlockedPage(html) {
		return "", fmt.Errorf("glassdoor: request blocked by bot-challenge/security interstitial: %s", url)
	}
	return html, nil
}

func glassdoorIsBlockedPage(html string) bool {
	lower := strings.ToLower(html)
	return strings.Contains(lower, "security | glassdoor") &&
		strings.Contains(lower, "noindex") && strings.Contains(lower, "nofollow")
}

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

		htmlStr, err := d.fetchPage(ctx, pageURL.String(), nil)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("glassdoor subscription fetch: %w", err)
			}
			slog.Warn("glassdoor subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
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

func glassdoorSalaryIsEstimate(salary string) bool {
	lower := strings.ToLower(salary)
	return strings.Contains(lower, "glassdoor est") || strings.Contains(lower, "estimate")
}

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

type GlassdoorDetailPatch struct {
	Description string
	SalaryRaw   *string
	PostedAt    *string
	Available   bool
	Raw         map[string]any
}

func (d GlassdoorAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (GlassdoorDetailPatch, error) {
	htmlStr, err := d.fetchPage(ctx, jobURL, nil)
	if err != nil {
		return GlassdoorDetailPatch{}, fmt.Errorf("glassdoor detail fetch: %w", err)
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
