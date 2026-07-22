package ghostjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/queue"
)

// Handler processes "ghost:score" asynq tasks, mirroring matching.Handler /
// salary.Handler. Triggered by ingestion (handler.go's enqueueGhostScore)
// and by the manual POST /api/jobs/{id}/ghost-score endpoint only — no
// scheduled or background re-scoring path exists anywhere (FR-014).
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.GhostScorePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("ghostjob: invalid payload: %w", err)
	}

	_, err := h.svc.ScoreJob(ctx, payload.JobID)
	if err != nil {
		if errors.Is(err, ErrDeclinedToScore) {
			// Not an error: every signal was unknown, so the service
			// correctly declined rather than guessing (SC-003). A
			// scoring "failure" for one job must never affect another
			// job or the ingestion run (FR-018), so this always returns
			// nil to asynq — there is nothing to retry.
			slog.Info("ghostjob: declined to score (insufficient signal)", "job", payload.JobID)
			return nil
		}
		slog.Error("ghostjob: scoring failed", "job", payload.JobID, "error", err)
		return nil
	}

	slog.Info("ghostjob: scoring complete", "job", payload.JobID)
	return nil
}
