// Package adapters — djinni.co job-source adapter.
//
// This file defines the Djinni URL-shape discriminator used by both the
// subscription validator (save-time acceptance) and the adapter Search() path
// (run-time mode selection), per spec FR-002, research.md R2, and
// contracts/djinni-url-shapes.md.
//
// Shape A — Dashboard:  /my/dashboard/subs/<id>/
// Shape B — BasicSearch: /jobs/?search_type=basic-search&...
// Everything else → Unknown (rejected at save time).
package adapters

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// DjinniSearchMode encodes the subscription-URL shape.
type DjinniSearchMode int

const (
	DjinniModeUnknown DjinniSearchMode = iota
	DjinniModeBasicSearch
)

// DjinniDetect returns the Djinni search mode a saved subscription URL maps to
// by inspecting host, path, and query parameters (pure net/url — no HTTP,
// no goquery). When the URL does not match either the dashboard shape or the
// basic-search shape, the caller (validator or run path) treats the result as
// Unknown and produces the appropriate human-readable reason.
func DjinniDetect(rawURL string) DjinniSearchMode {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return DjinniModeUnknown
	}
	host := strings.ToLower(parsed.Host)
	if host != "djinni.co" && host != "www.djinni.co" {
		return DjinniModeUnknown
	}
	return djinniDetectShape(parsed)
}

// djinniDetectShape performs the path/query shape check (no host check) so the
// run-time Search() path can route subscriptions without re-validating the
// host (which was already validated at save time). Called by DjinniDetect.
func djinniDetectShape(parsed *url.URL) DjinniSearchMode {
	if (parsed.Path == "/jobs" || parsed.Path == "/jobs/") &&
		parsed.Query().Get("search_type") == "basic-search" {
		return DjinniModeBasicSearch
	}

	return DjinniModeUnknown
}

// BasicSearchFilters is the non-persisted parse of a basic-search URL's query
// parameters, used for logging clarity (Go) and display (TS).
type BasicSearchFilters struct {
	PrimaryKeyword string
	Salary         string
	ExpLevels      []string
	Employment     string
}

// ParseBasicSearch extracts basic-search filter values from the query string
// when the URL matches the basic-search shape. Returns (filters, true) in that
// case and zero-value / false otherwise. Parsed values are preserved as-is —
// no normalization, no currency conversion.
func ParseBasicSearch(rawURL string) (BasicSearchFilters, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return BasicSearchFilters{}, false
	}
	if djinniDetectShape(parsed) != DjinniModeBasicSearch {
		return BasicSearchFilters{}, false
	}
	q := parsed.Query()
	f := BasicSearchFilters{
		PrimaryKeyword: q.Get("primary_keyword"),
		Salary:         q.Get("salary"),
		ExpLevels:      q["exp_level"],
		Employment:     q.Get("employment"),
	}
	return f, true
}

// summarizeExpLevels renders a deduplicated, sorted set of Djinni exp_level
// "Ny" tokens as a human-readable string per contracts/djinni-url-shapes.md
// §4. Consecutive integer years collapse to "min–max years", non-consecutive
// to "a, b years", and any non-parseable token forces a safe list fallback
// (never mis-collapse).
func summarizeExpLevels(values []string) string {
	if len(values) == 0 {
		return ""
	}

	set := make(map[string]struct{}, len(values))
	order := make([]string, 0, len(values))
	for _, v := range values {
		if _, seen := set[v]; seen {
			continue
		}
		set[v] = struct{}{}
		order = append(order, v)
	}
	if len(order) == 0 {
		return ""
	}

	years := make([]int, 0, len(order))
	allParseable := true
	for _, v := range order {
		if !strings.HasSuffix(v, "y") {
			allParseable = false
			break
		}
		n, err := strconv.Atoi(strings.TrimSuffix(v, "y"))
		if err != nil {
			allParseable = false
			break
		}
		years = append(years, n)
	}

	if !allParseable {
		deduped := make([]string, 0, len(set))
		for k := range set {
			deduped = append(deduped, k)
		}
		sort.Strings(deduped)
		return strings.Join(deduped, ", ")
	}

	sort.Ints(years)
	if len(years) == 1 {
		return strconv.Itoa(years[0]) + " years"
	}

	consecutive := true
	for i := 1; i < len(years); i++ {
		if years[i]-years[i-1] != 1 {
			consecutive = false
			break
		}
	}

	if consecutive {
		return strconv.Itoa(years[0]) + "\u2013" + strconv.Itoa(years[len(years)-1]) + " years"
	}

	parts := make([]string, len(years))
	for i, y := range years {
		parts[i] = strconv.Itoa(y)
	}
	return strings.Join(parts, ", ") + " years"
}
