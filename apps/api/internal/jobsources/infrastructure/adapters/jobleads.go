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
	jobLeadsMaxSubscriptionPages = 50
	jobLeadsRequestDelay         = 500 * time.Millisecond
)

var jobLeadsRemoteRe = regexp.MustCompile(`(?i)\bremote\b|work from home`)

type JobLeadsAdapter struct {
	Scraping *scraping.Service
	Session  JobLeadsSessionProvider
}

func (JobLeadsAdapter) Key() string          { return "jobleads" }
func (JobLeadsAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

func (JobLeadsAdapter) NeedsDetail() bool { return true }

func (JobLeadsAdapter) UsesUserAccount() bool { return true }

func (d JobLeadsAdapter) authHeaders(ctx context.Context) (map[string]string, error) {
	headers := map[string]string{}
	if d.Session == nil {
		return nil, fmt.Errorf("jobleads requires login but no credentials configured: set JOBLEADS_EMAIL and JOBLEADS_PASSWORD")
	}
	cookie, err := d.Session.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	if cookie == "" {
		return nil, fmt.Errorf("jobleads requires login but no credentials configured: set JOBLEADS_EMAIL and JOBLEADS_PASSWORD")
	}
	setJobLeadsCookie(headers, cookie)
	return headers, nil
}

func setJobLeadsCookie(headers map[string]string, cookie string) {
	if cookie != "" {
		headers["Cookie"] = "session=" + cookie
	} else {
		delete(headers, "Cookie")
	}
}

func (d JobLeadsAdapter) fetchDoc(ctx context.Context, pageURL string, headers map[string]string) (*goquery.Document, error) {
	doc, err := d.fetchParse(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	if !jobLeadsIsLoginPage(doc) {
		return doc, nil
	}
	if d.Session == nil {
		return nil, fmt.Errorf("jobleads requires login but no credentials configured: set JOBLEADS_EMAIL and JOBLEADS_PASSWORD")
	}
	cookie, err := d.Session.Refresh(ctx)
	if err != nil {
		return nil, fmt.Errorf("jobleads session expired and re-login failed: %w", err)
	}
	setJobLeadsCookie(headers, cookie)

	doc, err = d.fetchParse(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	if jobLeadsIsLoginPage(doc) {
		return nil, fmt.Errorf("jobleads still at login after re-login (check JOBLEADS_EMAIL/JOBLEADS_PASSWORD)")
	}
	return doc, nil
}

func (d JobLeadsAdapter) fetchParse(ctx context.Context, pageURL string, headers map[string]string) (*goquery.Document, error) {
	html, err := d.Scraping.FetchHTML(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	return goquery.NewDocumentFromReader(strings.NewReader(html))
}

func jobLeadsIsLoginPage(doc *goquery.Document) bool {
	return doc.Find(`input[name="password"]`).Length() > 0
}

func (d JobLeadsAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("jobleads keyword search not implemented — use subscription URL instead")
	}

	headers, err := d.authHeaders(ctx)
	if err != nil {
		return nil, err
	}

	jobs, err := d.scrapeSubscription(ctx, query.SubscriptionURL, headers)
	if len(jobs) == 0 && err == nil {
		slog.Warn("jobleads subscription returned 0 jobs — markup may have changed or search has no matches", "url", query.SubscriptionURL)
	}
	return jobs, err
}

func (d JobLeadsAdapter) scrapeSubscription(ctx context.Context, subURL string, headers map[string]string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("jobleads: invalid subscription url %q: %w", subURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstHref := ""
	for page := 1; page <= jobLeadsMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("page", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		if page > 1 {
			time.Sleep(jobLeadsRequestDelay)
		}

		doc, err := d.fetchDoc(ctx, pageURL.String(), headers)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			slog.Warn("jobleads subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}

		cards := parseJobLeadsListings(doc, pageURL.String())
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

func parseJobLeadsListings(doc *goquery.Document, pageURL string) []dto.NormalizedJob {
	base, _ := url.Parse(pageURL)
	if base == nil {
		base, _ = url.Parse(jobLeadsBaseURL)
	}

	var jobs []dto.NormalizedJob
	doc.Find(`.job-card`).Each(func(_ int, card *goquery.Selection) {
		link := card.Find(`.job-card__link`).First()
		href, hasHref := link.Attr("href")
		title := strings.TrimSpace(card.Find(`.job-card__title`).First().Text())
		if !hasHref || href == "" || title == "" {
			return
		}

		full, err := url.Parse(href)
		absURL := href
		if err == nil {
			absURL = base.ResolveReference(full).String()
		}

		company := strings.TrimSpace(card.Find(`.job-card__company`).First().Text())
		location := strings.TrimSpace(card.Find(`.job-card__location`).First().Text())
		salary := strings.TrimSpace(card.Find(`.job-card__salary`).First().Text())
		description := jobsources.SelectionText(card.Find(`.job-card__summary`).First())
		postedAt, _ := card.Find(`.job-card__date`).First().Attr("datetime")

		jobs = append(jobs, dto.NormalizedJob{
			SourceKey:   "jobleads",
			ExternalID:  jobLeadsExternalID(href),
			Title:       title,
			Company:     firstNonEmpty(company, "Unknown"),
			Location:    jobsources.NilIfEmpty(location),
			Remote:      jobLeadsRemoteRe.MatchString(card.Text()),
			SalaryRaw:   jobsources.NilIfEmpty(salary),
			URL:         absURL,
			Description: description,
			PostedAt:    jobsources.NilIfEmpty(postedAt),
		})
	})
	return jobs
}

func jobLeadsExternalID(href string) *string {
	segs := strings.Split(strings.Trim(href, "/"), "/")
	if len(segs) > 0 && segs[len(segs)-1] != "" {
		return jobsources.Ptr(segs[len(segs)-1])
	}
	return nil
}

func (d JobLeadsAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	headers, err := d.authHeaders(ctx)
	if err != nil {
		return false, nil
	}
	doc, err := d.fetchDoc(ctx, jobLeadsBaseURL+"/job-search", headers)
	if err != nil {
		return false, nil
	}
	return doc != nil, nil
}

type JobLeadsDetailPatch struct {
	Description string
	SalaryRaw   *string
	PostedAt    *string
	Available   bool
	Raw         map[string]any
}

var jobLeadsUnavailableRe = regexp.MustCompile(`(?i)no longer available|job has been removed|position has expired`)

func (d JobLeadsAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (JobLeadsDetailPatch, error) {
	headers, err := d.authHeaders(ctx)
	if err != nil {
		return JobLeadsDetailPatch{}, err
	}
	doc, err := d.fetchDoc(ctx, jobURL, headers)
	if err != nil {
		return JobLeadsDetailPatch{}, err
	}

	if jobLeadsUnavailableRe.MatchString(doc.Text()) || doc.Find(`.job-detail`).Length() == 0 {
		return JobLeadsDetailPatch{Available: false}, nil
	}

	description := jobsources.SelectionText(doc.Find(`.job-detail__description`).First())
	salary := strings.TrimSpace(doc.Find(`.job-detail__salary`).First().Text())
	postedAt, _ := doc.Find(`.job-detail__date`).First().Attr("datetime")

	return JobLeadsDetailPatch{
		Description: description,
		SalaryRaw:   jobsources.NilIfEmpty(salary),
		PostedAt:    jobsources.NilIfEmpty(postedAt),
		Available:   true,
		Raw:         map[string]any{},
	}, nil
}
