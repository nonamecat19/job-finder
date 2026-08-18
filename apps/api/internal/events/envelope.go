package events

import "time"

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

func (t EventType) Valid() bool {
	_, ok := eventTypeRegistry[t]
	return ok
}

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

func (c FailureCategory) DefaultRetryable() bool {
	switch c {
	case FailureRateLimited, FailureProviderUnavailable, FailureTimeout, FailureInternal:
		return true
	default:
		return false
	}
}

type Failure struct {
	Category   FailureCategory `json:"category"`
	Retryable  bool            `json:"retryable"`
	Message    string          `json:"message"`
	FailedStep *string         `json:"failed_step,omitempty"`
}
