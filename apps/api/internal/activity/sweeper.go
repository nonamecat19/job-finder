package activity

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
)

// Inspector is the subset of *asynq.Inspector the sweeper needs to confirm
// whether a queued row's task still exists before closing it out.
type Inspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
}

// SweeperStore is the persistence port the sweeper depends on.
type SweeperStore interface {
	SweepStaleRunningActivityRuns(ctx context.Context, arg sqlcgen.SweepStaleRunningActivityRunsParams) ([]sqlcgen.ActivityRun, error)
	ListStaleQueuedActivityRuns(ctx context.Context, cutoff pgtype.Timestamp) ([]sqlcgen.ActivityRun, error)
	FinishActivityRunInterrupted(ctx context.Context, arg sqlcgen.FinishActivityRunInterruptedParams) error
}

// queueForOp maps an ActivityRun.Op back to its asynq queue, mirroring
// httpapi.queueForOp — needed here to look up a queued row's task by id.
var queueForOp = map[string]string{
	"ingest":       "ingest",
	"match":        "match",
	"generate":     "generate",
	"enrich":       "enrich",
	"ghost_score":  "ghost:score",
	"salary_infer": "salary:infer",
}

// Sweeper runs once at startup then every interval (019-ai-job-throughput,
// research.md R4): closes out ActivityRun rows whose worker vanished
// (stale/null heartbeat while running) or whose queued task no longer
// exists, as "interrupted". Never touches a terminal row — the underlying
// queries filter to 'running'/'queued' — so a run that finished between
// sweep read and write is never re-opened.
type Sweeper struct {
	store         SweeperStore
	inspector     Inspector
	staleAfter    time.Duration
	sweepInterval time.Duration
	queuedGrace   time.Duration
}

func NewSweeper(store SweeperStore, inspector Inspector, staleAfter, sweepInterval, queuedGrace time.Duration) *Sweeper {
	return &Sweeper{
		store:         store,
		inspector:     inspector,
		staleAfter:    staleAfter,
		sweepInterval: sweepInterval,
		queuedGrace:   queuedGrace,
	}
}

// Run sweeps once immediately, then every s.sweepInterval, until ctx is done.
func (s *Sweeper) Run(ctx context.Context) {
	s.sweepOnce(ctx)
	ticker := time.NewTicker(s.sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.sweepOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Sweeper) sweepOnce(ctx context.Context) {
	s.sweepRunning(ctx)
	s.sweepQueued(ctx)
}

func (s *Sweeper) sweepRunning(ctx context.Context) {
	cutoff := time.Now().Add(-s.staleAfter)
	reason := fmt.Sprintf("interrupted: no worker heartbeat for at least %s", s.staleAfter)
	rows, err := s.store.SweepStaleRunningActivityRuns(ctx, sqlcgen.SweepStaleRunningActivityRunsParams{
		Reason: &reason,
		Cutoff: pgtype.Timestamp{Time: cutoff, Valid: true},
	})
	if err != nil {
		slog.Error("activity: sweep running failed", "error", err)
		return
	}
	if len(rows) > 0 {
		slog.Info("activity: swept stale running runs", "count", len(rows))
	}
}

func (s *Sweeper) sweepQueued(ctx context.Context) {
	cutoff := time.Now().Add(-s.queuedGrace)
	rows, err := s.store.ListStaleQueuedActivityRuns(ctx, pgtype.Timestamp{Time: cutoff, Valid: true})
	if err != nil {
		slog.Error("activity: list stale queued failed", "error", err)
		return
	}
	for _, row := range rows {
		if s.queuedTaskStillExists(row) {
			continue
		}
		reason := "interrupted: queued task no longer exists"
		if err := s.store.FinishActivityRunInterrupted(ctx, sqlcgen.FinishActivityRunInterruptedParams{
			ID:    row.ID,
			Error: &reason,
		}); err != nil {
			slog.Error("activity: finish interrupted (queued) failed", "error", err)
		}
	}
}

func (s *Sweeper) queuedTaskStillExists(row sqlcgen.ActivityRun) bool {
	if row.QueueTaskId == nil || *row.QueueTaskId == "" {
		return false
	}
	qname, ok := queueForOp[row.Op]
	if !ok {
		return false
	}
	_, err := s.inspector.GetTaskInfo(qname, *row.QueueTaskId)
	if err != nil {
		if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
			return false
		}
		// Unexpected Inspector error (e.g. Redis hiccup): assume the task is
		// still live rather than closing out a run on flaky infrastructure.
		slog.Warn("activity: inspector lookup failed, assuming task still live", "queue", qname, "id", *row.QueueTaskId, "error", err)
		return true
	}
	return true
}
