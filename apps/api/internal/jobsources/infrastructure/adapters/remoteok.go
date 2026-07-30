// Package adapters — RemoteOK, backed by RemoteOK's public JSON API
// (https://remoteok.com/api) rather than HTML scraping. Search only supports
// an operator-saved tag/category subscription (query.SubscriptionURL) — no
// keyword search, matching indeed.go's stance. The API's first array element
// is a legal-notice object, not a job (see specs/003-remoteok-job-provider/
// research.md R3); every job record is looked up defensively so a malformed
// or reshaped element degrades to being skipped, never a panic.
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
	// remoteokAPIURL is RemoteOK's public, unauthenticated JSON feed.
	remoteokAPIURL = "https://remoteok.com/api"
	// remoteokUserAgent identifies this app to RemoteOK per its documented
	// API etiquette (FR-017).
	remoteokUserAgent = "job-finder/1.0 (+https://github.com/job-finder; job discovery bot)"
)

// RemoteOKAdapter — remoteok.com public JSON API, no credentials. APIURL
// overrides remoteokAPIURL when set, letting tests point at an httptest
// server instead of the real host; production code leaves it empty.
type RemoteOKAdapter struct {
	Scraping *scraping.Service
	APIURL   string
}

func (RemoteOKAdapter) Key() string          { return "remoteok" }
func (RemoteOKAdapter) Kind() dto.SourceKind { return dto.SourceKindAPI }

// NeedsDetail reports true: the feed endpoint truncates the description;
// FetchDetail re-reads the full entry, so ingestion defers match/ghost
// scoring until enrichment has run.
func (RemoteOKAdapter) NeedsDetail() bool { return true }

func (d RemoteOKAdapter) apiURL() string {
	if d.APIURL != "" {
		return d.APIURL
	}
	return remoteokAPIURL
}

func (d RemoteOKAdapter) HealthCheck(ctx context.Context, config map[string]any) (bool, error) {
	body, err := d.Scraping.FetchHTML(ctx, d.apiURL(), map[string]string{"User-Agent": remoteokUserAgent})
	if err != nil {
		return false, nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return false, nil
	}
	return len(raw) > 0, nil
}

// remoteokIsJobRecord reports whether a decoded API array element looks like
// a job listing rather than the leading legal-notice element (research.md
// R3): the legal-notice object has neither an "id" nor a "position" key.
func remoteokIsJobRecord(raw map[string]any) bool {
	_, hasID := raw["id"]
	_, hasPosition := raw["position"]
	return hasID && hasPosition
}

// Search only supports the saved-subscription flow (FR-014); keyword search
// is out of scope, matching IndeedAdapter.Search's stance exactly.
func (d RemoteOKAdapter) Search(ctx context.Context, query dto.SearchQuery, _ map[string]any) ([]dto.NormalizedJob, error) {
	if query.SubscriptionURL == "" {
		return nil, fmt.Errorf("remoteok keyword search not implemented — use subscription URL instead")
	}

	reqURL := d.requestURL(query.SubscriptionURL)

	body, err := d.Scraping.FetchHTML(ctx, reqURL, map[string]string{"User-Agent": remoteokUserAgent})
	if err != nil {
		return nil, fmt.Errorf("remoteok subscription fetch: %w", err)
	}

	jobs, err := parseRemoteOKJobs([]byte(body))
	if err != nil {
		return nil, fmt.Errorf("parse remoteok response: %w", err)
	}
	if len(jobs) == 0 {
		slog.Warn("remoteok subscription returned 0 jobs — response shape may have changed or tag has no matches", "url", reqURL)
	}
	return jobs, nil
}

// requestURL resolves a saved subscription URL (a
// remoteok.com/remote-<tag>-jobs listing URL, or the bare API root) to the
// concrete API URL to fetch, per research.md R6.
func (d RemoteOKAdapter) requestURL(subURL string) string {
	tag := remoteokTagFromURL(subURL)
	if tag == "" {
		return d.apiURL()
	}
	return d.apiURL() + "?tags=" + url.QueryEscape(tag)
}

// remoteokTagFromURL extracts "<tag>" out of a
// "https://remoteok.com/remote-<tag>-jobs" path. Returns "" when the URL is
// the bare API root or doesn't match that shape.
func remoteokTagFromURL(subURL string) string {
	parsed, err := url.Parse(subURL)
	if err != nil {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" || path == "api" {
		return ""
	}
	if !strings.HasPrefix(path, "remote-") || !strings.HasSuffix(path, "-jobs") {
		return ""
	}
	tag := strings.TrimSuffix(strings.TrimPrefix(path, "remote-"), "-jobs")
	return tag
}

// parseRemoteOKJobs decodes a RemoteOK API response body into normalized
// jobs, skipping the leading legal-notice element (research.md R3) and any
// other element that doesn't look like a job record. A non-nil error means
// the body itself wasn't valid JSON — distinct from a valid-but-empty
// listing set (FR-011).
func parseRemoteOKJobs(body []byte) ([]dto.NormalizedJob, error) {
	var raws []map[string]any
	if err := json.Unmarshal(body, &raws); err != nil {
		return nil, fmt.Errorf("remoteok: response is not a JSON array: %w", err)
	}

	var results []dto.NormalizedJob
	for _, raw := range raws {
		if !remoteokIsJobRecord(raw) {
			continue
		}
		job := remoteokJobFromRaw(raw)
		if job.Title == "" || job.URL == "" {
			continue
		}
		results = append(results, job)
	}
	return results, nil
}

