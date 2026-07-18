package adapters

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/strutil"
)

// djinniMaxSubscriptionPages caps subscription-page pagination so a
// redirect-to-page-1 loop (or an unbounded feed) can't run forever.
const djinniMaxSubscriptionPages = 50

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
	headers := map[string]string{}
	if cookie != "" {
		headers["Cookie"] = "sessionid=" + cookie
	}

	if query.SubscriptionURL != "" {
		if cookie == "" {
			return nil, fmt.Errorf("djinni subscription %q requires a login session: set DJINNI_SESSION_COOKIE or the source's sessionCookie config", query.SubscriptionURL)
		}
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

	html, err := d.Scraping.FetchHTML(ctx, pageURL, headers)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	jobs := parseDjinniCards(doc)
	if len(jobs) == 0 {
		slog.Warn("djinni returned 0 jobs — markup may have changed or session cookie expired")
	}
	return jobs, nil
}

// scrapeSubscription pages through a logged-in djinni "subs" listing
// (https://djinni.co/my/dashboard/subs/{id}/) — same card markup as the
// public /jobs/ search, reused via parseDjinniCards. Stops on an empty page,
// a hard page cap, or the page redirecting back to an already-seen first
// card (guards an infinite pagination loop).
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

		html, err := d.Scraping.FetchHTML(ctx, pageURL.String(), headers)
		if err != nil {
			slog.Warn("djinni subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			break
		}

		// An expired/absent session cookie makes djinni 302 the subs URL to
		// /login (followed to a 200 login page), which parses to 0 cards.
		// Surface that as a clear error instead of a silent empty result.
		if djinniIsLoginPage(doc) {
			return nil, fmt.Errorf("djinni session cookie invalid or expired: subscription %q redirected to login", subURL)
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

// djinniIsLoginPage reports whether doc is djinni's /login page (served with a
// 200 after an auth redirect). The password input is unique to that page and
// never present on job-listing markup.
func djinniIsLoginPage(doc *goquery.Document) bool {
	return doc.Find(`input[name="password"]`).Length() > 0
}

// parseDjinniCards extracts job cards from a djinni listing page (shared by
// the /jobs/ search and the authenticated subs pages — same markup).
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
		description := strings.TrimSpace(item.Find(`.js-truncated-text, .js-original-text, .text-card`).First().Text())
		salary := strings.TrimSpace(item.Find(`.public-salary-item, .text-success`).First().Text())
		location := strings.TrimSpace(item.Find(`.location-text`).First().Text())

		itemHTML, _ := item.Html()
		// This page's raw HTML routinely contains Cyrillic (Ukrainian job
		// board); a byte slice can split a multi-byte UTF-8 sequence and
		// mangle/corrupt the last character, unlike a rune-safe truncate.
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

// DjinniDetailPatch is the parsed subset of a djinni job detail page used to
// fill in a shallow (list-only) Job row. Not part of the Adapter interface —
// djinni-specific, called directly by the enrichment handler.
type DjinniDetailPatch struct {
	Description string
	SalaryRaw   *string
	Location    *string
	Remote      bool
	PostedAt    *string
	Raw         map[string]string
}

// FetchDetail fetches a single djinni job page and parses the full
// description/salary/location/remote/posted-date. Selectors are best-effort
// and defensive (unverified against live markup — see plan risk #2): a
// missing field just stays empty rather than erroring.
func (d DjinniAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (DjinniDetailPatch, error) {
	cookie := jobsources.StringOr(config["sessionCookie"], os.Getenv("DJINNI_SESSION_COOKIE"))
	headers := map[string]string{}
	if cookie != "" {
		headers["Cookie"] = "sessionid=" + cookie
	}

	html, err := d.Scraping.FetchHTML(ctx, jobURL, headers)
	if err != nil {
		return DjinniDetailPatch{}, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return DjinniDetailPatch{}, err
	}

	description := strings.TrimSpace(doc.Find(`.job-post__description, .job-post-page__description, .js-original-text, [data-qa="job-description"], article`).First().Text())
	salary := strings.TrimSpace(doc.Find(`.public-salary-item, .job-additional-info-item .text-success, .text-success`).First().Text())
	location := strings.TrimSpace(doc.Find(`.location-text, .job-additional-info-item .location`).First().Text())
	postedAt, _ := doc.Find(`.job-post__details time, time[datetime]`).First().Attr("datetime")

	remote := djinniRemoteRe.MatchString(doc.Find("body").Text())

	bodyHTML, _ := doc.Find("body").Html()
	// Same rune-safety concern as parseDjinniCards — this page is routinely
	// Cyrillic.
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
