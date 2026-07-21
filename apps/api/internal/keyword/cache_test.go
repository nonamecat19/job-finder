package keyword

import (
	"context"
	"sync"
	"testing"
	"time"
)

// countingRephraser records how many times SuggestAll ran and returns a fixed,
// identifiable suggestion so the cache behaviour is observable.
type countingRephraser struct {
	mu    sync.Mutex
	calls int
}

func (c *countingRephraser) SuggestAll(_ context.Context, missingRequired []DiffTerm, _ []string) []RephraseSuggestion {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	out := make([]RephraseSuggestion, 0, len(missingRequired))
	for _, t := range missingRequired {
		r := "reframed " + t.Term
		out = append(out, RephraseSuggestion{Term: t.Term, Canonical: t.Canonical, Rephrase: &r})
	}
	return out
}

func (c *countingRephraser) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// syncSpawn runs background work inline so the cache is deterministic in tests.
func syncSpawn(f func()) { f() }

func TestCachedRephraser_FirstCallEmptyThenCached(t *testing.T) {
	inner := &countingRephraser{}
	c := NewCachedRephraser(inner, time.Minute).WithSpawner(syncSpawn)

	terms := []DiffTerm{reqTerm("kubernetes", "Kubernetes")}
	bullets := []string{"Ran Docker in prod"}

	// First call returns empty (compute kicked off in background) but, with the
	// synchronous spawner, the entry is populated before the second call.
	if got := c.SuggestAll(context.Background(), terms, bullets); len(got) != 0 {
		t.Fatalf("first call = %d suggestions, want 0 (async miss)", len(got))
	}
	if inner.count() != 1 {
		t.Fatalf("inner calls = %d, want 1", inner.count())
	}

	got := c.SuggestAll(context.Background(), terms, bullets)
	if len(got) != 1 || got[0].Term != "kubernetes" {
		t.Fatalf("second call = %+v, want cached suggestion for kubernetes", got)
	}
	// Cache hit must not re-run the inner rephraser.
	if inner.count() != 1 {
		t.Fatalf("inner calls after cache hit = %d, want 1", inner.count())
	}
}

func TestCachedRephraser_EmptyTermsNoCompute(t *testing.T) {
	inner := &countingRephraser{}
	c := NewCachedRephraser(inner, time.Minute).WithSpawner(syncSpawn)

	if got := c.SuggestAll(context.Background(), nil, []string{"x"}); len(got) != 0 {
		t.Fatalf("empty terms = %d suggestions, want 0", len(got))
	}
	if inner.count() != 0 {
		t.Fatalf("inner calls = %d, want 0 (no work for empty terms)", inner.count())
	}
}

func TestCachedRephraser_ExpiryRecomputes(t *testing.T) {
	inner := &countingRephraser{}
	now := time.Unix(0, 0)
	c := NewCachedRephraser(inner, time.Minute).
		WithSpawner(syncSpawn).
		WithClock(func() time.Time { return now })

	terms := []DiffTerm{reqTerm("go", "Go")}
	bullets := []string{"wrote services"}

	c.SuggestAll(context.Background(), terms, bullets) // compute #1
	if got := c.SuggestAll(context.Background(), terms, bullets); len(got) != 1 {
		t.Fatalf("within ttl = %d, want cached 1", len(got))
	}
	if inner.count() != 1 {
		t.Fatalf("inner calls before expiry = %d, want 1", inner.count())
	}

	now = now.Add(2 * time.Minute) // entry now stale
	if got := c.SuggestAll(context.Background(), terms, bullets); len(got) != 0 {
		t.Fatalf("after expiry = %d, want 0 (recompute miss)", len(got))
	}
	if inner.count() != 2 {
		t.Fatalf("inner calls after expiry = %d, want 2", inner.count())
	}
}

func TestCachedRephraser_KeyVariesByInput(t *testing.T) {
	inner := &countingRephraser{}
	c := NewCachedRephraser(inner, time.Minute).WithSpawner(syncSpawn)

	c.SuggestAll(context.Background(), []DiffTerm{reqTerm("go", "Go")}, []string{"a"})
	c.SuggestAll(context.Background(), []DiffTerm{reqTerm("rust", "Rust")}, []string{"a"})
	c.SuggestAll(context.Background(), []DiffTerm{reqTerm("go", "Go")}, []string{"b"})

	if inner.count() != 3 {
		t.Fatalf("inner calls = %d, want 3 (distinct keys)", inner.count())
	}
}

func TestCachedRephraser_SingleInflight(t *testing.T) {
	release := make(chan struct{})
	inner := &blockingRephraser{release: release, counter: &countingRephraser{}}
	c := NewCachedRephraser(inner, time.Minute) // real goroutine spawner

	terms := []DiffTerm{reqTerm("go", "Go")}
	bullets := []string{"a"}

	// Fire several concurrent misses for the same key while the first compute
	// is blocked; only one inner computation should be started.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.SuggestAll(context.Background(), terms, bullets)
		}()
	}
	wg.Wait()
	close(release)

	// Give the single background compute a moment to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if inner.counter.count() >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := inner.counter.count(); got != 1 {
		t.Fatalf("inner computations = %d, want exactly 1 (single-flight)", got)
	}
}

// blockingRephraser blocks until release is closed, so a test can hold a
// computation open and assert single-flight behaviour.
type blockingRephraser struct {
	release chan struct{}
	counter *countingRephraser
}

func (b *blockingRephraser) SuggestAll(ctx context.Context, missingRequired []DiffTerm, bullets []string) []RephraseSuggestion {
	<-b.release
	return b.counter.SuggestAll(ctx, missingRequired, bullets)
}
