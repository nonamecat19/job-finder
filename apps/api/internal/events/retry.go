package events

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
	"github.com/job-finder/api/internal/queue"
)

const (
	HeaderAttempt            = "x-attempt"
	HeaderWorkType           = "x-work-type"
	HeaderFirstFailureReason = "x-first-failure-reason"
)

const defaultMaxAttempts = 5

func MaxAttempts(workType string) int {
	if workType == queue.TypeIngest {
		return queue.IngestMaxRetry + 1
	}
	return defaultMaxAttempts
}

func RateLimitRetryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	delay := shared.RateLimitCooldown << attempt
	if delay > shared.MaxRateLimitCooldown || delay <= 0 {
		delay = shared.MaxRateLimitCooldown
	}
	return delay
}

func rungForAttempt(attempt int) rungEntry {
	idx := attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(RetryRungs) {
		idx = len(RetryRungs) - 1
	}
	return RetryRungs[idx]
}

func rungForDuration(d time.Duration) rungEntry {
	for _, r := range RetryRungs {
		if time.Duration(r.TTL)*time.Millisecond >= d {
			return r
		}
	}
	return RetryRungs[len(RetryRungs)-1]
}

type rungEntry = struct {
	Name string
	TTL  int64
}

type Decision struct {
	DeadLetter bool
	Rung       string
	Attempt    int
}

func Decide(workType string, currentAttempt int, failure Failure) Decision {
	nextAttempt := currentAttempt + 1

	if !failure.Retryable {
		return Decision{DeadLetter: true, Attempt: nextAttempt}
	}
	if nextAttempt > MaxAttempts(workType) {
		return Decision{DeadLetter: true, Attempt: nextAttempt}
	}

	var rung rungEntry
	if failure.Category == FailureRateLimited {
		rung = rungForDuration(RateLimitRetryDelay(nextAttempt - 1))
	} else {
		rung = rungForAttempt(nextAttempt)
	}
	return Decision{DeadLetter: false, Rung: rung.Name, Attempt: nextAttempt}
}

func HandleFailure(ctx context.Context, pub *Publisher, workType string, body []byte, incomingHeaders amqp.Table, currentAttempt int, failure Failure) error {
	decision := Decide(workType, currentAttempt, failure)

	firstReason, hadFirst := incomingHeaders[HeaderFirstFailureReason]
	if !hadFirst {
		firstReason = string(failure.Category)
	}

	headers := amqp.Table{
		HeaderAttempt:            int32(decision.Attempt),
		HeaderWorkType:           workType,
		HeaderFirstFailureReason: firstReason,
	}

	if decision.DeadLetter {
		if err := pub.Publish(ctx, DLX, workType, body, headers); err != nil {
			return fmt.Errorf("events: dead-letter %s: %w", workType, err)
		}
		return nil
	}

	routingKey := DelayRoutingKey(workType, decision.Rung)
	if err := pub.Publish(ctx, DelayExchange, routingKey, body, headers); err != nil {
		return fmt.Errorf("events: retry publish %s at rung %s: %w", workType, decision.Rung, err)
	}
	return nil
}
