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
	"github.com/job-finder/api/internal/retrieval"
)

func loadWellfoundFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func docFromWellfoundFixture(t *testing.T, name string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(loadWellfoundFixture(t, name)))
	if err != nil {
		t.Fatalf("failed to parse fixture %s: %v", name, err)
	}
	return doc
}

func TestWellfoundKey(t *testing.T) {
	var a WellfoundAdapter
	if got := a.Key(); got != "wellfound" {
		t.Errorf("expected key 'wellfound', got %q", got)
	}
}

func TestWellfoundKind(t *testing.T) {
	var a WellfoundAdapter
	if got := a.Kind(); got != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, got)
	}
}

func TestParseWellfoundCards(t *testing.T) {
	doc := docFromWellfoundFixture(t, "wellfound_list.html")
	jobs := parseWellfoundCards(doc, "https://wellfound.com/role/r/golang-engineer?page=1")

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	first := jobs[0]
	if first.SourceKey != "wellfound" {
		t.Errorf("sourceKey: got %q", first.SourceKey)
	}
	if first.Title != "Senior Golang Engineer" {
		t.Errorf("title: got %q", first.Title)
	}
	if first.Company != "NovaTech Labs" {
		t.Errorf("company: got %q", first.Company)
	}
	if first.ExternalID == nil || *first.ExternalID != "wf-1001" {
		t.Errorf("externalID: got %v, want wf-1001", first.ExternalID)
	}
	if !strings.Contains(first.URL, "wf-1001-senior-golang-engineer") {
		t.Errorf("url: got %q", first.URL)
	}
	if !first.Remote {
		t.Errorf("expected first card (location 'Remote') to be remote")
	}
	if first.SalaryRaw == nil || *first.SalaryRaw == "" {
		t.Errorf("expected salary/equity to be captured for first card")
	}
	if first.Description == "" {
		t.Errorf("expected description to be captured for first card")
	}
	if first.PostedAt == nil {
		t.Errorf("expected postedAt to be resolved from '3d ago'")
	}

	second := jobs[1]
	if second.Remote {
		t.Errorf("expected second card (hybrid, non-remote-labeled location) to not match remote — got remote=true for location %v", second.Location)
	}
	if second.SalaryRaw != nil {
		t.Errorf("expected second card to have no salary/equity text, got %v", second.SalaryRaw)
	}

	third := jobs[2]
	if !third.Remote {
		t.Errorf("expected third card ('Remote (US)') to be remote")
	}
	if third.SalaryRaw == nil || !strings.Contains(*third.SalaryRaw, "%") {
		t.Errorf("expected third card to carry an equity-only compensation string, got %v", third.SalaryRaw)
	}
}

func TestParseWellfoundCards_Empty(t *testing.T) {
	doc := docFromWellfoundFixture(t, "wellfound_empty.html")
	jobs := parseWellfoundCards(doc, "https://wellfound.com/role/r/zzzznonexistentquery")

	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs on empty results page, got %d", len(jobs))
	}
}

