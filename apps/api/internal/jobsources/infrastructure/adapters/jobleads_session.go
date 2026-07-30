package adapters

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources"
	"github.com/job-finder/api/internal/retrieval"
)

const jobLeadsBaseURL = "https://www.jobleads.com"

// jobLeadsUserAgent mirrors the shared BrowserIdentity User-Agent; the login
// flow needs its own cookie-jar client, so it can't reuse scraping.Service.
var jobLeadsUserAgent = retrieval.Chrome126UserAgent

// JobLeadsSessionProvider hands the adapter a valid session cookie, logging
// in with stored credentials when none exists or the current one has
// expired. Mirrors DjinniSessionProvider.
type JobLeadsSessionProvider interface {
	// Ensure returns a usable session cookie, logging in if none is stored
	// and credentials are configured. Returns ("", nil) when neither a
	// stored cookie nor credentials are available — callers decide how to
	// surface that as an error.
	Ensure(ctx context.Context) (string, error)
	// Refresh forces a fresh login and persists the new cookie.
	Refresh(ctx context.Context) (string, error)
}

// JobLeadsConfigStore is the slice of jobsources.Service the session manager
// needs: read the decrypted source config (where the cookie lives) and
// persist a config patch. *jobsources.Service satisfies it.
type JobLeadsConfigStore interface {
	Config(ctx context.Context, key string) (map[string]any, error)
	Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
}

// JobLeadsSession owns the JobLeads login lifecycle: credentials come from
// env (Email/Password), the resulting session cookie lives only in the DB
// (source config, encrypted by jobsources.Service). It is shared by pointer
// across the adapter copies held in the registry and enrichment handler, so
// a refresh is visible everywhere. Mirrors DjinniSession.
type JobLeadsSession struct {
	Sources  JobLeadsConfigStore
	Email    string
	Password string
	Key      string // source key, "jobleads"
	Base     string // override base URL for tests; "" -> jobLeadsBaseURL

	mu sync.Mutex // serializes Refresh so concurrent workers don't stampede login
}

func (s *JobLeadsSession) base() string {
	if s.Base != "" {
		return s.Base
	}
	return jobLeadsBaseURL
}

func (s *JobLeadsSession) key() string {
	if s.Key != "" {
		return s.Key
	}
	return "jobleads"
}

// Ensure returns the stored cookie, logging in if there is none and
// credentials are configured. With no credentials configured it returns
// ("", nil) rather than erroring — unlike Djinni, JobLeads has no meaningful
// anonymous access, so the adapter (not the session) turns an empty cookie
// with no configured credentials into a clear "credentials not configured"
// error at the Search/HealthCheck call site.
func (s *JobLeadsSession) Ensure(ctx context.Context) (string, error) {
	cfg, err := s.Sources.Config(ctx, s.key())
	if err != nil {
		return "", err
	}
	if cookie := jobsources.StringOr(cfg["sessionCookie"], ""); cookie != "" {
		return cookie, nil
	}
	if s.Email == "" || s.Password == "" {
		return "", nil
	}
	return s.Refresh(ctx)
}

// Refresh logs in with the configured credentials and persists the new
// cookie to the DB. Serialized by mu so parallel ingest workers log in at
// most once at a time.
func (s *JobLeadsSession) Refresh(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cookie, err := jobLeadsLogin(ctx, s.base(), s.Email, s.Password)
	if err != nil {
		return "", err
	}
	if _, err := s.Sources.Update(ctx, s.key(), nil, map[string]any{"sessionCookie": cookie}); err != nil {
		return "", fmt.Errorf("jobleads: persist session cookie: %w", err)
	}
	return cookie, nil
}

// jobLeadsLogin performs JobLeads's form login: GET /login for the CSRF
// token + cookie, then POST the credentials, returning the resulting
// session cookie. Mirrors djinniLogin.
func jobLeadsLogin(ctx context.Context, base, email, password string) (string, error) {
	if email == "" || password == "" {
		return "", fmt.Errorf("jobleads login requires JOBLEADS_EMAIL and JOBLEADS_PASSWORD")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	loginURL := strings.TrimRight(base, "/") + "/login"

	token, err := jobLeadsCSRFToken(ctx, client, loginURL)
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("email", email)
	form.Set("password", password)
	if token != "" {
		form.Set("csrf_token", token)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginURL) // some login flows reject a POST without a same-origin Referer
	req.Header.Set("User-Agent", jobLeadsUserAgent)

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("jobleads login POST failed: %w", err)
	}
	defer res.Body.Close()

	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	for _, c := range jar.Cookies(u) {
		if c.Name == "session" && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("jobleads login failed: no session cookie returned (check JOBLEADS_EMAIL/JOBLEADS_PASSWORD)")
}

// jobLeadsCSRFToken fetches the login page and extracts the hidden
// csrf_token, if present; the matching session cookie (if any) is captured
// in the jar. A missing token is not fatal — some login flows don't use one.
func jobLeadsCSRFToken(ctx context.Context, client *http.Client, loginURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", jobLeadsUserAgent)

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("jobleads login GET failed: %w", err)
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}
	token, _ := doc.Find(`input[name="csrf_token"]`).First().Attr("value")
	return token, nil
}
