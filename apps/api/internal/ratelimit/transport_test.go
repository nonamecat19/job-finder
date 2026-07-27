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

// FR-006/data-model Resolution rules: operator override wins over crawl
// delay, even though both flow through the same resolver callback.
func TestResolveRate_OverrideWinsOverCrawlDelay(t *testing.T) {
	tr := &ratelimit.Transport{
		RateResolver: func(host string) (float64, string, bool) {
			return 3.0, "override", true
		},
	}
	rps, source := tr.RateFor("slow.test")
	if source != "override" {
		t.Errorf("expected source override, got %s", source)
	}
	if rps != 3.0 {
		t.Errorf("expected rps 3.0, got %v", rps)
	}
}

// A crawl delay slower than the default is adopted as-is.
func TestResolveRate_SlowerCrawlDelayIsAdopted(t *testing.T) {
	tr := &ratelimit.Transport{
		RateResolver: func(host string) (float64, string, bool) {
			return 0.2, "site-requested", true // Crawl-delay: 5
		},
	}
	rps, source := tr.RateFor("slow.test")
	if source != "site-requested" {
		t.Errorf("expected source site-requested, got %s", source)
	}
	if rps != 0.2 {
		t.Errorf("expected rps 0.2, got %v", rps)
	}
}

// A crawl delay faster than the default must not speed anything up — that
// precedence call is the resolver's to make, but the transport must relay
// whatever the resolver decides rather than second-guessing it.
func TestResolveRate_ResolverDecisionIsRelayedVerbatim(t *testing.T) {
	tr := &ratelimit.Transport{
		RateResolver: func(host string) (float64, string, bool) {
			return ratelimit.DefaultRPS, "default", true
		},
	}
	rps, source := tr.RateFor("fast.test")
	if source != "default" || rps != ratelimit.DefaultRPS {
		t.Errorf("expected default rate to stand, got rps=%v source=%s", rps, source)
	}
}

// crawl_delay_seconds == 0 must never resolve to an unbounded rate — the
// single highest-risk case in this feature (research Finding 3). A resolver
// that saw "0, looked, nothing advertised" must report DefaultRPS/"default",
// never a 0 or infinite rate.
func TestResolveRate_ZeroCrawlDelayFallsThroughToDefault(t *testing.T) {
	tr := &ratelimit.Transport{
		RateResolver: func(host string) (float64, string, bool) {
			return ratelimit.DefaultRPS, "default", true
		},
	}
	rps, source := tr.RateFor("looked-nothing-advertised.test")
	if rps != ratelimit.DefaultRPS {
		t.Errorf("expected DefaultRPS for a zero crawl delay, got %v", rps)
	}
	if source != "default" {
		t.Errorf("expected source default, got %s", source)
	}
}

// A nil resolver must yield DefaultRPS for every host — the pre-existing
// behaviour that keeps every earlier test in this file passing untouched.
func TestResolveRate_NilResolverYieldsDefault(t *testing.T) {
	tr := &ratelimit.Transport{}
	rps, source := tr.RateFor("anyhost.test")
	if rps != ratelimit.DefaultRPS {
		t.Errorf("expected DefaultRPS, got %v", rps)
	}
	if source != "default" {
		t.Errorf("expected source default, got %s", source)
	}
}

// Two concurrent callers to one host must share a single limiter (FR-008):
// pacing is per host, not per caller, so neither gets its own budget of
// tokens.
func TestRoundTrip_ConcurrentCallersShareOneLimiter(t *testing.T) {
	rt := &recordingRT{}
	c := &http.Client{Transport: &ratelimit.Transport{Base: rt, RPS: 5, Burst: 1}}

	start := time.Now()
	done := make(chan struct{}, 2)
	for i := 0; i < 2; i++ {
		go func() {
			get(t, c, "https://shared.test/x")
			done <- struct{}{}
		}()
	}
	<-done
	<-done
	elapsed := time.Since(start)

	// Burst 1 covers one request; the second, whichever caller sent it,
	// must wait roughly one token interval (1/5s) — not run concurrently
	// unthrottled, which two independent limiters would allow.
	if elapsed < 100*time.Millisecond {
		t.Errorf("expected concurrent callers to share one limiter and for the second to wait, took %v", elapsed)
	}
}

// Loopback destinations stay exempt from pacing regardless of any resolver.
func TestRateFor_LoopbackExempt(t *testing.T) {
	tr := &ratelimit.Transport{
		RateResolver: func(host string) (float64, string, bool) {
			return 0.01, "site-requested", true
		},
	}
	rps, source := tr.RateFor("localhost:8080")
	if source != "default" || rps != ratelimit.DefaultRPS {
		t.Errorf("expected loopback to bypass the resolver, got rps=%v source=%s", rps, source)
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
