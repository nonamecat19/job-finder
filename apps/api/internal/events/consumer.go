package events

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// DefaultShutdownGrace bounds how long Run waits for in-flight deliveries
// to finish after its context is cancelled before nacking them (M3-5).
const DefaultShutdownGrace = 30 * time.Second

// DefaultMinBackoff and DefaultMaxBackoff bound the reconnect backoff
// (M3-4, FR-035).
const (
	DefaultMinBackoff = 500 * time.Millisecond
	DefaultMaxBackoff = 30 * time.Second
)

// Handler processes one delivery. Returning nil acks the delivery — which
// MUST happen only after the unit of work is durably handled (M3-2), so a
// Handler that republishes a retry (retry.go) must confirm that publish
// before returning. Returning an error nacks the delivery with requeue.
type Handler func(ctx context.Context, d amqp.Delivery) error

// Dialer opens a new broker connection, used both for the initial connect
// and for every reconnect attempt (M3-4).
type Dialer func() (*amqp.Connection, error)

// Consumer consumes one work queue with manual ack, prefetch equal to its
// configured concurrency, and automatic reconnection with bounded
// exponential backoff (contracts/messaging.md M3).
type Consumer struct {
	Dial          Dialer
	Queue         string
	Concurrency   int
	HandlerFunc   Handler
	ShutdownGrace time.Duration
	MinBackoff    time.Duration
	MaxBackoff    time.Duration
	Logger        *slog.Logger
}

// Run consumes until ctx is cancelled, reconnecting automatically on any
// connection failure with bounded exponential backoff (M3-4). It returns
// nil on a clean shutdown triggered by ctx.
func (c *Consumer) Run(ctx context.Context) error {
	minBackoff := c.MinBackoff
	if minBackoff <= 0 {
		minBackoff = DefaultMinBackoff
	}
	maxBackoff := c.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = DefaultMaxBackoff
	}
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}

	backoff := minBackoff
	for {
		err := c.runOnce(ctx, logger)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logger.Error("events: consumer disconnected, reconnecting", "queue", c.Queue, "error", err, "backoff", backoff)
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce dials, re-declares topology, re-establishes prefetch (M3-4) and
// consumes until ctx is cancelled or the connection drops.
func (c *Consumer) runOnce(ctx context.Context, logger *slog.Logger) error {
	conn, err := c.Dial()
	if err != nil {
		return fmt.Errorf("events: dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("events: open channel: %w", err)
	}
	defer ch.Close()

	if err := DeclareTopology(ch); err != nil {
		return err
	}
	if err := ch.Qos(c.Concurrency, 0, false); err != nil {
		return fmt.Errorf("events: set prefetch: %w", err)
	}

	deliveries, err := ch.Consume(c.Queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("events: consume %s: %w", c.Queue, err)
	}

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))

	inflight := &inflightSet{deliveries: make(map[uint64]amqp.Delivery)}
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.Concurrency)

	shutdownGrace := c.ShutdownGrace
	if shutdownGrace <= 0 {
		shutdownGrace = DefaultShutdownGrace
	}

	for {
		select {
		case <-ctx.Done():
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(shutdownGrace):
				inflight.nackRemaining(logger)
			}
			return nil

		case amqpErr, ok := <-connClosed:
			wg.Wait()
			if !ok {
				return fmt.Errorf("events: connection closed")
			}
			return fmt.Errorf("events: connection closed: %w", amqpErr)

		case d, ok := <-deliveries:
			if !ok {
				wg.Wait()
				return fmt.Errorf("events: delivery channel closed")
			}
			sem <- struct{}{}
			wg.Add(1)
			inflight.add(d)
			go func(d amqp.Delivery) {
				defer wg.Done()
				defer func() { <-sem }()
				c.handleDelivery(ctx, d, inflight, logger)
			}(d)
		}
	}
}

// handleDelivery runs the handler and acks (success) or nacks with requeue
// (failure) — never before the handler returns (M3-2). If shutdown already
// force-nacked this delivery, its result is discarded.
func (c *Consumer) handleDelivery(ctx context.Context, d amqp.Delivery, inflight *inflightSet, logger *slog.Logger) {
	err := c.HandlerFunc(ctx, d)

	if !inflight.remove(d.DeliveryTag) {
		return
	}
	if err != nil {
		logger.Error("events: handler failed, nacking with requeue", "queue", c.Queue, "error", err)
		if nackErr := d.Nack(false, true); nackErr != nil {
			logger.Error("events: nack failed", "queue", c.Queue, "error", nackErr)
		}
		return
	}
	if ackErr := d.Ack(false); ackErr != nil {
		logger.Error("events: ack failed", "queue", c.Queue, "error", ackErr)
	}
}

// inflightSet tracks deliveries currently being handled, so a bounded-grace
// shutdown can nack with requeue anything still outstanding (M3-5) without
// racing the handler's own ack/nack.
type inflightSet struct {
	mu         sync.Mutex
	deliveries map[uint64]amqp.Delivery
}

func (s *inflightSet) add(d amqp.Delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliveries[d.DeliveryTag] = d
}

// remove reports whether d was still tracked (true) or was already claimed
// by nackRemaining (false), so the caller knows whether it may still
// ack/nack it.
func (s *inflightSet) remove(tag uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.deliveries[tag]; !ok {
		return false
	}
	delete(s.deliveries, tag)
	return true
}

func (s *inflightSet) nackRemaining(logger *slog.Logger) {
	s.mu.Lock()
	remaining := s.deliveries
	s.deliveries = make(map[uint64]amqp.Delivery)
	s.mu.Unlock()

	for _, d := range remaining {
		if err := d.Nack(false, true); err != nil {
			logger.Error("events: shutdown nack failed", "error", err)
		}
	}
}
