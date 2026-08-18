// Package events defines the message contract shared by every work and
// result event on the broker (data-model.md § 1, contracts/events.md E1-E2).
// Go is the source of truth: apps/ai's Pydantic models are generated from
// this package (E7).
package events

import "time"

// Envelope wraps every message published to or consumed from RabbitMQ. It
// MUST NOT carry profile, posting or resume content (E1-4) — that lives in
// the payload, not the envelope.
type Envelope struct {
	EventID        string    `json:"event_id"`
	EventType      EventType `json:"event_type"`
	SchemaVersion  int       `json:"schema_version"`
	OccurredAt     time.Time `json:"occurred_at"`
	WorkID         string    `json:"work_id"`
	CorrelationID  string    `json:"correlation_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	RunID          string    `json:"run_id"`
	ActivityID     *string   `json:"activity_id,omitempty"`
	TraceID        *string   `json:"trace_id,omitempty"`
}

// EventType is the closed registry of message types (E2). An event type
// outside this set is dead-lettered without body deserialization (M6-2).
type EventType string

const (
	EventIngestRequested   EventType = "ingest.requested"
	EventIngestCompleted   EventType = "ingest.completed"
	EventEnrichRequested   EventType = "enrich.requested"
	EventEnrichCompleted   EventType = "enrich.completed"
	EventMatchRequested    EventType = "match.requested"
	EventMatchCompleted    EventType = "match.completed"
	EventGenerateRequested EventType = "generate.requested"
	EventGenerateCompleted EventType = "generate.completed"
	EventSalaryRequested   EventType = "salary.requested"
	EventSalaryCompleted   EventType = "salary.completed"
	EventGhostRequested    EventType = "ghost.requested"
	EventGhostCompleted    EventType = "ghost.completed"
)

// eventTypeRegistry is the closed set of valid event types (E2).
var eventTypeRegistry = map[EventType]struct{}{
	EventIngestRequested:   {},
	EventIngestCompleted:   {},
	EventEnrichRequested:   {},
	EventEnrichCompleted:   {},
	EventMatchRequested:    {},
	EventMatchCompleted:    {},
	EventGenerateRequested: {},
	EventGenerateCompleted: {},
	EventSalaryRequested:   {},
	EventSalaryCompleted:   {},
	EventGhostRequested:    {},
	EventGhostCompleted:    {},
}

// Valid reports whether t is a member of the closed event type registry.
func (t EventType) Valid() bool {
	_, ok := eventTypeRegistry[t]
	return ok
}

// FailureCategory is the closed set of result-event failure categories
// (contracts/events.md E5). The first five map one-to-one onto the existing
// sentinel errors in internal/platform/llm/infrastructure/shared/errors.go
// and MUST keep those names (E5-1).
type FailureCategory string

const (
	FailureRateLimited         FailureCategory = "rate_limited"
	FailureCredentialRejected  FailureCategory = "credential_rejected"
	FailureInsufficientCredits FailureCategory = "insufficient_credits"
	FailureModelUnavailable    FailureCategory = "model_unavailable"
	FailureProviderUnavailable FailureCategory = "provider_unavailable"
	FailureInvalidInput        FailureCategory = "invalid_input"
	FailureBoundExceeded       FailureCategory = "bound_exceeded"
	FailureTimeout             FailureCategory = "timeout"
	FailureInternal            FailureCategory = "internal"
)

// DefaultRetryable gives the producer-side default retryability for a
// category (E5-2). A producer may override this default when it has better
// information; the consumer always trusts the value carried on the Failure,
// never re-deriving it (FR-004).
func (c FailureCategory) DefaultRetryable() bool {
	switch c {
	case FailureRateLimited, FailureProviderUnavailable, FailureTimeout, FailureInternal:
		return true
	default:
		return false
	}
}

// Failure describes a failed result event (E4, E5). Exactly one of
// Failure/result is present on a result event, selected by status (E4-1).
type Failure struct {
	Category   FailureCategory `json:"category"`
	Retryable  bool            `json:"retryable"`
	Message    string          `json:"message"`
	FailedStep *string         `json:"failed_step,omitempty"`
}
