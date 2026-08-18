package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/matching/application"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
)

type AutoGenerateGate interface {
	ShouldGenerate(score int) bool
}

type Generator interface {
	EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error)
}

type Handler struct {
	svc       *application.Service
	notifier  *notifier.Service
	autogen   AutoGenerateGate
	generator Generator
}

func NewHandler(svc *application.Service, notifier *notifier.Service, autogen AutoGenerateGate, generator Generator) *Handler {
	return &Handler{svc: svc, notifier: notifier, autogen: autogen, generator: generator}
}

func (h *Handler) ProcessTask(ctx context.Context, t *queue.Task) (err error) {
	var payload queue.MatchPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("matching: invalid payload: %w", err)
	}

	var rec *activity.Recorder
	if payload.ActivityID != nil && *payload.ActivityID != "" {
		rec = activity.FromID(h.svc.Store(), *payload.ActivityID)
	}

	if rec != nil {
		rec.Start(ctx)
	}

	retrying := false
	defer func() {
		if rec != nil {
			if err != nil && !retrying {
				rec.Fail(ctx, err)
			}
		}
	}()

	result, err := h.svc.MatchJob(ctx, payload.JobID, rec)
	if err != nil {
		if errors.Is(err, llm.ErrRateLimited) {
			n, _ := queue.GetRetryCount(ctx)
			max, _ := queue.GetMaxRetry(ctx)
			if n >= max {
				slog.Warn("matching cancelled: upstream rate limited, retries exhausted", "jobId", payload.JobID)
				if rec != nil {
					rec.Cancel(ctx, err.Error())
				}
				return nil
			}
			slog.Warn("matching retrying: upstream rate limited", "jobId", payload.JobID, "attempt", n)
			retrying = true
			return err
		}
		if errors.Is(err, application.ErrNoProfileConfig) {
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

		if h.autogen != nil && h.generator != nil && h.autogen.ShouldGenerate(*result.Score) {
			if _, err := h.generator.EnqueueGeneration(ctx, payload.JobID, "resume", nil); err != nil {
				slog.Warn("matching: auto-generate skipped", "jobId", payload.JobID, "score", *result.Score, "error", err)
			}
		}
	}

	return nil
}
