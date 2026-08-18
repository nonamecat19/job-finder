// Metrics exposes per-work-type observability for the broker (M8-1):
// queue depth, consumer count, DLQ depth, redelivery rate and
// publish-confirm latency. Counters are updated by the publisher and
// consumer as they process deliveries; queue and DLQ depth are read live
// from the broker via a passive queue inspection, since RabbitMQ — not this
// process — is authoritative for them.
package events

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// QueueInspector reports live depth and consumer count for a named queue.
// Implemented by an AMQP channel's passive queue inspection (M8-2, M8-1).
type QueueInspector interface {
	InspectQueue(ctx context.Context, name string) (messages, consumers int, err error)
}

// amqpInspectChannel is the subset of *amqp091.Channel AMQPInspector
// depends on.
type amqpInspectChannel interface {
	QueueInspect(name string) (amqp.Queue, error)
}

// AMQPInspector implements QueueInspector via an AMQP channel's passive
// queue inspection (QueueInspect issues a passive queue.declare, which
// fails if the queue does not exist rather than creating it).
type AMQPInspector struct {
	Channel amqpInspectChannel
}

func (a AMQPInspector) InspectQueue(ctx context.Context, name string) (int, int, error) {
	q, err := a.Channel.QueueInspect(name)
	if err != nil {
		return 0, 0, err
	}
	return q.Messages, q.Consumers, nil
}

type workTypeCounters struct {
	deliveries     atomic.Int64
	redeliveries   atomic.Int64
	confirmCount   atomic.Int64
	confirmTotalNs atomic.Int64
}

// Metrics accumulates the counters M8-1 requires that the broker itself
// does not track: redelivery rate and publish-confirm latency, per work
// type. Queue depth, consumer count and DLQ depth are not accumulated here
// — Snapshot reads them live via a QueueInspector.
type Metrics struct {
	mu       sync.Mutex
	counters map[string]*workTypeCounters
}

// NewMetrics returns an empty Metrics ready to record and snapshot.
func NewMetrics() *Metrics {
	return &Metrics{counters: make(map[string]*workTypeCounters)}
}

func (m *Metrics) counterFor(workType string) *workTypeCounters {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.counters[workType]
	if !ok {
		c = &workTypeCounters{}
		m.counters[workType] = c
	}
	return c
}

// RecordDelivery counts one consumed delivery for workType, and — when
// redelivered is true (the broker's `redelivered` flag) — one redelivery,
// so Snapshot can derive a redelivery rate (M8-1). Called by the consumer
// for every delivery it hands to a handler.
func (m *Metrics) RecordDelivery(workType string, redelivered bool) {
	c := m.counterFor(workType)
	c.deliveries.Add(1)
	if redelivered {
		c.redeliveries.Add(1)
	}
}

// RecordPublishConfirm records one publish-confirm round trip's latency for
// workType (M8-1). Called by the publisher after a confirm (ack or nack)
// arrives.
func (m *Metrics) RecordPublishConfirm(workType string, latency time.Duration) {
	c := m.counterFor(workType)
	c.confirmCount.Add(1)
	c.confirmTotalNs.Add(latency.Nanoseconds())
}

// WorkTypeSnapshot is one work type's point-in-time observability, combining
// this process's counters with a live broker read (M8-1).
type WorkTypeSnapshot struct {
	WorkType             string
	QueueDepth           int
	ConsumerCount        int
	DLQDepth             int
	Deliveries           int64
	Redeliveries         int64
	RedeliveryRate       float64
	PublishConfirmAvgMs  float64
	PublishConfirmSample int64
}

// Snapshot reads every work type's live queue and DLQ depth from the broker
// via inspector, and combines it with this process's accumulated counters
// (M8-1). It fails loudly, naming the queue, rather than returning a
// partial or stale snapshot.
func (m *Metrics) Snapshot(ctx context.Context, inspector QueueInspector) ([]WorkTypeSnapshot, error) {
	snapshots := make([]WorkTypeSnapshot, 0, len(WorkTypes))
	for _, wt := range WorkTypes {
		workQ := workQueueName(wt)
		messages, consumers, err := inspector.InspectQueue(ctx, workQ)
		if err != nil {
			return nil, fmt.Errorf("events: inspect queue %s: %w", workQ, err)
		}
		dlq := dlqName(wt)
		dlqMessages, _, err := inspector.InspectQueue(ctx, dlq)
		if err != nil {
			return nil, fmt.Errorf("events: inspect queue %s: %w", dlq, err)
		}

		c := m.counterFor(wt)
		deliveries := c.deliveries.Load()
		redeliveries := c.redeliveries.Load()
		var redeliveryRate float64
		if deliveries > 0 {
			redeliveryRate = float64(redeliveries) / float64(deliveries)
		}
		var confirmAvgMs float64
		confirmSample := c.confirmCount.Load()
		if confirmSample > 0 {
			confirmAvgMs = float64(c.confirmTotalNs.Load()) / float64(confirmSample) / float64(time.Millisecond)
		}

		snapshots = append(snapshots, WorkTypeSnapshot{
			WorkType:             wt,
			QueueDepth:           messages,
			ConsumerCount:        consumers,
			DLQDepth:             dlqMessages,
			Deliveries:           deliveries,
			Redeliveries:         redeliveries,
			RedeliveryRate:       redeliveryRate,
			PublishConfirmAvgMs:  confirmAvgMs,
			PublishConfirmSample: confirmSample,
		})
	}
	return snapshots, nil
}

// DLQDepths reads only the DLQ depth per work type (M8-2), for callers that
// need the health endpoint's narrower signal without a full Snapshot.
func (m *Metrics) DLQDepths(ctx context.Context, inspector QueueInspector) (map[string]int, error) {
	depths := make(map[string]int, len(WorkTypes))
	for _, wt := range WorkTypes {
		dlq := dlqName(wt)
		messages, _, err := inspector.InspectQueue(ctx, dlq)
		if err != nil {
			return nil, fmt.Errorf("events: inspect queue %s: %w", dlq, err)
		}
		depths[wt] = messages
	}
	return depths, nil
}

// WriteProm renders snapshots as Prometheus text exposition format
// (M8-1), one gauge/counter family per metric, labelled by work_type.
func WriteProm(w io.Writer, snapshots []WorkTypeSnapshot) error {
	families := []struct {
		name string
		help string
		typ  string
		get  func(WorkTypeSnapshot) float64
	}{
		{"jobfinder_queue_depth", "Messages ready in the work queue.", "gauge", func(s WorkTypeSnapshot) float64 { return float64(s.QueueDepth) }},
		{"jobfinder_queue_consumers", "Active consumers on the work queue.", "gauge", func(s WorkTypeSnapshot) float64 { return float64(s.ConsumerCount) }},
		{"jobfinder_dlq_depth", "Messages in the dead-letter queue.", "gauge", func(s WorkTypeSnapshot) float64 { return float64(s.DLQDepth) }},
		{"jobfinder_redelivery_rate", "Fraction of deliveries that were redeliveries.", "gauge", func(s WorkTypeSnapshot) float64 { return s.RedeliveryRate }},
		{"jobfinder_publish_confirm_latency_ms", "Average publish-confirm round-trip latency.", "gauge", func(s WorkTypeSnapshot) float64 { return s.PublishConfirmAvgMs }},
	}
	for _, family := range families {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", family.name, family.help, family.name, family.typ); err != nil {
			return err
		}
		for _, s := range snapshots {
			if _, err := fmt.Fprintf(w, "%s{work_type=%q} %v\n", family.name, s.WorkType, family.get(s)); err != nil {
				return err
			}
		}
	}
	return nil
}
