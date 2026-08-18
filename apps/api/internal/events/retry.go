package events

import (
	"context"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
	"github.com/job-finder/api/internal/queue"
)

// HeaderAttempt, HeaderWorkType and HeaderFirstFailureReason are the
// message headers used for retry and dead-letter routing (data-model.md
// § 5).
const (
	HeaderAttempt            = "x-attempt"
	HeaderWorkType           = "x-work-type"
	HeaderFirstFailureReason = "x-first-failure-reason"
)

// defaultMaxAttempts is the budget every work type gets except ingest,
// which keeps its pre-existing, narrower budget (data-model.md § 6).
const defaultMaxAttempts = 5

// MaxAttempts returns the retry budget for a work type (data-model.md § 6).
// asynq's inherited default of 25 is a deliberate reduction, called out in
// the migration notes rather than discovered from a queue count.
func MaxAttempts(workType string) int {
	if workType == queue.TypeIngest {
		return queue.IngestMaxRetry + 1
	}
	return defaultMaxAttempts
}

// RateLimitRetryDelay is RateLimitRetryDelay ported from
// internal/queue/policy.go (data-model.md § 6): the same doubling from
// shared.RateLimitCooldown, capped at shared.MaxRateLimitCooldown. attempt
// is 0 for the first rate-limited retry. Unlike the asynq original, the
// rate-limited check itself moves to the caller — retryable/category are
// carried on the Failure, never re-derived here (FR-004).
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

// rungForAttempt maps a 1-based retry attempt onto the fixed ladder
// (data-model.md § 6): 1s → 10s → 1m → 10m. An attempt beyond the ladder's
// length reuses its longest rung (M4-2).
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

// rungForDuration returns the shortest ladder rung whose TTL is at least
// d, or the longest rung if d exceeds them all. Used to place a
// rate-limited retry on the fixed ladder (M4-2: "rate-limited failures
// entering at the 1m rung") while preserving RateLimitRetryDelay's exact
// doubling math.
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

// Decision is the outcome of evaluating one failed delivery against its
// work type's retry budget (contracts/messaging.md M4).
type Decision struct {
	DeadLetter bool
	Rung       string // delay-queue rung name; empty when DeadLetter
	Attempt    int    // the x-attempt value to publish with
}

// Decide computes the retry decision for a failed delivery. It reads
// retryable from failure rather than re-deriving it from the category
// (FR-004, E5-2): a non-retryable failure goes straight to the DLQ without
// consuming budget (M4-3); a retryable failure that has exhausted its
// budget also goes to the DLQ (M4-4); otherwise it retries at the rung the
// ladder assigns this attempt, with a rate-limited failure entering at the
// 1m rung (M4-2).
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

// HandleFailure republishes body per Decide's outcome: to the matching
// delay-queue rung for a retry (M4-1) or to jobfinder.dlx for a
// dead-letter (M4-4), carrying x-first-failure-reason forward from
// incomingHeaders if already set, or setting it from this failure if not
// (so it is present the moment the message is ever dead-lettered).
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
