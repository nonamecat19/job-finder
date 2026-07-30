package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/scraping"
)

func loadRemoteOKFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

func TestRemoteOKKey(t *testing.T) {
	var a RemoteOKAdapter
	if got := a.Key(); got != "remoteok" {
		t.Errorf("expected key 'remoteok', got %q", got)
	}
}

func TestRemoteOKKind(t *testing.T) {
	var a RemoteOKAdapter
	if got := a.Kind(); got != dto.SourceKindAPI {
		t.Errorf("expected kind %q, got %q", dto.SourceKindAPI, got)
	}
}

func TestParseRemoteOKJobs(t *testing.T) {
	body := loadRemoteOKFixture(t, "remoteok_api.json")
	jobs, err := parseRemoteOKJobs([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs (legal notice skipped), got %d", len(jobs))
	}

	first := jobs[0]
	if first.SourceKey != "remoteok" {
		t.Errorf("sourceKey: got %q", first.SourceKey)
	}
	if !first.Remote {
		t.Errorf("expected Remote=true for every remoteok job")
	}
	if first.Title != "Senior Golang Developer" {
		t.Errorf("title: got %q", first.Title)
	}
	if first.ExternalID == nil || *first.ExternalID != "1000001" {
		t.Errorf("externalID: got %v, want 1000001", first.ExternalID)
	}
	if first.SalaryRaw == nil || *first.SalaryRaw == "" {
		t.Errorf("expected salary to be captured for first job")
	}
	if first.Location != nil {
		t.Errorf("expected nil Location for empty-location job, got %v", first.Location)
	}

	second := jobs[1]
	if second.SalaryRaw != nil {
		t.Errorf("expected nil SalaryRaw for job without salary_min/salary_max, got %v", *second.SalaryRaw)
	}
}

func TestParseRemoteOKJobs_Empty(t *testing.T) {
	body := loadRemoteOKFixture(t, "remoteok_empty.json")
	jobs, err := parseRemoteOKJobs([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs (only legal notice present), got %d", len(jobs))
	}
}

func TestRemoteOKSearch_NoSubscriptionURL(t *testing.T) {
	a := RemoteOKAdapter{Scraping: scraping.New(nil)}
	_, err := a.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty (keyword search out of scope)")
	}
}

func TestRemoteOKSearch_TagResolution(t *testing.T) {
	body := loadRemoteOKFixture(t, "remoteok_api.json")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	a := RemoteOKAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
	jobs, err := a.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://remoteok.com/remote-golang-jobs"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if hits != 1 {
		t.Fatalf("expected exactly 1 request (no pagination), got %d", hits)
	}
}

func TestRemoteOKTagFromURL(t *testing.T) {
	cases := map[string]string{
		"https://remoteok.com/remote-golang-jobs": "golang",
		"https://remoteok.com/api":                "",
		"https://remoteok.com/":                   "",
	}
	for input, want := range cases {
		if got := remoteokTagFromURL(input); got != want {
			t.Errorf("remoteokTagFromURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRemoteOKHealthCheck(t *testing.T) {
	body := loadRemoteOKFixture(t, "remoteok_api.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	// HealthCheck targets the fixed remoteok.com API host; this test only
	// exercises the "never a non-nil error" contract using the real host,
	// matching TestIndeedHealthCheck's approach for an external dependency.
	a := RemoteOKAdapter{Scraping: scraping.New(nil)}
	if _, err := a.HealthCheck(context.Background(), nil); err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}
}

func TestRemoteOKFetchDetail(t *testing.T) {
	body := loadRemoteOKFixture(t, "remoteok_api.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	a := RemoteOKAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
	patch, err := a.FetchDetail(context.Background(), "https://remoteok.com/remote-jobs/1000001-senior-golang-developer-novatech-llc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !patch.Available {
		t.Fatal("expected Available=true for a listing present in the fixture")
	}
	if patch.Description == "" {
		t.Errorf("expected non-empty description")
	}
	if patch.PostedAt == nil || *patch.PostedAt == "" {
		t.Errorf("expected postedAt to be resolved from fixture")
	}
}

func TestRemoteOKFetchDetail_RotatedOut(t *testing.T) {
	body := loadRemoteOKFixture(t, "remoteok_api.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	a := RemoteOKAdapter{Scraping: scraping.New(nil), APIURL: srv.URL}
	patch, err := a.FetchDetail(context.Background(), "https://remoteok.com/remote-jobs/9999999-gone", nil)
	if err != nil {
		t.Fatalf("expected nil error for a rotated-out listing, got %v", err)
	}
	if patch.Available {
		t.Fatal("expected Available=false for a listing no longer in the feed")
	}
}
