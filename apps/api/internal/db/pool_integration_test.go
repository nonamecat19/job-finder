//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/job-finder/api/internal/testinfra"
)

func TestPoolSaturationFailsFast(t *testing.T) {
	ctx := context.Background()

	dsn, err := testinfra.PostgresDSN(ctx)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}

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
	if elapsed > 5*acquireTimeout {
		t.Errorf("third acquire blocked for %s, want ~%s", elapsed, acquireTimeout)
	}

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
