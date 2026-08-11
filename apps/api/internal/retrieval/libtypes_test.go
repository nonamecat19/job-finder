package retrieval

import (
	"context"
	"testing"
	"time"

	"github.com/nonamecat19/job-scraper/ports"
	js "github.com/nonamecat19/job-scraper/retrieval"
)

// The ladder, the outcome vocabulary and the fetch/status types moved into the
// job-scraper library. These tests came with the app and stay here as the app's
// regression gate on the library surface it depends on.

func TestPageOutcome(t *testing.T) {
	o := ports.PageOutcome{
		Status: ports.PageRead,
		Method: "direct",
		URL:    "https://example.com/foo",
	}
	if o.Status != "read" {
		t.Errorf("expected read, got %s", o.Status)
	}
}

func TestRunVerdict(t *testing.T) {
	if ports.VerdictSuccess != "success" {
		t.Error("ports.VerdictSuccess should be 'success'")
	}
	if ports.VerdictPartial != "partial" {
		t.Error("ports.VerdictPartial should be 'partial'")
	}
	if ports.VerdictBlocked != "blocked" {
		t.Error("ports.VerdictBlocked should be 'blocked'")
	}
}

// stubRung stands in for a real strategy: the ladder cares only about a rung's
// key and its position, so escalation can be exercised without a browser or a
// sidecar.
type stubRung struct{ key string }

func (r stubRung) Key() string { return r.key }

func (r stubRung) Fetch(context.Context, ports.FetchRequest) (ports.PageOutcome, string) {
	return ports.PageOutcome{Status: ports.PageRead, Method: r.key}, ""
}

func (r stubRung) Available(context.Context) bool { return true }

func (r stubRung) Close() error { return nil }

func newTestLadder() *js.Ladder {
	return js.NewLadder(
		stubRung{js.KeyDirect},
		stubRung{js.KeyBrowser},
		stubRung{js.KeyFlareSolverr},
	)
}

func TestLadderFind(t *testing.T) {
	ladder := newTestLadder()
	for _, key := range []string{js.KeyDirect, js.KeyBrowser, js.KeyFlareSolverr} {
		if _, ok := ladder.Find(key); !ok {
			t.Errorf("%s rung: not found", key)
		}
	}
	if _, ok := ladder.Find("nonexistent"); ok {
		t.Error("nonexistent rung should not be found")
	}
	if !ladder.IsCheapest(js.KeyDirect) {
		t.Error("direct should be the cheapest rung")
	}
}

func TestLadderNext(t *testing.T) {
	tests := []struct {
		key      string
		wantNext string
		wantOk   bool
	}{
		{js.KeyDirect, js.KeyBrowser, true},
		{js.KeyBrowser, js.KeyFlareSolverr, true},
		{js.KeyFlareSolverr, "", false},
	}
	ladder := newTestLadder()
	for _, tt := range tests {
		next, ok := ladder.Next(tt.key)
		if ok != tt.wantOk {
			t.Errorf("Next(%s) ok=%v, want %v", tt.key, ok, tt.wantOk)
		}
		if ok && next.Key() != tt.wantNext {
			t.Errorf("Next(%s).Key()=%s, want %s", tt.key, next.Key(), tt.wantNext)
		}
	}
}

func TestLadderRungsAvailable(t *testing.T) {
	for _, r := range newTestLadder().Rungs() {
		if !r.Available(context.Background()) {
			t.Errorf("%s should be available", r.Key())
		}
	}
}

func TestFetchRequest(t *testing.T) {
	req := ports.FetchRequest{
		URL:             "https://example.com",
		Headers:         map[string]string{"Accept": "text/html"},
		UsesUserAccount: true,
		RefererPage:     "https://google.com",
	}
	if req.URL != "https://example.com" {
		t.Error("URL not set")
	}
	if !req.UsesUserAccount {
		t.Error("UsesUserAccount should be true")
	}
}

func TestHostStatus(t *testing.T) {
	now := time.Now()
	cd := 2
	s := ports.HostStatus{
		Host:              "example.com",
		IdentityVersion:   "v1",
		CurrentRung:       "direct",
		LastBlockAt:       &now,
		LastBlockReason:   "403",
		CrawlDelaySeconds: &cd,
		Pacing:            ports.HostPacing{RequestsPerSecond: 0.2, IntervalSeconds: 5, Source: "site-requested"},
	}
	if s.Host != "example.com" {
		t.Error("Host not set")
	}
	if s.CurrentRung != "direct" {
		t.Error("CurrentRung not set")
	}
	if s.Pacing.Source != "site-requested" {
		t.Error("Pacing.Source not set")
	}
	if s.Pacing.RequestsPerSecond != 0.2 {
		t.Error("Pacing.RequestsPerSecond not set")
	}
	if s.CrawlDelaySeconds == nil || *s.CrawlDelaySeconds != 2 {
		t.Error("CrawlDelaySeconds not set correctly")
	}
}
