package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/platform/scraping"
)

func loadHimalayasFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func TestHimalayasKey(t *testing.T) {
	var a HimalayasAdapter
	if got := a.Key(); got != "himalayas" {
		t.Errorf("expected key 'himalayas', got %q", got)
	}
}

func TestHimalayasKind(t *testing.T) {
	var a HimalayasAdapter
	if got := a.Kind(); got != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, got)
	}
}

// TestHimalayasSearch_MapsAndFiltersFixture covers T007: Search against a
// fixture-backed httptest.Server returns normalized jobs with correct field
// mapping, and keeps only jobs matching the subscription's category slug.
func TestHimalayasSearch_MapsAndFiltersFixture(t *testing.T) {
	body := loadHimalayasFixture(t, "himalayas_list.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://himalayas.app/jobs?categories=Backend-Engineering"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 matching job (category filter), got %d", len(jobs))
	}

	job := jobs[0]
	if job.SourceKey != "himalayas" {
		t.Errorf("sourceKey: got %q", job.SourceKey)
	}
	if !job.Remote {
		t.Errorf("expected Remote=true for every himalayas job")
	}
	if job.Title != "Senior Backend Engineer" {
		t.Errorf("title: got %q", job.Title)
	}
	if job.Company != "Acme Corp" {
		t.Errorf("company: got %q", job.Company)
	}
	wantGUID := "https://himalayas.app/companies/acme-corp/jobs/senior-backend-engineer"
	if job.ExternalID == nil || *job.ExternalID != wantGUID {
		t.Errorf("externalID: got %v, want %v", job.ExternalID, wantGUID)
	}
	if job.URL != wantGUID {
		t.Errorf("url: got %q, want %q", job.URL, wantGUID)
	}
	if job.SalaryRaw == nil || *job.SalaryRaw == "" {
		t.Errorf("expected salary to be captured for the matched job")
	}
	if job.Location == nil || *job.Location == "" {
		t.Errorf("expected non-nil location for the matched job")
	}
	if job.PostedAt == nil || *job.PostedAt == "" {
		t.Errorf("expected postedAt to be resolved")
	}
}

// TestHimalayasSearch_ZeroResultsVsUnparseable covers T008: zero-results is
// not an error (FR-011), while unparseable JSON returns a distinguishable
// error.
func TestHimalayasSearch_ZeroResultsVsUnparseable(t *testing.T) {
	t.Run("zero results", func(t *testing.T) {
		body := loadHimalayasFixture(t, "himalayas_empty.json")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()

		a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
		jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://himalayas.app/jobs?categories=Backend-Engineering"}, nil)
		if err != nil {
			t.Fatalf("expected nil error for zero results, got %v", err)
		}
		if len(jobs) != 0 {
			t.Fatalf("expected 0 jobs, got %d", len(jobs))
		}
	})

	t.Run("unparseable response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()

		a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
		_, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://himalayas.app/jobs?categories=Backend-Engineering"}, nil)
		if err == nil {
			t.Fatal("expected a non-nil error for an unparseable response")
		}
	})
}

// TestHimalayasSearch_NoSubscriptionURL covers T009: keyword search is out
// of scope, mirroring RemoteOKAdapter.Search's stance.
func TestHimalayasSearch_NoSubscriptionURL(t *testing.T) {
	a := HimalayasAdapter{Scraping: scraping.New(nil)}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty (keyword search out of scope)")
	}
}

// TestHimalayasSearch_MissingCategoriesParam covers the invalid-subscription
// precondition: a URL missing "categories" returns a distinguishable error.
func TestHimalayasSearch_MissingCategoriesParam(t *testing.T) {
	a := HimalayasAdapter{Scraping: scraping.New(nil)}
	_, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://himalayas.app/jobs"}, nil)
	if err == nil {
		t.Fatal("expected error when subscription url has no categories parameter")
	}
}

// TestHimalayasSearch_Pagination covers T010: Search pages through multiple
// responses up to himalayasMaxSubscriptionPages, stops early once a page's
// offset >= totalCount, and paces requests at himalayasRequestDelay.
func TestHimalayasSearch_Pagination(t *testing.T) {
	var requests []time.Time
	var offsets []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, time.Now())
		offsets = append(offsets, r.URL.Query().Get("offset"))

		offset := r.URL.Query().Get("offset")
		var res himalayasResponse
		res.Limit = himalayasPageLimit
		res.TotalCount = 45 // spans 3 pages of 20, last page partial

		switch offset {
		case "0":
			res.Offset = 0
			res.Jobs = []himalayasJob{
				{GUID: "job-1", Title: "Job 1", CompanyName: "Co", Categories: []string{"Backend-Engineering"}},
			}
		case "20":
			res.Offset = 20
			res.Jobs = []himalayasJob{
				{GUID: "job-2", Title: "Job 2", CompanyName: "Co", Categories: []string{"Backend-Engineering"}},
			}
		default:
			res.Offset = 40
			res.Jobs = []himalayasJob{
				{GUID: "job-3", Title: "Job 3", CompanyName: "Co", Categories: []string{"Backend-Engineering"}},
			}
		}

		body, _ := json.Marshal(res)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://himalayas.app/jobs?categories=Backend-Engineering"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs across 3 pages, got %d", len(jobs))
	}
	if len(requests) != 3 {
		t.Fatalf("expected exactly 3 requests (stopping once offset >= totalCount), got %d", len(requests))
	}
	if offsets[0] != "0" || offsets[1] != "20" || offsets[2] != "40" {
		t.Fatalf("expected offsets 0,20,40 in order, got %v", offsets)
	}
	for i := 1; i < len(requests); i++ {
		if gap := requests[i].Sub(requests[i-1]); gap < himalayasRequestDelay {
			t.Errorf("expected at least %v between requests, got %v", himalayasRequestDelay, gap)
		}
	}
}

