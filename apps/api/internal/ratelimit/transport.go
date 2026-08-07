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

const rateResolutionTTL = 5 * time.Minute

const DefaultRPS = 0.7

const DefaultBurst = 2

type Transport struct {
	Base         http.RoundTripper
	RPS          float64
	Burst        int
	PerHostRPS   map[string]float64
	RateResolver func(host string) (rps float64, source string, ok bool)

	mu       sync.Mutex
	limiters map[string]*hostLimiter
}

type hostLimiter struct {
	limiter    *rate.Limiter
	rps        float64
	source     string
	resolvedAt time.Time
}

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

func isLoopback(hostname string) bool {
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}
