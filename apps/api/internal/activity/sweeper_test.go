package activity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type fakeSweeperStore struct {
	sweptRows      []sqlcgen.ActivityRun
	sweptReason    *string
	sweptCutoff    time.Time
	staleQueued    []sqlcgen.ActivityRun
	queuedCutoff   time.Time
	interruptedIDs []pgtype.UUID
	prunedCutoff   time.Time
	prunedCount    int64
}

func (f *fakeSweeperStore) SweepStaleRunningActivityRuns(ctx context.Context, arg sqlcgen.SweepStaleRunningActivityRunsParams) ([]sqlcgen.ActivityRun, error) {
	f.sweptReason = arg.Reason
	f.sweptCutoff = arg.Cutoff.Time
	return f.sweptRows, nil
}

func (f *fakeSweeperStore) ListStaleQueuedActivityRuns(ctx context.Context, cutoff pgtype.Timestamp) ([]sqlcgen.ActivityRun, error) {
	f.queuedCutoff = cutoff.Time
	return f.staleQueued, nil
}

func (f *fakeSweeperStore) FinishActivityRunInterrupted(ctx context.Context, arg sqlcgen.FinishActivityRunInterruptedParams) error {
	f.interruptedIDs = append(f.interruptedIDs, arg.ID)
	return nil
}

func (f *fakeSweeperStore) PruneIdempotencyLedger(ctx context.Context, acceptedAt pgtype.Timestamptz) (int64, error) {
	f.prunedCutoff = acceptedAt.Time
	return f.prunedCount, nil
}

func idFor(n byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[15] = n
	u.Valid = true
	return u
}

func TestSweeper_SweepsRunningWithCorrectCutoffAndReason(t *testing.T) {
	store := &fakeSweeperStore{sweptRows: []sqlcgen.ActivityRun{{ID: idFor(1)}}}
	staleAfter := 2 * time.Minute
	sweeper := NewSweeper(store, staleAfter, time.Minute, 30*time.Minute)

	before := time.Now()
	sweeper.sweepOnce(context.Background())
	after := time.Now()

	if store.sweptReason == nil {
		t.Fatal("expected a reason string to be passed")
	}
	wantCutoffMin := before.Add(-staleAfter)
	wantCutoffMax := after.Add(-staleAfter)
	if store.sweptCutoff.Before(wantCutoffMin) || store.sweptCutoff.After(wantCutoffMax) {
		t.Errorf("cutoff %v not within expected window [%v, %v]", store.sweptCutoff, wantCutoffMin, wantCutoffMax)
	}
}

// TestSweeper_StaleQueuedRowIsInterrupted covers T043/047: RabbitMQ has no
// broker-native "does this task still exist" query the way asynq's
// Inspector did, so every row past ACTIVITY_QUEUED_GRACE is interrupted —
// the grace window itself is what protects a genuinely-still-queued run.
func TestSweeper_StaleQueuedRowIsInterrupted(t *testing.T) {
	taskID := "task-2"
	store := &fakeSweeperStore{
		staleQueued: []sqlcgen.ActivityRun{{ID: idFor(3), Op: "match", QueueTaskId: &taskID}},
	}
	sweeper := NewSweeper(store, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	if len(store.interruptedIDs) != 1 {
		t.Fatalf("expected exactly one interrupted row, got %d", len(store.interruptedIDs))
	}
}

func TestSweeper_QueuedRowWithNoTaskIDIsInterrupted(t *testing.T) {
	store := &fakeSweeperStore{
		staleQueued: []sqlcgen.ActivityRun{{ID: idFor(4), Op: "match", QueueTaskId: nil}},
	}
	sweeper := NewSweeper(store, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	if len(store.interruptedIDs) != 1 {
		t.Fatalf("expected the no-task-id row to be interrupted, got %d", len(store.interruptedIDs))
	}
}

func TestSweeper_RunSweepsOnceImmediatelyThenStopsOnCtxDone(t *testing.T) {
	store := &fakeSweeperStore{}
	sweeper := NewSweeper(store, 2*time.Minute, 5*time.Millisecond, 30*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after ctx cancellation")
	}
}

func TestSweeper_CutoffsAreUTCWallClock(t *testing.T) {
	store := &fakeSweeperStore{}
	sweeper := NewSweeper(store, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	nowUTC := time.Now().UTC()
	for name, got := range map[string]time.Time{
		"running": store.sweptCutoff,
		"queued":  store.queuedCutoff,
	} {
		if got.After(nowUTC) {
			t.Errorf("%s cutoff wall clock %v is ahead of UTC now %v — local time leaked into a naive timestamp", name, got, nowUTC)
		}
	}
}

func TestSweeper_PrunesIdempotencyLedgerAtLongestRetryBudgetPlusMargin(t *testing.T) {
	store := &fakeSweeperStore{}
	sweeper := NewSweeper(store, 2*time.Minute, time.Minute, 30*time.Minute)

	before := time.Now()
	sweeper.sweepOnce(context.Background())
	after := time.Now()

	wantMin := before.Add(-IdempotencyLedgerRetention)
	wantMax := after.Add(-IdempotencyLedgerRetention)
	if store.prunedCutoff.Before(wantMin) || store.prunedCutoff.After(wantMax) {
		t.Errorf("prune cutoff %v not within expected window [%v, %v]", store.prunedCutoff, wantMin, wantMax)
	}
}
