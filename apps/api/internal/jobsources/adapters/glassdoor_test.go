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
	"github.com/job-finder/api/internal/scraping"
)

func loadGlassdoorFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func docFromGlassdoorFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(loadGlassdoorFixture(t, name)))
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return doc
}

func TestGlassdoorKey(t *testing.T) {
	var a GlassdoorAdapter
	if got := a.Key(); got != "glassdoor" {
		t.Errorf("expected key 'glassdoor', got %q", got)
	}
}

func TestGlassdoorKind(t *testing.T) {
	var a GlassdoorAdapter
	if got := a.Kind(); got != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, got)
	}
}

func TestParseGlassdoorCards(t *testing.T) {
	doc := docFromGlassdoorFixture(t, "glassdoor_list.html")
	jobs := parseGlassdoorCards(doc, "https://www.glassdoor.com/Job/remote-golang-jobs-SRCH_KO0,14.htm?p=1")

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	first := jobs[0]
	if first.SourceKey != "glassdoor" {
		t.Errorf("sourceKey: got %q", first.SourceKey)
	}
	if first.Title != "Senior Golang Developer" {
		t.Errorf("title: got %q", first.Title)
	}
	if first.Company != "NovaTech LLC" {
		t.Errorf("company: got %q", first.Company)
	}
	if first.ExternalID == nil || *first.ExternalID != "1010196986009" {
		t.Errorf("externalID: got %v, want 1010196986009", first.ExternalID)
	}
	if !strings.Contains(first.URL, "senior-golang-developer-novatech") {
		t.Errorf("url: got %q", first.URL)
	}
	if !first.Remote {
		t.Errorf("expected first card (location 'Remote') to be remote")
	}
	if first.SalaryRaw == nil || *first.SalaryRaw == "" {
		t.Errorf("expected salary to be captured for first card")
	}
	if first.PostedAt == nil {
		t.Errorf("expected postedAt to be resolved from job-age '3d'")
	}

	second := jobs[1]
	if second.Remote {
		t.Errorf("expected second card (hybrid, non-remote-labeled location) to not match remote — got remote=true for location %v", second.Location)
	}
	if second.SalaryRaw != nil {
		t.Errorf("expected second card to have no salary, got %v", second.SalaryRaw)
	}

	third := jobs[2]
	if !third.Remote {
		t.Errorf("expected third card ('Remote in United States') to be remote")
	}
}

func TestParseGlassdoorCards_Empty(t *testing.T) {
	doc := docFromGlassdoorFixture(t, "glassdoor_empty.html")
	jobs := parseGlassdoorCards(doc, "https://www.glassdoor.com/Job/zzzznonexistentquery.htm")

	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs on empty results page, got %d", len(jobs))
	}
}

func TestGlassdoorIsBlockedPage(t *testing.T) {
	blocked := loadGlassdoorFixture(t, "glassdoor_blocked.html")
	if !glassdoorIsBlockedPage(blocked) {
		t.Fatal("expected glassdoor_blocked.html to be detected as a blocked/challenge page")
	}

	notBlocked := loadGlassdoorFixture(t, "glassdoor_list.html")
	if glassdoorIsBlockedPage(notBlocked) {
		t.Fatal("expected glassdoor_list.html to NOT be detected as a blocked/challenge page")
	}
}

func TestGlassdoorSearch_NoSubscriptionURL(t *testing.T) {
	a := GlassdoorAdapter{Scraping: scraping.New()}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty (keyword search out of scope)")
	}
}

func TestGlassdoorSearch_Pagination(t *testing.T) {
	page1 := loadGlassdoorFixture(t, "glassdoor_list.html")
	page2 := loadGlassdoorFixture(t, "glassdoor_list_page2.html")
	empty := loadGlassdoorFixture(t, "glassdoor_empty.html")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("p")
		hits++
		w.Header().Set("Content-Type", "text/html")
		switch p {
		case "1":
			_, _ = w.Write([]byte(page1))
		case "2":
			_, _ = w.Write([]byte(page2))
		default:
			_, _ = w.Write([]byte(empty))
		}
	}))
	defer srv.Close()

	a := GlassdoorAdapter{Scraping: scraping.New()}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/Job/remote-golang-jobs-SRCH_KO0,14.htm"}, nil)
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

func TestGlassdoorSearch_Blocked(t *testing.T) {
	blocked := loadGlassdoorFixture(t, "glassdoor_blocked.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := GlassdoorAdapter{Scraping: scraping.New()}
	_, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/Job/remote-golang-jobs-SRCH_KO0,14.htm"}, nil)
	if err == nil {
		t.Fatal("expected a distinguishable error when the response is a bot-challenge/security page")
	}
}

func TestGlassdoorHealthCheck(t *testing.T) {
	// HealthCheck targets the fixed glassdoor.com host; reachability in a
	// sandboxed/offline test environment is not guaranteed, so this only
	// asserts the "never a non-nil error" contract, matching IndeedAdapter's
	// equivalent test.
	a := GlassdoorAdapter{Scraping: scraping.New()}
	if _, err := a.HealthCheck(context.Background(), nil); err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}
}

func TestGlassdoorFetchDetail(t *testing.T) {
	detail := loadGlassdoorFixture(t, "glassdoor_detail.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(detail))
	}))
	defer srv.Close()

	a := GlassdoorAdapter{Scraping: scraping.New()}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/job-listing/senior-golang-developer-novatech-JV_KO0,24_KE25,41.htm?jl=1010196986009", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !patch.Available {
		t.Fatal("expected patch.Available = true")
	}
	if len(patch.Description) < 100 {
		t.Errorf("expected full description longer than a list snippet, got %d chars", len(patch.Description))
	}
	if patch.PostedAt == nil {
		t.Errorf("expected postedAt to be resolved from fixture")
	}
	if patch.SalaryRaw == nil || *patch.SalaryRaw == "" {
		t.Errorf("expected salary to be captured from detail page")
	}
}

func TestGlassdoorFetchDetail_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>This job is no longer available</body></html>"))
	}))
	defer srv.Close()

	a := GlassdoorAdapter{Scraping: scraping.New()}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/job-listing/gone.htm?jl=0", nil)
	if err != nil {
		t.Fatalf("expected nil error for a gone/unavailable listing (FR-009 edge case), got %v", err)
	}
	if patch.Available {
		t.Fatal("expected patch.Available = false for a gone/unavailable listing")
	}
}

func TestGlassdoorFetchDetail_Blocked(t *testing.T) {
	blocked := loadGlassdoorFixture(t, "glassdoor_blocked.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := GlassdoorAdapter{Scraping: scraping.New()}
	_, err := a.FetchDetail(context.Background(), srv.URL+"/job-listing/x.htm?jl=1", nil)
	if err == nil {
		t.Fatal("expected a distinguishable error when the detail response is a bot-challenge/security page")
	}
}
