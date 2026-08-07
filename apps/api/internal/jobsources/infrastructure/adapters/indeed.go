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
)

const (
	indeedMaxSubscriptionPages = 50
	indeedRequestDelay         = 500 * time.Millisecond
	indeedPageSize             = 10
)

var indeedRemoteRe = regexp.MustCompile(`(?i)\bremote\b|work from home`)

type IndeedAdapter struct {
	Scraping *scraping.Service
}

func (IndeedAdapter) Key() string          { return "indeed" }
func (IndeedAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (IndeedAdapter) NeedsDetail() bool { return true }

func (d IndeedAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.Scraping.FetchHTML(ctx, "https://www.indeed.com/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(strings.ToLower(html), "indeed"), nil
}

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

func extractIndeedJK(href string) string {
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return parsed.Query().Get("jk")
}

type IndeedDetailPatch struct {
	Description string
	SalaryRaw   *string
	Location    *string
	Remote      bool
	PostedAt    *string
	Raw         map[string]string
}

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
