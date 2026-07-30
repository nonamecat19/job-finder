package adapters

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/scraping"
)

func mustReadTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return string(b)
}

func TestJobLeadsKey(t *testing.T) {
	adapter := JobLeadsAdapter{}
	if adapter.Key() != "jobleads" {
		t.Errorf("expected key 'jobleads', got %q", adapter.Key())
	}
}

func TestJobLeadsKind(t *testing.T) {
	adapter := JobLeadsAdapter{}
	if adapter.Kind() != dto.SourceKindScrape {
		t.Errorf("expected kind %q, got %q", dto.SourceKindScrape, adapter.Kind())
	}
}

func TestJobLeadsIsLoginPage(t *testing.T) {
	login, _ := goquery.NewDocumentFromReader(strings.NewReader(mustReadTestdata(t, "jobleads_login.html")))
	if !jobLeadsIsLoginPage(login) {
		t.Error("expected login page to be detected")
	}

	listing, _ := goquery.NewDocumentFromReader(strings.NewReader(mustReadTestdata(t, "jobleads_list.html")))
	if jobLeadsIsLoginPage(listing) {
		t.Error("job listing must not be detected as a login page")
	}
}

func TestParseJobLeadsListings(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(mustReadTestdata(t, "jobleads_list.html")))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	jobs := parseJobLeadsListings(doc, "https://www.jobleads.com/job-search?q=golang")
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	first := jobs[0]
	if first.SourceKey != "jobleads" {
		t.Errorf("expected sourceKey 'jobleads', got %q", first.SourceKey)
	}
	if first.Title == "" || first.URL == "" || first.Company == "" {
		t.Errorf("expected non-empty title/url/company, got %+v", first)
	}
	if first.ExternalID == nil || *first.ExternalID != "senior-golang-engineer-novatech-abc123" {
		t.Errorf("expected externalID from URL path, got %v", first.ExternalID)
	}
	if first.SalaryRaw == nil || *first.SalaryRaw == "" {
		t.Errorf("expected first job to have a salary, got %v", first.SalaryRaw)
	}
	if !first.Remote {
		t.Error("expected first job to be detected as remote")
	}

	second := jobs[1]
	if second.Location != nil {
		t.Errorf("expected nil location for empty location card, got %v", second.Location)
	}
	if second.SalaryRaw != nil {
		t.Errorf("expected nil salary for card with no salary, got %v", second.SalaryRaw)
	}
}

func TestParseJobLeadsListings_Empty(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(mustReadTestdata(t, "jobleads_empty.html")))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	jobs := parseJobLeadsListings(doc, "https://www.jobleads.com/job-search?q=golang")
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestJobLeadsSearch_NoSubscriptionURL(t *testing.T) {
	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: "cookie-abc"}}
	_, err := adapter.Search(context.Background(), dto.SearchQuery{}, nil)
	if err == nil {
		t.Fatal("expected error when SubscriptionURL is empty")
	}
}

func TestJobLeadsSearch_NoCredentials(t *testing.T) {
	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: ""}}
	_, err := adapter.Search(context.Background(), dto.SearchQuery{SubscriptionURL: "https://www.jobleads.com/job-search?q=golang"}, nil)
	if err == nil {
		t.Fatal("expected error when no credentials are configured")
	}
}

// stubJobLeadsSession is a fixed-response JobLeadsSessionProvider for tests
// that don't need to exercise the real login flow.
type stubJobLeadsSession struct {
	cookie      string
	refreshes   int
	refreshFail bool
}

func (s *stubJobLeadsSession) Ensure(_ context.Context) (string, error) {
	return s.cookie, nil
}

func (s *stubJobLeadsSession) Refresh(_ context.Context) (string, error) {
	s.refreshes++
	if s.refreshFail {
		return "", errors.New("refresh failed")
	}
	return s.cookie, nil
}

func TestJobLeadsSearch_AuthenticatedFetch(t *testing.T) {
	listHTML := mustReadTestdata(t, "jobleads_list.html")
	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		_, _ = w.Write([]byte(listHTML))
	}))
	defer srv.Close()

	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: "cookie-xyz"}}
	jobs, err := adapter.Search(context.Background(), dto.SearchQuery{SubscriptionURL: srv.URL + "/job-search?q=golang"}, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if gotCookie != "session=cookie-xyz" {
		t.Errorf("expected session cookie header, got %q", gotCookie)
	}
}

func TestJobLeadsHealthCheck(t *testing.T) {
	// HealthCheck targets the fixed jobleads.com host; reachability in a
	// unit-test sandbox isn't guaranteed, so this only asserts the
	// never-a-non-nil-error contract, matching TestIndeedHealthCheck's/
	// TestGlassdoorHealthCheck's approach for an external dependency. The
	// authenticated-fetch code path itself is exercised via
	// TestJobLeadsSearch_AuthenticatedFetch against an httptest.Server.
	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: "cookie-xyz"}}
	if _, err := adapter.HealthCheck(context.Background(), nil); err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}

	noCredsAdapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: ""}}
	healthy, err := noCredsAdapter.HealthCheck(context.Background(), nil)
	if err != nil {
		t.Fatalf("HealthCheck must never return a non-nil error, got %v", err)
	}
	if healthy {
		t.Error("expected unhealthy when no credentials are configured")
	}
}

