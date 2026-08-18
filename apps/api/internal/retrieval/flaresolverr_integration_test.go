//go:build integration

package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/job-finder/api/internal/testinfra"
)

var (
	flareSite     *httptest.Server
	flareSitePort int
	flareSiteBase string

	flareOnce sync.Once
	flareURL  string
	flareErr  error
)

func startFlareSolverr(t *testing.T) string {
	t.Helper()
	flareOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		flareURL, flareErr = testinfra.FlareSolverrURL(ctx, flareSitePort)
	})
	if flareErr != nil {
		t.Fatalf("start flaresolverr: %v", flareErr)
	}
	return flareURL
}

const scriptWrittenText = "Senior Go Engineer at Acme"

func startFlareSite() (*httptest.Server, int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, err
	}
	srv := &httptest.Server{
		Listener: listener,
		Config: &http.Server{
			ReadHeaderTimeout: 10 * time.Second,
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				switch r.URL.Path {
				case "/rendered":
					_, _ = w.Write([]byte(`<!doctype html><html><body>
						<div id="listing">loading…</div>
						<script>
						  var parts = ['Senior', 'Go', 'Engineer', 'at', 'Acme'];
						  document.getElementById('listing').textContent = parts.join(' ');
						</script></body></html>`))
				case "/missing":
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`<html><body><h1>no such posting</h1></body></html>`))
				default:
					_, _ = w.Write([]byte(`<html><body><h1>jobfinder-flaresolverr-fixture</h1></body></html>`))
				}
			}),
		},
	}
	srv.Start()

	_, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		srv.Close()
		return nil, 0, err
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		srv.Close()
		return nil, 0, err
	}
	return srv, port, nil
}

type flareResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL      string            `json:"url"`
		Status   int               `json:"status"`
		Response string            `json:"response"`
		Headers  map[string]string `json:"headers"`
	} `json:"solution"`
}

func solve(t *testing.T, url string) (flareResponse, int, []byte) {
	t.Helper()
	return solveCmd(t, map[string]any{"cmd": "request.get", "url": url, "maxTimeout": 60000})
}

func solveCmd(t *testing.T, command map[string]any) (flareResponse, int, []byte) {
	t.Helper()

	base := startFlareSolverr(t)

	body, err := json.Marshal(command)
	if err != nil {
		t.Fatalf("marshal command: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		t.Fatalf("POST /v1 %v: %v", command, err)
	}
	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read reply: %v", err)
	}

	var parsed flareResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("the reply is not the JSON object the library unmarshals: %v (body %s)", err, truncate(raw))
	}
	return parsed, res.StatusCode, raw
}

func truncate(raw []byte) string {
	const max = 600
	if len(raw) <= max {
		return string(raw)
	}
	return string(raw[:max]) + "…"
}

func TestFlareSolverrRequestGetContract(t *testing.T) {
	target := flareSiteBase + "/static"
	parsed, httpStatus, raw := solve(t, target)

	if httpStatus != http.StatusOK {
		t.Errorf("POST /v1 answered HTTP %d for a successful solve; the library never inspects the HTTP status, so a service that reports failure this way would look like success", httpStatus)
	}
	if parsed.Status != "ok" {
		t.Fatalf("status = %q, want %q — the library treats anything else as an error: %s", parsed.Status, "ok", truncate(raw))
	}

	if parsed.Solution.Status != http.StatusOK {
		t.Errorf("solution.status = %d, want 200: this is the status the ladder classifies the fetch by", parsed.Solution.Status)
	}
	if !strings.Contains(parsed.Solution.Response, "jobfinder-flaresolverr-fixture") {
		t.Fatalf("solution.response does not contain the served page; the library returns this field as the page body: %s", truncate([]byte(parsed.Solution.Response)))
	}
	if parsed.Solution.URL != target {
		t.Errorf("solution.url = %q, want the requested %q: the field the library decodes for the resolved URL no longer identifies what was fetched", parsed.Solution.URL, target)
	}
}

