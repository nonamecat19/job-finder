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
)

var defaultClient = &http.Client{Timeout: 30 * time.Second}

// GetJSON performs a GET with query params and decodes the JSON response
// body into out. Equivalent to `axios.get(url, { params, timeout })`.
func GetJSON(ctx context.Context, client *http.Client, rawURL string, params url.Values, out any) error {
	if client == nil {
		client = defaultClient
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("GET %s returned %d: %s", rawURL, res.StatusCode, string(data))
	}
	return json.Unmarshal(data, out)
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
