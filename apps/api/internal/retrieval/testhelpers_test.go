//go:build integration

package retrieval

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	js "github.com/nonamecat19/jobscraper/retrieval"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
	"github.com/job-finder/api/internal/dbtest"
)

type stubHost struct {
	t        *testing.T
	server   *httptest.Server
	svc      js.Service
	store    *StateStore
	host     string
	url      string
	dbConn   *db.DB
	mu       sync.Mutex
	delay    string
	pageHTML string
}

func newStubHost(t *testing.T) *stubHost {
	t.Helper()

	database := dbtest.New(t)

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

	identity, err := js.NewBrowserIdentity("chrome126")
	if err != nil {
		t.Fatalf("new browser identity: %v", err)
	}
	sh.store = NewStateStore(database.Queries, "")

	cfg := &config.Config{
		CoolingOffThreshold:    3,
		CoolingOffBaseDuration: 0,
	}

	svc := NewService(identity, sh.store, cfg)
	if closer, ok := svc.(interface{ Close() }); ok {
		t.Cleanup(closer.Close)
	}
	sh.svc = svc

	return sh
}

func (sh *stubHost) setCrawlDelay(seconds int) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.delay = strconv.Itoa(seconds)
}

func (sh *stubHost) pageURL(path string) string {
	return sh.url + path
}

func (sh *stubHost) fetchMany(t *testing.T, n int) []js.PageOutcome {
	t.Helper()
	outcomes := make([]js.PageOutcome, 0, n)
	for i := 0; i < n; i++ {
		result, err := sh.svc.Fetch(context.Background(), js.FetchRequest{
			URL: sh.pageURL(fmt.Sprintf("/page-%d", i)),
		})
		if err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
		outcomes = append(outcomes, result.Outcome)
	}
	return outcomes
}

func assertNoDeferralsOrBudgetLanguage(t *testing.T, outcomes []js.PageOutcome) {
	t.Helper()
	banned := []string{"budget", "quota", "allowance", "limit"}
	for i, o := range outcomes {
		if o.Status == js.PageDeferred {
			t.Errorf("outcome %d: unexpected js.PageDeferred, reason=%q", i, o.Reason)
		}
		lower := strings.ToLower(o.Reason)
		for _, word := range banned {
			if strings.Contains(lower, word) {
				t.Errorf("outcome %d: reason %q mentions banned word %q", i, o.Reason, word)
			}
		}
	}
}
