package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

type fakeSweeperStore struct {
	sweptRows      []sqlcgen.ActivityRun
	sweptReason    *string
	sweptCutoff    time.Time
	staleQueued    []sqlcgen.ActivityRun
	interruptedIDs []pgtype.UUID
}

func (f *fakeSweeperStore) SweepStaleRunningActivityRuns(ctx context.Context, arg sqlcgen.SweepStaleRunningActivityRunsParams) ([]sqlcgen.ActivityRun, error) {
	f.sweptReason = arg.Reason
	f.sweptCutoff = arg.Cutoff.Time
	return f.sweptRows, nil
}

func (f *fakeSweeperStore) ListStaleQueuedActivityRuns(ctx context.Context, cutoff pgtype.Timestamp) ([]sqlcgen.ActivityRun, error) {
	return f.staleQueued, nil
}

func (f *fakeSweeperStore) FinishActivityRunInterrupted(ctx context.Context, arg sqlcgen.FinishActivityRunInterruptedParams) error {
	f.interruptedIDs = append(f.interruptedIDs, arg.ID)
	return nil
}

type fakeInspector struct {
	exists map[string]bool // keyed "queue/id"
}

func (f *fakeInspector) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	if f.exists[queue+"/"+id] {
		return &asynq.TaskInfo{}, nil
	}
	return nil, asynq.ErrTaskNotFound
}

func idFor(n byte) pgtype.UUID {
	var u pgtype.UUID
	u.Bytes[15] = n
	u.Valid = true
	return u
}

// TestSweeper_MarksStaleRunningInterrupted: the DB-side filtering (only
// 'running' rows past the cutoff or with a null heartbeat) is exercised by
// the integration test; this unit test verifies the sweeper issues the
// query with the right cutoff/reason and reacts to whatever rows come back.
func TestSweeper_SweepsRunningWithCorrectCutoffAndReason(t *testing.T) {
	store := &fakeSweeperStore{sweptRows: []sqlcgen.ActivityRun{{ID: idFor(1)}}}
	insp := &fakeInspector{}
	staleAfter := 2 * time.Minute
	sweeper := NewSweeper(store, insp, staleAfter, time.Minute, 30*time.Minute)

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

func TestSweeper_QueuedTaskStillLiveIsNotInterrupted(t *testing.T) {
	taskID := "task-1"
	store := &fakeSweeperStore{
		staleQueued: []sqlcgen.ActivityRun{{ID: idFor(2), Op: "match", QueueTaskId: &taskID}},
	}
	insp := &fakeInspector{exists: map[string]bool{"match/task-1": true}}
	sweeper := NewSweeper(store, insp, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	if len(store.interruptedIDs) != 0 {
		t.Errorf("expected no interrupted rows for a live task, got %d", len(store.interruptedIDs))
	}
}

func TestSweeper_QueuedTaskGoneIsInterrupted(t *testing.T) {
	taskID := "task-2"
	store := &fakeSweeperStore{
		staleQueued: []sqlcgen.ActivityRun{{ID: idFor(3), Op: "match", QueueTaskId: &taskID}},
	}
	insp := &fakeInspector{} // no tasks exist
	sweeper := NewSweeper(store, insp, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	if len(store.interruptedIDs) != 1 {
		t.Fatalf("expected exactly one interrupted row, got %d", len(store.interruptedIDs))
	}
}

func TestSweeper_QueuedRowWithNoTaskIDIsInterrupted(t *testing.T) {
	store := &fakeSweeperStore{
		staleQueued: []sqlcgen.ActivityRun{{ID: idFor(4), Op: "match", QueueTaskId: nil}},
	}
	insp := &fakeInspector{}
	sweeper := NewSweeper(store, insp, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	if len(store.interruptedIDs) != 1 {
		t.Fatalf("expected the no-task-id row to be interrupted, got %d", len(store.interruptedIDs))
	}
}

func TestSweeper_InspectorErrorAssumesTaskLive(t *testing.T) {
	taskID := "task-3"
	store := &fakeSweeperStore{
		staleQueued: []sqlcgen.ActivityRun{{ID: idFor(5), Op: "match", QueueTaskId: &taskID}},
	}
	insp := &erroringInspector{err: errors.New("redis: connection refused")}
	sweeper := NewSweeper(store, insp, 2*time.Minute, time.Minute, 30*time.Minute)

	sweeper.sweepOnce(context.Background())

	if len(store.interruptedIDs) != 0 {
		t.Errorf("expected no interruption on unexpected Inspector error, got %d", len(store.interruptedIDs))
	}
}

type erroringInspector struct{ err error }

func (e *erroringInspector) GetTaskInfo(queue, id string) (*asynq.TaskInfo, error) {
	return nil, e.err
}

func TestSweeper_RunSweepsOnceImmediatelyThenStopsOnCtxDone(t *testing.T) {
	store := &fakeSweeperStore{}
	insp := &fakeInspector{}
	sweeper := NewSweeper(store, insp, 2*time.Minute, 5*time.Millisecond, 30*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()

	// Give it time for the immediate sweep plus a tick or two, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after ctx cancellation")
	}
}