func TestFlareSolverrReturnsRenderedHTML(t *testing.T) {
	parsed, _, raw := solve(t, flareSiteBase+"/rendered")

	if parsed.Status != "ok" {
		t.Fatalf("status = %q, want %q: %s", parsed.Status, "ok", truncate(raw))
	}
	if !strings.Contains(parsed.Solution.Response, scriptWrittenText) {
		t.Fatalf("solution.response is missing the script-written text %q: the page was returned unrendered, which is the one thing this rung exists to do:\n%s",
			scriptWrittenText, truncate([]byte(parsed.Solution.Response)))
	}

	plainCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(plainCtx, http.MethodGet, flareSite.URL+"/rendered", nil)
	if err != nil {
		t.Fatalf("build plain request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("plain fetch: %v", err)
	}
	defer res.Body.Close()
	plain, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read plain body: %v", err)
	}
	if strings.Contains(string(plain), scriptWrittenText) {
		t.Fatal("the served HTML already contains the script-written text; this fixture no longer tests rendering")
	}
}

func TestFlareSolverrHidesTheOriginStatusCode(t *testing.T) {
	parsed, httpStatus, raw := solve(t, flareSiteBase+"/missing")

	if httpStatus != http.StatusOK {
		t.Errorf("POST /v1 answered HTTP %d for a 404 origin, want 200: the library does not look at the HTTP status", httpStatus)
	}
	if parsed.Status != "ok" {
		t.Fatalf("status = %q for a 404 origin, want %q: the library would report a transport failure instead of a fetched page, and the ladder would cool the whole host off over one dead posting: %s",
			parsed.Status, "ok", truncate(raw))
	}
	if !strings.Contains(parsed.Solution.Response, "no such posting") {
		t.Errorf("solution.response does not carry the origin's error page, so the app has no way at all to recognise a dead posting: %s", truncate([]byte(parsed.Solution.Response)))
	}
	if parsed.Solution.Status != http.StatusOK {
		t.Fatalf("solution.status = %d for a 404 origin, but this service has always reported a flat 200; it now reports something real, and the callers that assume 200-means-nothing (the ladder's status handling) should be revisited", parsed.Solution.Status)
	}
}

func TestFlareSolverrReportsUnreachableHostAsError(t *testing.T) {
	// 192.0.2.0/24 is TEST-NET-1 (RFC 5737): reserved for documentation and
	// guaranteed never routed on the public internet. A hostname under the
	// .invalid TLD would rely on DNS resolution failing, which some CI
	// resolvers hijack into a synthetic "ok" search/landing page instead of
	// a real failure; an IP literal skips DNS entirely so the connection
	// itself is what fails.
	parsed, _, raw := solve(t, "http://192.0.2.1/listing")

	if parsed.Status == "ok" {
		t.Fatalf("status = %q for an unreachable host, want a failure value: the library would treat a non-page as a fetched page: %s", parsed.Status, truncate(raw))
	}
	if parsed.Message == "" {
		t.Errorf("status = %q but message is empty: the library's error text comes entirely from this field: %s", parsed.Status, truncate(raw))
	}
}

func TestFlareSolverrRejectsAnUnknownCommand(t *testing.T) {
	parsed, _, raw := solveCmd(t, map[string]any{
		"cmd":        "request.jobfinder-not-a-command",
		"url":        flareSiteBase + "/static",
		"maxTimeout": 60000,
	})

	if parsed.Status == "ok" {
		t.Fatalf("an unknown cmd was answered with status %q; the service is not dispatching on cmd at all: %s", parsed.Status, truncate(raw))
	}
	if parsed.Solution.Response != "" {
		t.Errorf("a rejected command still carried solution.response %q, so a failed solve can look like a page", truncate([]byte(parsed.Solution.Response)))
	}
}

func TestMain(m *testing.M) {
	srv, port, err := startFlareSite()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start test site: %v\n", err)
		os.Exit(1)
	}
	flareSite = srv
	flareSitePort = port
	flareSiteBase = fmt.Sprintf("http://host.testcontainers.internal:%d", port)

	code := m.Run()
	srv.Close()
	os.Exit(code)
}
