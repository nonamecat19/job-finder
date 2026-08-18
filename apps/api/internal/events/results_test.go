package events

import (
	"context"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestHandleResultDelivery_UnknownEventTypeDiscardedWithCounter(t *testing.T) {
	before := MalformedResultTotal()
	reg := ResultRegistry{}
	handler := reg.HandleResultDelivery(nil)

	body := []byte(`{"event_type":"not.a.real.type","schema_version":1}`)
	if err := handler(context.Background(), amqp.Delivery{Body: body}); err != nil {
		t.Fatalf("handler returned error, want ack (nil): %v", err)
	}
	if got := MalformedResultTotal(); got <= before {
		t.Errorf("MalformedResultTotal did not increase: before=%d after=%d", before, got)
	}
}

func TestHandleResultDelivery_UnimplementedSchemaVersionDiscardedWithCounter(t *testing.T) {
	before := MalformedResultTotal()
	reg := ResultRegistry{}
	handler := reg.HandleResultDelivery(nil)

	body := []byte(`{"event_type":"ghost.completed","schema_version":99}`)
	if err := handler(context.Background(), amqp.Delivery{Body: body}); err != nil {
		t.Fatalf("handler returned error, want ack (nil): %v", err)
	}
	if got := MalformedResultTotal(); got <= before {
		t.Errorf("MalformedResultTotal did not increase: before=%d after=%d", before, got)
	}
}

func TestHandleResultDelivery_UnregisteredCapabilityDiscardedWithCounter(t *testing.T) {
	before := UnregisteredCapabilityTotal()
	reg := ResultRegistry{} // no handler for ghost.completed
	handler := reg.HandleResultDelivery(nil)

	body := []byte(`{"event_type":"ghost.completed","schema_version":1,"status":"succeeded"}`)
	if err := handler(context.Background(), amqp.Delivery{Body: body}); err != nil {
		t.Fatalf("handler returned error, want ack (nil): %v", err)
	}
	if got := UnregisteredCapabilityTotal(); got <= before {
		t.Errorf("UnregisteredCapabilityTotal did not increase: before=%d after=%d", before, got)
	}
}

func TestHandleResultDelivery_DispatchesToRegisteredHandler(t *testing.T) {
	var gotEnvelope Envelope
	var gotResult Result
	called := false

	reg := ResultRegistry{
		EventGhostCompleted: func(ctx context.Context, envelope Envelope, result Result) error {
			called = true
			gotEnvelope = envelope
			gotResult = result
			return nil
		},
	}
	handler := reg.HandleResultDelivery(nil)

	body := []byte(`{
		"event_id": "evt-1",
		"event_type": "ghost.completed",
		"schema_version": 1,
		"work_id": "job-1",
		"correlation_id": "corr-1",
		"idempotency_key": "ghost:job-1:corr-1",
		"run_id": "run-1",
		"trace_id": "trace-1",
		"status": "succeeded",
		"result": {"score": 42},
		"snapshot_hash": "sha256:abc"
	}`)
	if err := handler(context.Background(), amqp.Delivery{Body: body}); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !called {
		t.Fatal("registered handler was not called")
	}
	if gotEnvelope.WorkID != "job-1" {
		t.Errorf("WorkID = %q, want job-1", gotEnvelope.WorkID)
	}
	if gotEnvelope.CorrelationID != "corr-1" {
		t.Errorf("CorrelationID = %q, want corr-1", gotEnvelope.CorrelationID)
	}
	if gotResult.TraceID != "trace-1" {
		t.Errorf("Result.TraceID = %q, want trace-1 (sourced from the envelope, not a duplicated field)", gotResult.TraceID)
	}
	if gotResult.Status != ResultSucceeded {
		t.Errorf("Result.Status = %q, want succeeded", gotResult.Status)
	}
	if gotResult.SnapshotHash != "sha256:abc" {
		t.Errorf("Result.SnapshotHash = %q, want sha256:abc", gotResult.SnapshotHash)
	}
}

func TestHandleResultDelivery_HandlerErrorPropagatesForRequeue(t *testing.T) {
	wantErr := errors.New("persist failed")
	reg := ResultRegistry{
		EventGhostCompleted: func(ctx context.Context, envelope Envelope, result Result) error {
			return wantErr
		},
	}
	handler := reg.HandleResultDelivery(nil)

	body := []byte(`{"event_type":"ghost.completed","schema_version":1,"status":"succeeded"}`)
	err := handler(context.Background(), amqp.Delivery{Body: body})
	if err == nil {
		t.Fatal("handler: want error to propagate so consumer.go nacks with requeue, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("handler error = %v, want it to wrap %v", err, wantErr)
	}
}
