package matching

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api-go/internal/queue"
)

// Handler processes "match" asynq tasks, mirroring matching.processor.ts
// (concurrency 1: local LLM handles one request at a time comfortably —
// enforced by the asynq server's queue concurrency configuration in main).
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.MatchPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("matching: invalid payload: %w", err)
	}
	result, err := h.svc.MatchJob(ctx, payload.JobID)
	if err != nil {
		slog.Error("matching job failed", "jobId", payload.JobID, "error", err)
		return err
	}
	slog.Info("matching complete", "jobId", payload.JobID, "score", result.Score, "similarity", result.Similarity)
	return nil
}
