package application

// Async/cached rephrase wiring (task 008 follow-up).
//
// The truthful rephrase suggester (008-5) makes a live LLM call per
// missing-required term, so running it inline on every keyword-diff request
// (008-6) would put one-or-more model round-trips on the hot path of a GET.
// CachedRephraser puts it behind an async, TTL'd in-memory cache instead:
//
//   - The first request for a given (missing-required terms, profile bullets)
//     key returns empty suggestions immediately and kicks off the real
//     computation in the background.
//   - Subsequent requests return the cached suggestions once the background
//     computation has finished. The diff panel is expected to poll / refresh.
//   - Cache entries older than the TTL are recomputed (the profile or the diff
//     may have changed, and the suggestions are advisory anyway).
//
// It satisfies the Rephraser port, so it drops straight into
// DiffService.WithRephraser with no change to the endpoint.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/job-finder/api/internal/keyword/domain"
)

// DefaultRephraseCacheTTL is the fallback entry lifetime used when a
// non-positive TTL is supplied to NewCachedRephraser.
const DefaultRephraseCacheTTL = 15 * time.Minute

// cacheEntry is a completed background computation for one cache key.
type cacheEntry struct {
	suggestions []RephraseSuggestion
	computedAt  time.Time
}

// CachedRephraser wraps an inner Rephraser (the Suggester) with an async,
// TTL'd in-memory cache. It is safe for concurrent use.
type CachedRephraser struct {
	inner Rephraser
	ttl   time.Duration
	now   func() time.Time
	spawn func(func())
	log   *slog.Logger

	mu       sync.Mutex
	entries  map[string]*cacheEntry
	inflight map[string]bool
}

var _ Rephraser = (*CachedRephraser)(nil)

// NewCachedRephraser returns a cache in front of inner. A non-positive ttl
// falls back to DefaultRephraseCacheTTL. Background computations run in their
// own goroutine and use a detached context so they outlive the request that
// triggered them.
func NewCachedRephraser(inner Rephraser, ttl time.Duration) *CachedRephraser {
	if ttl <= 0 {
		ttl = DefaultRephraseCacheTTL
	}
	return &CachedRephraser{
		inner:    inner,
		ttl:      ttl,
		now:      time.Now,
		spawn:    func(f func()) { go f() },
		log:      slog.Default(),
		entries:  make(map[string]*cacheEntry),
		inflight: make(map[string]bool),
	}
}

// WithClock overrides the time source (tests).
func (c *CachedRephraser) WithClock(now func() time.Time) *CachedRephraser {
	if now != nil {
		c.now = now
	}
	return c
}

// WithSpawner overrides how background work is launched. The default runs each
// computation in a new goroutine; a synchronous spawner makes the cache
// deterministic in tests.
func (c *CachedRephraser) WithSpawner(spawn func(func())) *CachedRephraser {
	if spawn != nil {
		c.spawn = spawn
	}
	return c
}

// WithLogger sets the logger used for background-computation observability.
func (c *CachedRephraser) WithLogger(l *slog.Logger) *CachedRephraser {
	if l != nil {
		c.log = l
	}
	return c
}

// SuggestAll returns cached suggestions for the (missingRequired, profileBullets)
// key when a fresh entry exists, and otherwise returns an empty slice while
// (at most once per key) computing them in the background. It never blocks on
// the inner rephraser, so it is safe to call inline on the diff request.
func (c *CachedRephraser) SuggestAll(_ context.Context, missingRequired []domain.DiffTerm, profileBullets []string) []RephraseSuggestion {
	if len(missingRequired) == 0 {
		return []RephraseSuggestion{}
	}
	key := rephraseCacheKey(missingRequired, profileBullets)

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.now().Sub(e.computedAt) < c.ttl {
		out := cloneSuggestions(e.suggestions)
		c.mu.Unlock()
		return out
	}
	if c.inflight[key] {
		c.mu.Unlock()
		return []RephraseSuggestion{}
	}
	c.inflight[key] = true
	c.mu.Unlock()

	// Copy inputs so a concurrent mutation by the caller cannot race the
	// background computation.
	terms := append([]domain.DiffTerm(nil), missingRequired...)
	bullets := append([]string(nil), profileBullets...)
	c.spawn(func() { c.compute(key, terms, bullets) })

	return []RephraseSuggestion{}
}

// compute runs the inner rephraser and stores the result. It always clears the
// inflight marker so a failed/empty computation can be retried on the next
// request once the entry (if any) expires.
func (c *CachedRephraser) compute(key string, terms []domain.DiffTerm, bullets []string) {
	defer func() {
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
	}()

	// Detached context: the triggering request's context is cancelled once its
	// response is written, but this work must run to completion.
	suggestions := c.inner.SuggestAll(context.Background(), terms, bullets)

	c.mu.Lock()
	c.entries[key] = &cacheEntry{suggestions: suggestions, computedAt: c.now()}
	c.mu.Unlock()

	if c.log != nil {
		c.log.Debug("keyword: cached rephrase suggestions", "terms", len(terms), "suggestions", len(suggestions))
	}
}

// cloneSuggestions returns a shallow copy so callers cannot mutate the cached
// slice. RephraseSuggestion's *string fields are read-only, so a shallow copy
// is sufficient.
func cloneSuggestions(in []RephraseSuggestion) []RephraseSuggestion {
	out := make([]RephraseSuggestion, len(in))
	copy(out, in)
	return out
}

// rephraseCacheKey derives a stable key from the missing-required terms and the
// profile bullets. Term order is deterministic (it comes straight from the
// persisted diff), so no sorting is needed; the delimiters keep distinct field
// boundaries from colliding.
func rephraseCacheKey(terms []domain.DiffTerm, bullets []string) string {
	h := sha256.New()
	for _, t := range terms {
		io.WriteString(h, t.Term)
		h.Write([]byte{0x1f})
		io.WriteString(h, t.Canonical)
		h.Write([]byte{0x1e})
	}
	h.Write([]byte{0x1d})
	for _, b := range bullets {
		io.WriteString(h, b)
		h.Write([]byte{0x1f})
	}
	return hex.EncodeToString(h.Sum(nil))
}
