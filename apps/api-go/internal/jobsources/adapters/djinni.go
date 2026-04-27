package adapters

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api-go/internal/dto"
	"github.com/job-finder/api-go/internal/jobsources"
	"github.com/job-finder/api-go/internal/scraping"
)

var djinniRemoteRe = regexp.MustCompile(`(?i)remote|віддалено`)

// DjinniAdapter — djinni.co, Ukrainian dev job board, server-rendered HTML.
// A session cookie (env DJINNI_SESSION_COOKIE or config.sessionCookie)
// unlocks full listings; selectors are best-effort/defensive, matching
// djinni.adapter.ts.
type DjinniAdapter struct {
	Scraping *scraping.Service
}

func (DjinniAdapter) Key() string          { return "djinni" }
func (DjinniAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (d DjinniAdapter) Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error) {
	cookie := jobsources.StringOr(config["sessionCookie"], os.Getenv("DJINNI_SESSION_COOKIE"))
	params := url.Values{}
	if query.Keywords != "" {
		params.Set("keywords", query.Keywords)
	}
	if query.Remote != nil && *query.Remote {
		params.Set("employment", "remote")
	}
	pageURL := "https://djinni.co/jobs/?" + params.Encode()

	headers := map[string]string{}
	if cookie != "" {
		headers["Cookie"] = "sessionid=" + cookie
	}
	html, err := d.Scraping.FetchHTML(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var jobs []dto.NormalizedJob
	doc.Find(`[id^="job-item-"], li.list-jobs__item`).Each(func(_ int, item *goquery.Selection) {
		link := item.Find(`a[href^="/jobs/"]`).First()
		href, hasHref := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		if !hasHref || href == "" || title == "" {
			return
		}

		company := strings.TrimSpace(item.Find(`a[href^="/company/"], .js-analytics-company, [data-analytics="company_page"]`).First().Text())
		description := strings.TrimSpace(item.Find(`.js-truncated-text, .js-original-text, .text-card`).First().Text())
		salary := strings.TrimSpace(item.Find(`.public-salary-item, .text-success`).First().Text())
		location := strings.TrimSpace(item.Find(`.location-text`).First().Text())

		itemHTML, _ := item.Html()
		if len(itemHTML) > 4000 {
			itemHTML = itemHTML[:4000]
		}

		full, err := url.Parse(href)
		absURL := href
		if err == nil {
			base, _ := url.Parse("https://djinni.co")
			absURL = base.ResolveReference(full).String()
		}

		segs := strings.Split(strings.Trim(href, "/"), "/")
		var externalID *string
		if len(segs) > 0 && segs[len(segs)-1] != "" {
			externalID = jobsources.Ptr(segs[len(segs)-1])
		}

		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "djinni",
			ExternalID:  externalID,
			Title:       title,
			Company:     firstNonEmpty(company, "Unknown"),
			Location:    jobsources.NilIfEmpty(location),
			Remote:      djinniRemoteRe.MatchString(item.Text()),
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         absURL,
			Description: description,
			Raw:         map[string]string{"html": itemHTML},
		})
	})

	if len(jobs) == 0 {
		slog.Warn("djinni returned 0 jobs — markup may have changed or session cookie expired")
	}
	return jobs, nil
}

func (d DjinniAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	html, err := d.Scraping.FetchHTML(ctx, "https://djinni.co/jobs/", nil)
	if err != nil {
		return false, nil
	}
	return strings.Contains(html, "djinni"), nil
}

func firstNonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
