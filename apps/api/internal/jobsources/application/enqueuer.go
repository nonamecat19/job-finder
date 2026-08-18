package application

import (
	"context"
)

type Enqueuer interface {
	EnqueueContext(ctx context.Context, workType string, payload []byte) error
}
