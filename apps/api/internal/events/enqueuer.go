package events

import (
	"context"
	"fmt"
)

// PublishEnqueuer adapts a Publisher to every domain package's local
// Enqueuer interface (queue.Enqueuer's shape: EnqueueContext(ctx, workType
// string, payload []byte) error), replacing the asynq.Client each of those
// packages used to depend on directly.
type PublishEnqueuer struct {
	Publisher *Publisher
}

// EnqueueContext publishes payload as one unit of work of the given type to
// the work exchange, routed by topology.go's per-work-type binding.
func (e *PublishEnqueuer) EnqueueContext(ctx context.Context, workType string, payload []byte) error {
	if err := e.Publisher.Publish(ctx, WorkExchange, workType, payload, nil); err != nil {
		return fmt.Errorf("events: enqueue %s: %w", workType, err)
	}
	return nil
}
