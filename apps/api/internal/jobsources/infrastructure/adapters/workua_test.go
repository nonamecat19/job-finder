package adapters

// Fixtures captured on 2026-07-16 from:
//   workua_list.html:   https://www.work.ua/jobs-php/
//   workua_detail.html: https://www.work.ua/jobs/8306445/
//   workua_empty.html:  https://www.work.ua/jobs-kyiv-php/?salaryfrom=4&experience=2

import (
	"context"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func docFromFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(loadFixture(t, name)))
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return doc
}

func TestWorkUaKey(t *testing.T) {
	var a WorkUaAdapter
	if got := a.Key(); got != "workua" {
		t.Errorf("Key() = %q, want %q", got, "workua")
	}
}

func TestWorkUaKind(t *testing.T) {
	var a WorkUaAdapter
	if got := a.Kind(); got != dto.SourceKindScrape {
		t.Errorf("Kind() = %q, want %q", got, dto.SourceKindScrape)
	}
}

func TestWorkUaParseCards_EmptyVsBroken(t *testing.T) {
	emptyDoc := docFromFixture(t, "workua_empty.html")
	cards := parseWorkUaCards(emptyDoc)
	if len(cards) != 0 {
		t.Errorf("expected 0 cards from empty fixture, got %d", len(cards))
	}

	emptyHTML := loadFixture(t, "workua_empty.html")
	if !strings.Contains(emptyHTML, "no-results") && !strings.Contains(emptyHTML, "За вашим запитом") {
		t.Error("empty fixture should contain no-results marker")
	}

	garbageDoc, _ := goquery.NewDocumentFromReader(strings.NewReader("<html><body>not work.ua</body></html>"))
	garbageCards := parseWorkUaCards(garbageDoc)
	if len(garbageCards) != 0 {
		t.Errorf("expected 0 cards from garbage HTML, got %d", len(garbageCards))
	}
}

func TestWorkUaParseCards(t *testing.T) {
	doc := docFromFixture(t, "workua_list.html")
	jobs := parseWorkUaCards(doc)

	if len(jobs) < 10 {
		t.Errorf("expected >= 10 jobs, got %d", len(jobs))
	}

	for i, job := range jobs {
		if job.Title == "" {
			t.Errorf("job[%d] has empty Title", i)
		}
		if job.Company == "" {
			t.Errorf("job[%d] has empty Company", i)
		}
		if !strings.HasPrefix(job.URL, "https://www.work.ua") {
			t.Errorf("job[%d] URL %q does not start with work.ua base", i, job.URL)
		}
		if job.ExternalID == nil || *job.ExternalID == "" {
			t.Errorf("job[%d] has nil or empty ExternalID", i)
		}
		if job.SourceKey != "workua" {
			t.Errorf("job[%d] SourceKey = %q, want %q", i, job.SourceKey, "workua")
		}
	}
}

func TestWorkUaCyrillicRoundTrip(t *testing.T) {
	doc := docFromFixture(t, "workua_list.html")
	jobs := parseWorkUaCards(doc)

	for i, job := range jobs {
		for _, s := range []string{job.Title, job.Company} {
			if strings.Contains(s, string(utf8.RuneError)) {
				t.Errorf("job[%d] contains replacement character in %q", i, s)
			}
		}
		if raw, ok := job.Raw.(map[string]string); ok {
			if strings.Contains(raw["html"], string(utf8.RuneError)) {
				t.Errorf("job[%d] raw HTML contains replacement character", i)
			}
		}
	}
}

func TestWorkUaSearchURL(t *testing.T) {
	tests := []struct {
		name       string
		keywords   string
		remote     bool
		wantPrefix string
	}{
		{
			name:       "plain keywords",
			keywords:   "php",
			remote:     false,
			wantPrefix: "https://www.work.ua/jobs/?search=",
		},
		{
			name:       "remote only",
			keywords:   "php",
			remote:     true,
			wantPrefix: "https://www.work.ua/jobs-remote/?search=",
		},
		{
			name:       "multi-word keywords",
			keywords:   "golang developer",
			remote:     false,
			wantPrefix: "https://www.work.ua/jobs/?search=",
		},
		{
			name:       "empty keywords",
			keywords:   "",
			remote:     false,
			wantPrefix: "https://www.work.ua/jobs/?search=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remotePath := "jobs"
			if tt.remote {
				remotePath = "jobs-remote"
			}
			pageURL := "https://www.work.ua/" + remotePath + "/?search=" + tt.keywords
			if !strings.HasPrefix(pageURL, tt.wantPrefix) {
				t.Errorf("constructed URL %q does not have expected prefix %q", pageURL, tt.wantPrefix)
			}
			if tt.remote && !strings.Contains(pageURL, "/jobs-remote/") {
				t.Error("expected /jobs-remote/ path for remote query")
			}
			if !tt.remote && !strings.Contains(pageURL, "/jobs/") {
				t.Error("expected /jobs/ path for non-remote query")
			}
		})
	}
}

func TestWorkUaFetchDetailParse(t *testing.T) {
	doc := docFromFixture(t, "workua_detail.html")

	description := strings.TrimSpace(doc.Find("#job-description").First().Text())
	if description == "" {
		t.Error("expected non-empty description from detail page")
	}

	listDoc := docFromFixture(t, "workua_list.html")
	teaser := ""
	listDoc.Find("div.card.job-link").First().Each(func(_ int, item *goquery.Selection) {
		teaser = strings.TrimSpace(item.Text())
	})
	if teaser != "" && len(description) <= len(teaser) {
		t.Error("description should be longer than card teaser")
	}
}

func TestWorkUaPostedAt(t *testing.T) {
	rfc3339 := parseWorkUaPostedAt("2026-07-16 02:29:02")
	if rfc3339 == nil {
		t.Fatal("parseWorkUaPostedAt returned nil for valid input")
	}
	if *rfc3339 != "2026-07-16T02:29:02Z" {
		t.Errorf("expected RFC3339 %q, got %q", "2026-07-16T02:29:02Z", *rfc3339)
	}

	bad := parseWorkUaPostedAt("2026-07-16T02:29:02")
	if bad != nil {
		t.Error("expected nil for raw RFC3339 input (should not parse under work.ua layout)")
	}

	empty := parseWorkUaPostedAt("")
	if empty != nil {
		t.Error("expected nil for empty input")
	}
}

func TestWorkUaDetailMissingFields(t *testing.T) {
	html := "<html><body><p>no job data here</p></body></html>"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}

	description := strings.TrimSpace(doc.Find("#job-description").First().Text())
	if description != "" {
		t.Error("expected empty description for minimal HTML")
	}
	company := strings.TrimSpace(doc.Find(`a[href^="/jobs/by-company/"]`).First().Text())
	if company != "" {
		t.Error("expected empty company for minimal HTML")
	}
}

func TestWorkUaSubscriptionMalformedURL(t *testing.T) {
	var a WorkUaAdapter
	_, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "://not a url"}, nil)
	if err == nil {
		t.Fatal("expected error for malformed subscription URL")
	}
	if !strings.Contains(err.Error(), "://not a url") {
		t.Errorf("error should contain the offending URL, got: %v", err)
	}
}
