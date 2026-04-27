package adapters

import (
	"context"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/jobsources"
	"github.com/job-finder/api-go/internal/scraping"
)

var douRemoteRe = regexp.MustCompile(`(?i)remote|віддалено`)

// DouAdapter — jobs.dou.ua, Ukrainian dev job board, server-rendered
// vacancy list. Best-effort selectors; failures degrade this source only.
// Mirrors dou.adapter.ts.
type DouAdapter struct {
	Scraping *scraping.Service
}

func (DouAdapter) Key() string          { return "dou" }
func (DouAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (d DouAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	params := url.Values{}
	if query.Keywords != "" {
		params.Set("search", query.Keywords)
	}
	if query.Remote != nil && *query.Remote {
		params.Set("remote", "")
	}
	pageURL := "https://jobs.dou.ua/vacancies/?" + params.Encode()

	html, err := d.Scraping.FetchHTML(ctx, pageURL, nil)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var jobs []dto.NormalizedJob
	doc.Find("li.l-vacancy").Each(func(_ int, item *goquery.Selection) {
		link := item.Find("a.vt").First()
		href, hasHref := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		if !hasHref || href == "" || title == "" {
			return
		}

		company := strings.TrimSpace(item.Find("a.company").First().Text())
		cities := strings.TrimSpace(item.Find(".cities").First().Text())
		salary := strings.TrimSpace(item.Find("span.salary").First().Text())
		sh := strings.TrimSpace(item.Find(".sh-info").First().Text())
		date := strings.TrimSpace(item.Find(".date").First().Text())

		itemHTML, _ := item.Html()
		if len(itemHTML) > 4000 {
			itemHTML = itemHTML[:4000]
		}

		segs := strings.Split(strings.Trim(href, "/"), "/")
		var externalID *string
		if len(segs) > 0 && segs[len(segs)-1] != "" {
			externalID = jobsources.Ptr(segs[len(segs)-1])
		}

		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "dou",
			ExternalID:  externalID,
			Title:       title,
			Company:     firstNonEmpty(company, "Unknown"),
			Location:    jobsources.NilIfEmpty(cities),
			Remote:      douRemoteRe.MatchString(cities),
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         href,
			Description: sh,
			// DOU renders relative/localized dates ("14 июня"); skip parsing, keep raw.
			Raw: map[string]string{"date": date, "html": itemHTML},
		})
	})

	if len(jobs) == 0 {
		slog.Warn("dou returned 0 jobs — markup may have changed")
	}
	return jobs, nil
}

func (d DouAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.Scraping.FetchHTML(ctx, "https://jobs.dou.ua/vacancies/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(html, "vacanc"), nil
}
