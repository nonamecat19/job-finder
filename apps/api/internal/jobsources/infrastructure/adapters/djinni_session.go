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
)

const djinniBaseURL = "https://djinni.co"

// djinniUserAgent mirrors scraping.userAgent (unexported there); the login flow
// needs its own cookie-jar client, so it can't reuse scraping.Service.
const djinniUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// DjinniSessionProvider hands the adapter a valid session cookie, logging in
// with stored credentials when none exists or the current one has expired.
type DjinniSessionProvider interface {
	// Ensure returns a usable session cookie, logging in if none is stored.
	Ensure(ctx context.Context) (string, error)
	// Refresh forces a fresh login and persists the new cookie.
	Refresh(ctx context.Context) (string, error)
}

// DjinniConfigStore is the slice of jobsources.Service the session manager
// needs: read the decrypted source config (where the cookie lives) and persist
// a config patch. *jobsources.Service satisfies it.
type DjinniConfigStore interface {
	Config(ctx context.Context, key string) (map[string]any, error)
	Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
}

// DjinniSession owns the djinni login lifecycle: credentials come from env
// (Email/Password), the resulting sessionid cookie lives only in the DB (source
// config, encrypted by jobsources.Service). It is shared by pointer across the
// adapter copies held in the registry and enrichment handler, so a refresh is
// visible everywhere.
type DjinniSession struct {
	Sources  DjinniConfigStore
	Email    string
	Password string
	Key      string // source key, "djinni"
	Base     string // override base URL for tests; "" -> djinniBaseURL

	mu sync.Mutex // serializes Refresh so concurrent workers don't stampede login
}

func (s *DjinniSession) base() string {
	if s.Base != "" {
		return s.Base
	}
	return djinniBaseURL
}

func (s *DjinniSession) key() string {
	if s.Key != "" {
		return s.Key
	}
	return "djinni"
}

// Ensure returns the stored cookie, logging in if there is none. With no
// credentials configured it degrades to anonymous ("") rather than erroring, so
// public /jobs search still works; the auth-gated paths then surface a clear
// login error via the adapter's login-page detection.
func (s *DjinniSession) Ensure(ctx context.Context) (string, error) {
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

// Refresh logs in with the configured credentials and persists the new cookie
// to the DB. Serialized by mu so parallel ingest workers log in at most once at
// a time.
func (s *DjinniSession) Refresh(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cookie, err := djinniLogin(ctx, s.base(), s.Email, s.Password)
	if err != nil {
		return "", err
	}
	if _, err := s.Sources.Update(ctx, s.key(), nil, map[string]any{"sessionCookie": cookie}); err != nil {
		return "", fmt.Errorf("djinni: persist session cookie: %w", err)
	}
	return cookie, nil
}

// djinniLogin performs djinni's Django form login: GET /login for the CSRF
// token + cookie, then POST the credentials, returning the resulting sessionid.
func djinniLogin(ctx context.Context, base, email, password string) (string, error) {
	if email == "" || password == "" {
		return "", fmt.Errorf("djinni login requires DJINNI_EMAIL and DJINNI_PASSWORD")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	loginURL := strings.TrimRight(base, "/") + "/login"

	token, err := djinniCSRFToken(ctx, client, loginURL)
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("email", email)
	form.Set("password", password)
	form.Set("csrfmiddlewaretoken", token)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginURL) // Django rejects HTTPS POST without a same-origin Referer
	req.Header.Set("User-Agent", djinniUserAgent)

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("djinni login POST failed: %w", err)
	}
	defer res.Body.Close()

	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	for _, c := range jar.Cookies(u) {
		if c.Name == "sessionid" && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("djinni login failed: no session cookie returned (check DJINNI_EMAIL/DJINNI_PASSWORD)")
}

// djinniCSRFToken fetches the login page and extracts the hidden
// csrfmiddlewaretoken; the matching csrftoken cookie is captured in the jar.
func djinniCSRFToken(ctx context.Context, client *http.Client, loginURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", djinniUserAgent)

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("djinni login GET failed: %w", err)
	}
	defer res.Body.Close()

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}
	token, ok := doc.Find(`input[name="csrfmiddlewaretoken"]`).First().Attr("value")
	if !ok || token == "" {
		return "", fmt.Errorf("djinni login: csrfmiddlewaretoken not found on login page")
	}
	return token, nil
}
