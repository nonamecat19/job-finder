package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/job-finder/api/internal/ratelimit"
)

// recordingRT stands in for the network so the tests measure pacing, not
// round-trip latency.
type recordingRT struct{ calls int }

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: req}, nil
}

func get(t *testing.T, c *http.Client, url string) {
	t.Helper()
	res, err := c.Get(url)
	if err != nil {
		t.Fatalf("request to %s failed: %v", url, err)
	}
	res.Body.Close()
}

// Past the burst, requests to one host are spaced — this is the whole point:
// a multi-page search used to fire its pages as fast as the network allowed.
func TestRoundTrip_PacesRequestsToTheSameHost(t *testing.T) {
	rt := &recordingRT{}
	c := &http.Client{Transport: &ratelimit.Transport{Base: rt, RPS: 20, Burst: 1}}

	start := time.Now()
	get(t, c, "https://example.test/a")
	get(t, c, "https://example.test/b")
	elapsed := time.Since(start)

	// Burst 1 covers the first request; the second waits out one 1/20s token.
	if elapsed < 40*time.Millisecond {
		t.Errorf("expected the second request to wait for a token, took only %v", elapsed)
	}
	if rt.calls != 2 {
		t.Errorf("expected both requests to reach the base transport, got %d", rt.calls)
	}
}

// Limiters are per host, so a slow board never holds up a fast one.
func TestRoundTrip_DoesNotPaceAcrossHosts(t *testing.T) {
	rt := &recordingRT{}
	c := &http.Client{Transport: &ratelimit.Transport{Base: rt, RPS: 1, Burst: 1}}

	start := time.Now()
	get(t, c, "https://one.test/x")
	get(t, c, "https://two.test/x")
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("expected separate hosts to have separate limiters, took %v", elapsed)
	}
}

func TestRoundTrip_PerHostOverrideWins(t *testing.T) {
	rt := &recordingRT{}
	c := &http.Client{Transport: &ratelimit.Transport{
		Base:       rt,
		RPS:        0.01, // would stall for ~100s on the second request
		Burst:      1,
		PerHostRPS: map[string]float64{"fast.test": 1000},
	}}

	start := time.Now()
	get(t, c, "https://fast.test/a")
	get(t, c, "https://fast.test/b")
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("expected the per-host override to apply, took %v", elapsed)
	}
}

// Local dependencies (FlareSolverr, Ollama, MinIO) are ours to hammer — the
// politeness here is owed to third-party hosts.
func TestRoundTrip_SkipsLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	c := &http.Client{Transport: &ratelimit.Transport{RPS: 0.01, Burst: 1}}

	start := time.Now()
	get(t, c, srv.URL+"/a")
	get(t, c, srv.URL+"/b")
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("expected loopback requests to bypass the limiter, took %v", elapsed)
	}
}

// A cancelled context has to unblock the wait, or shutdown would be held up
// by a queue of requests waiting on tokens.
func TestRoundTrip_CancelledContextDoesNotBlock(t *testing.T) {
	rt := &recordingRT{}
	c := &http.Client{Transport: &ratelimit.Transport{Base: rt, RPS: 0.01, Burst: 1}}

	get(t, c, "https://example.test/first") // consume the burst

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test/second", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(req); err == nil {
		t.Error("expected a cancelled request to fail rather than wait for its token")
	}
}
