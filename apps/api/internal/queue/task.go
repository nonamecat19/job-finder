package queue

import (
	"context"
	"errors"
)

type Task struct {
	taskType string
	payload  []byte
}

func NewTask(taskType string, payload []byte) *Task {
	return &Task{taskType: taskType, payload: payload}
}

func (t *Task) Type() string    { return t.taskType }
func (t *Task) Payload() []byte { return t.payload }

var ErrSkipRetry = errors.New("queue: task is not retryable")

type retryCtxKey struct{}

type retryInfo struct {
	count int
	max   int
}

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

type Enqueuer interface {
	EnqueueContext(ctx context.Context, workType string, payload []byte) error
}