func remoteokJobFromRaw(raw map[string]any) dto.NormalizedJob {
	id := remoteokStringField(raw, "id")
	title := remoteokStringField(raw, "position")
	company := remoteokStringField(raw, "company")
	location := remoteokStringField(raw, "location")
	description := remoteokStringField(raw, "description")

	jobURL := remoteokStringField(raw, "url")
	if jobURL == "" {
		jobURL = remoteokStringField(raw, "apply_url")
	}

	var externalID *string
	if id != "" {
		externalID = jobsources.Ptr(id)
	}

	var postedAt *string
	if dateText := remoteokStringField(raw, "date"); dateText != "" {
		if t, err := time.Parse(time.RFC3339, dateText); err == nil {
			postedAt = jobsources.Ptr(t.Format(time.RFC3339))
		}
	}

	var tags []string
	if rawTags, ok := raw["tags"].([]any); ok {
		for _, t := range rawTags {
			if s, ok := t.(string); ok {
				tags = append(tags, s)
			}
		}
	}

	return dto.NormalizedJob{
		SourceKey:   "remoteok",
		ExternalID:  externalID,
		Title:       title,
		Company:     company,
		Location:    jobsources.NilIfEmpty(location),
		Remote:      true,
		SalaryRaw:   remoteokSalaryRaw(raw),
		URL:         jobURL,
		Description: description,
		PostedAt:    postedAt,
		Raw:         map[string]any{"tags": tags},
	}
}

// remoteokStringField reads a string field defensively — RemoteOK's API is
// not strictly typed, so a missing or wrong-typed field degrades to "".
func remoteokStringField(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// remoteokSalaryRaw formats salary_min/salary_max (both commonly absent)
// into a display string, or nil when neither is present.
func remoteokSalaryRaw(raw map[string]any) *string {
	min := remoteokNumberField(raw, "salary_min")
	max := remoteokNumberField(raw, "salary_max")
	if min == "" && max == "" {
		return nil
	}
	if min != "" && max != "" {
		return jobsources.Ptr(fmt.Sprintf("$%s - $%s", min, max))
	}
	if min != "" {
		return jobsources.Ptr("$" + min)
	}
	return jobsources.Ptr("$" + max)
}

func remoteokNumberField(raw map[string]any, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	switch n := v.(type) {
	case float64:
		if n == 0 {
			return ""
		}
		return strconv.FormatFloat(n, 'f', 0, 64)
	case string:
		return strings.TrimSpace(n)
	default:
		return ""
	}
}

// RemoteOKDetailPatch is the result of re-confirming a single RemoteOK
// listing is still present in the current API feed and refreshing its
// detail fields. Not part of the Adapter interface — RemoteOK-specific,
// called directly by the enrichment handler.
type RemoteOKDetailPatch struct {
	Description string
	Tags        []string
	SalaryRaw   *string
	PostedAt    *string
	Available   bool
	Raw         map[string]any
}

// FetchDetail re-fetches the RemoteOK API (there is no per-listing detail
// endpoint — research.md R5) and locates the record matching jobURL. When no
// record matches (the listing has rotated out of RemoteOK's current feed),
// it returns Available: false with a nil error so the caller can leave the
// job's existing summary data untouched (FR-009 edge case) rather than
// treating it as a fetch failure.
func (d RemoteOKAdapter) FetchDetail(ctx context.Context, jobURL string, _ map[string]any) (RemoteOKDetailPatch, error) {
	body, err := d.Scraping.FetchHTML(ctx, d.apiURL(), map[string]string{"User-Agent": remoteokUserAgent})
	if err != nil {
		return RemoteOKDetailPatch{}, fmt.Errorf("remoteok detail fetch: %w", err)
	}

	var raws []map[string]any
	if err := json.Unmarshal([]byte(body), &raws); err != nil {
		return RemoteOKDetailPatch{}, fmt.Errorf("parse remoteok response: %w", err)
	}

	for _, raw := range raws {
		if !remoteokIsJobRecord(raw) {
			continue
		}
		job := remoteokJobFromRaw(raw)
		if job.URL != jobURL {
			continue
		}

		var tags []string
		if rawTags, ok := raw["tags"].([]any); ok {
			for _, t := range rawTags {
				if s, ok := t.(string); ok {
					tags = append(tags, s)
				}
			}
		}

		return RemoteOKDetailPatch{
			Description: job.Description,
			Tags:        tags,
			SalaryRaw:   job.SalaryRaw,
			PostedAt:    job.PostedAt,
			Available:   true,
			Raw:         map[string]any{"tags": tags},
		}, nil
	}

	return RemoteOKDetailPatch{Available: false}, nil
}
