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
	"github.com/job-finder/api/internal/testinfra"
)

func testBrokerURL(t *testing.T) string {
	t.Helper()
	url, err := testinfra.RabbitMQURL(context.Background())
	if err != nil {
		t.Fatalf("events: start rabbitmq container: %v", err)
	}
	return url
}

func dialTestBroker(t *testing.T) *amqp.Connection {
	t.Helper()
	url := testBrokerURL(t)
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(10 * time.Second)})
	if err != nil {
		t.Fatalf("events: dial broker at %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func declareTopologyOrFail(t *testing.T, conn *amqp.Connection) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel for topology: %v", err)
	}
	defer ch.Close()
	if err := events.DeclareTopology(ch); err != nil {
		t.Fatalf("declare topology: %v", err)
	}
}

func newTestPublisher(t *testing.T, conn *amqp.Connection) (*events.Publisher, *amqp.Channel) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open publisher channel: %v", err)
	}
	pub, err := events.NewPublisher(ch, 5*time.Second)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	return pub, ch
}

func purgeQueue(t *testing.T, conn *amqp.Connection, queue string) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel to purge %s: %v", queue, err)
	}
	defer ch.Close()
	if _, err := ch.QueuePurge(queue, false); err != nil {
		t.Fatalf("purge %s: %v", queue, err)
	}
}

type testEnvelope struct {
	EventID        string `json:"event_id"`
	EventType      string `json:"event_type"`
	SchemaVersion  int    `json:"schema_version"`
	WorkID         string `json:"work_id"`
	IdempotencyKey string `json:"idempotency_key"`
	RunID          string `json:"run_id"`
}

func newTestEnvelope(eventType, workID, idempotencyKey, runID string) testEnvelope {
	return testEnvelope{
		EventID:        uuid.NewString(),
		EventType:      eventType,
		SchemaVersion:  1,
		WorkID:         workID,
		IdempotencyKey: idempotencyKey,
		RunID:          runID,
	}
}

func unmarshalEnvelope(body []byte, into *testEnvelope) error {
	return json.Unmarshal(body, into)
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func publishWork(t *testing.T, pub *events.Publisher, workType string, body []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	headers := amqp.Table{
		events.HeaderAttempt:  int32(0),
		events.HeaderWorkType: workType,
	}
	if err := pub.Publish(ctx, events.WorkExchange, workType, body, headers); err != nil {
		t.Fatalf("publish work to %s: %v", workType, err)
	}
}
