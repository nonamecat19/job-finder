package adapters

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type DjinniSearchMode int

const (
	DjinniModeUnknown DjinniSearchMode = iota
	DjinniModeBasicSearch
)

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

func djinniDetectShape(parsed *url.URL) DjinniSearchMode {
	if (parsed.Path == "/jobs" || parsed.Path == "/jobs/") &&
		parsed.Query().Get("search_type") == "basic-search" {
		return DjinniModeBasicSearch
	}

	return DjinniModeUnknown
}

type BasicSearchFilters struct {
	PrimaryKeyword string
	Salary         string
	ExpLevels      []string
	Employment     string
}

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
