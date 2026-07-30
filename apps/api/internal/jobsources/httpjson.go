package jobsources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/job-finder/api/internal/retrieval"
)

// defaultClient paces its requests per host like the scraping service's
// client does, so the JSON/API adapters that don't carry their own client
// (and the enrichment fetches that reuse them) stay within a polite rate
// rather than being bounded only by how fast the remote answers.
var defaultClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: retrieval.DefaultTransport,
}

// SetDefaultClient replaces the HTTP client used by adapters when no explicit
// client is provided. Intended for tests that mock vendor API responses.
func SetDefaultClient(c *http.Client) { defaultClient = c }

// DefaultClient returns the HTTP client currently used by adapters when no
// explicit client is provided. Intended for tests that save/restore it
// around a SetDefaultClient swap.
func DefaultClient() *http.Client { return defaultClient }

// GetJSON performs a GET with query params and decodes the JSON response
// body into out. Equivalent to `axios.get(url, { params, timeout })`.
func GetJSON(ctx context.Context, client *http.Client, rawURL string, params url.Values, out any) error {
	_, err := GetJSONStatus(ctx, client, rawURL, params, out)
	return err
}

// GetJSONStatus behaves like GetJSON but also returns the HTTP status code
// on every attempt (including non-2xx), reusing the same paced
// defaultClient — so callers that need to distinguish "not found" from
// "refused"/"rate-limited" from a generic failure (the ATS board sources'
// per-employer outcome reporting, FR-020) don't stand up a second client
// with its own, un-shared rate limiter for the same hosts.
func GetJSONStatus(ctx context.Context, client *http.Client, rawURL string, params url.Values, out any) (statusCode int, err error) {
	if client == nil {
		client = defaultClient
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, err
	}
	if res.StatusCode >= 400 {
		return res.StatusCode, fmt.Errorf("GET %s returned %d: %s", rawURL, res.StatusCode, string(data))
	}
	return res.StatusCode, json.Unmarshal(data, out)
}

// PostJSON performs a POST with a JSON body and decodes the JSON response
// into out. Equivalent to `axios.post(url, body, { timeout })`.
func PostJSON(ctx context.Context, client *http.Client, rawURL string, body any, out any, timeout time.Duration) error {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("POST %s returned %d: %s", rawURL, res.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
}
