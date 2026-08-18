//go:build integration

package events_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
)

// TestDurability_Integration_PublishedWhileConsumerStoppedIsProcessedOnRestart
// proves US5 scenario 1 / FR-033: work published to a durable queue while no
// consumer is running is not lost — it is processed once a consumer starts.
func TestDurability_Integration_PublishedWhileConsumerStoppedIsProcessedOnRestart(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	const workType = "match"
	purgeQueue(t, conn, "work."+workType)

	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	const n = 10
	published := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		env := newTestEnvelope(workType+".requested", "job_"+uuid.NewString(), "durability:"+uuid.NewString(), uuid.NewString())
		published[env.EventID] = true
		publishWork(t, pub, workType, mustMarshal(t, env))
	}

	// No consumer is running yet: the queue durably holds all n messages.
	depCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("open inspect channel: %v", err)
	}
	q, err := depCh.QueueInspect("work." + workType)
	depCh.Close()
	if err != nil {
		t.Fatalf("inspect queue: %v", err)
	}
	if q.Messages != n {
		t.Fatalf("queue depth = %d before any consumer ran, want %d (no loss while stopped)", q.Messages, n)
	}

	// Now start a consumer and confirm every published message is
	// processed exactly once, with none lost.
	var mu sync.Mutex
	received := make(map[string]int)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	consumer := &events.Consumer{
		Dial:        func() (*amqp.Connection, error) { return amqp.Dial(testBrokerURL(t)) },
		Queue:       "work." + workType,
		Concurrency: 4,
		HandlerFunc: func(_ context.Context, d amqp.Delivery) error {
			var env testEnvelope
			if err := unmarshalEnvelope(d.Body, &env); err != nil {
				return err
			}
			mu.Lock()
			received[env.EventID]++
			count := len(received)
			mu.Unlock()
			if count >= n {
				cancel()
			}
			return nil
		},
	}

	runErr := make(chan error, 1)
	go func() { runErr <- consumer.Run(ctx) }()

	select {
	case <-ctx.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for consumer to drain the queue that filled up while it was stopped")
	}
	<-runErr

	mu.Lock()
	defer mu.Unlock()
	if len(received) != n {
		t.Fatalf("received %d distinct events, want %d — work published while the consumer was stopped must not be lost", len(received), n)
	}
	for id := range published {
		if received[id] < 1 {
			t.Errorf("event %s published while consumer was stopped was never delivered", id)
		}
	}
}

// TestDurability_Integration_ReconnectAfterConnectionLossLosesNoAcceptedWork
// approximates US5 scenario 5 / SC-011 (broker restart under continuous
// load loses zero accepted units of work). Actually killing and restarting
// the RabbitMQ container isn't feasible from this test process, so this
// exercises the strongest available proxy: the consumer's connection drops
// mid-stream while the publisher keeps accepting and confirming publishes,
// and the consumer's built-in reconnect logic (M3-4) resumes and drains
// everything with zero loss. A true broker-process restart would also need
// to prove the publisher side reconnects and messages published during the
// outage are not silently dropped by the publisher (M2-3 covers that
// failure mode directly in publish_failure_test.go); this test only proves
// the consumer side of durability across a connection loss.
func TestDurability_Integration_ReconnectAfterConnectionLossLosesNoAcceptedWork(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	const workType = "enrich"
	purgeQueue(t, conn, "work."+workType)

	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	const total = 20
	const dropAfter = 5

	var mu sync.Mutex
	received := make(map[string]int)
	processed := 0
	var dropOnce sync.Once

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var consumerConnMu sync.Mutex
	var currentConn *amqp.Connection

	consumer := &events.Consumer{
		Dial: func() (*amqp.Connection, error) {
			c, err := amqp.Dial(testBrokerURL(t))
			if err == nil {
				consumerConnMu.Lock()
				currentConn = c
				consumerConnMu.Unlock()
			}
			return c, err
		},
		Queue:       "work." + workType,
		Concurrency: 1, // serialize so dropAfter is deterministic
		MinBackoff:  100 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
		HandlerFunc: func(_ context.Context, d amqp.Delivery) error {
			var env testEnvelope
			if err := unmarshalEnvelope(d.Body, &env); err != nil {
				return err
			}
			mu.Lock()
			received[env.EventID]++
			processed++
			n := processed
			mu.Unlock()

			if n == dropAfter {
				dropOnce.Do(func() {
					consumerConnMu.Lock()
					c := currentConn
					consumerConnMu.Unlock()
					if c != nil {
						_ = c.Close() // simulate an abrupt connection loss mid-stream
					}
				})
			}

			mu.Lock()
			done := len(received) >= total
			mu.Unlock()
			if done {
				cancel()
			}
			return nil
		},
	}

	runErr := make(chan error, 1)
	go func() { runErr <- consumer.Run(ctx) }()

	// Publish continuously, independent of the consumer's connection state.
	for i := 0; i < total; i++ {
		env := newTestEnvelope(workType+".requested", "job_"+uuid.NewString(), "reconnect:"+uuid.NewString(), uuid.NewString())
		publishWork(t, pub, workType, mustMarshal(t, env))
	}

	select {
	case <-ctx.Done():
	case <-time.After(25 * time.Second):
		t.Fatal("timed out waiting for the consumer to reconnect and drain remaining work after a simulated connection loss")
	}
	<-runErr

	mu.Lock()
	defer mu.Unlock()
	if len(received) != total {
		t.Fatalf("received %d distinct events after reconnect, want %d — a mid-stream connection loss must lose zero accepted work", len(received), total)
	}
}