func TestParseWellfoundCards_MissingTitleOrURLSkipped(t *testing.T) {
	html := `<div data-test="JobSearchResult" data-job-id="wf-9999">
		<a data-test="JobSearchResult-title" href="">Untitled</a>
		<a data-test="JobSearchResult-company">NoURL Inc</a>
	</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	jobs := parseWellfoundCards(doc, "https://wellfound.com/role/r/golang-engineer")
	if len(jobs) != 0 {
		t.Fatalf("expected card missing a URL to be skipped, got %d jobs", len(jobs))
	}
}

func TestWellfoundIsBlockedPage(t *testing.T) {
	blocked := loadWellfoundFixture(t, "wellfound_blocked.html")
	if !retrieval.IsChallenged(blocked, 0) {
		t.Fatal("expected wellfound_blocked.html to be detected as a blocked/challenge page")
	}

	notBlocked := loadWellfoundFixture(t, "wellfound_list.html")
	if retrieval.IsChallenged(notBlocked, 0) {
		t.Fatal("expected wellfound_list.html to NOT be detected as a blocked/challenge page")
	}

	empty := loadWellfoundFixture(t, "wellfound_empty.html")
	if retrieval.IsChallenged(empty, 0) {
		t.Fatal("expected a legitimate zero-results page to NOT be detected as blocked")
	}
}

func TestWellfoundSearch_NoSubscriptionURL(t *testing.T) {
	a := WellfoundAdapter{Scraping: scraping.New()}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty (keyword search out of scope)")
	}
}

func TestWellfoundSearch_Pagination(t *testing.T) {
	page1 := loadWellfoundFixture(t, "wellfound_list.html")
	page2 := loadWellfoundFixture(t, "wellfound_list_page2.html")
	empty := loadWellfoundFixture(t, "wellfound_empty.html")

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

	a := WellfoundAdapter{Scraping: scraping.New()}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/role/r/golang-engineer"}, nil)
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

func TestWellfoundSearch_Blocked(t *testing.T) {
	blocked := loadWellfoundFixture(t, "wellfound_blocked.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := WellfoundAdapter{Scraping: scraping.New()}
	_, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/role/r/golang-engineer"}, nil)
	if err == nil {
		t.Fatal("expected a distinguishable error when the response is a bot-challenge/rate-limit page")
	}
}

func TestWellfoundSearch_EmptyResultsSucceeds(t *testing.T) {
	empty := loadWellfoundFixture(t, "wellfound_empty.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(empty))
	}))
	defer srv.Close()

	a := WellfoundAdapter{Scraping: scraping.New()}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/role/r/zzzznonexistentquery"}, nil)
	if err != nil {
		t.Fatalf("expected a legitimate zero-results page to succeed (not fail), got %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestWellfoundHealthCheck(t *testing.T) {
	a := WellfoundAdapter{Scraping: scraping.New()}
	if _, err := a.HealthCheck(context.Background(), nil); err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}
}

func TestWellfoundHealthCheck_BlockedDetection(t *testing.T) {
	blocked := loadWellfoundFixture(t, "wellfound_blocked.html")
	if !retrieval.IsChallenged(blocked, 0) {
		t.Fatal("expected blocked fixture to be detected as blocked")
	}
}

func TestWellfoundFetchDetail(t *testing.T) {
	detail := loadWellfoundFixture(t, "wellfound_detail.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(detail))
	}))
	defer srv.Close()

	a := WellfoundAdapter{Scraping: scraping.New()}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/jobs/wf-1001-senior-golang-engineer", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !patch.Available {
		t.Fatal("expected patch.Available = true")
	}
	if len(patch.Description) < 100 {
		t.Errorf("expected full description (folding in qualifications) longer than a list snippet, got %d chars", len(patch.Description))
	}
	if !strings.Contains(patch.Description, "Qualifications") {
		t.Errorf("expected qualifications section to be folded into description")
	}
	if patch.PostedAt == nil {
		t.Errorf("expected postedAt to be resolved from fixture")
	}
	if patch.SalaryRaw == nil || *patch.SalaryRaw == "" {
		t.Errorf("expected salary/equity to be captured from detail page")
	}
}

func TestWellfoundFetchDetail_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>This listing is no longer available</body></html>"))
	}))
	defer srv.Close()

	a := WellfoundAdapter{Scraping: scraping.New()}
	patch, err := a.FetchDetail(context.Background(), srv.URL+"/jobs/gone", nil)
	if err != nil {
		t.Fatalf("expected nil error for a gone/unavailable listing (FR-009 edge case), got %v", err)
	}
	if patch.Available {
		t.Fatal("expected patch.Available = false for a gone/unavailable listing")
	}
}

func TestWellfoundFetchDetail_Blocked(t *testing.T) {
	blocked := loadWellfoundFixture(t, "wellfound_blocked.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(blocked))
	}))
	defer srv.Close()

	a := WellfoundAdapter{Scraping: scraping.New()}
	_, err := a.FetchDetail(context.Background(), srv.URL+"/jobs/x", nil)
	if err == nil {
		t.Fatal("expected a distinguishable error when the detail response is a bot-challenge/rate-limit page")
	}
}
