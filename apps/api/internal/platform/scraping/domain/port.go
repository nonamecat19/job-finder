package domain

import (
	"context"
	"net/http"
)

type Scraper interface {
	HTTPClient() *http.Client
	FetchHTML(ctx context.Context, url string, headers map[string]string) (string, error)
	BrowserContext(ctx context.Context) (context.Context, error)
	Close()
}
