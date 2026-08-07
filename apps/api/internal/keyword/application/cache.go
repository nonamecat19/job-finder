package application

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

const DefaultRephraseCacheTTL = 15 * time.Minute

type cacheEntry struct {
	suggestions []RephraseSuggestion
	computedAt  time.Time
}

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

func (c *CachedRephraser) WithClock(now func() time.Time) *CachedRephraser {
	if now != nil {
		c.now = now
	}
	return c
}

func (c *CachedRephraser) WithSpawner(spawn func(func())) *CachedRephraser {
	if spawn != nil {
		c.spawn = spawn
	}
	return c
}

func (c *CachedRephraser) WithLogger(l *slog.Logger) *CachedRephraser {
	if l != nil {
		c.log = l
	}
	return c
}

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

	terms := append([]domain.DiffTerm(nil), missingRequired...)
	bullets := append([]string(nil), profileBullets...)
	c.spawn(func() { c.compute(key, terms, bullets) })

	return []RephraseSuggestion{}
}

func (c *CachedRephraser) compute(key string, terms []domain.DiffTerm, bullets []string) {
	defer func() {
		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
	}()

	suggestions := c.inner.SuggestAll(context.Background(), terms, bullets)

	c.mu.Lock()
	c.entries[key] = &cacheEntry{suggestions: suggestions, computedAt: c.now()}
	c.mu.Unlock()

	if c.log != nil {
		c.log.Debug("keyword: cached rephrase suggestions", "terms", len(terms), "suggestions", len(suggestions))
	}
}

func cloneSuggestions(in []RephraseSuggestion) []RephraseSuggestion {
	out := make([]RephraseSuggestion, len(in))
	copy(out, in)
	return out
}

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
