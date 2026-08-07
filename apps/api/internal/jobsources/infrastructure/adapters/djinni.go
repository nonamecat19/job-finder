package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/platform/scraping"
	"github.com/job-finder/api/internal/strutil"
)

const djinniMaxSubscriptionPages = 50

var djinniRemoteRe = regexp.MustCompile(`(?i)remote|віддалено`)

type DjinniAdapter struct {
	Scraping scraping.Scraper
	Session  DjinniSessionProvider
}

func (d DjinniAdapter) authHeaders(ctx context.Context) (map[string]string, error) {
	headers := map[string]string{}
	if d.Session == nil {
		return headers, nil
	}
	cookie, err := d.Session.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	setDjinniCookie(headers, cookie)
	return headers, nil
}

func setDjinniCookie(headers map[string]string, cookie string) {
	if cookie != "" {
		headers["Cookie"] = "sessionid=" + cookie
	} else {
		delete(headers, "Cookie")
	}
}

func (d DjinniAdapter) fetchDoc(ctx context.Context, pageURL string, headers map[string]string) (*goquery.Document, error) {
	doc, err := d.fetchParse(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	if !djinniIsLoginPage(doc) {
		return doc, nil
	}
	if d.Session == nil {
		return nil, fmt.Errorf("djinni requires login but no credentials configured: set DJINNI_EMAIL and DJINNI_PASSWORD")
	}
	cookie, err := d.Session.Refresh(ctx)
	if err != nil {
		return nil, fmt.Errorf("djinni session expired and re-login failed: %w", err)
	}
	setDjinniCookie(headers, cookie)

	doc, err = d.fetchParse(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	if djinniIsLoginPage(doc) {
		return nil, fmt.Errorf("djinni still at login after re-login (check DJINNI_EMAIL/DJINNI_PASSWORD)")
	}
	return doc, nil
}

func (d DjinniAdapter) fetchParse(ctx context.Context, pageURL string, headers map[string]string) (*goquery.Document, error) {
	html, err := d.Scraping.FetchHTML(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

func (DjinniAdapter) Key() string          { return "djinni" }
func (DjinniAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (d DjinniAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	headers, err := d.authHeaders(ctx)
	if err != nil {
		return nil, err
	}

	if query.SubscriptionURL != "" {
		jobs, err := d.scrapeSubscription(ctx, query.SubscriptionURL, headers)
		if len(jobs) == 0 && err == nil {
			slog.Warn("djinni subscription returned 0 jobs — markup may have changed", "url", query.SubscriptionURL)
		}
		return jobs, err
	}

	params := url.Values{}
	if query.Keywords != "" {
		params.Set("keywords", query.Keywords)
	}
	if query.Remote != nil && *query.Remote {
		params.Set("employment", "remote")
	}
	pageURL := "https://djinni.co/jobs/?" + params.Encode()

	doc, err := d.fetchDoc(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	jobs := parseDjinniCards(doc)
	if len(jobs) == 0 {
		slog.Warn("djinni returned 0 jobs — markup may have changed")
	}
	return jobs, nil
}

func (d DjinniAdapter) scrapeSubscription(ctx context.Context, subURL string, headers map[string]string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("djinni: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstHref := ""
	for page := 1; page <= djinniMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("page", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		doc, err := d.fetchDoc(ctx, pageURL.String(), headers)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			slog.Warn("djinni subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		cards := parseDjinniCards(doc)
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

func djinniIsLoginPage(doc *goquery.Document) bool {
	return doc.Find(`input[name="password"]`).Length() > 0
}

func parseDjinniCards(doc *goquery.Document) []dto.NormalizedJob {
	var jobs []dto.NormalizedJob
	doc.Find(`[id^="job-item-"], li.list-jobs__item`).Each(func(_ int, item *goquery.Selection) {
		link := item.Find(`a[href^="/jobs/"]`).First()
		href, hasHref := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		if !hasHref || href == "" || title == "" {
			return
		}

		company := strings.TrimSpace(item.Find(`a[href^="/company/"], .js-analytics-company, [data-analytics="company_page"]`).First().Text())
		description := jobsources.SelectionText(item.Find(`.js-truncated-text, .js-original-text, .text-card`).First())
		salary := strings.TrimSpace(item.Find(`.public-salary-item, .text-success`).First().Text())
		location := strings.TrimSpace(item.Find(`.location-text`).First().Text())

		itemHTML, _ := item.Html()
		itemHTML = strutil.Truncate(itemHTML, 4000)

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
	return jobs
}

type DjinniDetailPatch struct {
	Description string
	SalaryRaw   *string
	Location    *string
	Remote      bool
	PostedAt    *string
	Raw         map[string]string
}

func (d DjinniAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (DjinniDetailPatch, error) {
	headers, err := d.authHeaders(ctx)
	if err != nil {
		return DjinniDetailPatch{}, err
	}
	doc, err := d.fetchDoc(ctx, jobURL, headers)
	if err != nil {
		return DjinniDetailPatch{}, err
	}

	description := jobsources.SelectionText(doc.Find(`.job-post__description, .job-post-page__description, .js-original-text, [data-qa="job-description"], article`).First())
	salary := strings.TrimSpace(doc.Find(`.public-salary-item, .job-additional-info-item .text-success, .text-success`).First().Text())
	location := strings.TrimSpace(doc.Find(`.location-text, .job-additional-info-item .location`).First().Text())
	postedAt, _ := doc.Find(`.job-post__details time, time[datetime]`).First().Attr("datetime")

	remote := djinniRemoteRe.MatchString(doc.Find("body").Text())

	bodyHTML, _ := doc.Find("body").Html()
	bodyHTML = strutil.Truncate(bodyHTML, 8000)

	patch := DjinniDetailPatch{
		Description: description,
		SalaryRaw:   jobsources.NilIfEmpty(salary),
		Location:    jobsources.NilIfEmpty(location),
		Remote:      remote,
		Raw:         map[string]string{"html": bodyHTML},
	}
	if postedAt != "" {
		patch.PostedAt = &postedAt
	}
	return patch, nil
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
