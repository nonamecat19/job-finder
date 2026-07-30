// Package adapters — Himalayas, backed by Himalayas's own undocumented but
// functional public JSON feed (https://himalayas.app/jobs/api), following
// the RemoteOK pattern: a public, unauthenticated API fetched via
// scraping.Service.FetchHTML (not raw jobsources.GetJSON) with a descriptive
// User-Agent (FR-017), no login/session of any kind. Unlike RemoteOK, the
// upstream feed ignores every filter query parameter tried against it
// (research.md R3), so Search pages through the raw firehose and filters
// each page's jobs locally against the operator's saved category/timezone
// subscription — mirroring ArbeitnowAdapter's "fetch bounded pages, filter
// client-side" shape rather than RemoteOK's "pass the filter through as a
// query param" shape.
//
// Himalayas has no FetchDetail method and is intentionally absent from
// ingestion.Handler's enrich-eligible source list and enrichment.Handler's
// dispatch switch (both read-only, no code change needed there): the feed's
// description field is already the full posting body at ingestion time, and
// there is no working per-listing detail endpoint to enrich from anyway
// (research.md R6).
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/scraping"
)

const (
	// himalayasAPIURL is Himalayas's undocumented but functional,
	// unauthenticated JSON feed backing its own /jobs search page.
	himalayasAPIURL = "https://himalayas.app/jobs/api"
	// himalayasUserAgent identifies this app to Himalayas (FR-017),
	// mirroring remoteokUserAgent's exact format/tone.
	himalayasUserAgent = "job-finder/1.0 (+https://github.com/job-finder; job discovery bot)"
	// himalayasRequestDelay is the floor between paginated requests
	// (FR-010, research.md R7).
	himalayasRequestDelay = 500 * time.Millisecond
	// himalayasMaxSubscriptionPages caps pagination so a narrow/rare
	// category can't sweep unbounded across Himalayas's ~96k-listing feed
	// (research.md R7).
	himalayasMaxSubscriptionPages = 50
	// himalayasPageLimit is the page size requested per call; Himalayas
	// server-caps this at 20 regardless of the requested value.
	himalayasPageLimit = 20
)

// HimalayasAdapter — himalayas.app public JSON API, no credentials. APIURL
// overrides himalayasAPIURL when set, letting tests point at an httptest
// server instead of the real host; production code leaves it empty.
type HimalayasAdapter struct {
	Scraping *scraping.Service
	APIURL   string
}

