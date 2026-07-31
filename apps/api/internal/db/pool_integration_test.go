//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestPoolSaturationFailsFast is the behaviour SC-001 and FR-008a depend on:
// with every connection held, a further acquire must fail on its own deadline
// rather than blocking indefinitely, and must succeed again as soon as one is
// returned. Run against real Postgres — a mocked pool would only prove that
// the mock was written to agree.
func TestPoolSaturationFailsFast(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgresql://jobfinder:jobfinder@localhost:5432/jobfinder"
	}

	ctx := context.Background()
	pool, err := Open(ctx, dsn, WithPoolConfig(PoolConfig{
		MaxConns:        2,
		MinConns:        1,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
	}))
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	defer pool.Close()

	if got := pool.Pool.Stat().MaxConns(); got != 2 {
		t.Fatalf("MaxConns = %d, want 2: WithPoolConfig was not applied", got)
	}

	first, err := pool.Pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire first: %v", err)
	}
	second, err := pool.Pool.Acquire(ctx)
	if err != nil {
		first.Release()
		t.Fatalf("acquire second: %v", err)
	}

	stats := pool.PoolStats()
	if !stats.Saturated {
		t.Errorf("PoolStats().Saturated = false with %d/%d acquired, want true", stats.AcquiredConns, stats.MaxConns)
	}

	const acquireTimeout = 500 * time.Millisecond
	deadlineCtx, cancel := context.WithTimeout(ctx, acquireTimeout)
	defer cancel()

	start := time.Now()
	extra, err := pool.Pool.Acquire(deadlineCtx)
	elapsed := time.Since(start)
	if err == nil {
		extra.Release()
		first.Release()
		second.Release()
		t.Fatal("third acquire succeeded against a pool of 2 with both connections held")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		first.Release()
		second.Release()
		t.Fatalf("third acquire failed with %v, want context.DeadlineExceeded", err)
	}
	// The point of the bound is that the wait ends; a generous ceiling here
	// keeps the assertion about "bounded", not about scheduler precision.
	if elapsed > 5*acquireTimeout {
		t.Errorf("third acquire blocked for %s, want ~%s", elapsed, acquireTimeout)
	}

	// Releasing one connection must make the pool usable again.
	second.Release()
	recoverCtx, cancelRecover := context.WithTimeout(ctx, 2*time.Second)
	defer cancelRecover()
	reacquired, err := pool.Pool.Acquire(recoverCtx)
	if err != nil {
		first.Release()
		t.Fatalf("acquire after release: %v", err)
	}
	reacquired.Release()
	first.Release()
}
