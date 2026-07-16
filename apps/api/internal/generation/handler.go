package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
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
		slog.Error("generation failed", "jobId", payload.JobID, "type", payload.Type, "error", err)
		return err
	}
	slog.Info("generation complete", "jobId", payload.JobID, "type", payload.Type, "documentId", doc.ID, "version", doc.Version)
	return nil
}