func (HimalayasAdapter) Key() string          { return "himalayas" }
func (HimalayasAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

func (d HimalayasAdapter) apiURL() string {
	if d.APIURL != "" {
		return d.APIURL
	}
	return himalayasAPIURL
}

// himalayasResponse is the raw decode shape of a /jobs/api page.
type himalayasResponse struct {
	Offset     int            `json:"offset"`
	Limit      int            `json:"limit"`
	TotalCount int            `json:"totalCount"`
	Jobs       []himalayasJob `json:"jobs"`
}

// himalayasJob is the raw decode shape of a single job record within a
// /jobs/api page's "jobs" array — see research.md#r5-job-field-mapping.
type himalayasJob struct {
	GUID                 string   `json:"guid"`
	Title                string   `json:"title"`
	CompanyName          string   `json:"companyName"`
	Categories           []string `json:"categories"`
	ParentCategories     []string `json:"parentCategories"`
	TimezoneRestrictions []int    `json:"timezoneRestrictions"`
	LocationRestrictions []string `json:"locationRestrictions"`
	MinSalary            int      `json:"minSalary"`
	MaxSalary            int      `json:"maxSalary"`
	Currency             string   `json:"currency"`
	SalaryPeriod         string   `json:"salaryPeriod"`
	PubDate              int64    `json:"pubDate"`
	Description          string   `json:"description"`
	Seniority            string   `json:"seniority"`
	EmploymentType       string   `json:"employmentType"`
}

// himalayasJobFromRaw maps a raw Himalayas job record to the shared
// NormalizedJob DTO per research.md#r5-job-field-mapping.
func himalayasJobFromRaw(raw himalayasJob) dto.NormalizedJob {
	var location *string
	if len(raw.LocationRestrictions) > 0 {
		location = jobsources.Ptr(strings.Join(raw.LocationRestrictions, ", "))
	}

	var postedAt *string
	if raw.PubDate != 0 {
		postedAt = jobsources.Ptr(time.Unix(raw.PubDate, 0).UTC().Format(time.RFC3339))
	}

	return dto.NormalizedJob{
		SourceKey:   "himalayas",
		ExternalID:  jobsources.NilIfEmpty(raw.GUID),
		Title:       raw.Title,
		Company:     raw.CompanyName,
		Location:    location,
		Remote:      true,
		SalaryRaw:   himalayasSalaryRaw(raw),
		URL:         raw.GUID,
		Description: jobsources.HTMLToText(raw.Description),
		PostedAt:    postedAt,
		Raw: map[string]any{
			"timezoneRestrictions": himalayasTimezoneText(raw.TimezoneRestrictions),
			"categories":           raw.Categories,
			"parentCategories":     raw.ParentCategories,
			"seniority":            raw.Seniority,
			"employmentType":       raw.EmploymentType,
		},
	}
}

// himalayasSalaryRaw formats minSalary/maxSalary/currency/salaryPeriod into
// a display string, or nil when both bounds are absent/zero (spec's "no
// salary range published" edge case).
func himalayasSalaryRaw(raw himalayasJob) *string {
	if raw.MinSalary == 0 && raw.MaxSalary == 0 {
		return nil
	}
	currency := raw.Currency
	if currency == "" {
		currency = "USD"
	}
	period := raw.SalaryPeriod
	var amount string
	switch {
	case raw.MinSalary != 0 && raw.MaxSalary != 0:
		amount = fmt.Sprintf("%s %d - %d", currency, raw.MinSalary, raw.MaxSalary)
	case raw.MinSalary != 0:
		amount = fmt.Sprintf("%s %d", currency, raw.MinSalary)
	default:
		amount = fmt.Sprintf("%s %d", currency, raw.MaxSalary)
	}
	if period != "" {
		amount = fmt.Sprintf("%s/%s", amount, period)
	}
	return jobsources.Ptr(amount)
}

// himalayasTimezoneText folds timezoneRestrictions (UTC-offset ints, empty =
// unrestricted) into descriptive text (spec's "listing restricts applicants
// to a specific timezone band" edge case) — captured for display, never used
// to exclude the listing from the feed itself.
func himalayasTimezoneText(offsets []int) string {
	if len(offsets) == 0 {
		return "no timezone restriction"
	}
	parts := make([]string, 0, len(offsets))
	for _, o := range offsets {
		parts = append(parts, formatUTCOffset(o))
	}
	return "restricted to " + strings.Join(parts, ", ")
}

func formatUTCOffset(o int) string {
	if o >= 0 {
		return fmt.Sprintf("UTC+%d", o)
	}
	return fmt.Sprintf("UTC%d", o)
}

// himalayasSubscriptionFilter is the parsed shape of an operator-saved
// Himalayas /jobs?categories=<slug>[,<slug>...][&timezones=<a,b>] search-page
// URL (research.md R4).
type himalayasSubscriptionFilter struct {
	categories map[string]struct{}
	timezones  map[int]struct{} // empty means "no timezone restriction requested"
}

// matches reports whether a raw job satisfies the filter: at least one of
// its categories/parentCategories is in the filter's category set, and —
// when timezones were requested — its timezoneRestrictions is empty
// (unrestricted) or overlaps the requested set.
func (f himalayasSubscriptionFilter) matches(raw himalayasJob) bool {
	categoryMatch := false
	for _, c := range raw.Categories {
		if _, ok := f.categories[strings.ToLower(c)]; ok {
			categoryMatch = true
			break
		}
	}
	if !categoryMatch {
		for _, c := range raw.ParentCategories {
			if _, ok := f.categories[strings.ToLower(c)]; ok {
				categoryMatch = true
				break
			}
		}
	}
	if !categoryMatch {
		return false
	}

	if len(f.timezones) == 0 {
		return true
	}
	if len(raw.TimezoneRestrictions) == 0 {
		return true
	}
	for _, tz := range raw.TimezoneRestrictions {
		if _, ok := f.timezones[tz]; ok {
			return true
		}
	}
	return false
}

// parseHimalayasSubscriptionFilter parses a Himalayas /jobs search-page URL
// into a category-slug set and optional timezone-offset set (research.md
// R4). Returns a distinguishable error when the URL doesn't parse or its
// "categories" query parameter is empty (contracts/himalayas-adapter.md's
// Search precondition).
func parseHimalayasSubscriptionFilter(subURL string) (himalayasSubscriptionFilter, error) {
	parsed, err := url.Parse(subURL)
	if err != nil {
		return himalayasSubscriptionFilter{}, fmt.Errorf("himalayas: invalid subscription url %q: %w", subURL, err)
	}

	rawCategories := parsed.Query().Get("categories")
	if strings.TrimSpace(rawCategories) == "" {
		return himalayasSubscriptionFilter{}, fmt.Errorf("himalayas: subscription url %q is missing a required 'categories' query parameter", subURL)
	}

	categories := make(map[string]struct{})
	for _, c := range strings.Split(rawCategories, ",") {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" {
			categories[c] = struct{}{}
		}
	}

	timezones := make(map[int]struct{})
	if rawTimezones := parsed.Query().Get("timezones"); rawTimezones != "" {
		for _, t := range strings.Split(rawTimezones, ",") {
			if off, ok := parseUTCOffset(strings.TrimSpace(t)); ok {
				timezones[off] = struct{}{}
			}
		}
	}

	return himalayasSubscriptionFilter{categories: categories, timezones: timezones}, nil
}

