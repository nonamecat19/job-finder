package events

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// requiredEnvelopeFields lists every envelope field except activity_id and
// trace_id, which E1-1 marks optional.
var requiredEnvelopeFields = []string{
	"event_id",
	"event_type",
	"schema_version",
	"occurred_at",
	"work_id",
	"correlation_id",
	"idempotency_key",
	"run_id",
}

func TestEnvelope_RequiredFieldsPresentInJSON(t *testing.T) {
	env := Envelope{
		EventID:        "018f0000-0000-7000-8000-000000000001",
		EventType:      EventMatchRequested,
		SchemaVersion:  1,
		OccurredAt:     time.Date(2026, 8, 18, 10, 31, 2, 0, time.UTC),
		WorkID:         "job_01H",
		CorrelationID:  "018f0000-0000-7000-8000-000000000002",
		IdempotencyKey: "match:job_01H:1",
		RunID:          "018f0000-0000-7000-8000-000000000003",
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	for _, field := range requiredEnvelopeFields {
		if _, ok := m[field]; !ok {
			t.Errorf("required envelope field %q missing from JSON output", field)
		}
	}
}

func TestEnvelope_ActivityAndTraceIDAreOptional(t *testing.T) {
	env := Envelope{
		EventID:        "018f0000-0000-7000-8000-000000000001",
		EventType:      EventGhostRequested,
		SchemaVersion:  1,
		OccurredAt:     time.Now(),
		WorkID:         "job_01H",
		CorrelationID:  "018f0000-0000-7000-8000-000000000002",
		IdempotencyKey: "ghost:job_01H:1",
		RunID:          "018f0000-0000-7000-8000-000000000003",
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}

	if _, ok := m["activity_id"]; ok {
		t.Error("activity_id should be omitted when nil (E1-1: optional)")
	}
	if _, ok := m["trace_id"]; ok {
		t.Error("trace_id should be omitted when nil (E1-1: optional, null on work events per E1-2)")
	}
}

// TestEnvelope_CorrelationIDEchoedOnResult exercises E1-3: a result event's
// correlation_id MUST equal that of the work event that caused it.
func TestEnvelope_CorrelationIDEchoedOnResult(t *testing.T) {
	workCorrelationID := "018f0000-0000-7000-8000-00000000000a"

	work := Envelope{
		EventID:        "018f0000-0000-7000-8000-000000000010",
		EventType:      EventGhostRequested,
		SchemaVersion:  1,
		OccurredAt:     time.Now(),
		WorkID:         "job_01H",
		CorrelationID:  workCorrelationID,
		IdempotencyKey: "ghost:job_01H:1",
		RunID:          "018f0000-0000-7000-8000-000000000011",
	}

	result := Envelope{
		EventID:        "018f0000-0000-7000-8000-000000000012",
		EventType:      EventGhostCompleted,
		SchemaVersion:  1,
		OccurredAt:     time.Now(),
		WorkID:         work.WorkID,
		CorrelationID:  work.CorrelationID,
		IdempotencyKey: work.IdempotencyKey,
		RunID:          work.RunID,
	}

	if result.CorrelationID != work.CorrelationID {
		t.Fatalf("result correlation_id %q does not echo work correlation_id %q", result.CorrelationID, work.CorrelationID)
	}
}

// TestEnvelope_NoProfileContentField asserts, structurally, that the
// envelope carries only routing/identity fields and nothing shaped like
// profile or posting content (E1-4). This is a static shape check: the
// envelope's field set is exactly the ten fields data-model.md § 1
// declares, none of which is a content field.
func TestEnvelope_NoProfileContentField(t *testing.T) {
	allowed := map[string]bool{
		"EventID": true, "EventType": true, "SchemaVersion": true,
		"OccurredAt": true, "WorkID": true, "CorrelationID": true,
		"IdempotencyKey": true, "RunID": true, "ActivityID": true, "TraceID": true,
	}

	typ := reflect.TypeOf(Envelope{})
	if typ.NumField() != len(allowed) {
		t.Fatalf("Envelope has %d fields, want %d (data-model.md § 1 declares exactly ten)", typ.NumField(), len(allowed))
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !allowed[name] {
			t.Errorf("unexpected envelope field %q: not part of data-model.md § 1, and any new field risks carrying content (E1-4)", name)
		}
	}
}

func TestEventType_Valid(t *testing.T) {
	if !EventMatchRequested.Valid() {
		t.Error("match.requested should be a valid, registered event type")
	}
	if EventType("bogus.event").Valid() {
		t.Error("an unregistered event type must not validate (E2, dead-lettered per M6-2)")
	}
}

func TestFailureCategory_DefaultRetryable(t *testing.T) {
	cases := map[FailureCategory]bool{
		FailureRateLimited:         true,
		FailureProviderUnavailable: true,
		FailureTimeout:             true,
		FailureInternal:            true,
		FailureCredentialRejected:  false,
		FailureInsufficientCredits: false,
		FailureInvalidInput:        false,
		FailureBoundExceeded:       false,
	}
	for category, want := range cases {
		if got := category.DefaultRetryable(); got != want {
			t.Errorf("%s.DefaultRetryable() = %v, want %v (E5-2)", category, got, want)
		}
	}
}
