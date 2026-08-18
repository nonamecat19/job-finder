//go:build integration

package events_test

import (
	"context"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/testinfra"
)

// docker/rabbitmq/init-ai-user.sh gives the AI service its own broker account
// with permissions restricted to exactly what contracts/messaging.md M7-3
// allows: read the AI work queues, write results, configure nothing. Until
// this file that restriction was asserted nowhere — the script ran in compose
// and its effect was taken on trust, so a regex typo would have handed the AI
// service the ability to delete the backend's topology and nothing would have
// noticed.
//
// The script itself is what runs here (testinfra.ProvisionRabbitMQAIUser
// mounts the repository file into the image compose pins), against the real
// broker, so what is proven is the file that ships.

const (
	aiUser = "ai_service"
	aiPass = "testinfra-ai-secret"
)

// provisionAIUser runs the init script once and returns a connection opened
// as the AI service account.
func provisionAIUser(t *testing.T) *amqp.Connection {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Topology is the publisher's job (M1-1) and the AI account is not
	// allowed to declare any of it, so the admin connection must create it
	// first — exactly the ordering compose enforces with depends_on.
	admin := dialTestBroker(t)
	declareTopologyOrFail(t, admin)

	if err := testinfra.ProvisionRabbitMQAIUser(ctx, aiUser, aiPass); err != nil {
		t.Fatalf("run init-ai-user.sh: %v", err)
	}

	url, err := testinfra.RabbitMQURLAs(ctx, aiUser, aiPass)
	if err != nil {
		t.Fatalf("build ai service broker url: %v", err)
	}
	conn, err := amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(10 * time.Second)})
	if err != nil {
		t.Fatalf("dial broker as %s: %v — the script did not create a usable account", aiUser, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestAIUserCanConsumeItsWorkQueues proves the account the script creates can
// do the job it exists for: consume the four AI work queues.
func TestAIUserCanConsumeItsWorkQueues(t *testing.T) {
	conn := provisionAIUser(t)

	for _, queue := range []string{"work.match", "work.generate", "work.salary", "work.ghost"} {
		t.Run(queue, func(t *testing.T) {
			ch, err := conn.Channel()
			if err != nil {
				t.Fatalf("open channel: %v", err)
			}
			defer ch.Close()

			if _, err := ch.Consume(queue, "", false, false, false, false, nil); err != nil {
				t.Fatalf("consume %s as the AI service: %v — read permission does not cover its own work queue", queue, err)
			}
		})
	}
}

// TestAIUserCannotConsumeBackendQueues proves the read regex is a whitelist,
// not a formality: ingest and enrich are the backend's own work types and the
// AI service must not be able to take messages off them.
func TestAIUserCannotConsumeBackendQueues(t *testing.T) {
	conn := provisionAIUser(t)

	for _, queue := range []string{"work.ingest", "work.enrich"} {
		t.Run(queue, func(t *testing.T) {
			ch, err := conn.Channel()
			if err != nil {
				t.Fatalf("open channel: %v", err)
			}
			defer ch.Close()

			if _, err := ch.Consume(queue, "", false, false, false, false, nil); err == nil {
				t.Fatalf("the AI service was allowed to consume %s", queue)
			} else if !isAccessRefused(err) {
				t.Fatalf("consuming %s failed for the wrong reason: %v", queue, err)
			}
		})
	}
}

// TestAIUserCanPublishResults proves the write regex covers the results
// exchange — without it every capability would run and then fail to report.
func TestAIUserCanPublishResults(t *testing.T) {
	conn := provisionAIUser(t)

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	defer ch.Close()
	if err := ch.Confirm(false); err != nil {
		t.Fatalf("put channel in confirm mode: %v", err)
	}

	confirmation, err := ch.PublishWithDeferredConfirmWithContext(context.Background(),
		"jobfinder.results", "ghost.completed", false, false,
		amqp.Publishing{ContentType: "application/json", Body: []byte(`{"probe":true}`)})
	if err != nil {
		t.Fatalf("publish a result as the AI service: %v", err)
	}
	ok, err := confirmation.WaitContext(context.Background())
	if err != nil {
		t.Fatalf("await publisher confirm: %v", err)
	}
	if !ok {
		t.Fatal("the broker nacked a result published by the AI service")
	}
}

// TestAIUserCannotPublishWork proves the write regex is exactly
// `^jobfinder\.results$`: the AI service must not be able to inject work for
// itself or for the backend into the work exchange.
func TestAIUserCannotPublishWork(t *testing.T) {
	conn := provisionAIUser(t)

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	closed := ch.NotifyClose(make(chan *amqp.Error, 1))

	err = ch.PublishWithContext(context.Background(),
		"jobfinder.work", "ghost", false, false,
		amqp.Publishing{ContentType: "application/json", Body: []byte(`{"probe":true}`)})
	if err == nil {
		// A channel-level refusal arrives asynchronously; the publish call
		// itself can return before the broker closes the channel.
		select {
		case amqpErr := <-closed:
			if amqpErr == nil {
				t.Fatal("the AI service published to the work exchange")
			}
			if amqpErr.Code != amqp.AccessRefused {
				t.Fatalf("channel closed with %d (%s), want ACCESS_REFUSED", amqpErr.Code, amqpErr.Reason)
			}
			return
		case <-time.After(5 * time.Second):
			t.Fatal("the AI service published to the work exchange and the broker never objected")
		}
	}
	if !isAccessRefused(err) {
		t.Fatalf("publishing to the work exchange failed for the wrong reason: %v", err)
	}
}

// TestAIUserCannotDeclareTopology proves `"configure":"^$"` holds: topology
// belongs to the publisher (M1-1), and an AI service that could declare could
// also redeclare a queue with different arguments and silently break
// durability guarantees for everyone.
func TestAIUserCannotDeclareTopology(t *testing.T) {
	conn := provisionAIUser(t)

	t.Run("queue", func(t *testing.T) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel: %v", err)
		}
		defer ch.Close()

		if _, err := ch.QueueDeclare("work.ghost.rogue", true, false, false, false, nil); err == nil {
			t.Fatal("the AI service declared a queue")
		} else if !isAccessRefused(err) {
			t.Fatalf("queue declaration failed for the wrong reason: %v", err)
		}
	})

	t.Run("exchange", func(t *testing.T) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel: %v", err)
		}
		defer ch.Close()

		if err := ch.ExchangeDeclare("jobfinder.rogue", "topic", true, false, false, false, nil); err == nil {
			t.Fatal("the AI service declared an exchange")
		} else if !isAccessRefused(err) {
			t.Fatalf("exchange declaration failed for the wrong reason: %v", err)
		}
	})

	t.Run("existing topology", func(t *testing.T) {
		ch, err := conn.Channel()
		if err != nil {
			t.Fatalf("open channel: %v", err)
		}
		defer ch.Close()

		// Even a redeclaration of what already exists needs configure
		// permission, so the whole of events.DeclareTopology is refused.
		if err := events.DeclareTopology(ch); err == nil {
			t.Fatal("the AI service redeclared the backend's topology")
		}
	})
}

func isAccessRefused(err error) bool {
	var amqpErr *amqp.Error
	if ok := asAMQPError(err, &amqpErr); ok {
		return amqpErr.Code == amqp.AccessRefused
	}
	return strings.Contains(strings.ToUpper(err.Error()), "ACCESS_REFUSED")
}

func asAMQPError(err error, target **amqp.Error) bool {
	if converted, ok := err.(*amqp.Error); ok {
		*target = converted
		return true
	}
	return false
}
