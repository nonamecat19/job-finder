// Package worker holds the salary bounded context's inbound worker adapter:
// the asynq "salary_infer" task handler.
package worker

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
	"github.com/job-finder/api/internal/salary/application"
)

type Handler struct {
	svc   *application.Service
	store activity.Store
}

func NewHandler(svc *application.Service, store activity.Store) *Handler {
	return &Handler{svc: svc, store: store}
}

func (h *Handler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload queue.SalaryInferPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("salary: invalid payload: %w", err)
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.store, *payload.ActivityID)
		rec.Start(ctx)
	}

	if err := h.svc.Infer(ctx, payload.JobID); err != nil {
		if errors.Is(err, llm.ErrRateLimited) {
			slog.Warn("salary: cancelled: cerebras rate limited", "job", payload.JobID)
			if rec != nil {
				rec.Cancel(ctx, err.Error())
			}
			return nil
		}
		slog.Error("salary: inference failed", "job", payload.JobID, "error", err)
		if rec != nil {
			rec.Fail(ctx, err)
		}
		return err
	}

	slog.Info("salary: inference complete", "job", payload.JobID)
	if rec != nil {
		rec.Ok(ctx, "", nil)
	}
	return nil
}
