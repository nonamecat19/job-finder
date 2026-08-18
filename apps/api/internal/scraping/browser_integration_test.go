//go:build integration

package scraping_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/job-finder/api/internal/scraping"
	"github.com/job-finder/api/internal/testinfra"
)

// The browser half of this package had no test at all: BrowserContext
// launches Chrome from a local binary, which a CI runner does not have, so
// everything downstream of it — the job sources that need a rendered page,
// company intel, and the PDF renderer — was covered only by whatever a
// developer ran by hand. These tests drive a headless Chrome container
// instead (testinfra.ChromeWebSocketURL), which is why the seam
// NewWithRemoteBrowser exists.
//
// The page under test is served from this process; the browser reaches it
// over testcontainers' host-port access, so nothing here touches the
// internet.

// The site is one server for the whole package, started in TestMain: the
// browser container is process-wide and is granted access to exactly the host
// ports it was created with, so a per-test server on a fresh port would be
// unreachable from the browser started by whichever test ran first.
var (
	site     *httptest.Server
	siteBase string // the address the BROWSER uses — the host, seen from a container
)

func startSite() (*httptest.Server, int, error) {
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
					// The whole reason a browser exists in this codebase:
					// the interesting content is not in the served HTML.
					// The text is assembled from fragments so it appears
					// nowhere in the bytes a plain fetch receives.
					_, _ = w.Write([]byte(`<!doctype html><html><body>
						<div id="listing">loading…</div>
						<script>
						  var parts = ['Senior', 'Go', 'Engineer', 'at', 'Acme'];
						  document.getElementById('listing').textContent = parts.join(' ');
						</script></body></html>`))
				case "/user-agent":
					_, _ = fmt.Fprintf(w, "<html><body>%s</body></html>", r.Header.Get("User-Agent"))
				default:
					_, _ = w.Write([]byte(`<html><body><h1>static</h1></body></html>`))
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

func remoteScraper(t *testing.T) *scraping.HTTPScraper {
	t.Helper()

	if chromeErr != nil {
		t.Fatalf("start chrome: %v", chromeErr)
	}
	scraper := scraping.NewWithRemoteBrowser(chromeWS)
	t.Cleanup(scraper.Close)
	return scraper
}

// TestBrowserContextRendersJavaScript is the property the browser rung of the
// retrieval ladder is for: content a plain fetch never sees, because it is
// written by script after load.
func TestBrowserContextRendersJavaScript(t *testing.T) {
	scraper := remoteScraper(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	browserCtx, err := scraper.BrowserContext(ctx)
	if err != nil {
		t.Fatalf("BrowserContext: %v", err)
	}

	tabCtx, cancelTab := chromedp.NewContext(browserCtx)
	defer cancelTab()

	var text string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(siteBase+"/rendered"),
		chromedp.WaitVisible("#listing", chromedp.ByID),
		chromedp.Text("#listing", &text, chromedp.ByID),
	); err != nil {
		t.Fatalf("drive the browser: %v", err)
	}

	if text != "Senior Go Engineer at Acme" {
		t.Fatalf("rendered text = %q, want the script-written content: the page was read without executing its script", text)
	}

	// The plain fetch, for contrast: it sees the placeholder, which is
	// exactly why the browser rung exists.
	raw, err := scraper.FetchHTML(ctx, site.URL+"/rendered", nil)
	if err != nil {
		t.Fatalf("FetchHTML: %v", err)
	}
	if strings.Contains(raw, "Senior Go Engineer at Acme") {
		t.Fatal("the plain fetch already contains the script-written content; this fixture no longer tests rendering")
	}
}

// TestBrowserContextIsReusedAcrossCalls proves the caching in BrowserContext:
// one browser is shared by the job sources, company intel and the PDF
// renderer (that sharing is why this package exists rather than the
// library's own scraper), so a second call must not launch a second browser.
func TestBrowserContextIsReusedAcrossCalls(t *testing.T) {
	scraper := remoteScraper(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	first, err := scraper.BrowserContext(ctx)
	if err != nil {
		t.Fatalf("first BrowserContext: %v", err)
	}
	second, err := scraper.BrowserContext(ctx)
	if err != nil {
		t.Fatalf("second BrowserContext: %v", err)
	}
	if first != second {
		t.Fatal("BrowserContext returned a different context on the second call: the browser is not being reused")
	}
}

// TestCloseReleasesTheBrowser proves Close is honest — the context it
// cancelled cannot be driven afterwards — and that a scraper can be revived
// by asking for a context again, which is what the reconnect path in
// BrowserContext is written for.
func TestCloseReleasesTheBrowser(t *testing.T) {
	scraper := remoteScraper(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	browserCtx, err := scraper.BrowserContext(ctx)
	if err != nil {
		t.Fatalf("BrowserContext: %v", err)
	}
	scraper.Close()

	if browserCtx.Err() == nil {
		t.Fatal("Close left the browser context live")
	}

	revived, err := scraper.BrowserContext(ctx)
	if err != nil {
		t.Fatalf("BrowserContext after Close: %v", err)
	}
	tabCtx, cancelTab := chromedp.NewContext(revived)
	defer cancelTab()
	var heading string
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(siteBase+"/static"),
		chromedp.Text("h1", &heading, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("drive the revived browser: %v", err)
	}
	if heading != "static" {
		t.Fatalf("heading = %q, want %q", heading, "static")
	}
}

// TestFetchHTMLSendsTheDeclaredUserAgent covers the plain path's one
// non-obvious behaviour: hosts that check the User-Agent are meant to see a
// browser-shaped one, and a caller's own headers are merged on top.
func TestFetchHTMLSendsTheDeclaredUserAgent(t *testing.T) {
	scraper := scraping.New()
	t.Cleanup(scraper.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	body, err := scraper.FetchHTML(ctx, site.URL+"/user-agent", nil)
	if err != nil {
		t.Fatalf("FetchHTML: %v", err)
	}
	if !strings.Contains(body, "Chrome/") {
		t.Fatalf("server saw User-Agent %q, want a browser-shaped one", body)
	}

	overridden, err := scraper.FetchHTML(ctx, site.URL+"/user-agent", map[string]string{"User-Agent": "jobfinder-test/1.0"})
	if err != nil {
		t.Fatalf("FetchHTML with headers: %v", err)
	}
	if !strings.Contains(overridden, "jobfinder-test/1.0") {
		t.Fatalf("server saw %q, want the caller's User-Agent", overridden)
	}
}

var (
	chromeWS  string
	chromeErr error
)

// TestMain starts the one site server and the one browser, in that order:
// the browser container has to be told at creation time which host port it
// may reach.
func TestMain(m *testing.M) {
	srv, port, err := startSite()
	if err != nil {
		fmt.Fprintf(os.Stderr, "start test site: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()
	site = srv
	siteBase = fmt.Sprintf("http://host.testcontainers.internal:%d", port)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	chromeWS, chromeErr = testinfra.ChromeWebSocketURL(ctx, port)
	cancel()

	os.Exit(m.Run())
}
