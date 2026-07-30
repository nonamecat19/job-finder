package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/queue"
)

const (
	reconcileMinAge  = 30 * time.Minute
	reconcileMaxAge  = 7 * 24 * time.Hour
	reconcileBatch   = 50
)

// ReconcileUnmatched re-enqueues match tasks for jobs that were inserted but
// never received a MatchResult. Returns the number of match tasks enqueued.
func (s *Service) ReconcileUnmatched(ctx context.Context) (int, error) {
	now := time.Now()
	rows, err := s.q.ListJobsMissingMatch(ctx, sqlcgen.ListJobsMissingMatchParams{
		OlderThan: pgtype.Timestamp{Time: now.Add(-reconcileMinAge), Valid: true},
		NewerThan: pgtype.Timestamp{Time: now.Add(-reconcileMaxAge), Valid: true},
		Limit:     reconcileBatch,
	})
	if err != nil {
		return 0, err
	}

	queued := 0
	for _, row := range rows {
		jobID := dbutil.UUIDString(row.ID)

		var actID *string
		rec := activity.New(ctx, s.q, "match", fmt.Sprintf("%s — %s", row.Company, row.Title), &jobID, nil, "")
		if rec != nil {
			idStr := dbutil.UUIDString(rec.ID())
			actID = &idStr
		}

		payload, err := json.Marshal(queue.MatchPayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			continue
		}
		opts := []asynq.Option{asynq.MaxRetry(1), asynq.Queue(queue.QueueMatch)}
		if actID != nil {
			opts = append(opts, asynq.TaskID(*actID))
		}
		if _, err := s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeMatch, payload), opts...); err != nil {
			slog.Warn("ingestion: reconcile enqueue match failed", "job", jobID, "error", err)
			continue
		}
		queued++
	}
	if queued > 0 {
		slog.Info("reconciled jobs with no match result", "count", queued)
	}
	return queued, nil
}
