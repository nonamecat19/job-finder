package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
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
