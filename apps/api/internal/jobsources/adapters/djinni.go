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
	salaryparser "github.com/job-finder/api/internal/salary"
	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/strutil"
)

// djinniMaxSubscriptionPages caps subscription-page pagination so a
// redirect-to-page-1 loop (or an unbounded feed) can't run forever.
const djinniMaxSubscriptionPages = 50

var djinniRemoteRe = regexp.MustCompile(`(?i)remote|віддалено`)

var djinniExpEnRe = regexp.MustCompile(`(?i)(\d+)\+?\s*(?:years?|yrs?\.?)\s*(?:of\s+)?(?:(?:commercial|professional|full-stack|software)\s+)*(?:development\s+)?experience`)
var djinniExpUaRe = regexp.MustCompile(`(?:від|не менше|мінімум)?\s*(\d+)\+?\s*(?:рок\S*|р\.|р)(?:\s+досвід)`)
var djinniExpUaAltRe = regexp.MustCompile(`досвід\s+(?:роботи\s+)?(?:від\s+)?(\d+)`)
var djinniAnalyticsUrlRe = regexp.MustCompile(`[?&]exp=(\d+)`)

var djinniEnglishRe = regexp.MustCompile(`(?i)(?:english|англійська)\s*(?:level|мова)?\s*[—:\-–]\s*(\S+)`)
var djinniEnglishLabelRe = regexp.MustCompile(`(?i)(Upper-?Intermediate|Advanced/Fluent|Fluent|Advanced|Intermediate|Pre-Intermediate|Elementary|Beginner)\s*(?:level\s*)?(?:english|англ)`)
var djinniEnglishUaRe = regexp.MustCompile(`(?i)рівень\s+англійської\s*[—:\-–]\s*(\S+(?:\s+\S+)?)`)

// DjinniAdapter — djinni.co, Ukrainian dev job board, server-rendered HTML.
// Selectors are best-effort/defensive, matching djinni.adapter.ts.
type DjinniAdapter struct {
	Scraping *scraping.Service
}

func (DjinniAdapter) Key() string          { return "djinni" }
func (DjinniAdapter) Kind() dto.SourceKind { return dto.SourceKindScrape }

// NeedsDetail reports true: the list page carries a truncated description;
// FetchDetail fetches the full one, so ingestion defers match/ghost scoring
// until enrichment has run.
func (DjinniAdapter) NeedsDetail() bool { return true }

func (d DjinniAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL != "" {
		parsed, err := url.Parse(query.SubscriptionURL)
		if err != nil {
			return nil, fmt.Errorf("djinni: invalid subscription url %q: %w", query.SubscriptionURL, err)
		}
		switch djinniDetectShape(parsed) {
		case DjinniModeBasicSearch:
			return d.scrapeBasicSearch(ctx, query.SubscriptionURL)
		default:
			return nil, fmt.Errorf("djinni subscription url is not a basic-search shape: %s", query.SubscriptionURL)
		}
	}

	params := url.Values{}
	if query.Keywords != "" {
		params.Set("keywords", query.Keywords)
	}
	if query.Remote != nil && *query.Remote {
		params.Set("employment", "remote")
	}
	pageURL := "https://djinni.co/jobs/?" + params.Encode()

	html, err := d.Scraping.FetchHTML(ctx, pageURL, nil)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	jobs := parseDjinniCards(doc)
	if len(jobs) == 0 {
		slog.Warn("djinni returned 0 jobs — markup may have changed")
	}
	return jobs, nil
}

