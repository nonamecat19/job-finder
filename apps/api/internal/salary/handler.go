package salary

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/queue"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.SalaryInferPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("salary: invalid payload: %w", err)
	}

	if err := h.svc.Infer(ctx, payload.JobID); err != nil {
		slog.Error("salary: inference failed", "job", payload.JobID, "error", err)
		return err
	}

	slog.Info("salary: inference complete", "job", payload.JobID)
	return nil
}
