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
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
)

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
