// Package ratelimit provides a per-host outbound HTTP rate limiter, applied
// as an http.RoundTripper so every request made through a wrapped client is
// paced whether it goes through a helper or through the raw *http.Client.
package ratelimit

import (
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateResolutionTTL bounds how long a resolved rate is trusted before the
// resolver is consulted again. Short enough that a crawl delay discovered
// mid-session takes effect without a restart; long enough that resolution
// never sits in the per-request hot path.
const rateResolutionTTL = 5 * time.Minute

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
	// RateResolver optionally resolves a per-host rate from an external
	// source (crawl-delay discovery, operator overrides) at limiter
	// construction time, never per request. A nil resolver, or one that
	// returns ok == false, falls through to RPS/DefaultRPS — the pre-existing
	// behaviour, so existing callers are unaffected.
	RateResolver func(host string) (rps float64, source string, ok bool)

	mu       sync.Mutex
	limiters map[string]*hostLimiter
}

// hostLimiter pairs a host's live token-bucket limiter with the resolved
// rate that produced it, so RateFor can report the effective rate without
// re-resolving, and limiterFor knows when the TTL calls for a refresh.
type hostLimiter struct {
	limiter    *rate.Limiter
	rps        float64
	source     string
	resolvedAt time.Time
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
		t.limiters = make(map[string]*hostLimiter)
	}

	now := time.Now()
	if hl, ok := t.limiters[host]; ok {
		if now.Sub(hl.resolvedAt) < rateResolutionTTL {
			return hl.limiter
		}
		rps, source := t.resolveRate(host)
		if rps != hl.rps {
			hl.limiter.SetLimit(rate.Limit(jitterRPS(rps)))
		}
		hl.rps = rps
		hl.source = source
		hl.resolvedAt = now
		return hl.limiter
	}

	rps, source := t.resolveRate(host)
	burst := t.Burst
	if burst <= 0 {
		burst = DefaultBurst
	}
	hl := &hostLimiter{
		limiter:    rate.NewLimiter(rate.Limit(jitterRPS(rps)), burst),
		rps:        rps,
		source:     source,
		resolvedAt: now,
	}
	t.limiters[host] = hl
	return hl.limiter
}

// resolveRate determines the steady-state rate for host and where it came
// from. Consulted only at limiter construction / TTL refresh, never per
// request — resolution may involve a database lookup upstream.
func (t *Transport) resolveRate(host string) (rps float64, source string) {
	if t.RateResolver != nil {
		if r, s, ok := t.RateResolver(host); ok {
			return r, s
		}
		return t.baseRPS(), "default"
	}
	if override, ok := t.PerHostRPS[host]; ok && override > 0 {
		return override, "override"
	}
	return t.baseRPS(), "default"
}

func (t *Transport) baseRPS() float64 {
	if t.RPS > 0 {
		return t.RPS
	}
	return DefaultRPS
}

// RateFor reports the rate currently in force for host and its source,
// without issuing a request. Constructs the limiter (and so triggers
// resolution) if this is the first time host has been seen.
func (t *Transport) RateFor(host string) (rps float64, source string) {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	if isLoopback(hostname) {
		return t.baseRPS(), "default"
	}
	t.limiterFor(host)
	t.mu.Lock()
	defer t.mu.Unlock()
	hl := t.limiters[host]
	return hl.rps, hl.source
}

func jitterRPS(rps float64) float64 {
	scale := 0.75 + 0.5*rand.Float64()
	return rps * scale
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