// TestHimalayasHealthCheck covers T013: (true, nil) for a decodable
// response, (false, nil) — never a non-nil error — for unreachable/
// unparseable responses.
func TestHimalayasHealthCheck(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		body := loadHimalayasFixture(t, "himalayas_list.json")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()

		a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
		ok, err := a.HealthCheck(context.Background(), nil)
		if err != nil {
			t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
		}
		if !ok {
			t.Fatal("expected healthy=true for a decodable response")
		}
	})

	t.Run("empty page is still healthy", func(t *testing.T) {
		body := loadHimalayasFixture(t, "himalayas_empty.json")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		defer srv.Close()

		a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
		ok, err := a.HealthCheck(context.Background(), nil)
		if err != nil {
			t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
		}
		if !ok {
			t.Fatal("expected healthy=true for an empty-but-decodable page")
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: "http://127.0.0.1:1"}
		ok, err := a.HealthCheck(context.Background(), nil)
		if err != nil {
			t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
		}
		if ok {
			t.Fatal("expected healthy=false for an unreachable host")
		}
	})

	t.Run("unparseable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not json"))
		}))
		defer srv.Close()

		a := HimalayasAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
		ok, err := a.HealthCheck(context.Background(), nil)
		if err != nil {
			t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
		}
		if ok {
			t.Fatal("expected healthy=false for an unparseable response")
		}
	})
}

// TestHimalayasJobFromRaw_FullDescriptionAndTimezoneText covers T018: the
// normalized description is the fixture's full text (not a truncated
// teaser), and Raw carries the folded-in timezone-restriction text for a job
// with non-empty timezoneRestrictions.
func TestHimalayasJobFromRaw_FullDescriptionAndTimezoneText(t *testing.T) {
	raw := himalayasJob{
		GUID:                 "https://himalayas.app/companies/acme/jobs/senior-backend-engineer",
		Title:                "Senior Backend Engineer",
		CompanyName:          "Acme Corp",
		Description:          "<h3>Overview</h3><p>Full description text, including responsibilities and qualifications.</p>",
		TimezoneRestrictions: []int{-5, -8},
		PubDate:              1784964128,
	}

	job := himalayasJobFromRaw(raw)

	wantText := "Overview\nFull description text, including responsibilities and qualifications."
	if job.Description == "" {
		t.Fatal("expected non-empty description")
	}
	if job.Description != wantText {
		t.Errorf("description: got %q, want %q (HTMLToText-converted, not truncated)", job.Description, wantText)
	}

	rawMap, ok := job.Raw.(map[string]any)
	if !ok {
		t.Fatalf("expected Raw to be a map[string]any, got %T", job.Raw)
	}
	tzText, _ := rawMap["timezoneRestrictions"].(string)
	if tzText == "" || tzText == "no timezone restriction" {
		t.Errorf("expected Raw[timezoneRestrictions] to reflect the non-empty restriction, got %q", tzText)
	}
}

// TestHimalayasJobFromRaw_PostedAtConversion covers T019: pubDate (Unix
// seconds) is converted to an RFC3339 UTC string.
func TestHimalayasJobFromRaw_PostedAtConversion(t *testing.T) {
	raw := himalayasJob{
		GUID:    "https://himalayas.app/companies/acme/jobs/senior-backend-engineer",
		Title:   "Senior Backend Engineer",
		PubDate: 1784964128,
	}

	job := himalayasJobFromRaw(raw)

	if job.PostedAt == nil {
		t.Fatal("expected non-nil PostedAt")
	}
	want := time.Unix(1784964128, 0).UTC().Format(time.RFC3339)
	if *job.PostedAt != want {
		t.Errorf("postedAt: got %q, want %q", *job.PostedAt, want)
	}
	if _, err := time.Parse(time.RFC3339, *job.PostedAt); err != nil {
		t.Errorf("postedAt is not valid RFC3339: %v", err)
	}
}

func TestHimalayasJobFromRaw_NoSalaryOrLocation(t *testing.T) {
	raw := himalayasJob{
		GUID:        "https://himalayas.app/companies/beta/jobs/product-designer",
		Title:       "Product Designer",
		CompanyName: "Beta Inc",
	}

	job := himalayasJobFromRaw(raw)

	if job.SalaryRaw != nil {
		t.Errorf("expected nil SalaryRaw when minSalary/maxSalary are absent, got %v", *job.SalaryRaw)
	}
	if job.Location != nil {
		t.Errorf("expected nil Location for empty locationRestrictions, got %v", *job.Location)
	}
}
