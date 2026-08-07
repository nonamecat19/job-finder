package application

import (
	"context"

	"github.com/hibiken/asynq"
)

type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
