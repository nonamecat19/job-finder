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

const djinniUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

type DjinniSessionProvider interface {
	Ensure(ctx context.Context) (string, error)
	Refresh(ctx context.Context) (string, error)
}

type DjinniConfigStore interface {
	Config(ctx context.Context, key string) (map[string]any, error)
	Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
}

type DjinniSession struct {
	Sources  DjinniConfigStore
	Email    string
	Password string
	Key      string
	Base     string

	mu sync.Mutex
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
	req.Header.Set("Referer", loginURL)
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
