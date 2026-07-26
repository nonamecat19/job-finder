package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type flareSolverrRung struct {
	baseURL string
	client  *http.Client
}

func newFlareSolverrRung(baseURL string) *flareSolverrRung {
	return &flareSolverrRung{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 90 * time.Second},
	}
}

type flsRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
}

type flsResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL      string `json:"url"`
		Status   int    `json:"status"`
		Response string `json:"response"`
		Headers  map[string]string
	} `json:"solution"`
}

func (f *flareSolverrRung) Fetch(ctx context.Context, url string) (string, int, error) {
	reqBody, err := json.Marshal(flsRequest{
		Cmd:        "request.get",
		URL:        url,
		MaxTimeout: 60000,
	})
	if err != nil {
		return "", 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.baseURL+"/v1", bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := f.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("flaresolverr: %w", err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return "", 0, err
	}

	var parsed flsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", 0, fmt.Errorf("flaresolverr: parse response: %w", err)
	}

	if parsed.Status != "ok" {
		return "", 0, fmt.Errorf("flaresolverr: %s: %s", parsed.Status, parsed.Message)
	}

	return parsed.Solution.Response, parsed.Solution.Status, nil
}

func (f *flareSolverrRung) Available(ctx context.Context) bool {
	if f.baseURL == "" {
		return false
	}
	healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	body, _, err := f.Fetch(healthCtx, "https://httpbin.org/get")
	if err != nil {
		slog.Warn("retrieval: flaresolverr health check failed", "url", f.baseURL, "error", err)
		return false
	}
	return body != ""
}

func (f *flareSolverrRung) Close() {}
