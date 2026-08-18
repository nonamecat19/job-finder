// Topology declares every exchange, queue and binding on the broker
// (data-model.md § 4, contracts/messaging.md M1). It is declared once, at
// startup, by the publisher; a consumer never declares topology of its own
// (M1-1) except to re-declare it after a reconnect (M3-4).
package events

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	WorkExchange   = "jobfinder.work"
	DelayExchange  = "jobfinder.delay"
	ResultExchange = "jobfinder.results"
	DLX            = "jobfinder.dlx"

	ResultQueue = "results.backend"
)

// RetryRungs is the fixed backoff ladder every work type shares
// (data-model.md § 6): 1s → 10s → 1m → 10m. An attempt beyond the ladder's
// length reuses its longest rung (M4-2).
var RetryRungs = []struct {
	Name string
	TTL  int64 // milliseconds
}{
	{Name: "1s", TTL: 1000},
	{Name: "10s", TTL: 10_000},
	{Name: "1m", TTL: 60_000},
	{Name: "10m", TTL: 600_000},
}

// WorkTypes is the closed set of work types with a declared queue
// (data-model.md § 6).
var WorkTypes = []string{"ingest", "enrich", "match", "generate", "salary", "ghost"}

func workQueueName(workType string) string { return "work." + workType }
func dlqName(workType string) string       { return "dlq." + workType }
func delayQueueName(workType, rung string) string {
	return "delay." + workType + "." + rung
}

// DeclareTopology declares every exchange, queue and binding in
// data-model.md § 4 idempotently (M1-1). A conflicting declaration (e.g. an
// existing classic or non-durable queue) fails loudly, naming the object
// (M1-3), rather than silently falling back to the existing definition.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(WorkExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("events: declare exchange %s: %w", WorkExchange, err)
	}
	if err := ch.ExchangeDeclare(DelayExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("events: declare exchange %s: %w", DelayExchange, err)
	}
	if err := ch.ExchangeDeclare(ResultExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("events: declare exchange %s: %w", ResultExchange, err)
	}
	if err := ch.ExchangeDeclare(DLX, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("events: declare exchange %s: %w", DLX, err)
	}

	quorum := amqp.Table{"x-queue-type": "quorum"}

	for _, wt := range WorkTypes {
		workQ := workQueueName(wt)
		workArgs := amqp.Table{
			"x-queue-type":              "quorum",
			"x-dead-letter-exchange":    DLX,
			"x-dead-letter-routing-key": wt,
		}
		if _, err := ch.QueueDeclare(workQ, true, false, false, false, workArgs); err != nil {
			return fmt.Errorf("events: declare queue %s: %w", workQ, err)
		}
		if err := ch.QueueBind(workQ, wt, WorkExchange, false, nil); err != nil {
			return fmt.Errorf("events: bind queue %s to %s: %w", workQ, WorkExchange, err)
		}

		for _, rung := range RetryRungs {
			delayQ := delayQueueName(wt, rung.Name)
			delayArgs := amqp.Table{
				"x-queue-type":              "quorum",
				"x-message-ttl":             rung.TTL,
				"x-dead-letter-exchange":    WorkExchange,
				"x-dead-letter-routing-key": wt,
			}
			if _, err := ch.QueueDeclare(delayQ, true, false, false, false, delayArgs); err != nil {
				return fmt.Errorf("events: declare queue %s: %w", delayQ, err)
			}
			routingKey := wt + "." + rung.Name
			if err := ch.QueueBind(delayQ, routingKey, DelayExchange, false, nil); err != nil {
				return fmt.Errorf("events: bind queue %s to %s: %w", delayQ, DelayExchange, err)
			}
		}

		dlq := dlqName(wt)
		if _, err := ch.QueueDeclare(dlq, true, false, false, false, quorum); err != nil {
			return fmt.Errorf("events: declare queue %s: %w", dlq, err)
		}
		if err := ch.QueueBind(dlq, wt, DLX, false, nil); err != nil {
			return fmt.Errorf("events: bind queue %s to %s: %w", dlq, DLX, err)
		}
	}

	if _, err := ch.QueueDeclare(ResultQueue, true, false, false, false, quorum); err != nil {
		return fmt.Errorf("events: declare queue %s: %w", ResultQueue, err)
	}
	if err := ch.QueueBind(ResultQueue, "*.completed", ResultExchange, false, nil); err != nil {
		return fmt.Errorf("events: bind queue %s to %s: %w", ResultQueue, ResultExchange, err)
	}

	return nil
}

// DelayRoutingKey returns the routing key for republishing to DelayExchange
// at the given rung, matching the binding declared in DeclareTopology.
func DelayRoutingKey(workType, rung string) string {
	return workType + "." + rung
}
