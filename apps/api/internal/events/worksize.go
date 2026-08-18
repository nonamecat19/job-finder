package events

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const MaxMessageSize = 512 * 1024

func PublishWork(ctx context.Context, pub *Publisher, workType, workID string, body []byte, headers amqp.Table) error {
	if len(body) > MaxMessageSize {
		return fmt.Errorf("events: publish %s for work %s: message size %d bytes exceeds maximum %d bytes", workType, workID, len(body), MaxMessageSize)
	}
	if err := pub.Publish(ctx, WorkExchange, workType, body, headers); err != nil {
		return fmt.Errorf("events: publish %s for work %s: %w", workType, workID, err)
	}
	return nil
}
