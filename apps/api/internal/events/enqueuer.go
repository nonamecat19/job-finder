package events

import (
	"context"
	"fmt"
)

type PublishEnqueuer struct {
	Publisher *Publisher
}

func (e *PublishEnqueuer) EnqueueContext(ctx context.Context, workType string, payload []byte) error {
	if err := e.Publisher.Publish(ctx, WorkExchange, workType, payload, nil); err != nil {
		return fmt.Errorf("events: enqueue %s: %w", workType, err)
	}
	return nil
}
