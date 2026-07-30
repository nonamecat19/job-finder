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

func loadJobgetherFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func docFromJobgetherFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(loadJobgetherFixture(t, name)))
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return doc
}

func TestJobgetherKey(t *testing.T) {
	var a JobgetherAdapter
	if got := a.Key(); got != "jobgether" {
		t.Errorf("expected key 'jobgether', got %q", got)
	}
}

func TestJobgetherKind(t *testing.T) {
	var a JobgetherAdapter
	if got := a.Kind(); got != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, got)
	}
}

func TestParseJobgetherCards(t *testing.T) {
	doc := docFromJobgetherFixture(t, "jobgether_list.html")
	jobs := parseJobgetherCards(doc, "https://jobgether.com/jobs/search?technology=go&remote=true&page=1")

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	first := jobs[0]
	if first.SourceKey != "jobgether" {
		t.Errorf("sourceKey: got %q", first.SourceKey)
	}
	if first.Title != "Senior Backend Engineer (Go)" {
		t.Errorf("title: got %q", first.Title)
	}
	if first.Company != "Waveform Labs" {
		t.Errorf("company: got %q", first.Company)
	}
	if first.ExternalID == nil || *first.ExternalID != "77213" {
		t.Errorf("externalID: got %v, want 77213", first.ExternalID)
	}
	if !strings.Contains(first.URL, "senior-backend-engineer-go-waveform-labs") {
		t.Errorf("url: got %q", first.URL)
	}
	if !first.Remote {
		t.Errorf("expected first card (location 'Remote') to be remote")
	}
	if first.SalaryRaw == nil || *first.SalaryRaw == "" {
		t.Errorf("expected salary to be captured for first card")
	}
	if first.PostedAt == nil {
		t.Errorf("expected postedAt to be resolved from 'posted 2 days ago'")
	}
	if first.Description == "" {
		t.Errorf("expected list-level summary description to be captured")
	}
	firstRaw, _ := first.Raw.(map[string]any)
	if score, ok := firstRaw["jobgetherMatchScore"]; !ok || score != "88% Match" {
		t.Errorf("expected Raw[jobgetherMatchScore]='88%% Match', got %v", firstRaw["jobgetherMatchScore"])
	}

	second := jobs[1]
	if second.Remote {
		t.Errorf("expected second card (hybrid, non-remote-labeled location) to not match remote — got remote=true for location %v", second.Location)
	}
	if second.SalaryRaw != nil {
		t.Errorf("expected second card to have no salary, got %v", second.SalaryRaw)
	}
	secondRaw, _ := second.Raw.(map[string]any)
	if _, ok := secondRaw["jobgetherMatchScore"]; ok {
		t.Errorf("expected second card to have no match score, got %v", secondRaw["jobgetherMatchScore"])
	}

	third := jobs[2]
	if !third.Remote {
		t.Errorf("expected third card ('Remote - Worldwide') to be remote")
	}
}

func TestParseJobgetherCards_Empty(t *testing.T) {
	doc := docFromJobgetherFixture(t, "jobgether_empty.html")
	jobs := parseJobgetherCards(doc, "https://jobgether.com/jobs/search?technology=zzzznonexistentquery")

	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs on empty results page, got %d", len(jobs))
	}
}

func TestJobgetherIsBlockedPage(t *testing.T) {
	blocked := loadJobgetherFixture(t, "jobgether_blocked.html")
	if !jobgetherIsBlockedPage(blocked) {
		t.Fatal("expected jobgether_blocked.html to be detected as a blocked/rate-limit page")
	}

	notBlocked := loadJobgetherFixture(t, "jobgether_list.html")
	if jobgetherIsBlockedPage(notBlocked) {
		t.Fatal("expected jobgether_list.html to NOT be detected as a blocked/rate-limit page")
	}

	empty := loadJobgetherFixture(t, "jobgether_empty.html")
	if jobgetherIsBlockedPage(empty) {
		t.Fatal("expected jobgether_empty.html (zero results) to NOT be detected as blocked — must stay distinguishable per FR-011")
	}
}

