package infrastructure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/job-finder/api/internal/platform/scraping/domain"
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

type HTTPScraper struct {
	http *http.Client

	mu          sync.Mutex
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserCncl context.CancelFunc
}

func New() *HTTPScraper {
	return &HTTPScraper{http: &http.Client{Timeout: 20 * time.Second}}
}

var _ domain.Scraper = (*HTTPScraper)(nil)

func (s *HTTPScraper) HTTPClient() *http.Client {
	return s.http
}

func (s *HTTPScraper) FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("scraping: fetch %s failed: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		return "", fmt.Errorf("scraping: fetch %s returned %d", url, res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *HTTPScraper) BrowserContext(ctx context.Context) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browserCtx != nil {
		if err := s.browserCtx.Err(); err == nil {
			return s.browserCtx, nil
		}
		if s.allocCancel != nil {
			s.allocCancel()
		}
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("no-sandbox", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocCancel()
		return nil, fmt.Errorf("scraping: launch chromium: %w", err)
	}

	s.allocCancel = allocCancel
	s.browserCtx = browserCtx
	s.browserCncl = browserCancel
	return s.browserCtx, nil
}

func (s *HTTPScraper) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browserCncl != nil {
		s.browserCncl()
	}
	if s.allocCancel != nil {
		s.allocCancel()
	}
	s.browserCtx = nil
}
