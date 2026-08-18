package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ResultHandler func(ctx context.Context, envelope Envelope, result Result) error

var (
	malformedResultTotal atomic.Int64

	unregisteredCapabilityTotal atomic.Int64
)

func MalformedResultTotal() int64 { return malformedResultTotal.Load() }

func UnregisteredCapabilityTotal() int64 { return unregisteredCapabilityTotal.Load() }

type resultShell struct {
	EventType     EventType `json:"event_type"`
	SchemaVersion int       `json:"schema_version"`
}

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

type ResultRegistry map[EventType]ResultHandler

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