func TestJobgetherSearch_NoSubscriptionURL(t *testing.T) {
	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty (keyword search out of scope)")
	}
}

func TestJobgetherSearch_Pagination(t *testing.T) {
	page1 := loadJobgetherFixture(t, "jobgether_list.html")
	page2 := loadJobgetherFixture(t, "jobgether_list_page2.html")
	empty := loadJobgetherFixture(t, "jobgether_empty.html")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
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

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/search?technology=go&remote=true"}, nil)
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

func TestJobgetherSearch_RespectsPageCap(t *testing.T) {
	page1 := loadJobgetherFixture(t, "jobgether_list.html")

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		// Every page returns the same first card, tripping the loop guard
		// after page 1 — this asserts the cap/guard bounds the request
		// count rather than looping forever on an unbounded/repeating feed.
		_, _ = w.Write([]byte(page1))
	}))
	defer srv.Close()

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/search?technology=go"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs from page 1 only (loop guard stops repeat), got %d", len(jobs))
	}
	if hits > jobgetherMaxSubscriptionPages {
		t.Fatalf("expected at most %d requests (page cap), got %d", jobgetherMaxSubscriptionPages, hits)
	}
	if hits != 2 {
		t.Fatalf("expected exactly 2 requests (page 1, then page 2 repeating page 1's first card, then stop), got %d", hits)
	}
}

func TestJobgetherSearch_Blocked(t *testing.T) {
	blocked := loadJobgetherFixture(t, "jobgether_blocked.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	_, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/search?technology=go"}, nil)
	if err == nil {
		t.Fatal("expected a distinguishable error when the response is a rate-limit/challenge page")
	}
}

func TestJobgetherSearch_BlockedMidPagination(t *testing.T) {
	page1 := loadJobgetherFixture(t, "jobgether_list.html")
	blocked := loadJobgetherFixture(t, "jobgether_blocked.html")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "text/html")
		if p == "1" {
			_, _ = w.Write([]byte(page1))
			return
		}
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/search?technology=go"}, nil)
	if err != nil {
		t.Fatalf("expected page 1's results to be kept when a later page is blocked, got error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected page 1's 3 jobs to be kept despite page 2 being blocked, got %d", len(jobs))
	}
}

func TestJobgetherHealthCheck(t *testing.T) {
	// HealthCheck targets the fixed jobgether.com host; reachability in a
	// sandboxed/offline test environment is not guaranteed, so this only
	// asserts the "never a non-nil error" contract, matching
	// GlassdoorAdapter's equivalent test.
	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	if _, err := a.HealthCheck(context.Background(), nil); err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}
}

func TestJobgetherFetchDetail(t *testing.T) {
	detail := loadJobgetherFixture(t, "jobgether_detail.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(detail))
	}))
	defer srv.Close()

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/jobs/senior-backend-engineer-go-waveform-labs-77213", nil)
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
	if score, ok := patch.Raw["jobgetherMatchScore"]; !ok || score != "88% Match" {
		t.Errorf("expected Raw[jobgetherMatchScore]='88%% Match', got %v", patch.Raw["jobgetherMatchScore"])
	}
}

func TestJobgetherFetchDetail_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>This job is no longer available</body></html>"))
	}))
	defer srv.Close()

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/jobs/gone", nil)
	if err != nil {
		t.Fatalf("expected nil error for a gone/unavailable listing (FR-009 edge case), got %v", err)
	}
	if patch.Available {
		t.Fatal("expected patch.Available = false for a gone/unavailable listing")
	}
}

func TestJobgetherFetchDetail_Blocked(t *testing.T) {
	blocked := loadJobgetherFixture(t, "jobgether_blocked.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := JobgetherAdapter{Scraping: scraping.New(nil)}
	_, err := a.FetchDetail(context.Background(), srv.URL+"/jobs/x", nil)
	if err == nil {
		t.Fatal("expected a distinguishable error when the detail response is a rate-limit/challenge page")
	}
}
