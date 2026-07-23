// Package worker holds the ghost-job bounded context's inbound worker
// adapter: the asynq "ghost:score" task handler.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/ghostjob/application"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/queue"
)

// Handler processes "ghost:score" asynq tasks, mirroring matching.Handler /
// salary.Handler. Triggered by ingestion (handler.go's enqueueGhostScore)
// and by the manual POST /api/jobs/{id}/ghost-score endpoint only — no
// scheduled or background re-scoring path exists anywhere (FR-014).
type Handler struct {
	svc   *application.Service
	store activity.Store
}

func NewHandler(svc *application.Service, store activity.Store) *Handler {
	return &Handler{svc: svc, store: store}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.GhostScorePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("ghostjob: invalid payload: %w", err)
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.store, *payload.ActivityID)
		rec.Start(ctx)
	}

	_, err := h.svc.ScoreJob(ctx, payload.JobID)
	if err != nil {
		if errors.Is(err, application.ErrDeclinedToScore) {
			// Not an error: every signal was unknown, so the service
			// correctly declined rather than guessing (SC-003). A
			// scoring "failure" for one job must never affect another
			// job or the ingestion run (FR-018), so this always returns
			// nil to asynq — there is nothing to retry.
			slog.Info("ghostjob: declined to score (insufficient signal)", "job", payload.JobID)
			if rec != nil {
				rec.Ok(ctx, "", map[string]any{"declined": true})
			}
			return nil
		}
		if errors.Is(err, llm.ErrRateLimited) {
			slog.Warn("ghostjob: cancelled: cerebras rate limited", "job", payload.JobID)
			if rec != nil {
				rec.Cancel(ctx, err.Error())
			}
			return nil
		}
		slog.Error("ghostjob: scoring failed", "job", payload.JobID, "error", err)
		if rec != nil {
			rec.Fail(ctx, err)
		}
		return nil
	}

	slog.Info("ghostjob: scoring complete", "job", payload.JobID)
	if rec != nil {
		rec.Ok(ctx, "", nil)
	}
	return nil
}
