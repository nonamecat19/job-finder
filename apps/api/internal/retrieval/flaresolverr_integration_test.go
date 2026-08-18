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

// FlareSolverr is the last rung of the retrieval ladder — the one the app
// falls back to when a host answers a plain fetch and a headless browser with
// a challenge page. The app never speaks to it directly: cfg.FlaresolverrURL
// is handed to the job-scraper library (see NewService in service_impl.go),
// which POSTs a JSON command to <base>/v1 and reads the rendered page back
// out of the reply. That client's request and response structs are
// unexported, so nothing in this repository — and no unit test — can notice if
// the service changes the shape of either one.
//
// The stack floats FlareSolverr on :latest (docker-compose.yml, mirrored by
// testinfra), which makes that a live risk rather than a theoretical one: a
// renamed field, a status value spelled differently, or an error surfaced as
// an HTTP status instead of a JSON body would leave the library silently
// unable to fetch anything, with every affected job source degrading to "no
// jobs found" instead of failing loudly.
//
// These tests therefore assert the wire contract itself against the real
// image: the exact request the library sends, and every field of the reply the
// library reads. If one fails, the library's flareSolverrRung is broken
// against the running service even though the Go build is perfectly happy.
//
// The pages under test are served from this process; the container reaches
// them over testcontainers' host-port access, so nothing here touches the
// internet or a Cloudflare-protected site.

// The site is one server for the whole package, started in TestMain: the
// FlareSolverr container is process-wide and is granted access to exactly the
// host ports it was created with, so a per-test server on a fresh port would
// be unreachable from the browser inside it.
var (
	flareSite     *httptest.Server
	flareSitePort int
	flareSiteBase string // the address FLARESOLVERR uses — the host, seen from a container

	flareOnce sync.Once
	flareURL  string
	flareErr  error
)

// startFlareSolverr brings the container up on first use and hands back its
// base URL — the same value cfg.FlaresolverrURL carries in production. A
// container that will not start fails the test; there is no skip here, because
// a silently skipped contract test is indistinguishable from a passing one.
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

// scriptWrittenText is assembled from fragments by the /rendered page's own
// script, so these words appear nowhere in the bytes a plain fetch receives.
// Finding them in a reply is proof the page was executed, not just downloaded.
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

// flareResponse mirrors the library's own unexported reply struct field for
// field (jobscraper/retrieval/flaresolverr.go). Decoding into a copy of it is
// the point: a field the service renames or moves stops arriving here in
// exactly the way it would stop arriving there.
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

// solve sends the request the library sends — same command, same field names,
// same maxTimeout — and returns both the decoded reply and the HTTP status the
// service answered with, because the library ignores the latter entirely and a
// test has to know whether that is safe.
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

	// The browser inside the container is cold on the first call and the image
	// bundles a full Chrome, so the budget here is deliberately larger than
	// the library's own 90s client timeout.
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

// TestFlareSolverrRequestGetContract is the contract the library's Fetch
// depends on in full: cmd "request.get" is still the command, "ok" is still
// the success value of status, the fetched page still comes back in
// solution.response, and the origin's own status code still comes back in
// solution.status. Fetch returns exactly those last two values to the ladder,
// so any one of them changing shape breaks every FlareSolverr fetch.
func TestFlareSolverrRequestGetContract(t *testing.T) {
	target := flareSiteBase + "/static"
	parsed, httpStatus, raw := solve(t, target)

	if httpStatus != http.StatusOK {
		t.Errorf("POST /v1 answered HTTP %d for a successful solve; the library never inspects the HTTP status, so a service that reports failure this way would look like success", httpStatus)
	}
	if parsed.Status != "ok" {
		t.Fatalf("status = %q, want %q — the library treats anything else as an error: %s", parsed.Status, "ok", truncate(raw))
	}
	// This is the value the library returns as the fetch's status code. It is
	// a flat 200 for every solve, error pages included — see
	// TestFlareSolverrHidesTheOriginStatusCode.
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

// TestFlareSolverrReturnsRenderedHTML is the entire reason this service is in
// the stack. A plain HTTP fetch of a job board returns a shell; the content
// arrives when the page's script runs. If FlareSolverr ever answered with the
// raw document instead of the rendered DOM, every fetch would still "succeed"
// and every parser downstream would find nothing — the worst possible failure
// mode, because it is silent.
func TestFlareSolverrReturnsRenderedHTML(t *testing.T) {
	parsed, _, raw := solve(t, flareSiteBase+"/rendered")

	if parsed.Status != "ok" {
		t.Fatalf("status = %q, want %q: %s", parsed.Status, "ok", truncate(raw))
	}
	if !strings.Contains(parsed.Solution.Response, scriptWrittenText) {
		t.Fatalf("solution.response is missing the script-written text %q: the page was returned unrendered, which is the one thing this rung exists to do:\n%s",
			scriptWrittenText, truncate([]byte(parsed.Solution.Response)))
	}

	// The fixture only proves rendering while the text is absent from the
	// served bytes. Fetch the same page plainly to keep that honest.
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

// TestFlareSolverrHidesTheOriginStatusCode pins the most surprising part of
// this contract, and the reason it is worth a test rather than a comment:
// FlareSolverr 3.x does NOT report the origin's status code. It drives a real
// browser, which cannot see one, so solution.status is the constant 200 — a
// 404 page comes back as a successful solve whose response body happens to be
// the origin's error page.
//
// The library hands that 200 straight to the ladder as the fetch's status
// code, so nothing downstream of the FlareSolverr rung can tell a dead posting
// from a live one by status alone. That is a property of the stack the app has
// to live with, and this test is where it is written down: if a future image
// starts reporting the real code, this fails and the dead-posting handling
// downstream should be revisited rather than left guessing.
//
// The compensating half — that an origin error still arrives as status "ok"
// with the body intact, rather than as a failed solve — is asserted too,
// because the library turns a non-"ok" envelope into an error, which the
// ladder reads as the host refusing it and answers by cooling the host off.
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

// TestFlareSolverrReportsUnreachableHostAsError covers the other error shape:
// a host the browser cannot reach at all. The library distinguishes this from
// the case above purely by status != "ok", and puts message into the error it
// returns — so a service that stopped populating message, or that answered
// with an empty envelope, would produce an unattributable failure in the logs.
func TestFlareSolverrReportsUnreachableHostAsError(t *testing.T) {
	parsed, _, raw := solve(t, "http://jobfinder-no-such-host.invalid/listing")

	if parsed.Status == "ok" {
		t.Fatalf("status = %q for an unresolvable host, want a failure value: the library would treat a non-page as a fetched page: %s", parsed.Status, truncate(raw))
	}
	if parsed.Message == "" {
		t.Errorf("status = %q but message is empty: the library's error text comes entirely from this field: %s", parsed.Status, truncate(raw))
	}
}

// TestFlareSolverrRejectsAnUnknownCommand proves the "cmd" field is still
// dispatched on rather than ignored. If an unknown command were answered with
// a successful-looking envelope, a future rename of "request.get" would not
// fail loudly anywhere — this test is what makes the assertion above that
// "request.get" works mean something.
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

// TestMain starts the one site server for the whole package before any test
// runs, because the FlareSolverr container has to be told at creation time
// which host port it may reach and the port has to exist by then.
//
// The container itself is started lazily, by the first test that asks for it
// (startFlareSolverr): the rest of this package's integration tests only need
// dbtest, and pulling a browser-bundling image for them would cost minutes for
// nothing.
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
