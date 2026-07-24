// Package ratelimit provides a per-host outbound HTTP rate limiter, applied
// as an http.RoundTripper so every request made through a wrapped client is
// paced whether it goes through a helper or through the raw *http.Client.
package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

// DefaultRPS is the per-host request rate used for hosts without an explicit
// override. Deliberately below one request per second: the scrapers are
// hitting job boards that publish crawl delays in that range, and nothing in
// this system is latency-sensitive enough to justify going faster.
const DefaultRPS = 0.7

// DefaultBurst lets a run open with a couple of requests back to back — a
// search page plus its first detail fetch — before settling into the steady
// rate.
const DefaultBurst = 2

// Transport paces requests per destination host. Adapters used to be
// unthrottled apart from the fixed sleeps in the enrich path, so a single
// multi-page search could fire its pages as fast as the network allowed;
// concurrent ingest tasks for two searches on the same board multiplied that
// again. A host that answers by rate-limiting or banning costs far more than
// the delay does.
//
// Limiting by host rather than by source key is deliberate: it stays correct
// when several sources share a host, and it covers the enrichment fetches too,
// which target the same hosts the search pass just hit.
type Transport struct {
	// Base is the wrapped RoundTripper. nil means http.DefaultTransport.
	Base http.RoundTripper
	// RPS is the steady-state per-host rate. Zero means DefaultRPS.
	RPS float64
	// Burst is the per-host burst size. Zero means DefaultBurst.
	Burst int
	// PerHostRPS overrides RPS for specific hosts (keyed as req.URL.Host,
	// so "api.example.com" and "example.com" are separate entries).
	PerHostRPS map[string]float64

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewTransport wraps base with the default pacing. Pass nil for base to wrap
// http.DefaultTransport.
func NewTransport(base http.RoundTripper) *Transport {
	return &Transport{Base: base}
}

func (t *Transport) limiterFor(host string) *rate.Limiter {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.limiters == nil {
		t.limiters = make(map[string]*rate.Limiter)
	}
	if l, ok := t.limiters[host]; ok {
		return l
	}

	rps := t.RPS
	if rps <= 0 {
		rps = DefaultRPS
	}
	if override, ok := t.PerHostRPS[host]; ok && override > 0 {
		rps = override
	}
	burst := t.Burst
	if burst <= 0 {
		burst = DefaultBurst
	}

	l := rate.NewLimiter(rate.Limit(rps), burst)
	t.limiters[host] = l
	return l
}

// RoundTrip waits for the destination host's token before delegating. It
// blocks rather than rejecting: a paced request is the point, and callers are
// workers with no user waiting on them. A cancelled or expired request context
// still returns immediately, so a shutdown isn't held up by the wait.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isLoopback(req.URL.Hostname()) {
		if err := t.limiterFor(req.URL.Host).Wait(req.Context()); err != nil {
			return nil, fmt.Errorf("ratelimit: waiting for %s: %w", req.URL.Host, err)
		}
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// isLoopback reports whether a hostname addresses this machine. Local
// dependencies — FlareSolverr, Ollama, MinIO, and the httptest servers the
// adapter tests run against — are ours to hammer; the politeness this package
// enforces is owed to third-party hosts only.
func isLoopback(hostname string) bool {
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}
