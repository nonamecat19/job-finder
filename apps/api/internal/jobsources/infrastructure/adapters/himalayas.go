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
	"github.com/job-finder/api/internal/platform/scraping"
)

const (
	himalayasAPIURL               = "https://himalayas.app/jobs/api"
	himalayasUserAgent            = "job-finder/1.0 (+https://github.com/job-finder; job discovery bot)"
	himalayasRequestDelay         = 500 * time.Millisecond
	himalayasMaxSubscriptionPages = 50
	himalayasPageLimit            = 20
)

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

type himalayasResponse struct {
	Offset     int            `json:"offset"`
	Limit      int            `json:"limit"`
	TotalCount int            `json:"totalCount"`
	Jobs       []himalayasJob `json:"jobs"`
}

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

type himalayasSubscriptionFilter struct {
	categories map[string]struct{}
	timezones  map[int]struct{}
}

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
