package activity

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

type SweeperStore interface {
	SweepStaleRunningActivityRuns(ctx context.Context, arg sqlcgen.SweepStaleRunningActivityRunsParams) ([]sqlcgen.ActivityRun, error)
	ListStaleQueuedActivityRuns(ctx context.Context, cutoff pgtype.Timestamp) ([]sqlcgen.ActivityRun, error)
	FinishActivityRunInterrupted(ctx context.Context, arg sqlcgen.FinishActivityRunInterruptedParams) error
	PruneIdempotencyLedger(ctx context.Context, acceptedAt pgtype.Timestamptz) (int64, error)
}

const LongestRetryBudget = time.Second + 10*time.Second + time.Minute + 10*time.Minute

const IdempotencyLedgerRetention = LongestRetryBudget + 24*time.Hour

type Sweeper struct {
	store           SweeperStore
	staleAfter      time.Duration
	sweepInterval   time.Duration
	queuedGrace     time.Duration
	ledgerRetention time.Duration
}

func NewSweeper(store SweeperStore, staleAfter, sweepInterval, queuedGrace time.Duration) *Sweeper {
	return &Sweeper{
		store:           store,
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
		reason := "interrupted: queued task no longer exists"
		if err := s.store.FinishActivityRunInterrupted(ctx, sqlcgen.FinishActivityRunInterruptedParams{
			ID:    row.ID,
			Error: &reason,
		}); err != nil {
			slog.Error("activity: finish interrupted (queued) failed", "error", err)
		}
	}
}
