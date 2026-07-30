package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/queue"
)

// AutoGenerateGate reports whether a job's score should trigger an
// auto-enqueued resume, per the configurable Settings threshold.
type AutoGenerateGate interface {
	ShouldGenerate(score int) bool
}

// Generator enqueues a "generate" asynq task for a job. *jobs.Service
// satisfies it structurally.
type Generator interface {
	EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error)
}

// Handler processes "match" asynq tasks, mirroring matching.processor.ts
// (concurrency 1: local LLM handles one request at a time comfortably —
// enforced by the asynq server's queue concurrency configuration in main).
type Handler struct {
	svc       *Service
	notifier  *notifier.Service
	autogen   AutoGenerateGate
	generator Generator
}

func NewHandler(svc *Service, notifier *notifier.Service, autogen AutoGenerateGate, generator Generator) *Handler {
	return &Handler{svc: svc, notifier: notifier, autogen: autogen, generator: generator}
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
		if errors.Is(err, llm.ErrRateLimited) {
			// The breaker is already tripped process-wide, so every other
			// queued match task fails this same check and cancels too
			// instead of each burning a request against the exhausted quota.
			slog.Warn("matching cancelled: cerebras rate limited", "jobId", payload.JobID)
			if rec != nil {
				rec.Cancel(ctx, err.Error())
			}
			return nil
		}
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

		if h.autogen != nil && h.generator != nil && h.autogen.ShouldGenerate(*result.Score) {
			if _, err := h.generator.EnqueueGeneration(ctx, payload.JobID, "resume", nil); err != nil {
				// Best-effort: no profile/RenderCV config yet, or another
				// precondition failure. Never affects the match result itself.
				slog.Warn("matching: auto-generate skipped", "jobId", payload.JobID, "score", *result.Score, "error", err)
			}
		}
	}

	return nil
}
