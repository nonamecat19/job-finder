package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/generation/application"
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

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
	var payload queue.GeneratePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("generation: invalid payload: %w", err)
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.store, *payload.ActivityID)
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

	// 042: a workspace run carries its own id and is dispatched to the
	// workspace pipeline instead of the legacy merged-resume path. This is the
	// same wire-nullable additive pattern 020 specified for
	// TailoringDraftID — every payload written before this field existed has
	// it nil and takes the branch below unchanged.
	if payload.GenerationRunID != nil && *payload.GenerationRunID != "" {
		if err = h.svc.StartRun(ctx, *payload.GenerationRunID, rec); err != nil {
			slog.Error("generation workspace run failed", "runId", *payload.GenerationRunID, "error", err)
			return err
		}
		slog.Info("generation workspace run complete", "runId", *payload.GenerationRunID)
		return nil
	}

	doc, err := h.svc.Generate(ctx, payload.JobID, payload.Type, payload.ProfileID, rec)
	if err != nil {
		if errors.Is(err, llm.ErrRateLimited) {
			slog.Warn("generation cancelled: cerebras rate limited", "jobId", payload.JobID, "type", payload.Type)
			if rec != nil {
				rec.Cancel(ctx, err.Error())
			}
			return nil
		}
		slog.Error("generation failed", "jobId", payload.JobID, "type", payload.Type, "error", err)
		return err
	}
	slog.Info("generation complete", "jobId", payload.JobID, "type", payload.Type, "documentId", doc.ID, "version", doc.Version)
	return nil
}
