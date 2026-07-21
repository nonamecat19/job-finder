package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/queue"
)

// Handler processes "match" asynq tasks, mirroring matching.processor.ts
// (concurrency 1: local LLM handles one request at a time comfortably —
// enforced by the asynq server's queue concurrency configuration in main).
type Handler struct {
	svc      *Service
	notifier *notifier.Service
}

func NewHandler(svc *Service, notifier *notifier.Service) *Handler {
	return &Handler{svc: svc, notifier: notifier}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) (err error) {
	var payload queue.MatchPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("matching: invalid payload: %w", err)
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

	result, err := h.svc.MatchJob(ctx, payload.JobID, rec)
	if err != nil {
		if errors.Is(err, ErrNoProfileConfig) {
			slog.Warn("matching skipped: no profile config", "jobId", payload.JobID)
			if rec != nil {
				rec.Fail(ctx, err)
			}
			return nil
		}
		slog.Error("matching job failed", "jobId", payload.JobID, "error", err)
		return err
	}
	slog.Info("matching complete", "jobId", payload.JobID, "score", result.Score, "similarity", result.Similarity)

	if result.Score != nil {
		h.notifier.MaybeNotify(ctx, payload.JobID, result.ID, *result.Score)
	}

	return nil
}
