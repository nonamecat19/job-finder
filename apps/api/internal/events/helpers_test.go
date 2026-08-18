//go:build integration

package events_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
)

// defaultTestBrokerURL mirrors .env.example's dev default. RABBITMQ_URL
// overrides it, matching the convention TEST_DATABASE_URL/DATABASE_URL use
// for Postgres.
const defaultTestBrokerURL = "amqp://jobfinder:change-me@localhost:5672/"

func testBrokerURL() string {
	if url := os.Getenv("RABBITMQ_URL"); url != "" {
		return url
	}
	return defaultTestBrokerURL
}

// dialTestBroker opens a connection to a live RabbitMQ, skipping the test
// (never failing it) when none is reachable — these are integration tests
// against real broker behaviour (durability, redelivery, dead-lettering)
// that no fake can stand in for.
func dialTestBroker(t *testing.T) *amqp.Connection {
	t.Helper()
	url := testBrokerURL()
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(3 * time.Second)})
	if err != nil {
		t.Skipf("events: no live RabbitMQ reachable at %s: %v (set RABBITMQ_URL or start the broker; skipping)", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// declareTopologyOrFail declares the full topology on a fresh channel taken
// from conn, matching what a consumer does on every (re)connect (M1-1,
// M3-4).
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

// newTestPublisher opens a fresh channel on conn and wraps it in a
// events.Publisher with confirms enabled.
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

// purgeQueue empties a queue before a test runs, so tests sharing the fixed
// per-work-type queues (topology.go's WorkTypes is a closed set) don't see
// leftovers from a previous run.
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

// testEnvelope is a minimal stand-in wire body: envelope fields plus a
// trivial payload marker. It is not events.Envelope itself because these
// tests exercise broker mechanics (delivery, redelivery, retry, dead-letter
// routing), not envelope (de)serialisation, which envelope_test.go already
// covers.
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

// publishWork publishes body to the work exchange under workType's routing
// key, with x-attempt=0 and x-work-type set, waiting for the broker's
// publish confirm (M2-1).
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
