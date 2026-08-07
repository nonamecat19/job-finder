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

var jobLeadsUserAgent = retrieval.Chrome126UserAgent

type JobLeadsSessionProvider interface {
	Ensure(ctx context.Context) (string, error)
	Refresh(ctx context.Context) (string, error)
}

type JobLeadsConfigStore interface {
	Config(ctx context.Context, key string) (map[string]any, error)
	Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
}

type JobLeadsSession struct {
	Sources  JobLeadsConfigStore
	Email    string
	Password string
	Key      string
	Base     string

	mu sync.Mutex
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
	req.Header.Set("Referer", loginURL)
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
