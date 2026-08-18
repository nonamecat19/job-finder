package queue

import (
	"context"
	"errors"
)

// Task is the payload carrier handlers receive, replacing *asynq.Task now
// that dispatch runs over RabbitMQ (047). It intentionally mirrors asynq's
// Task shape (Type/Payload) so handler bodies needed no further changes
// beyond the type swap.
type Task struct {
	taskType string
	payload  []byte
}

func NewTask(taskType string, payload []byte) *Task {
	return &Task{taskType: taskType, payload: payload}
}

func (t *Task) Type() string    { return t.taskType }
func (t *Task) Payload() []byte { return t.payload }

// ErrSkipRetry marks an error as non-retryable, the same signal asynq.SkipRetry
// gave handlers — wrap it with fmt.Errorf("%w: %w", ErrSkipRetry, err) so the
// consumer classifies the failure as non-retryable (straight to DLQ).
var ErrSkipRetry = errors.New("queue: task is not retryable")

type retryCtxKey struct{}

type retryInfo struct {
	count int
	max   int
}

// ContextWithRetry attaches the current delivery's retry count and its work
// type's retry budget (max retries, i.e. attempts-1) to ctx, replacing
// asynq.GetRetryCount/GetMaxRetry for handlers that inspect their own retry
// state (e.g. to give up early on a rate limit).
func ContextWithRetry(ctx context.Context, count, max int) context.Context {
	return context.WithValue(ctx, retryCtxKey{}, retryInfo{count: count, max: max})
}

func GetRetryCount(ctx context.Context) (int, bool) {
	v, ok := ctx.Value(retryCtxKey{}).(retryInfo)
	return v.count, ok
}

func GetMaxRetry(ctx context.Context) (int, bool) {
	v, ok := ctx.Value(retryCtxKey{}).(retryInfo)
	return v.max, ok
}

// Enqueuer publishes one unit of work by work type, replacing the
// asynq-Client-shaped Enqueuer interfaces duplicated across domain
// packages. A single concrete implementation (internal/events) satisfies
// every package's local interface of this same shape.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, workType string, payload []byte) error
}
