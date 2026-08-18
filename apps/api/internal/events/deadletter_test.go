//go:build integration

package events_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
)

// TestDeadLetter_Integration_BudgetExhaustionLandsInDLQWithFirstFailureReason
// proves US5 scenario 3 / SC-012: a work item that exhausts its retry
// budget is dead-lettered to dlq.<work_type> with x-first-failure-reason
// set (M4-4, FR-031). The ladder's own timing (1s/10s/1m/10m) is unit
// tested in retry_test.go via Decide; this test proves the wiring —
// HandleFailure's terminal publish actually lands in a live, consumable DLQ
// with the right routing key and header — without waiting out the real
// backoff ladder for all five attempts.
func TestDeadLetter_Integration_BudgetExhaustionLandsInDLQWithFirstFailureReason(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	const workType = "generate"
	purgeQueue(t, conn, "dlq."+workType)

	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	env := newTestEnvelope(workType+".requested", "job_"+uuid.NewString(), "dlq:"+uuid.NewString(), uuid.NewString())
	body := mustMarshal(t, env)

	failure := events.Failure{
		Category:  events.FailureProviderUnavailable,
		Retryable: true,
		Message:   "provider unavailable after exhausting retry budget",
	}

	// currentAttempt = the work type's full budget: the next attempt
	// exceeds it, so Decide (and therefore HandleFailure) must dead-letter
	// rather than retry (M4-4).
	currentAttempt := events.MaxAttempts(workType)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := events.HandleFailure(ctx, pub, workType, body, amqp.Table{}, currentAttempt, failure); err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}

	consumeConn, err := amqp.Dial(testBrokerURL())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer consumeConn.Close()
	_, d := consumeOne(t, consumeConn, "dlq."+workType, 10*time.Second)
	if err := d.Ack(false); err != nil {
		t.Fatalf("ack dlq delivery: %v", err)
	}

	reason, ok := d.Headers[events.HeaderFirstFailureReason]
	if !ok {
		t.Fatal("dead-lettered message is missing x-first-failure-reason (FR-031)")
	}
	if got := reason.(string); got != string(events.FailureProviderUnavailable) {
		t.Errorf("x-first-failure-reason = %q, want %q", got, events.FailureProviderUnavailable)
	}

	var gotEnv testEnvelope
	if err := json.Unmarshal(d.Body, &gotEnv); err != nil {
		t.Fatalf("dead-lettered body is not the original message: %v", err)
	}
	if gotEnv.EventID != env.EventID {
		t.Errorf("dead-lettered event_id = %q, want %q", gotEnv.EventID, env.EventID)
	}
}

