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

func TestGhostPython_Integration_RequestedWorkSurvivesAiServiceDownAndDrainsOnRestart(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	const workType = "ghost"
	purgeQueue(t, conn, "work."+workType)

	pub, pubCh := newTestPublisher(t, conn)
	defer pubCh.Close()

	env := newTestEnvelope("ghost.requested", "job_"+uuid.NewString(), "ghost-python:"+uuid.NewString(), uuid.NewString())
	publishWork(t, pub, workType, mustMarshal(t, env))

	depCh, err := conn.Channel()
	if err != nil {
		t.Fatalf("open inspect channel: %v", err)
	}
	q, err := depCh.QueueInspect("work." + workType)
	depCh.Close()
	if err != nil {
		t.Fatalf("inspect queue: %v", err)
	}
	if q.Messages != 1 {
		t.Fatalf("queue depth = %d while the AI service is stopped, want 1 (work.ghost must hold the event untouched)", q.Messages)
	}

	var mu sync.Mutex
	var received []testEnvelope

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	consumer := &events.Consumer{
		Dial:        func() (*amqp.Connection, error) { return amqp.Dial(testBrokerURL(t)) },
		Queue:       "work." + workType,
		Concurrency: 1,
		HandlerFunc: func(_ context.Context, d amqp.Delivery) error {
			var got testEnvelope
			if err := unmarshalEnvelope(d.Body, &got); err != nil {
				return err
			}
			mu.Lock()
			received = append(received, got)
			mu.Unlock()
			cancel()
			return nil
		},
	}

	runErr := make(chan error, 1)
	go func() { runErr <- consumer.Run(ctx) }()

	select {
	case <-ctx.Done():
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for the restarted consumer to drain work.ghost that queued up while the AI service was stopped")
	}
	<-runErr

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("received %d deliveries after restart, want exactly 1 — nothing hangs and nothing is lost", len(received))
	}
	got := received[0]
	if got.EventID != env.EventID || got.WorkID != env.WorkID || got.IdempotencyKey != env.IdempotencyKey || got.RunID != env.RunID {
		t.Fatalf("delivered event %+v does not match the one published while the AI service was stopped %+v — got a substituted result instead of the real one", got, env)
	}
}
