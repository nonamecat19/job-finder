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
	"github.com/job-finder/api/internal/dbutil"
)

type Inspector interface {
	GetTaskInfo(queue, id string) (*asynq.TaskInfo, error)
}

type SweeperStore interface {
	SweepStaleRunningActivityRuns(ctx context.Context, arg sqlcgen.SweepStaleRunningActivityRunsParams) ([]sqlcgen.ActivityRun, error)
	ListStaleQueuedActivityRuns(ctx context.Context, cutoff pgtype.Timestamp) ([]sqlcgen.ActivityRun, error)
	FinishActivityRunInterrupted(ctx context.Context, arg sqlcgen.FinishActivityRunInterruptedParams) error
	PruneIdempotencyLedger(ctx context.Context, acceptedAt pgtype.Timestamptz) (int64, error)
}

// LongestRetryBudget is the sum of the fixed backoff ladder rungs
// (data-model.md § 6, internal/events/retry.go): 1s + 10s + 1m + 10m. It
// bounds how long after a work event's first attempt a legitimate
// redelivered result can still arrive.
const LongestRetryBudget = time.Second + 10*time.Second + time.Minute + 10*time.Minute

// IdempotencyLedgerRetention is how long a ledger row (internal/events,
// data-model.md § 7) is kept before pruning: the longest retry budget plus a
// generous margin for operational delay (a broker or consumer outage between
// attempts). The ledger is a short-horizon dedupe window, not an audit log.
const IdempotencyLedgerRetention = LongestRetryBudget + 24*time.Hour

var queueForOp = map[string]string{
	"ingest":       "ingest",
	"match":        "match",
	"generate":     "generate",
	"enrich":       "enrich",
	"ghost_score":  "ghost:score",
	"salary_infer": "salary:infer",
}

type Sweeper struct {
	store           SweeperStore
	inspector       Inspector
	staleAfter      time.Duration
	sweepInterval   time.Duration
	queuedGrace     time.Duration
	ledgerRetention time.Duration
}

func NewSweeper(store SweeperStore, inspector Inspector, staleAfter, sweepInterval, queuedGrace time.Duration) *Sweeper {
	return &Sweeper{
		store:           store,
		inspector:       inspector,
		staleAfter:      staleAfter,
		sweepInterval:   sweepInterval,
		queuedGrace:     queuedGrace,
		ledgerRetention: IdempotencyLedgerRetention,
	}
}

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
	s.pruneLedger(ctx)
}

// pruneLedger deletes idempotency ledger rows older than ledgerRetention
// (T031, data-model.md § 7). The ledger is a short-horizon dedupe window,
// not an audit log, so pruning is unconditional — no soft-delete, no
// archival.
func (s *Sweeper) pruneLedger(ctx context.Context) {
	cutoff := time.Now().Add(-s.ledgerRetention)
	n, err := s.store.PruneIdempotencyLedger(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
	if err != nil {
		slog.Error("activity: prune idempotency ledger failed", "error", err)
		return
	}
	if n > 0 {
		slog.Info("activity: pruned idempotency ledger", "count", n)
	}
}

func (s *Sweeper) sweepRunning(ctx context.Context) {
	cutoff := time.Now().Add(-s.staleAfter)
	reason := fmt.Sprintf("interrupted: no worker heartbeat for at least %s", s.staleAfter)
	rows, err := s.store.SweepStaleRunningActivityRuns(ctx, sqlcgen.SweepStaleRunningActivityRunsParams{
		Reason: &reason,
		Cutoff: dbutil.TimestampAt(cutoff),
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
	rows, err := s.store.ListStaleQueuedActivityRuns(ctx, dbutil.TimestampAt(cutoff))
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
		slog.Warn("activity: inspector lookup failed, assuming task still live", "queue", qname, "id", *row.QueueTaskId, "error", err)
		return true
	}
	return true
}