// TestDeadLetter_Integration_NonRetryableGoesStraightToDLQWithoutBudget
// exercises M4-3: a non-retryable failure (invalid_input) is dead-lettered
// on the very first attempt, without consuming any retry budget.
func TestDeadLetter_Integration_NonRetryableGoesStraightToDLQWithoutBudget(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	const workType = "match"
	purgeQueue(t, conn, "dlq."+workType)

	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	env := newTestEnvelope(workType+".requested", "job_"+uuid.NewString(), "dlq-nonretryable:"+uuid.NewString(), uuid.NewString())
	body := mustMarshal(t, env)

	failure := events.Failure{Category: events.FailureInvalidInput, Retryable: false, Message: "malformed snapshot"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := events.HandleFailure(ctx, pub, workType, body, amqp.Table{}, 0, failure); err != nil {
		t.Fatalf("HandleFailure: %v", err)
	}

	consumeConn, err := amqp.Dial(testBrokerURL())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer consumeConn.Close()
	_, d := consumeOne(t, consumeConn, "dlq."+workType, 10*time.Second)
	if err := d.Ack(false); err != nil {
		t.Fatalf("ack: %v", err)
	}
	if got := d.Headers[events.HeaderFirstFailureReason].(string); got != string(events.FailureInvalidInput) {
		t.Errorf("x-first-failure-reason = %q, want %q", got, events.FailureInvalidInput)
	}
}

// schemaProbe is the minimal, cheap decode a consumer performs before
// touching the rest of a message: just enough to route (M6-1, M6-2), never
// the capability-specific payload.
type schemaProbe struct {
	EventType     string `json:"event_type"`
	SchemaVersion int    `json:"schema_version"`
}

const implementedSchemaVersion = 1

// rejectUnroutable is the header-inspection-only rejection pattern M6
// requires: an unimplemented schema_version or an unknown event_type is
// nacked straight to the DLQ via the work queue's own dead-letter binding
// (no-requeue triggers x-dead-letter-exchange, set by topology.go),
// without ever unmarshalling into a capability payload type.
func rejectUnroutable(d amqp.Delivery) (rejected bool, err error) {
	var probe schemaProbe
	if err := json.Unmarshal(d.Body, &probe); err != nil {
		return false, err
	}
	if probe.SchemaVersion != implementedSchemaVersion {
		return true, d.Nack(false, false)
	}
	if !events.EventType(probe.EventType).Valid() {
		return true, d.Nack(false, false)
	}
	return false, nil
}

// TestDeadLetter_Integration_UnknownEventTypeAndUnimplementedSchemaVersionRejected
// proves FR-029 / M6-1 / M6-2: both an unimplemented schema_version and an
// unknown event_type are dead-lettered, not best-effort processed, using
// only the cheap envelope probe above — never deserializing into a
// capability-specific payload.
func TestDeadLetter_Integration_UnknownEventTypeAndUnimplementedSchemaVersionRejected(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	const workType = "ingest"
	purgeQueue(t, conn, "work."+workType)
	purgeQueue(t, conn, "dlq."+workType)

	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	unimplementedVersion := struct {
		EventID        string `json:"event_id"`
		EventType      string `json:"event_type"`
		SchemaVersion  int    `json:"schema_version"`
		WorkID         string `json:"work_id"`
		IdempotencyKey string `json:"idempotency_key"`
		RunID          string `json:"run_id"`
		// PayloadThatWouldFailToParse stands in for a capability-specific
		// payload shape the consumer must never attempt to touch before
		// rejecting on the header/probe check.
		PayloadThatWouldFailToParse json.RawMessage `json:"payload"`
	}{
		EventID:                     uuid.NewString(),
		EventType:                   workType + ".requested",
		SchemaVersion:               99,
		WorkID:                      "job_" + uuid.NewString(),
		IdempotencyKey:              "schema:" + uuid.NewString(),
		RunID:                       uuid.NewString(),
		PayloadThatWouldFailToParse: json.RawMessage(`"this is not a valid payload object"`),
	}
	publishWork(t, pub, workType, mustMarshal(t, unimplementedVersion))

	unknownType := newTestEnvelope("nonexistent.event", "job_"+uuid.NewString(), "unknown-type:"+uuid.NewString(), uuid.NewString())
	publishWork(t, pub, workType, mustMarshal(t, unknownType))

	consumeConn, err := amqp.Dial(testBrokerURL())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer consumeConn.Close()

	workCh, workDeliveries := openConsumer(t, consumeConn, "work."+workType)
	for i := 0; i < 2; i++ {
		d := nextDelivery(t, workDeliveries, 10*time.Second)
		rejected, err := rejectUnroutable(d)
		if err != nil {
			t.Fatalf("rejectUnroutable must not fail parsing a well-formed envelope with a bad payload: %v", err)
		}
		if !rejected {
			t.Fatalf("message %d (schema_version=%d event_type=%q) should have been rejected", i, mustProbeVersion(t, d), mustProbeType(t, d))
		}
	}
	workCh.Close()

	seen := map[string]bool{}
	dlqCh, dlqDeliveries := openConsumer(t, consumeConn, "dlq."+workType)
	defer dlqCh.Close()
	for i := 0; i < 2; i++ {
		d := nextDelivery(t, dlqDeliveries, 10*time.Second)
		var probe schemaProbe
		if err := json.Unmarshal(d.Body, &probe); err != nil {
			t.Fatalf("unmarshal dlq body: %v", err)
		}
		seen[probe.EventType] = true
		if err := d.Ack(false); err != nil {
			t.Fatalf("ack dlq delivery: %v", err)
		}
	}
	if !seen[workType+".requested"] {
		t.Error("the unimplemented-schema-version message never reached the DLQ")
	}
	if !seen["nonexistent.event"] {
		t.Error("the unknown-event-type message never reached the DLQ")
	}
}

func mustProbeVersion(t *testing.T, d amqp.Delivery) int {
	t.Helper()
	var probe schemaProbe
	_ = json.Unmarshal(d.Body, &probe)
	return probe.SchemaVersion
}

func mustProbeType(t *testing.T, d amqp.Delivery) string {
	t.Helper()
	var probe schemaProbe
	_ = json.Unmarshal(d.Body, &probe)
	return probe.EventType
}
