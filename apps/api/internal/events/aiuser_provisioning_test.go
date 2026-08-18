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

const (
	aiUser = "ai_service"
	aiPass = "testinfra-ai-secret"
)

func provisionAIUser(t *testing.T) *amqp.Connection {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

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
