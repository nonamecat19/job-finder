package events

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// MaxMessageSize bounds a single published work message body (E3-5). No
// configuration key for this is named in contracts/configuration.md, so it
// is a constant rather than an env var — generous over the largest snapshot
// today's capabilities need (a full profile plus a posting, per
// data-model.md § 2), while still catching a runaway snapshot before it
// reaches the broker.
const MaxMessageSize = 512 * 1024

// PublishWork publishes body — an envelope merged with its work payload,
// already marshaled by the caller — to WorkExchange under workType's
// routing key, enforcing MaxMessageSize before the message ever reaches the
// wire (E3-5). Exceeding the limit is a publish error naming workID and the
// oversized size, never a silently truncated snapshot.
func PublishWork(ctx context.Context, pub *Publisher, workType, workID string, body []byte, headers amqp.Table) error {
	if len(body) > MaxMessageSize {
		return fmt.Errorf("events: publish %s for work %s: message size %d bytes exceeds maximum %d bytes", workType, workID, len(body), MaxMessageSize)
	}
	if err := pub.Publish(ctx, WorkExchange, workType, body, headers); err != nil {
		return fmt.Errorf("events: publish %s for work %s: %w", workType, workID, err)
	}
	return nil
}
