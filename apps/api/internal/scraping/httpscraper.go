// Package scraping is the app's ports.Scraper: an ordinary HTTP client plus a
// lazily launched headless browser.
//
// It lives here rather than in the library because the library's own
// implementation moved to internal/ — a consumer is expected to bring its own
// plain fetcher and hand it over with jobscraper.WithScraper, which is also
// what lets the app share one browser between the job sources and the
// company-intel and PDF paths that never touch a job source at all.
package scraping

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/nonamecat19/job-scraper/ports"
)

// userAgent is what a plain scrape presents itself as. Hosts that check more
// than the User-Agent are read through the retrieval ladder instead, which
// wears a full browser identity.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// HTTPScraper is safe for concurrent use. Close releases the browser.
type HTTPScraper struct {
	http *http.Client

	mu          sync.Mutex
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserCncl context.CancelFunc
}

// New builds a scraper with a 20s request budget and no browser running yet.
func New() *HTTPScraper {
	return &HTTPScraper{http: &http.Client{Timeout: 20 * time.Second}}
}

var _ ports.Scraper = (*HTTPScraper)(nil)

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
