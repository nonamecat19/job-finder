package application

import (
	"context"
	"github.com/job-finder/api/internal/keyword/domain"
	"sync"
	"testing"
	"time"
)

type countingRephraser struct {
	mu    sync.Mutex
	calls int
}

func (c *countingRephraser) SuggestAll(_ context.Context, missingRequired []domain.DiffTerm, _ []string) []RephraseSuggestion {
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

func syncSpawn(f func()) { f() }

func TestCachedRephraser_FirstCallEmptyThenCached(t *testing.T) {
	inner := &countingRephraser{}
	c := NewCachedRephraser(inner, time.Minute).WithSpawner(syncSpawn)

	terms := []domain.DiffTerm{reqTerm("kubernetes", "Kubernetes")}
	bullets := []string{"Ran Docker in prod"}

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

	terms := []domain.DiffTerm{reqTerm("go", "Go")}
	bullets := []string{"wrote services"}

	c.SuggestAll(context.Background(), terms, bullets)
	if got := c.SuggestAll(context.Background(), terms, bullets); len(got) != 1 {
		t.Fatalf("within ttl = %d, want cached 1", len(got))
	}
	if inner.count() != 1 {
		t.Fatalf("inner calls before expiry = %d, want 1", inner.count())
	}

	now = now.Add(2 * time.Minute)
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

	c.SuggestAll(context.Background(), []domain.DiffTerm{reqTerm("go", "Go")}, []string{"a"})
	c.SuggestAll(context.Background(), []domain.DiffTerm{reqTerm("rust", "Rust")}, []string{"a"})
	c.SuggestAll(context.Background(), []domain.DiffTerm{reqTerm("go", "Go")}, []string{"b"})

	if inner.count() != 3 {
		t.Fatalf("inner calls = %d, want 3 (distinct keys)", inner.count())
	}
}

func TestCachedRephraser_SingleInflight(t *testing.T) {
	release := make(chan struct{})
	inner := &blockingRephraser{release: release, counter: &countingRephraser{}}
	c := NewCachedRephraser(inner, time.Minute)

	terms := []domain.DiffTerm{reqTerm("go", "Go")}
	bullets := []string{"a"}

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

type blockingRephraser struct {
	release chan struct{}
	counter *countingRephraser
}

func (b *blockingRephraser) SuggestAll(ctx context.Context, missingRequired []domain.DiffTerm, bullets []string) []RephraseSuggestion {
	<-b.release
	return b.counter.SuggestAll(ctx, missingRequired, bullets)
}
