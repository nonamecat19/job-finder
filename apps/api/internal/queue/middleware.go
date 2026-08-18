package queue

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/platform/llm"
)

// ClassResolver is kept for the backlog DTO (internal/dto/queue_backlog.go),
// even though Gate no longer consults it for admission decisions (044 T027):
// with Ollama gone there is only one concurrency pool, so there is nothing
// left to route between.
type ClassResolver interface {
	ProviderClass() llm.ProviderClass
}

type Gate struct {
	sem *semaphore.Weighted
}

// NewGate no longer takes a ClassResolver: admission is a single pool sized
// by TaskPolicy.Concurrency (044). The resolver argument is kept out of this
// constructor deliberately rather than accepted-and-ignored, so a caller
// cannot believe it still has an effect here.
func NewGate(policy TaskPolicy) *Gate {
	return &Gate{
		sem: semaphore.NewWeighted(int64(policy.Concurrency)),
	}
}

func (g *Gate) Acquire(ctx context.Context) (release func(), err error) {
	if err := g.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	return func() { g.sem.Release(1) }, nil
}

func (g *Gate) Middleware(handler func(context.Context, *Task) error) func(context.Context, *Task) error {
	return func(ctx context.Context, t *Task) error {
		release, err := g.Acquire(ctx)
		if err != nil {
			return err
		}
		defer release()
		return handler(ctx, t)
	}
}

var ErrDeadlineExceeded = errors.New("queue: task exceeded its deadline")

func payloadActivityID(payload []byte) *string {
	var p struct {
		ActivityID *string `json:"activityId"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil
	}
	return p.ActivityID
}

type DeadlineMiddleware struct {
	policy            TaskPolicy
	store             activity.Store
	heartbeatInterval time.Duration
}

func NewDeadlineMiddleware(policy TaskPolicy, store activity.Store, heartbeatInterval time.Duration) *DeadlineMiddleware {
	return &DeadlineMiddleware{policy: policy, store: store, heartbeatInterval: heartbeatInterval}
}

func (d *DeadlineMiddleware) Middleware(handler func(context.Context, *Task) error) func(context.Context, *Task) error {
	return func(ctx context.Context, t *Task) error {
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
