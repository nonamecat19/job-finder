//go:build integration

package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbtest"
	"github.com/job-finder/api/internal/events"
)

func openConsumer(t *testing.T, conn *amqp.Connection, queue string) (*amqp.Channel, <-chan amqp.Delivery) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		t.Fatalf("set prefetch: %v", err)
	}
	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume %s: %v", queue, err)
	}
	return ch, deliveries
}

func nextDelivery(t *testing.T, deliveries <-chan amqp.Delivery, timeout time.Duration) amqp.Delivery {
	t.Helper()
	select {
	case d, ok := <-deliveries:
		if !ok {
			t.Fatal("delivery channel closed before a message arrived")
		}
		return d
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a delivery")
		return amqp.Delivery{}
	}
}

func consumeOne(t *testing.T, conn *amqp.Connection, queue string, timeout time.Duration) (*amqp.Channel, amqp.Delivery) {
	t.Helper()
	ch, deliveries := openConsumer(t, conn, queue)
	return ch, nextDelivery(t, deliveries, timeout)
}

func TestRedelivery_Integration_ConsumerCrashMidProcessingYieldsExactlyOneAcceptedResult(t *testing.T) {
	brokerConn := dialTestBroker(t)
	declareTopologyOrFail(t, brokerConn)

	const workType = "ghost"
	purgeQueue(t, brokerConn, "work."+workType)

	pub, pubCh := newTestPublisher(t, brokerConn)
	defer pubCh.Close()

	key := "ghost:crash:" + uuid.NewString()
	runID := uuid.NewString()
	env := newTestEnvelope(workType+".requested", "job_"+uuid.NewString(), key, runID)
	publishWork(t, pub, workType, mustMarshal(t, env))

	crashConn, err := amqp.Dial(testBrokerURL(t))
	if err != nil {
		t.Fatalf("dial crashing consumer: %v", err)
	}
	_, first := consumeOne(t, crashConn, "work."+workType, 10*time.Second)
	if first.Redelivered {
		t.Fatal("first delivery must not already be marked redelivered")
	}
	if err := crashConn.Close(); err != nil {
		t.Fatalf("simulate crash by closing connection: %v", err)
	}

	recoverConn, err := amqp.Dial(testBrokerURL(t))
	if err != nil {
		t.Fatalf("dial recovering consumer: %v", err)
	}
	defer recoverConn.Close()
	recoverCh, second := consumeOne(t, recoverConn, "work."+workType, 10*time.Second)
	defer recoverCh.Close()
	if !second.Redelivered {
		t.Error("second delivery should be marked redelivered by the broker after the crash")
	}

	testDB := dbtest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var disposition events.Disposition
	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		d, aerr := events.Admit(ctx, q, workType, key, runID)
		disposition = d
		return aerr
	}); err != nil {
		t.Fatalf("admit on recovered delivery: %v", err)
	}
	if disposition != events.Accepted {
		t.Fatalf("expected the recovered delivery to be Accepted (nothing was ever persisted by the crashed consumer), got %s", disposition)
	}
	if err := second.Ack(false); err != nil {
		t.Fatalf("ack recovered delivery: %v", err)
	}

	if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
		d, aerr := events.Admit(ctx, q, workType, key, runID)
		disposition = d
		return aerr
	}); err != nil {
		t.Fatalf("admit on simulated further redelivery: %v", err)
	}
	if disposition != events.Duplicate {
		t.Fatalf("expected a further redelivery to be Duplicate, got %s", disposition)
	}

	entry, err := testDB.Queries.GetIdempotencyLedgerEntry(ctx, key)
	if err != nil {
		t.Fatalf("get ledger entry: %v", err)
	}
	if got := entry.WorkType; got != workType {
		t.Errorf("ledger work_type = %q, want %q", got, workType)
	}
}

func TestRedelivery_Integration_DuplicateDeliveryDoesNotDuplicateOrCorruptResult(t *testing.T) {
	brokerConn := dialTestBroker(t)
	declareTopologyOrFail(t, brokerConn)

	const workType = "salary"
	purgeQueue(t, brokerConn, "work."+workType)

	pub, pubCh := newTestPublisher(t, brokerConn)
	defer pubCh.Close()

	key := "salary:dup:" + uuid.NewString()
	runID := uuid.NewString()
	env := newTestEnvelope(workType+".requested", "job_"+uuid.NewString(), key, runID)
	publishWork(t, pub, workType, mustMarshal(t, env))

	testDB := dbtest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	stored := map[string]string{}
	writes := 0

	admitAndMaybePersist := func(d amqp.Delivery) events.Disposition {
		var disposition events.Disposition
		if err := testDB.WithinTx(ctx, func(q *sqlcgen.Queries) error {
			dd, aerr := events.Admit(ctx, q, workType, key, runID)
			disposition = dd
			return aerr
		}); err != nil {
			t.Fatalf("admit: %v", err)
		}
		if disposition == events.Accepted {
			stored[key] = "result-for-" + runID
			writes++
		}
		return disposition
	}

	consumerConn, err := amqp.Dial(testBrokerURL(t))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer consumerConn.Close()

	ch, deliveries := openConsumer(t, consumerConn, "work."+workType)
	first := nextDelivery(t, deliveries, 10*time.Second)
	firstDisposition := admitAndMaybePersist(first)
	if firstDisposition != events.Accepted {
		t.Fatalf("expected first delivery to be Accepted, got %s", firstDisposition)
	}

	if err := first.Nack(false, true); err != nil {
		t.Fatalf("nack with requeue: %v", err)
	}

	second := nextDelivery(t, deliveries, 10*time.Second)
	if !second.Redelivered {
		t.Error("second delivery should be marked redelivered")
	}
	secondDisposition := admitAndMaybePersist(second)
	if secondDisposition != events.Duplicate {
		t.Fatalf("expected duplicate delivery to be Duplicate, got %s", secondDisposition)
	}
	if err := second.Ack(false); err != nil {
		t.Fatalf("ack: %v", err)
	}
	ch.Close()

	if writes != 1 {
		t.Fatalf("result was persisted %d times, want exactly 1 — duplicate delivery must not duplicate the stored result", writes)
	}
	if got := stored[key]; got != "result-for-"+runID {
		t.Fatalf("stored result = %q, want %q — duplicate delivery must not corrupt the stored result", got, "result-for-"+runID)
	}

	entry, err := testDB.Queries.GetIdempotencyLedgerEntry(ctx, key)
	if err != nil {
		t.Fatalf("get ledger entry: %v", err)
	}
	if got := entry.WorkType; got != workType {
		t.Errorf("ledger work_type = %q, want %q", got, workType)
	}
}
