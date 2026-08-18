// Result-event consumption (contracts/messaging.md M5, M6; data-model.md
// § 3). One consumer reads results.backend for every capability; what
// "success" means and how to persist it is pluggable per capability via
// ResultRegistry, so a newly migrated capability registers a handler
// without touching this file (M5-3, M5-4, FR-037).
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ResultHandler persists one capability's result (data-model.md § 3). It is
// responsible for calling Admit (idempotency.go) within its own database
// transaction before persisting, so ledger admission and the persisted
// result commit atomically (data-model.md § 7) — this package has no
// database handle of its own (events stays free of any one domain's
// storage). A Duplicate or Superseded disposition from Admit is not an
// error: the handler MUST return nil for it, having done nothing else.
type ResultHandler func(ctx context.Context, envelope Envelope, result Result) error

var (
	// malformedResultTotal counts result deliveries discarded because they
	// failed the shell parse, named an event_type outside the closed
	// registry, or carried a schema_version this consumer does not
	// implement (M6-1, M6-2). Ideally these are rejected to the DLQ; see
	// the HandleResultDelivery doc comment for why this build discards with
	// a counter instead.
	malformedResultTotal atomic.Int64
	// unregisteredCapabilityTotal counts result deliveries for an
	// event_type with no registered ResultHandler — a capability that
	// published a result without this process ever registering a handler
	// for it, which should not happen in a correctly deployed system and is
	// counted so it is visible rather than silently dropped forever.
	unregisteredCapabilityTotal atomic.Int64
)

// MalformedResultTotal reports how many result deliveries were discarded as
// unparseable, unknown-type or unimplemented-schema since process start.
func MalformedResultTotal() int64 { return malformedResultTotal.Load() }

// UnregisteredCapabilityTotal reports how many result deliveries named an
// event_type with no registered handler since process start.
func UnregisteredCapabilityTotal() int64 { return unregisteredCapabilityTotal.Load() }

// resultShell is the minimal shape read first, so an unknown event_type or
// an unimplemented schema_version is rejected before the capability-specific
// result is deserialized (FR-029, M6-1, M6-2).
type resultShell struct {
	EventType     EventType `json:"event_type"`
	SchemaVersion int       `json:"schema_version"`
}

// resultMessage is the wire shape of a result event: the envelope merged
// with Result's fields, flattened the same way every work event is (E2).
// It is NOT built by embedding Envelope and Result directly — both declare
// a `trace_id` json tag, and Go's embedding-promotion rule silently drops
// an ambiguous field from both marshal and unmarshal, which would leave
// TraceID permanently empty. The envelope's trace_id is the wire's only
// trace_id (E1-2: null on work events, set by the orchestration service on
// the result); Result.TraceID in payloads.go exists for the HTTP transport
// (contracts/http.md H4), which has no envelope to source it from.
type resultMessage struct {
	Envelope
	Status       ResultStatus  `json:"status"`
	Result       InputSnapshot `json:"result,omitempty"`
	Failure      *Failure      `json:"failure,omitempty"`
	SnapshotHash string        `json:"snapshot_hash,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}

func (m resultMessage) toResult() Result {
	return Result{
		Status:       m.Status,
		Result:       m.Result,
		Failure:      m.Failure,
		TraceID:      derefString(m.TraceID),
		SnapshotHash: m.SnapshotHash,
		Usage:        m.Usage,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ResultRegistry maps a result event_type (e.g. "ghost.completed") to the
// handler that persists it. A capability with no entry is not itself an
// error at registry-build time — a capability still routed to Go never
// publishes a result event at all — but a delivery that names one is
// counted as unregistered (see UnregisteredCapabilityTotal).
type ResultRegistry map[EventType]ResultHandler

// HandleResultDelivery returns an events.Handler (consumer.go) for the
// result queue (results.backend), suitable for a Consumer whose Queue is
// events.ResultQueue.
//
// M6-1 and M6-2 call for rejecting an unimplemented schema_version or an
// unknown event_type to the DLQ without deserializing the body. The
// existing Consumer/Handler contract (consumer.go) only supports two
// outcomes — ack (return nil) or nack-with-requeue (return an error) — so a
// malformed result cannot be dead-lettered without extending that contract
// (results.backend also has no DLX bound today; only per-work-type queues
// do, data-model.md § 4). Until that extension lands, a malformed result is
// logged at error level, counted (MalformedResultTotal), and discarded
// (acked) rather than looped through infinite redelivery — the safer
// failure mode of the two, since these deliveries are provably never
// actionable by retrying.
func (r ResultRegistry) HandleResultDelivery(logger *slog.Logger) Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(ctx context.Context, d amqp.Delivery) error {
		var shell resultShell
		if err := json.Unmarshal(d.Body, &shell); err != nil {
			logger.Error("events: result delivery is not valid JSON, discarding", "error", err)
			malformedResultTotal.Add(1)
			return nil
		}
		if !shell.EventType.Valid() {
			logger.Error("events: unknown result event_type, discarding without further deserialization", "event_type", shell.EventType)
			malformedResultTotal.Add(1)
			return nil
		}
		if shell.SchemaVersion != 1 {
			logger.Error("events: unimplemented result schema_version, discarding", "event_type", shell.EventType, "schema_version", shell.SchemaVersion)
			malformedResultTotal.Add(1)
			return nil
		}

		handler, ok := r[shell.EventType]
		if !ok {
			logger.Warn("events: result names a capability with no registered handler, discarding", "event_type", shell.EventType)
			unregisteredCapabilityTotal.Add(1)
			return nil
		}

		var msg resultMessage
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			logger.Error("events: result body does not match the envelope+result shape, discarding", "event_type", shell.EventType, "error", err)
			malformedResultTotal.Add(1)
			return nil
		}

		if err := handler(ctx, msg.Envelope, msg.toResult()); err != nil {
			return fmt.Errorf("events: handle result %s: %w", shell.EventType, err)
		}
		return nil
	}
}
