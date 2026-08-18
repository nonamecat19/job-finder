//go:build integration

package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
)

// TestPublish_Integration_BrokerUnreachableReturnsError proves US5 scenario
// 5 / M2-3: a publish that cannot reach the broker returns an error to its
// caller rather than being enqueued-and-forgotten. This simulates "the
// broker is down" by pointing the publisher at a channel whose underlying
// connection has already been closed — the strongest available proxy
// without stopping a shared broker instance other tests in this package
// depend on (as the task instructions for T056 suggest).
func TestPublish_Integration_BrokerUnreachableReturnsError(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	deadConn, err := amqp.Dial(testBrokerURL())
	if err != nil {
		t.Fatalf("dial connection to close: %v", err)
	}
	pub, ch := newTestPublisher(t, deadConn)
	defer ch.Close()

	// Simulate the broker being down from this publisher's point of view.
	if err := deadConn.Close(); err != nil {
		t.Fatalf("close connection: %v", err)
	}

	env := newTestEnvelope("match.requested", "job_"+uuid.NewString(), "broker-down:"+uuid.NewString(), uuid.NewString())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = pub.Publish(ctx, events.WorkExchange, "match", mustMarshal(t, env), nil)
	if err == nil {
		t.Fatal("Publish succeeded against a closed connection, want an error (M2-3: an HTTP handler that cannot publish must return 5xx, never 202)")
	}
}

// TestPublish_Integration_InvalidBrokerAddressReturnsError proves the dial
// side of the same guarantee: connecting to an address nothing is
// listening on fails rather than hanging or silently succeeding.
func TestPublish_Integration_InvalidBrokerAddressReturnsError(t *testing.T) {
	_ = dialTestBroker(t) // only to skip cleanly when there is no broker infra at all in this environment

	_, err := amqp.DialConfig("amqp://jobfinder:change-me@127.0.0.1:1/", amqp.Config{
		Dial: amqp.DefaultDial(2 * time.Second),
	})
	if err == nil {
		t.Fatal("dial to an address nothing listens on succeeded, want an error")
	}
}
