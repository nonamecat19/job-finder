// Package domain defines the scraping platform kernel's outbound port:
// HTML fetching over plain HTTP and a shared headless-Chromium context for
// PDF rendering. Consumed across the Sourcing/Ingestion, Company
// Intelligence, and Document Generation bounded contexts.
package domain

import (
	"context"
	"net/http"
)

// Scraper is the port every consumer depends on instead of a concrete
// scraping client — introduced so the four consumers (jobsources adapters,
// companyintel scrapers, generation's PDF renderer, recruiter) can be
// substituted/faked without a concrete dependency.
type Scraper interface {
	// HTTPClient returns the underlying *http.Client so adapters can make
	// arbitrary requests (POST, custom headers, etc.) using the same timeout
	// and connection pool.
	HTTPClient() *http.Client
	// FetchHTML fetches server-rendered HTML over plain HTTP with a
	// browser-like UA. Only 5xx responses are treated as errors — 4xx pages
	// are still parseable HTML.
	FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error)
	// BrowserContext lazily launches a shared headless Chromium instance and
	// returns a chromedp context rooted on it.
	BrowserContext(ctx context.Context) (context.Context, error)
	// Close shuts down the shared browser, if launched.
	Close()
}
