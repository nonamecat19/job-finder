package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"golang.org/x/sync/semaphore"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/platform/llm"
)

// ClassResolver resolves the current provider class for a task at admission
// time (019-ai-job-throughput). Satisfied directly by *llm.Router. nil for
// non-LLM task types (ingest, enrich), which always use the local semaphore.
type ClassResolver interface {
	ProviderClass() llm.ProviderClass
}

// Gate is the per-task-type admission gate (data-model.md §4): two counting
// semaphores, one per provider class, sized from the task's TaskPolicy.
// Acquisition is ctx-aware so a task waiting for a slot still honours its
// deadline and shutdown. Class is resolved once per task and does not
// change mid-flight, even if settings flip while the task is queued for a
// slot.
type Gate struct {
	hosted   *semaphore.Weighted
	local    *semaphore.Weighted
	resolver ClassResolver
}

// NewGate builds a Gate sized from policy. resolver is nil for task types
// with no LLMTaskKey (ingest, enrich): they bypass class resolution
// entirely and always acquire from the local semaphore (T017), which for
// those task types is sized to the single configured concurrency.
func NewGate(policy TaskPolicy, resolver ClassResolver) *Gate {
	return &Gate{
		hosted:   semaphore.NewWeighted(int64(policy.HostedConcurrency)),
		local:    semaphore.NewWeighted(int64(policy.LocalConcurrency)),
		resolver: resolver,
	}
}

// Acquire blocks until a slot is free for the resolved class, or ctx is
// done. The returned release func must be called exactly once.
func (g *Gate) Acquire(ctx context.Context) (release func(), err error) {
	sem := g.local
	if g.resolver != nil && g.resolver.ProviderClass() == llm.ProviderClassHosted {
		sem = g.hosted
	}
	if err := sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	return func() { sem.Release(1) }, nil
}

// Middleware wraps an asynq handler with the admission gate: acquire before
// the handler runs, release unconditionally on return.
func (g *Gate) Middleware(handler func(context.Context, *asynq.Task) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		release, err := g.Acquire(ctx)
		if err != nil {
			return err
		}
		defer release()
		return handler(ctx, t)
	}
}

// ErrDeadlineExceeded is returned when a task's own context deadline
// (from its TaskPolicy.MaxDuration) elapses. The run is already finalized
// timed_out by the time this is returned, so asynq retrying the task is
// safe — it simply starts the run again (019-ai-job-throughput, FR-008).
var ErrDeadlineExceeded = errors.New("queue: task exceeded its deadline")

// payloadActivityID extracts the "activityId" field common to every AI task
// payload (queue.MatchPayload, EnrichPayload, ..., IngestPayload), without
// needing a per-task-type payload type.
func payloadActivityID(payload []byte) *string {
	var p struct {
		ActivityID *string `json:"activityId"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil
	}
	return p.ActivityID
}

// DeadlineMiddleware wraps a handler with a per-task-type context.WithTimeout
// and a heartbeat ticker (data-model.md §1, research.md R4). It records
// timeoutMs at admission and finalizes the run timed_out in-process on
// ctx.DeadlineExceeded, with elapsed time recorded — no downstream enqueue
// happens because the handler's own ctx-bound work fails first.
type DeadlineMiddleware struct {
	policy            TaskPolicy
	store             activity.Store
	heartbeatInterval time.Duration
}

func NewDeadlineMiddleware(policy TaskPolicy, store activity.Store, heartbeatInterval time.Duration) *DeadlineMiddleware {
	return &DeadlineMiddleware{policy: policy, store: store, heartbeatInterval: heartbeatInterval}
}

func (d *DeadlineMiddleware) Middleware(handler func(context.Context, *asynq.Task) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		var rec *activity.Recorder
		if id := payloadActivityID(t.Payload()); id != nil && *id != "" {
			rec = activity.FromID(d.store, *id)
		}
		if rec != nil {
			rec.SetTimeout(ctx, int32(d.policy.MaxDuration.Milliseconds()))
		}

		taskCtx, cancel := context.WithTimeout(ctx, d.policy.MaxDuration)
		defer cancel()

		if rec != nil {
			stop := startHeartbeat(taskCtx, rec, d.heartbeatInterval)
			defer stop()
		}

		started := time.Now()
		err := handler(taskCtx, t)
		if err != nil && errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
			if rec != nil {
				rec.TimedOut(context.Background(), time.Since(started), d.policy.MaxDuration)
			}
			return ErrDeadlineExceeded
		}
		return err
	}
}

func startHeartbeat(ctx context.Context, rec *activity.Recorder, interval time.Duration) (stop func()) {
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rec.Heartbeat(context.Background())
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { close(done) }
}
