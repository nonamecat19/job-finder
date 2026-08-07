package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/scraping"
)

func loadIndeedFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func docFromIndeedFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(loadIndeedFixture(t, name)))
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return doc
}

func TestIndeedKey(t *testing.T) {
	var a IndeedAdapter
	if got := a.Key(); got != "indeed" {
		t.Errorf("expected key 'indeed', got %q", got)
	}
}

func TestIndeedKind(t *testing.T) {
	var a IndeedAdapter
	if got := a.Kind(); got != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, got)
	}
}

func TestParseIndeedCards(t *testing.T) {
	doc := docFromIndeedFixture(t, "indeed_list.html")
	jobs := parseIndeedCards(doc, "https://www.indeed.com/jobs?q=golang&l=remote&start=0")

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	first := jobs[0]
	if first.SourceKey != "indeed" {
		t.Errorf("sourceKey: got %q", first.SourceKey)
	}
	if first.Title != "Senior Golang Developer" {
		t.Errorf("title: got %q", first.Title)
	}
	if first.URL == "" {
		t.Errorf("url must not be empty")
	}
	if first.Company != "NovaTech LLC" {
		t.Errorf("company: got %q", first.Company)
	}
	if !first.Remote {
		t.Errorf("expected first card to be remote")
	}
	if first.SalaryRaw == nil || *first.SalaryRaw == "" {
		t.Errorf("expected salary to be captured for first card")
	}
	if first.ExternalID == nil || *first.ExternalID != "abc123golang" {
		t.Errorf("externalID: got %v, want abc123golang", first.ExternalID)
	}

	second := jobs[1]
	if second.Remote {
		t.Errorf("expected second card (hybrid, non-remote-labeled location) to not match strict remote — got remote=true for location %v", second.Location)
	}

	third := jobs[2]
	if !third.Remote {
		t.Errorf("expected third card ('Remote in United States') to be remote")
	}
}

func TestParseIndeedCards_Empty(t *testing.T) {
	doc := docFromIndeedFixture(t, "indeed_empty.html")
	jobs := parseIndeedCards(doc, "https://www.indeed.com/jobs?q=zzzznonexistentquery")

	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs on empty results page, got %d", len(jobs))
	}
}

func TestIndeedSearch_NoSubscriptionURL(t *testing.T) {
	a := IndeedAdapter{Scraping: scraping.New()}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty (keyword search out of scope)")
	}
}

func TestIndeedSearch_Pagination(t *testing.T) {
	page1 := loadIndeedFixture(t, "indeed_list.html")
	page2 := loadIndeedFixture(t, "indeed_list_page2.html")
	empty := loadIndeedFixture(t, "indeed_empty.html")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		hits++
		w.Header().Set("Content-Type", "text/html")
		switch start {
		case "0":
			_, _ = w.Write([]byte(page1))
		case "10":
			_, _ = w.Write([]byte(page2))
		default:
			_, _ = w.Write([]byte(empty))
		}
	}))
	defer srv.Close()

	a := IndeedAdapter{Scraping: scraping.New()}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs?q=golang"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 5 {
		t.Fatalf("expected 5 jobs across 2 pages, got %d", len(jobs))
	}
	if hits < 3 {
		t.Fatalf("expected pagination to stop after an empty page, got %d requests", hits)
	}
}

func TestIndeedHealthCheck(t *testing.T) {
	a := IndeedAdapter{Scraping: scraping.New()}
	if _, err := a.HealthCheck(context.Background(), nil); err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}
}

func TestIndeedFetchDetail(t *testing.T) {
	detail := loadIndeedFixture(t, "indeed_detail.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(detail))
	}))
	defer srv.Close()

	a := IndeedAdapter{Scraping: scraping.New()}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/viewjob?jk=abc123golang", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(patch.Description) < 100 {
		t.Errorf("expected full description longer than a list snippet, got %d chars", len(patch.Description))
	}
	if !patch.Remote {
		t.Errorf("expected remote=true from detail page content")
	}
	if patch.PostedAt == nil || *patch.PostedAt == "" {
		t.Errorf("expected postedAt to be resolved from fixture")
	}
}

func TestIndeedFetchDetail_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>Job not found</body></html>"))
	}))
	defer srv.Close()

	a := IndeedAdapter{Scraping: scraping.New()}
	_, err := a.FetchDetail(context.Background(), srv.URL+"/viewjob?jk=gone", nil)
	if err == nil {
		t.Fatal("expected a non-nil error when the detail page has no recognizable job description (gone/unavailable)")
	}
}