// parseUTCOffset parses a "UTC-5", "UTC+8", or "UTC" style token into its
// integer offset. Returns false for anything else, so a malformed timezone
// token is skipped rather than rejecting the whole subscription.
func parseUTCOffset(s string) (int, bool) {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "UTC")
	if s == "" {
		return 0, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// Search only supports the saved-subscription flow (FR-014); keyword search
// is out of scope, matching RemoteOKAdapter.Search's stance exactly.
func (d HimalayasAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("himalayas keyword search not implemented — use subscription URL instead")
	}

	filter, err := parseHimalayasSubscriptionFilter(query.SubscriptionURL)
	if err != nil {
		return nil, err
	}

	var results []dto.NormalizedJob
	offset := 0
	for page := 0; page < himalayasMaxSubscriptionPages; page++ {
		if page > 0 {
			time.Sleep(himalayasRequestDelay)
		}

		reqURL := fmt.Sprintf("%s?limit=%d&offset=%d", d.apiURL(), himalayasPageLimit, offset)
		body, err := d.Scraping.FetchHTML(ctx, reqURL, map[string]string{"User-Agent": himalayasUserAgent})
		if err != nil {
			if page == 0 {
				return nil, fmt.Errorf("himalayas subscription fetch: %w", err)
			}
			slog.Warn("himalayas subscription page fetch failed, stopping pagination", "url", reqURL, "error", err)
			break
		}

		var res himalayasResponse
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("himalayas: response could not be interpreted: %w", err)
			}
			slog.Warn("himalayas subscription page could not be interpreted, stopping pagination", "url", reqURL, "error", err)
			break
		}

		for _, raw := range res.Jobs {
			if filter.matches(raw) {
				results = append(results, himalayasJobFromRaw(raw))
			}
		}

		offset += himalayasPageLimit
		if offset >= res.TotalCount || len(res.Jobs) == 0 {
			break
		}
	}

	if len(results) == 0 {
		slog.Warn("himalayas subscription returned 0 jobs — response shape may have changed or category has no matches", "url", query.SubscriptionURL)
	}
	return results, nil
}

// HealthCheck fetches a single page and reports whether the feed is
// reachable and returns the expected shape — never a non-nil error for the
// normal "unreachable"/"unparseable" case, mirroring
// RemoteOKAdapter.HealthCheck's convention exactly.
func (d HimalayasAdapter) HealthCheck(ctx context.Context, _ map[string]any) (bool, error) {
	reqURL := fmt.Sprintf("%s?limit=1&offset=0", d.apiURL())
	body, err := d.Scraping.FetchHTML(ctx, reqURL, map[string]string{"User-Agent": himalayasUserAgent})
	if err != nil {
		return false, nil
	}
	var res himalayasResponse
	if err := json.Unmarshal([]byte(body), &res); err != nil {
		return false, nil
	}
	return res.Jobs != nil, nil
}
