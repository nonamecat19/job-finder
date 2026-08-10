package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
	"github.com/job-finder/api/internal/platform/scraping"
)

func TestDjinniKey(t *testing.T) {
	adapter := DjinniAdapter{}
	if adapter.Key() != "djinni" {
		t.Errorf("expected key 'djinni', got %q", adapter.Key())
	}
}

func TestDjinniKind(t *testing.T) {
	adapter := DjinniAdapter{}
	if adapter.Kind() != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, adapter.Kind())
	}
}

func makeBasicSearchFixture(cardsHTML string) string {
	return `<html><body><ul class="list-jobs">` + cardsHTML + `</ul></body></html>`
}

func basicSearchCard(href, title string) string {
	return `<li class="list-jobs__item"><a href="` + href + `">` + title + `</a></li>`
}

func TestDjinniSearchBasicSearchSinglePage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			_, _ = w.Write([]byte(makeBasicSearchFixture(
				basicSearchCard("/jobs/1", "Go Dev") + basicSearchCard("/jobs/2", "Rust Dev"),
			)))
			return
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture("")))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 cards, got %d", len(jobs))
	}
	if requests != 2 {
		t.Errorf("expected 2 fetches (page 1 + page 2), got %d", requests)
	}
}

func TestDjinniSearchBasicSearchMultiPage(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			_, _ = w.Write([]byte(makeBasicSearchFixture(
				basicSearchCard("/jobs/1", "Go Dev") + basicSearchCard("/jobs/2", "Rust Dev"),
			)))
		case "2":
			_, _ = w.Write([]byte(makeBasicSearchFixture(
				basicSearchCard("/jobs/3", "Python Dev") + basicSearchCard("/jobs/4", "JS Dev"),
			)))
		default:
			_, _ = w.Write([]byte(makeBasicSearchFixture("")))
		}
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 4 {
		t.Errorf("expected 4 cards (2 pages), got %d", len(jobs))
	}
	if requests != 3 {
		t.Errorf("expected 3 fetches (pages 1-3), got %d", requests)
	}
}

func TestDjinniSearchBasicSearchRedirectLoop(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev") + basicSearchCard("/jobs/2", "Rust Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 cards (only page 1, loop guard), got %d", len(jobs))
	}
	if requests != 2 {
		t.Errorf("expected 2 fetches (stopped after page 2 redirect-loop), got %d", requests)
	}
}

