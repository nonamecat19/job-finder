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

var RetryRungs = []struct {
	Name string
	TTL  int64
}{
	{Name: "1s", TTL: 1000},
	{Name: "10s", TTL: 10_000},
	{Name: "1m", TTL: 60_000},
	{Name: "10m", TTL: 600_000},
}

var WorkTypes = []string{"ingest", "enrich", "match", "generate", "salary", "ghost"}

func workQueueName(workType string) string { return "work." + workType }
func dlqName(workType string) string       { return "dlq." + workType }
func delayQueueName(workType, rung string) string {
	return "delay." + workType + "." + rung
}

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

func DelayRoutingKey(workType, rung string) string {
	return workType + "." + rung
}
