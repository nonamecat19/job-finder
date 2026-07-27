//go:build integration

package retrieval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
)

// stubHost is a fake job-board host: an httptest server plus a real
// Postgres-backed ServiceImpl pointed at it, so a test can drive as many
// sequential fetches as it likes and inspect every PageOutcome without
// touching a live third party.
type stubHost struct {
	t       *testing.T
	server  *httptest.Server
	svc     *ServiceImpl
	store   *StateStore
	host    string
	url     string
	dbConn  *db.DB
	mu      sync.Mutex
	delay   string // Crawl-delay value served by /robots.txt; empty means no line
	pageHTML string
}

// newStubHost spins the httptest server, opens a real Postgres connection
// (via DATABASE_URL, matching the other integration suites in this repo),
// and wires a ServiceImpl at it exactly as production composition does.
func newStubHost(t *testing.T) *stubHost {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}
	database, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	// IsChallenged treats anything under 200 bytes as suspicious, so the stub
	// page must be padded well past that floor to read as a normal response.
	sh := &stubHost{t: t, pageHTML: "<html><body>" + strings.Repeat("ok ", 100) + "</body></html>"}

	mux := http.NewServeMux()
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		sh.mu.Lock()
		delay := sh.delay
		sh.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		if delay == "" {
			fmt.Fprint(w, "User-agent: *\nDisallow:\n")
			return
		}
		fmt.Fprintf(w, "User-agent: *\nCrawl-delay: %s\nDisallow:\n", delay)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sh.mu.Lock()
		body := sh.pageHTML
		sh.mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, body)
	})

	sh.server = httptest.NewServer(mux)
	t.Cleanup(sh.server.Close)
	sh.url = sh.server.URL
	sh.host = strings.TrimPrefix(strings.TrimPrefix(sh.url, "http://"), "https://")
	sh.dbConn = database

	identity, err := NewBrowserIdentity("chrome126")
	if err != nil {
		t.Fatalf("new browser identity: %v", err)
	}
	sh.store = NewStateStore(database.Queries, "")

	cfg := &config.Config{
		CoolingOffThreshold:    3,
		CoolingOffBaseDuration: 0,
	}

	svc, err := NewServiceImpl(identity, sh.store, cfg)
	if err != nil {
		t.Fatalf("new service impl: %v", err)
	}
	t.Cleanup(svc.Close)
	sh.svc = svc

	return sh
}

// setCrawlDelay configures the "Crawl-delay" value future /robots.txt
// responses advertise. An empty string omits the line entirely (NULL from
// the discovery path's point of view, before it is ever fetched).
func (sh *stubHost) setCrawlDelay(seconds int) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.delay = strconv.Itoa(seconds)
}

// pageURL builds a request URL against the stub host for the given path.
func (sh *stubHost) pageURL(path string) string {
	return sh.url + path
}

// fetchMany issues n sequential fetches against the stub host's root page
// and returns every collected outcome, in order.
func (sh *stubHost) fetchMany(t *testing.T, n int) []PageOutcome {
	t.Helper()
	outcomes := make([]PageOutcome, 0, n)
	for i := 0; i < n; i++ {
		result, err := sh.svc.Fetch(context.Background(), FetchRequest{
			URL: sh.pageURL(fmt.Sprintf("/page-%d", i)),
		})
		if err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
		outcomes = append(outcomes, result.Outcome)
	}
	return outcomes
}

// assertNoDeferralsOrBudgetLanguage is the reusable form of FR-002 and
// FR-017: it fails the test if any collected outcome was deferred for any
// reason, or if any reason string names the removed budget/quota/allowance/
// limit mechanism.
func assertNoDeferralsOrBudgetLanguage(t *testing.T, outcomes []PageOutcome) {
	t.Helper()
	banned := []string{"budget", "quota", "allowance", "limit"}
	for i, o := range outcomes {
		if o.Status == PageDeferred {
			t.Errorf("outcome %d: unexpected PageDeferred, reason=%q", i, o.Reason)
		}
		lower := strings.ToLower(o.Reason)
		for _, word := range banned {
			if strings.Contains(lower, word) {
				t.Errorf("outcome %d: reason %q mentions banned word %q", i, o.Reason, word)
			}
		}
	}
}
