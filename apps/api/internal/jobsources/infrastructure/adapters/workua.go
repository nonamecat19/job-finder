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
	"github.com/job-finder/api/internal/strutil"
)

const (
	workuaBaseURL              = "https://www.work.ua"
	workuaMaxSubscriptionPages = 50
)

const WorkUaMinDelay = 2 * time.Second

var workuaRemoteRe = regexp.MustCompile(`(?i)remote|віддалено|дистанційно`)

type WorkUaAdapter struct {
	Scraping scraping.Scraper
}

func (WorkUaAdapter) Key() string          { return "workua" }
func (WorkUaAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (WorkUaAdapter) NeedsDetail() bool { return true }

func (d WorkUaAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL != "" {
		jobs, err := d.scrapeWorkUaSubscription(ctx, query.SubscriptionURL)
		if len(jobs) == 0 && err == nil {
			slog.Warn("workua subscription returned 0 jobs — markup may have changed", "url", query.SubscriptionURL)
		}
		return jobs, err
	}

	var pageURL string
	if query.Remote != nil && *query.Remote {
		pageURL = fmt.Sprintf("%s/jobs-remote/?search=%s", workuaBaseURL, url.QueryEscape(query.Keywords))
	} else {
		pageURL = fmt.Sprintf("%s/jobs/?search=%s", workuaBaseURL, url.QueryEscape(query.Keywords))
	}

	html, err := d.Scraping.FetchHTML(ctx, pageURL, nil)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	jobs := parseWorkUaCards(doc)
	if len(jobs) == 0 {
		hasNoResults := strings.Contains(html, "no-results") || strings.Contains(html, "За вашим запитом")
		if hasNoResults {
			slog.Warn("workua: no results matched the search query", "url", pageURL)
		} else {
			slog.Warn("workua: returned 0 jobs — markup may have changed", "url", pageURL)
		}
	}
	return jobs, nil
}

func (d WorkUaAdapter) scrapeWorkUaSubscription(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("workua: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstHref := ""
	for page := 1; page <= workuaMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("page", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		html, err := d.Scraping.FetchHTML(ctx, pageURL.String(), nil)
		if err != nil {
			slog.Warn("workua subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			break
		}

		cards := parseWorkUaCards(doc)
		if len(cards) == 0 {
			break
		}
		if cards[0].URL == seenFirstHref {
			break
		}
		seenFirstHref = cards[0].URL

		jobs = append(jobs, cards...)

		select {
		case <-ctx.Done():
			return jobs, nil
		default:
		}

		time.Sleep(WorkUaMinDelay)
	}
	return jobs, nil
}

func parseWorkUaCards(doc *goquery.Document) []dto.NormalizedJob {
	var jobs []dto.NormalizedJob
	doc.Find("div.card.job-link").Each(func(_ int, item *goquery.Selection) {
		link := item.Find(`h2 a[href^="/jobs/"]`).First()
		href, hasHref := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		if !hasHref || href == "" || title == "" {
			return
		}

		company := strings.TrimSpace(item.Find("span.strong-600").First().Text())
		if company == "" {
			company = "Unknown"
		}

		salary := strings.TrimSpace(item.Find("li:has(span.glyphicon-hryvnia-fill)").First().Text())
		location := strings.TrimSpace(item.Find(".text-default-7 .location, .glyphicon-map-marker").First().Text())

		remote := workuaRemoteRe.MatchString(item.Text())

		itemHTML, _ := item.Html()
		itemHTML = strutil.Truncate(itemHTML, 4000)

		full, err := url.Parse(href)
		absURL := href
		if err == nil {
			base, _ := url.Parse(workuaBaseURL)
			absURL = base.ResolveReference(full).String()
		}

		segs := strings.Split(strings.Trim(href, "/"), "/")
		var externalID *string
		if len(segs) > 0 {
			if idStr := segs[len(segs)-1]; idStr != "" {
				if _, err := strconv.Atoi(idStr); err == nil {
					externalID = jobsources.Ptr(idStr)
				}
			}
		}
		if externalID == nil {
			if dataID, exists := item.Attr("data-id"); exists && dataID != "" {
				externalID = jobsources.Ptr(dataID)
			}
		}

		cardText := strings.TrimSpace(jobsources.SelectionText(item))
		descLen := len(cardText)
		if descLen > 200 {
			descLen = 200
		}
		description := cardText[:descLen]

		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "workua",
			ExternalID:  externalID,
			Title:       title,
			Company:     company,
			Location:    jobsources.NilIfEmpty(location),
			Remote:      remote,
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         absURL,
			Description: description,
			Raw:         map[string]string{"html": itemHTML},
		})
	})
	return jobs
}

type WorkUaDetailPatch struct {
	Description string
	SalaryRaw   *string
	Location    *string
	Remote      bool
	PostedAt    *string
	Raw         map[string]string
}

func (d WorkUaAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (WorkUaDetailPatch, error) {
	html, err := d.Scraping.FetchHTML(ctx, jobURL, nil)
	if err != nil {
		return WorkUaDetailPatch{}, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return WorkUaDetailPatch{}, err
	}

	description := strings.TrimSpace(jobsources.SelectionText(doc.Find("#job-description").First()))
	company := strings.TrimSpace(doc.Find(`a[href^="/jobs/by-company/"]`).First().Text())
	salary := strings.TrimSpace(doc.Find("li:has(span.glyphicon-hryvnia-fill)").First().Text())
	locationText := strings.TrimSpace(doc.Find(`span.glyphicon-map-marker`).First().Parent().Text())
	postedAt, _ := doc.Find(`time[datetime]`).First().Attr("datetime")

	remote := workuaRemoteRe.MatchString(doc.Find("body").Text())

	bodyHTML, _ := doc.Find("body").Html()
	bodyHTML = strutil.Truncate(bodyHTML, 8000)

	patch := WorkUaDetailPatch{
		Description: description,
		SalaryRaw:   jobsources.NilIfEmpty(salary),
		Location:    jobsources.NilIfEmpty(locationText),
		Remote:      remote,
		Raw:         map[string]string{"html": bodyHTML},
	}
	if company != "" {
		if patch.Location == nil {
			patch.Location = jobsources.Ptr("")
		}
	}

	patch.PostedAt = parseWorkUaPostedAt(postedAt)

	return patch, nil
}

func parseWorkUaPostedAt(datetimeAttr string) *string {
	if datetimeAttr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02 15:04:05", datetimeAttr)
	if err != nil {
		return nil
	}
	rfc3339 := t.Format(time.RFC3339)
	return &rfc3339
}

func (d WorkUaAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.Scraping.FetchHTML(ctx, "https://www.work.ua/jobs/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(html, "work.ua"), nil
}
