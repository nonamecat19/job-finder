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

type ClassResolver interface {
	ProviderClass() llm.ProviderClass
}

type Gate struct {
	hosted   *semaphore.Weighted
	local    *semaphore.Weighted
	resolver ClassResolver
}

func NewGate(policy TaskPolicy, resolver ClassResolver) *Gate {
	return &Gate{
		hosted:   semaphore.NewWeighted(int64(policy.HostedConcurrency)),
		local:    semaphore.NewWeighted(int64(policy.LocalConcurrency)),
		resolver: resolver,
	}
}

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