func TestDjinniSearchBasicSearchPreservesQueryParams(t *testing.T) {
	var reqURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqURL == "" {
			reqURL = r.URL.String()
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	subURL := srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote"
	query := dto.SearchQuery{SubscriptionURL: subURL}
	_, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	parsed, err := url.Parse("http://" + reqURL)
	if err != nil {
		t.Fatalf("could not parse request URL %q: %v", reqURL, err)
	}
	q := parsed.Query()
	if q.Get("search_type") != "basic-search" {
		t.Error("search_type missing or wrong")
	}
	if q.Get("primary_keyword") != "Node.js" {
		t.Error("primary_keyword mismatch")
	}
	if q.Get("salary") != "3000" {
		t.Error("salary mismatch")
	}
	if q.Get("employment") != "remote" {
		t.Error("employment mismatch")
	}
	levels := q["exp_level"]
	if len(levels) != 4 {
		t.Fatalf("expected 4 exp_level values, got %d: %v", len(levels), levels)
	}
	if levels[0] != "2y" || levels[1] != "3y" || levels[2] != "4y" || levels[3] != "5y" {
		t.Errorf("exp_level values mismatch: %v", levels)
	}
}

func TestDjinniSearchBasicSearchStripsPageParamFromSavedURL(t *testing.T) {
	var firstReqPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if firstReqPage == "" {
			firstReqPage = r.URL.Query().Get("page")
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New()}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang&page=4"}
	_, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if firstReqPage != "1" {
		t.Errorf("expected first fetch to use page=1, got page=%s", firstReqPage)
	}
}

// --- 041 PostingReader ---

func TestDjinniMatchesPostingURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"posting path accepted", "https://djinni.co/jobs/123456-senior-go-engineer/", true},
		{"posting path on www accepted", "https://www.djinni.co/jobs/123456-senior-go-engineer/", true},
		{"posting path with tracking query accepted", "https://djinni.co/jobs/123456-senior-go-engineer/?from=telegram", true},
		{"http scheme accepted", "http://djinni.co/jobs/1-a/", true},
		{"preset search rejected", "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang", false},
		{"bare jobs listing rejected", "https://djinni.co/jobs/", false},
		{"bare jobs listing without slash rejected", "https://djinni.co/jobs", false},
		{"numeric-only path rejected", "https://djinni.co/jobs/123456", false},
		{"company page rejected", "https://djinni.co/company/novatech/jobs/", false},
		{"dashboard rejected", "https://djinni.co/my/dashboard/subs/42/", false},
		{"other host rejected", "https://jobs.dou.ua/vacancies/123456-go/", false},
		{"lookalike host rejected", "https://djinni.co.evil.example/jobs/1-a/", false},
		{"non-http scheme rejected", "javascript:alert(1)", false},
		{"empty rejected", "", false},
		{"garbage rejected", "://///not a url", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (DjinniAdapter{}).MatchesPostingURL(tt.url); got != tt.want {
				t.Errorf("MatchesPostingURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDjinniReadPostingExtractsEveryField(t *testing.T) {
	fixture := loadFixture(t, "djinni_posting.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fixture))
	}))
	defer srv.Close()

	d := DjinniAdapter{Scraping: scraping.New()}
	job, err := d.ReadPosting(context.Background(), srv.URL+"/jobs/123456-senior-go-engineer/?from=telegram", nil)
	if err != nil {
		t.Fatalf("ReadPosting: %v", err)
	}

	if job.SourceKey != "djinni" {
		t.Errorf("sourceKey = %q, want djinni", job.SourceKey)
	}
	if job.Title != "Senior Go Engineer" {
		t.Errorf("title = %q", job.Title)
	}
	if job.Company != "NovaTech" {
		t.Errorf("company = %q", job.Company)
	}
	if !strings.Contains(job.Description, "ledger service") {
		t.Errorf("description missing the posting body, got %q", job.Description)
	}
	if job.Location == nil || *job.Location != "Kyiv, Lviv" {
		t.Errorf("location = %v", job.Location)
	}
	if !job.Remote {
		t.Error("expected remote to be detected from the page text")
	}
	if job.SalaryRaw == nil || *job.SalaryRaw != "$5000-7000" {
		t.Errorf("salaryRaw = %v", job.SalaryRaw)
	}
	if job.PostedAt == nil || *job.PostedAt != "2026-07-28T09:15:00+03:00" {
		t.Errorf("postedAt = %v — it must come from the page, never from the add time", job.PostedAt)
	}
	if strings.Contains(job.URL, "?") {
		t.Errorf("url = %q, want the tracking query dropped so the dedupe key matches a crawl", job.URL)
	}
}

func TestDjinniReadPostingReturnsPartialsRatherThanErroring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><h1>Go Engineer</h1></body></html>`))
	}))
	defer srv.Close()

	d := DjinniAdapter{Scraping: scraping.New()}
	job, err := d.ReadPosting(context.Background(), srv.URL+"/jobs/1-go-engineer/", nil)
	if err != nil {
		t.Fatalf("expected a partial read, not an error: %v", err)
	}
	if job.Title != "Go Engineer" {
		t.Errorf("title = %q", job.Title)
	}
	if job.Company != "" || job.Description != "" {
		t.Errorf("expected the absent fields to come back empty, got company=%q description=%q", job.Company, job.Description)
	}
}

func TestDjinniReadPostingReportsLoginWall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<html><body><form><input name="password" type="password"></form></body></html>`))
	}))
	defer srv.Close()

	d := DjinniAdapter{Scraping: scraping.New()}
	if _, err := d.ReadPosting(context.Background(), srv.URL+"/jobs/1-go/", nil); err == nil {
		t.Fatal("expected a login wall to be reported as an error, not read as an empty posting")
	}
}

func TestDjinniAdapterSatisfiesPostingReader(t *testing.T) {
	if _, ok := domain.AsPostingReader(DjinniAdapter{}); !ok {
		t.Fatal("expected DjinniAdapter to implement domain.PostingReader")
	}
}

func TestManualAdapterHasNoPostingReader(t *testing.T) {
	if _, ok := domain.AsPostingReader(ManualAdapter{}); ok {
		t.Fatal("the manual source reads nothing — it must not claim any host")
	}
}

func TestManualAdapterSearchFailsLoudly(t *testing.T) {
	jobs, err := (ManualAdapter{}).Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected the manual source to refuse being crawled rather than return an empty result")
	}
	if jobs != nil {
		t.Errorf("expected no jobs, got %d", len(jobs))
	}
	healthy, err := (ManualAdapter{}).HealthCheck(context.Background(), nil)
	if err != nil || !healthy {
		t.Errorf("expected the manual source to report healthy, got healthy=%v err=%v", healthy, err)
	}
}
