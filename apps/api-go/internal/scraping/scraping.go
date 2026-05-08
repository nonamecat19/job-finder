// Package scraping provides HTML fetching (goquery-friendly, plain HTTP) and
// a shared headless-Chromium context (chromedp) for PDF rendering, mirroring
// apps/api/src/modules/scraping/scraping.service.ts (which used axios + a
// shared Playwright browser).
package scraping

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// Service bundles the HTTP fetcher and the lazily-launched shared Chromium
// instance used for PDF rendering.
type Service struct {
	http *http.Client

	mu          sync.Mutex
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserCncl context.CancelFunc
}

func New() *Service {
	return &Service{http: &http.Client{Timeout: 20 * time.Second}}
}

// FetchHTML fetches server-rendered HTML over plain HTTP with a browser-like
// UA. Only 5xx responses are treated as errors — 4xx pages are still
// parseable HTML, matching axios's `validateStatus: (s) => s < 500`.
func (s *Service) FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error) {
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

// BrowserContext lazily launches a shared headless Chromium instance and
// returns a chromedp context rooted on it. Callers create a new tab context
// per render with chromedp.NewContext(browserCtx) so pages don't share state,
// mirroring getBrowser()+browser.newPage() in the TS ScrapingService.
func (s *Service) BrowserContext(ctx context.Context) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.browserCtx != nil {
		if err := s.browserCtx.Err(); err == nil {
			return s.browserCtx, nil
		}
		// previous browser context died — relaunch.
		if s.allocCancel != nil {
			s.allocCancel()
		}
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		// The container image runs as root with no user namespace, and
		// Chromium refuses to start as root without this flag ("Running as
		// root without --no-sandbox is not supported", crbug.com/638180).
		// Safe here: chromedp only ever renders our own locally-generated
		// resume/cover-letter HTML, never third-party/untrusted content.
		chromedp.Flag("no-sandbox", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	// Force the browser process to actually start so failures surface here,
	// not on the first render call.
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

// Close shuts down the shared browser, if launched.
func (s *Service) Close() {
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