// paginateDjinni pages through a djinni search-results URL starting from the
// given firstPageURL. Stops on an empty page, a hard page cap (50), or the
// page redirecting back to an already-seen first card (guards an infinite
// pagination loop).
func (d DjinniAdapter) paginateDjinni(ctx context.Context, firstPageURL string) ([]dto.NormalizedJob, error) {
	base, err := url.Parse(firstPageURL)
	if err != nil {
		return nil, fmt.Errorf("djinni: invalid pagination url %q: %w", firstPageURL, err)
	}

	var jobs []dto.NormalizedJob
	seenFirstHref := ""
	for page := 1; page <= djinniMaxSubscriptionPages; page++ {
		pageURL := *base
		q := pageURL.Query()
		q.Set("page", strconv.Itoa(page))
		pageURL.RawQuery = q.Encode()

		html, err := d.Scraping.FetchHTML(ctx, pageURL.String(), nil)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			slog.Warn("djinni subscription page fetch failed, stopping pagination", "url", pageURL.String(), "error", err)
			break
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			if page == 1 {
				return nil, err
			}
			slog.Warn("djinni subscription page parse failed, stopping pagination", "url", pageURL.String(), "error", err)
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

// scrapeBasicSearch pages through a public basic-search URL
// (https://djinni.co/jobs/?search_type=basic-search&...). It forces pagination
// starting at page=1 regardless of any page param in the saved URL (FR-003,
// SC-002), preserves all other query params verbatim, and reuses the shared
// paginateDjinni loop (including the single-page / empty-page / redirect-loop
// / 50-page cap guards from research.md R1).
func (d DjinniAdapter) scrapeBasicSearch(ctx context.Context, subURL string) ([]dto.NormalizedJob, error) {
	filters, _ := ParseBasicSearch(subURL)
	slog.Info("djinni: running basic-search", "url", subURL, "filters", filters)

	parsed, err := url.Parse(subURL)
	if err != nil {
		return nil, fmt.Errorf("djinni basic-search: invalid subscription url %q: %w", subURL, err)
	}
	q := parsed.Query()
	q.Set("page", "1")
	parsed.RawQuery = q.Encode()

	jobs, err := d.paginateDjinni(ctx, parsed.String())
	if len(jobs) == 0 && err == nil {
		slog.Warn("djinni basic-search returned 0 jobs — markup may have changed", "url", subURL)
	}
	return jobs, err
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
		description := jobsources.SelectionText(item.Find(`.js-truncated-text, .js-original-text, .text-card`).First())
		salary := strings.TrimSpace(item.Find(`.public-salary-item`).First().Text())
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

	Company               string
	ExperienceLevel       *string
	ExperienceMinYears    *int
	EnglishLevel          *string
	SalaryEstimateRaw     *string
	SalaryEstimateMin     *int
	SalaryEstimateMax     *int
	SalaryEstimateCurrency *string
}

// FetchDetail fetches a single djinni job page and parses the full
// description/salary/location/remote/posted-date. Selectors are best-effort
// and defensive (unverified against live markup — see plan risk #2): a
// missing field just stays empty rather than erroring.
func (d DjinniAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (DjinniDetailPatch, error) {
	html, err := d.Scraping.FetchHTMLDirect(ctx, jobURL, nil)
	if err != nil {
		return DjinniDetailPatch{}, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return DjinniDetailPatch{}, err
	}

	description := jobsources.SelectionText(doc.Find(`.job-post__description, .job-post-page__description, .js-original-text, [data-qa="job-description"], article`).First())
	salary := strings.TrimSpace(doc.Find(`.public-salary-item, .job-additional-info-item .text-success`).First().Text())
	location := strings.TrimSpace(doc.Find(`.location-text, .job-additional-info-item .location`).First().Text())
	postedAt, _ := doc.Find(`.job-post__details time, time[datetime]`).First().Attr("datetime")

	bodyText := doc.Find("body").Text()
	remote := djinniRemoteRe.MatchString(bodyText)

	// Company: try detail-page selector first, fall back to <title> parsing
	company := strings.TrimSpace(doc.Find(`a[href*="/company-"]`).First().Text())
	if company == "" {
		if m := regexp.MustCompile(`(.+?) в (.+?) – Djinni`).FindStringSubmatch(doc.Find("title").Text()); len(m) >= 3 {
			company = m[2]
		}
	}

	// Experience level: regex on plain-text body, fall back to analytics URL param
	plainDesc := jobsources.HTMLToText(description)
	if plainDesc == "" {
		plainDesc = jobsources.HTMLToText(bodyText)
	}
	var expLevel *string
	var expMinYears *int
	if m := djinniExpEnRe.FindStringSubmatch(plainDesc); m != nil {
		s := m[0]
		expLevel = &s
		if n, err := strconv.Atoi(m[1]); err == nil {
			expMinYears = &n
		}
	} else if m := djinniExpUaRe.FindStringSubmatch(plainDesc); m != nil {
		s := m[0]
		expLevel = &s
		if n, err := strconv.Atoi(m[1]); err == nil {
			expMinYears = &n
		}
	} else if m := djinniExpUaAltRe.FindStringSubmatch(plainDesc); m != nil {
		s := m[0]
		expLevel = &s
		if n, err := strconv.Atoi(m[1]); err == nil {
			expMinYears = &n
		}
	}
	// Fallback: parse exp=N from analytics URL
	if expLevel == nil {
		if href, ok := doc.Find(`a[href*="/salaries/"]`).First().Attr("href"); ok {
			if m := djinniAnalyticsUrlRe.FindStringSubmatch(href); m != nil {
				s := m[1] + " years"
				expLevel = &s
				if n, err := strconv.Atoi(m[1]); err == nil {
					expMinYears = &n
				}
			}
		}
	}

	// English level: regex on plain-text body
	var englishLevel *string
	if m := djinniEnglishRe.FindStringSubmatch(plainDesc); m != nil {
		v := strings.TrimSpace(m[1])
		englishLevel = &v
	} else if m := djinniEnglishLabelRe.FindStringSubmatch(plainDesc); m != nil {
		v := strings.TrimSpace(m[1])
		englishLevel = &v
	} else if m := djinniEnglishUaRe.FindStringSubmatch(plainDesc); m != nil {
		v := strings.TrimSpace(m[1])
		englishLevel = &v
	}

	// Salary analytics estimate
	var salaryEstimateRaw, salaryEstimateCurrency *string
	var salaryEstimateMin, salaryEstimateMax *int
	if seRaw := strings.TrimSpace(doc.Find(`div.salaries-info-link strong#salary-suggestion`).First().Text()); seRaw != "" {
		salaryEstimateRaw = &seRaw
		if band, ok := salaryparser.ParseSalaryRaw(seRaw); ok {
			salaryEstimateMin = &band.Min
			salaryEstimateMax = &band.Max
			salaryEstimateCurrency = &band.Currency
		}
	}

	bodyHTML, _ := doc.Find("body").Html()
	// Same rune-safety concern as parseDjinniCards — this page is routinely
	// Cyrillic.
	bodyHTML = strutil.Truncate(bodyHTML, 8000)

	patch := DjinniDetailPatch{
		Description:            description,
		SalaryRaw:              jobsources.NilIfEmpty(salary),
		Location:               jobsources.NilIfEmpty(location),
		Remote:                 remote,
		Raw:                    map[string]string{"html": bodyHTML},
		Company:                company,
		ExperienceLevel:        expLevel,
		ExperienceMinYears:     expMinYears,
		EnglishLevel:           englishLevel,
		SalaryEstimateRaw:      salaryEstimateRaw,
		SalaryEstimateMin:      salaryEstimateMin,
		SalaryEstimateMax:      salaryEstimateMax,
		SalaryEstimateCurrency: salaryEstimateCurrency,
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
