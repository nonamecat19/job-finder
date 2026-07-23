package application

import (
	"context"

	"github.com/hibiken/asynq"
)

// Enqueuer is the outbound task-queue port shared by SearchService and the
// interfaces/worker Handler. *asynq.Client satisfies it.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}
