package queue

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type fakeActivityStore struct {
	mu             sync.Mutex
	timeoutMs      *int32
	timedOutErr    *string
	heartbeatCalls int
	finishCalled   bool
}

func (f *fakeActivityStore) InsertActivityRun(ctx context.Context, arg sqlcgen.InsertActivityRunParams) (sqlcgen.ActivityRun, error) {
	return sqlcgen.ActivityRun{}, nil
}
func (f *fakeActivityStore) StartActivityRun(ctx context.Context, id pgtype.UUID) error { return nil }
func (f *fakeActivityStore) SetActivityStep(ctx context.Context, arg sqlcgen.SetActivityStepParams) error {
	return nil
}
func (f *fakeActivityStore) FinishActivityRunOk(ctx context.Context, arg sqlcgen.FinishActivityRunOkParams) error {
	return nil
}
func (f *fakeActivityStore) FinishActivityRunError(ctx context.Context, arg sqlcgen.FinishActivityRunErrorParams) error {
	return nil
}
func (f *fakeActivityStore) FinishActivityRunCancelled(ctx context.Context, arg sqlcgen.FinishActivityRunCancelledParams) error {
	return nil
}
func (f *fakeActivityStore) FinishActivityRunTimedOut(ctx context.Context, arg sqlcgen.FinishActivityRunTimedOutParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finishCalled = true
	f.timedOutErr = arg.Error
	return nil
}
func (f *fakeActivityStore) TouchActivityRunHeartbeat(ctx context.Context, id pgtype.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeatCalls++
	return nil
}
func (f *fakeActivityStore) SetActivityRunTimeout(ctx context.Context, arg sqlcgen.SetActivityRunTimeoutParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timeoutMs = arg.TimeoutMs
	return nil
}

func taskWithActivityID(id string) *Task {
	payload, _ := json.Marshal(map[string]string{"jobId": "job-1", "activityId": id})
	return NewTask("match", payload)
}

func TestGate_AdmitsConcurrentlyAndBlocksNext(t *testing.T) {
	policy := TaskPolicy{Concurrency: 2}
	g := NewGate(policy)

	release1, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	release2, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire 2: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := g.Acquire(ctx); err == nil {
		t.Error("expected third acquire to block/fail while two slots are held")
	}

	release1()
	release2()
}

func TestGate_AdmitsOnlyConfiguredCount(t *testing.T) {
	policy := TaskPolicy{Concurrency: 1}
	g := NewGate(policy)

	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := g.Acquire(ctx); err == nil {
		t.Error("expected second acquire to block while concurrency is 1")
	}
	release()
}

func TestGate_WaitingAcquireHonoursCtxCancellation(t *testing.T) {
	policy := TaskPolicy{Concurrency: 1}
	g := NewGate(policy)

	release, err := g.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := g.Acquire(ctx); err == nil {
		t.Error("expected Acquire to return an error for an already-cancelled ctx")
	}
}

const testActivityID = "11111111-1111-1111-1111-111111111111"

func TestDeadlineMiddleware_ExceedingMaxDurationFinalizesTimedOut(t *testing.T) {
	store := &fakeActivityStore{}
	policy := TaskPolicy{MaxDuration: 30 * time.Millisecond}
	dm := NewDeadlineMiddleware(policy, store, 5*time.Millisecond)

	downstreamEnqueued := false
	handler := func(ctx context.Context, t *Task) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			downstreamEnqueued = true
			return nil
		}
	}
	wrapped := dm.Middleware(handler)

	err := wrapped(context.Background(), taskWithActivityID(testActivityID))
	if !errors.Is(err, ErrDeadlineExceeded) {
		t.Fatalf("expected ErrDeadlineExceeded, got %v", err)
	}
	if downstreamEnqueued {
		t.Error("handler's downstream work must not complete after the deadline")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.finishCalled {
		t.Fatal("expected FinishActivityRunTimedOut to be called")
	}
	if store.timedOutErr == nil || !strings.Contains(*store.timedOutErr, "timed out after") || !strings.Contains(*store.timedOutErr, "limit") {
		t.Errorf("expected reason string with elapsed and limit, got %v", store.timedOutErr)
	}
	if store.timeoutMs == nil || *store.timeoutMs != 30 {
		t.Errorf("expected timeoutMs 30, got %v", store.timeoutMs)
	}
}

func TestDeadlineMiddleware_SuccessWithinDeadline(t *testing.T) {
	store := &fakeActivityStore{}
	policy := TaskPolicy{MaxDuration: 200 * time.Millisecond}
	dm := NewDeadlineMiddleware(policy, store, 5*time.Millisecond)

	handler := func(ctx context.Context, t *Task) error { return nil }
	wrapped := dm.Middleware(handler)

	if err := wrapped(context.Background(), taskWithActivityID(testActivityID)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.finishCalled {
		t.Error("FinishActivityRunTimedOut must not be called on success")
	}
}

func TestDeadlineMiddleware_HeartbeatTicksWhileRunning(t *testing.T) {
	store := &fakeActivityStore{}
	policy := TaskPolicy{MaxDuration: 500 * time.Millisecond}
	dm := NewDeadlineMiddleware(policy, store, 10*time.Millisecond)

	handler := func(ctx context.Context, t *Task) error {
		time.Sleep(60 * time.Millisecond)
		return nil
	}
	wrapped := dm.Middleware(handler)
	if err := wrapped(context.Background(), taskWithActivityID(testActivityID)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.heartbeatCalls == 0 {
		t.Error("expected at least one heartbeat tick during a 60ms handler with a 10ms interval")
	}
}

func TestGate_Middleware_AcquiresAndReleases(t *testing.T) {
	policy := TaskPolicy{Concurrency: 1}
	g := NewGate(policy)

	var running int32
	var maxRunning int32
	handler := func(ctx context.Context, task *Task) error {
		n := atomic.AddInt32(&running, 1)
		if n > atomic.LoadInt32(&maxRunning) {
			atomic.StoreInt32(&maxRunning, n)
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return nil
	}
	wrapped := g.Middleware(handler)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = wrapped(context.Background(), nil)
		}()
	}
	wg.Wait()

	if maxRunning > 1 {
		t.Errorf("expected at most 1 concurrent handler execution, got %d", maxRunning)
	}
}