func TestValidateJobLeadsSubscriptionURL_HostCheck(t *testing.T) {
	// Adapter-level sanity check that a non-JobLeads host is not silently
	// accepted by scrapeSubscription (URL parsing itself doesn't validate
	// host — that's subscriptions.Service's job, covered in
	// subscriptions/service_test.go; this just documents the parse-only
	// behavior so a regression there is caught early too).
	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: "cookie-xyz"}}
	_, err := adapter.scrapeSubscription(context.Background(), "://bad-url", map[string]string{})
	if err == nil {
		t.Fatal("expected error for an unparseable subscription URL")
	}
}

func TestJobLeadsFetchDetail(t *testing.T) {
	detailHTML := mustReadTestdata(t, "jobleads_detail.html")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: "cookie-xyz"}}
	patch, err := adapter.FetchDetail(context.Background(), srv.URL+"/job/senior-golang-engineer-novatech-abc123", nil)
	if err != nil {
		t.Fatalf("FetchDetail failed: %v", err)
	}
	if !patch.Available {
		t.Fatal("expected Available: true")
	}
	if patch.Description == "" {
		t.Error("expected non-empty description")
	}
	if patch.PostedAt == nil {
		t.Error("expected non-nil postedAt")
	}
}

func TestJobLeadsFetchDetail_Unavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><p>This job has been removed and is no longer available.</p></body></html>`))
	}))
	defer srv.Close()

	adapter := JobLeadsAdapter{Scraping: scraping.New(nil), Session: &stubJobLeadsSession{cookie: "cookie-xyz"}}
	patch, err := adapter.FetchDetail(context.Background(), srv.URL+"/job/gone", nil)
	if err != nil {
		t.Fatalf("expected nil error for an unavailable listing, got %v", err)
	}
	if patch.Available {
		t.Fatal("expected Available: false for a removed listing")
	}
}

// jobLeadsLoginServer mocks JobLeads's form login: GET /login serves the
// CSRF form; POST /login sets a session cookie when credentials are
// present. Mirrors djinniLoginServer.
func jobLeadsLoginServer(t *testing.T, wantEmail, wantPassword string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(mustReadTestdata(t, "jobleads_login.html")))
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("email") != wantEmail || r.Form.Get("password") != wantPassword {
			http.Error(w, "invalid", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "sess-abc", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
}

func TestJobLeadsLogin(t *testing.T) {
	srv := jobLeadsLoginServer(t, "me@example.com", "s3cret")
	defer srv.Close()

	cookie, err := jobLeadsLogin(context.Background(), srv.URL, "me@example.com", "s3cret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if cookie != "sess-abc" {
		t.Errorf("expected session 'sess-abc', got %q", cookie)
	}
}

func TestJobLeadsLoginMissingCreds(t *testing.T) {
	_, err := jobLeadsLogin(context.Background(), "https://www.jobleads.com", "", "")
	if err == nil {
		t.Fatal("expected error when credentials are empty")
	}
}

func TestJobLeadsLoginBadCreds(t *testing.T) {
	srv := jobLeadsLoginServer(t, "me@example.com", "s3cret")
	defer srv.Close()

	_, err := jobLeadsLogin(context.Background(), srv.URL, "me@example.com", "wrong")
	if err == nil {
		t.Fatal("expected error when login returns no session cookie")
	}
}

// jobLeadsFakeConfigStore is an in-memory JobLeadsConfigStore for session tests.
type jobLeadsFakeConfigStore struct {
	cfg     map[string]any
	updates int
}

func (f *jobLeadsFakeConfigStore) Config(_ context.Context, _ string) (map[string]any, error) {
	return f.cfg, nil
}

func (f *jobLeadsFakeConfigStore) Update(_ context.Context, _ string, _ *bool, patch map[string]any) (*dto.JobSourceDto, error) {
	f.updates++
	for k, v := range patch {
		f.cfg[k] = v
	}
	return &dto.JobSourceDto{Key: "jobleads"}, nil
}

func TestJobLeadsSessionEnsureUsesStoredCookie(t *testing.T) {
	store := &jobLeadsFakeConfigStore{cfg: map[string]any{"sessionCookie": "stored-xyz"}}
	sess := &JobLeadsSession{Sources: store, Email: "me@example.com", Password: "s3cret", Key: "jobleads"}

	cookie, err := sess.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if cookie != "stored-xyz" {
		t.Errorf("expected stored cookie, got %q", cookie)
	}
	if store.updates != 0 {
		t.Errorf("expected no login when a cookie is stored, got %d updates", store.updates)
	}
}

func TestJobLeadsSessionEnsureLogsInWhenEmpty(t *testing.T) {
	srv := jobLeadsLoginServer(t, "me@example.com", "s3cret")
	defer srv.Close()

	store := &jobLeadsFakeConfigStore{cfg: map[string]any{}}
	sess := &JobLeadsSession{Sources: store, Email: "me@example.com", Password: "s3cret", Key: "jobleads", Base: srv.URL}

	cookie, err := sess.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if cookie != "sess-abc" {
		t.Errorf("expected freshly logged-in cookie, got %q", cookie)
	}
	if store.updates != 1 {
		t.Errorf("expected cookie persisted once, got %d updates", store.updates)
	}
}

func TestJobLeadsSessionEnsureNoCredentialsDegradesToEmpty(t *testing.T) {
	store := &jobLeadsFakeConfigStore{cfg: map[string]any{}}
	sess := &JobLeadsSession{Sources: store, Key: "jobleads"}

	cookie, err := sess.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure failed: %v", err)
	}
	if cookie != "" {
		t.Errorf("expected empty cookie with no credentials configured, got %q", cookie)
	}
}
