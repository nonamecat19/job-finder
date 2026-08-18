package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultConfirmTimeout bounds how long Publish waits for the broker to
// confirm a publish before treating it as failed (M2-1, M2-3).
const DefaultConfirmTimeout = 5 * time.Second

// amqpChannel is the subset of *amqp091.Channel the publisher depends on,
// narrowed so tests can substitute a fake broker without a live connection.
type amqpChannel interface {
	Confirm(noWait bool) error
	NotifyPublish(chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(chan amqp.Return) chan amqp.Return
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// Publisher publishes envelopes with publisher confirms, persistent
// delivery and mandatory routing (contracts/messaging.md M2). A single
// Publisher serializes publishes on its channel so each publish's confirm
// (and any returned-message notification) can be correlated unambiguously.
type Publisher struct {
	mu             sync.Mutex
	ch             amqpChannel
	confirms       chan amqp.Confirmation
	returns        chan amqp.Return
	confirmTimeout time.Duration
}

// NewPublisher puts ch into confirm mode and wires the returned-message
// handler (M2-1, M2-4). confirmTimeout <= 0 uses DefaultConfirmTimeout.
func NewPublisher(ch amqpChannel, confirmTimeout time.Duration) (*Publisher, error) {
	if err := ch.Confirm(false); err != nil {
		return nil, fmt.Errorf("events: enable publisher confirms: %w", err)
	}
	if confirmTimeout <= 0 {
		confirmTimeout = DefaultConfirmTimeout
	}
	return &Publisher{
		ch:             ch,
		confirms:       ch.NotifyPublish(make(chan amqp.Confirmation, 1)),
		returns:        ch.NotifyReturn(make(chan amqp.Return, 1)),
		confirmTimeout: confirmTimeout,
	}, nil
}

// Publish sends body to exchange with routingKey, persistent and mandatory,
// and does not return until the broker has confirmed the publish (M2-1,
// M2-2). It returns an error — never a silent drop — when the broker nacks,
// the message is returned as unroutable, or confirmation does not arrive
// within the confirm timeout (M2-3, M2-4).
func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ch.PublishWithContext(ctx, exchange, routingKey, true, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         body,
		Headers:      headers,
	}); err != nil {
		return fmt.Errorf("events: publish to %s: %w", exchange, err)
	}

	timer := time.NewTimer(p.confirmTimeout)
	defer timer.Stop()

	var unroutable bool
	for {
		select {
		case ret, ok := <-p.returns:
			if ok {
				_ = ret
				unroutable = true
			}
		case confirm, ok := <-p.confirms:
			if !ok {
				return fmt.Errorf("events: publish to %s: confirm channel closed", exchange)
			}
			if unroutable {
				return fmt.Errorf("events: publish to %s: message unroutable (key %q)", exchange, routingKey)
			}
			if !confirm.Ack {
				return fmt.Errorf("events: publish to %s: nacked by broker", exchange)
			}
			return nil
		case <-timer.C:
			return fmt.Errorf("events: publish to %s: confirm timeout after %s", exchange, p.confirmTimeout)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
