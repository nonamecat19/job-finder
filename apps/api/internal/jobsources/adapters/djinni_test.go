package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/scraping"
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

func TestDjinniIsLoginPage(t *testing.T) {
	login, _ := goquery.NewDocumentFromReader(strings.NewReader(
		`<html><body><form action="/login"><input name="email"><input name="password" type="password"></form></body></html>`))
	if !djinniIsLoginPage(login) {
		t.Error("expected login page to be detected")
	}

	listing, _ := goquery.NewDocumentFromReader(strings.NewReader(
		`<html><body><li id="job-item-1"><a href="/jobs/123">Go Dev</a></li></body></html>`))
	if djinniIsLoginPage(listing) {
		t.Error("job listing must not be detected as a login page")
	}
}

// djinniLoginServer mocks djinni's Django login: GET /login serves the CSRF
// form; POST /login sets a sessionid cookie when credentials are present.
func djinniLoginServer(t *testing.T, wantEmail, wantPassword string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "csrf-cookie", Path: "/"})
			_, _ = w.Write([]byte(`<html><body><form method="post"><input type="hidden" name="csrfmiddlewaretoken" value="tok-123"></form></body></html>`))
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("email") != wantEmail || r.Form.Get("password") != wantPassword || r.Form.Get("csrfmiddlewaretoken") != "tok-123" {
			http.Error(w, "invalid", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "sessionid", Value: "sess-abc", Path: "/"})
		w.WriteHeader(http.StatusOK)
	}))
}

func TestDjinniLogin(t *testing.T) {
	srv := djinniLoginServer(t, "me@example.com", "s3cret")
	defer srv.Close()

	cookie, err := djinniLogin(context.Background(), srv.URL, "me@example.com", "s3cret")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if cookie != "sess-abc" {
		t.Errorf("expected sessionid 'sess-abc', got %q", cookie)
	}
}

func TestDjinniLoginMissingCreds(t *testing.T) {
	_, err := djinniLogin(context.Background(), "https://djinni.co", "", "")
	if err == nil {
		t.Fatal("expected error when credentials are empty")
	}
}

func TestDjinniLoginBadCreds(t *testing.T) {
	srv := djinniLoginServer(t, "me@example.com", "s3cret")
	defer srv.Close()

	_, err := djinniLogin(context.Background(), srv.URL, "me@example.com", "wrong")
	if err == nil {
		t.Fatal("expected error when login returns no sessionid")
	}
}

// fakeConfigStore is an in-memory DjinniConfigStore for session tests.
type fakeConfigStore struct {
	cfg     map[string]any
	updates int
}

func (f *fakeConfigStore) Config(_ context.Context, _ string) (map[string]any, error) {
	return f.cfg, nil
}

func (f *fakeConfigStore) Update(_ context.Context, _ string, _ *bool, patch map[string]any) (*dto.JobSourceDto, error) {
	f.updates++
	for k, v := range patch {
		f.cfg[k] = v
	}
	return &dto.JobSourceDto{Key: "djinni"}, nil
}

func TestDjinniSessionEnsureUsesStoredCookie(t *testing.T) {
	store := &fakeConfigStore{cfg: map[string]any{"sessionCookie": "stored-xyz"}}
	sess := &DjinniSession{Sources: store, Email: "me@example.com", Password: "s3cret", Key: "djinni"}

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

	dd := DjinniAdapter{Scraping: scraping.New(nil)}

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

	dd := DjinniAdapter{Scraping: scraping.New(nil)}

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

	dd := DjinniAdapter{Scraping: scraping.New(nil)}

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

	dd := DjinniAdapter{Scraping: scraping.New(nil)}

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

	dd := DjinniAdapter{Scraping: scraping.New(nil)}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/jobs/?search_type=basic-search&primary_keyword=Golang&page=4"}
	_, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if firstReqPage != "1" {
		t.Errorf("expected first fetch to use page=1, got page=%s", firstReqPage)
	}
}

func TestDjinniSearchDashboardModeUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if !strings.HasPrefix(path, "/my/dashboard/subs/") {
			t.Errorf("dashboard mode should only hit /my/dashboard/subs/ paths, got %s", path)
		}
		_, _ = w.Write([]byte(makeBasicSearchFixture(
			basicSearchCard("/jobs/1", "Go Dev"),
		)))
	}))
	defer srv.Close()

	dd := DjinniAdapter{Scraping: scraping.New(nil)}

	query := dto.SearchQuery{SubscriptionURL: srv.URL + "/my/dashboard/subs/123/"}
	jobs, err := dd.Search(context.Background(), query, nil)
	if err != nil {
		t.Fatalf("dashboard mode Search failed: %v", err)
	}
	if len(jobs) != 1 {
		t.Errorf("expected 1 card from dashboard mode, got %d", len(jobs))
	}
}

func TestDjinniSessionEnsureLogsInWhenEmpty(t *testing.T) {
	srv := djinniLoginServer(t, "me@example.com", "s3cret")
	defer srv.Close()

	store := &fakeConfigStore{cfg: map[string]any{}}
	sess := &DjinniSession{Sources: store, Email: "me@example.com", Password: "s3cret", Key: "djinni", Base: srv.URL}

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
	if store.cfg["sessionCookie"] != "sess-abc" {
		t.Errorf("expected persisted sessionCookie, got %v", store.cfg["sessionCookie"])
	}
}
