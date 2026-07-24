package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/queue"
)

// Handler processes "generate" asynq tasks, mirroring generation.processor.ts.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
	var payload queue.GeneratePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("generation: invalid payload: %w", err)
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.svc.q, *payload.ActivityID)
	}

	if rec != nil {
		rec.Start(ctx)
	}

	defer func() {
		if rec != nil {
			if err != nil {
				rec.Fail(ctx, err)
			}
		}
	}()

	doc, err := h.svc.Generate(ctx, payload.JobID, payload.Type, payload.ProfileID, rec)
	if err != nil {
		if errors.Is(err, llm.ErrRateLimited) {
			slog.Warn("generation cancelled: llm rate limited", "jobId", payload.JobID, "type", payload.Type, "error", err)
			if rec != nil {
				rec.Cancel(ctx, err.Error())
			}
			return nil
		}
		if llm.Terminal(err) {
			// Bad credential / missing model / no credits: an asynq retry
			// would fail identically, so record the reason for the operator
			// and stop instead of handing the task back to the queue.
			slog.Error("generation failed: llm misconfigured", "jobId", payload.JobID, "type", payload.Type, "error", err)
			if rec != nil {
				rec.Fail(ctx, err)
			}
			return nil
		}
		slog.Error("generation failed", "jobId", payload.JobID, "type", payload.Type, "error", err)
		return err
	}
	slog.Info("generation complete", "jobId", payload.JobID, "type", payload.Type, "documentId", doc.ID, "version", doc.Version)
	return nil
}
