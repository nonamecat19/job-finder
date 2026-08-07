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

var defaultClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: retrieval.DefaultTransport,
}

func SetDefaultClient(c *http.Client) { defaultClient = c }

func DefaultClient() *http.Client { return defaultClient }

func GetJSON(ctx context.Context, client *http.Client, rawURL string, params url.Values, out any) error {
	_, err := GetJSONStatus(ctx, client, rawURL, params, out)
	return err
}

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
