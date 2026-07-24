package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/aifeature"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/notifier"
	"github.com/job-finder/api/internal/queue"
)

// FeatureGate reports whether a job's score should trigger an AI feature
// (resume/cover-letter generation, salary inference) automatically, per the
// configurable Settings threshold for that feature.
type FeatureGate interface {
	ShouldRun(feature string, score int) bool
}

// Generator enqueues a "generate" asynq task for a job. *jobs.Service
// satisfies it structurally.
type Generator interface {
	EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error)
}

// SalaryEnqueuer enqueues a "salary_infer" asynq task for a job. *jobs.Service
// satisfies it structurally.
type SalaryEnqueuer interface {
	EnqueueSalaryInfer(ctx context.Context, jobID string) error
}

// Handler processes "match" asynq tasks, mirroring matching.processor.ts
// (concurrency 1: local LLM handles one request at a time comfortably —
// enforced by the asynq server's queue concurrency configuration in main).
type Handler struct {
	svc       *Service
	notifier  *notifier.Service
	features  FeatureGate
	generator Generator
	salary    SalaryEnqueuer
}

func NewHandler(svc *Service, notifier *notifier.Service, features FeatureGate, generator Generator, salary SalaryEnqueuer) *Handler {
	return &Handler{svc: svc, notifier: notifier, features: features, generator: generator, salary: salary}
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
			slog.Warn("matching cancelled: llm rate limited", "jobId", payload.JobID, "error", err)
			if rec != nil {
				rec.Cancel(ctx, err.Error())
			}
			return nil
		}
		if llm.Terminal(err) {
			// Bad credential / missing model / no credits: an asynq retry
			// would fail identically, so record the reason and stop.
			slog.Error("matching failed: llm misconfigured", "jobId", payload.JobID, "error", err)
			if rec != nil {
				rec.Fail(ctx, err)
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

		if h.features != nil {
			score := *result.Score
			if h.generator != nil && h.features.ShouldRun(aifeature.Resume, score) {
				if _, err := h.generator.EnqueueGeneration(ctx, payload.JobID, "resume", nil); err != nil {
					// Best-effort: no profile/RenderCV config yet, or another
					// precondition failure. Never affects the match result itself.
					slog.Warn("matching: auto-generate resume skipped", "jobId", payload.JobID, "score", score, "error", err)
				}
			}
			if h.generator != nil && h.features.ShouldRun(aifeature.CoverLetter, score) {
				if _, err := h.generator.EnqueueGeneration(ctx, payload.JobID, "cover_letter", nil); err != nil {
					slog.Warn("matching: auto-generate cover letter skipped", "jobId", payload.JobID, "score", score, "error", err)
				}
			}
			if h.salary != nil && h.features.ShouldRun(aifeature.SalaryInfer, score) {
				if err := h.salary.EnqueueSalaryInfer(ctx, payload.JobID); err != nil {
					slog.Warn("matching: auto salary inference skipped", "jobId", payload.JobID, "score", score, "error", err)
				}
			}
		}
	}

	return nil
}
