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

func TestPublish_Integration_BrokerUnreachableReturnsError(t *testing.T) {
	conn := dialTestBroker(t)
	declareTopologyOrFail(t, conn)

	deadConn, err := amqp.Dial(testBrokerURL(t))
	if err != nil {
		t.Fatalf("dial connection to close: %v", err)
	}
	pub, ch := newTestPublisher(t, deadConn)
	defer ch.Close()

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

func TestPublish_Integration_InvalidBrokerAddressReturnsError(t *testing.T) {
	_ = dialTestBroker(t)

	_, err := amqp.DialConfig("amqp://jobfinder:change-me@127.0.0.1:1/", amqp.Config{
		Dial: amqp.DefaultDial(2 * time.Second),
	})
	if err == nil {
		t.Fatal("dial to an address nothing listens on succeeded, want an error")
	}
}
