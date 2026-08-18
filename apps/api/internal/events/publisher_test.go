package events

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeChannel struct {
	publishErr   error
	confirmAck   bool
	sendConfirm  bool
	sendReturn   bool
	confirmDelay time.Duration

	confirmCh chan amqp.Confirmation
	returnCh  chan amqp.Return

	published []amqp.Publishing
}

func newFakeChannel() *fakeChannel {
	return &fakeChannel{
		confirmCh:   make(chan amqp.Confirmation, 1),
		returnCh:    make(chan amqp.Return, 1),
		sendConfirm: true,
		confirmAck:  true,
	}
}

func (f *fakeChannel) Confirm(noWait bool) error { return nil }

func (f *fakeChannel) NotifyPublish(ch chan amqp.Confirmation) chan amqp.Confirmation {
	return f.confirmCh
}

func (f *fakeChannel) NotifyReturn(ch chan amqp.Return) chan amqp.Return {
	return f.returnCh
}

func (f *fakeChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	f.published = append(f.published, msg)
	if f.publishErr != nil {
		return f.publishErr
	}
	go func() {
		if f.confirmDelay > 0 {
			time.Sleep(f.confirmDelay)
		}
		if f.sendReturn {
			f.returnCh <- amqp.Return{Exchange: exchange, RoutingKey: key}
		}
		if f.sendConfirm {
			f.confirmCh <- amqp.Confirmation{Ack: f.confirmAck}
		}
	}()
	return nil
}

func TestPublish_Success(t *testing.T) {
	fc := newFakeChannel()
	pub, err := NewPublisher(fc, time.Second)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	if err := pub.Publish(context.Background(), WorkExchange, "match", []byte(`{}`), nil); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(fc.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(fc.published))
	}
	if fc.published[0].DeliveryMode != amqp.Persistent {
		t.Errorf("DeliveryMode = %d, want amqp.Persistent", fc.published[0].DeliveryMode)
	}
}

func TestPublish_NackedReturnsError(t *testing.T) {
	fc := newFakeChannel()
	fc.confirmAck = false
	pub, err := NewPublisher(fc, time.Second)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	err = pub.Publish(context.Background(), WorkExchange, "match", []byte(`{}`), nil)
	if err == nil {
		t.Fatal("Publish succeeded on a nack, want error")
	}
}

func TestPublish_UnroutableReturnsError(t *testing.T) {
	fc := newFakeChannel()
	fc.sendReturn = true
	fc.confirmAck = true
	pub, err := NewPublisher(fc, time.Second)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	err = pub.Publish(context.Background(), WorkExchange, "no-such-route", []byte(`{}`), nil)
	if err == nil {
		t.Fatal("Publish succeeded on an unroutable (returned) message, want error")
	}
}

func TestPublish_ConfirmTimeoutReturnsError(t *testing.T) {
	fc := newFakeChannel()
	fc.sendConfirm = false
	pub, err := NewPublisher(fc, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	err = pub.Publish(context.Background(), WorkExchange, "match", []byte(`{}`), nil)
	if err == nil {
		t.Fatal("Publish succeeded despite no confirmation ever arriving, want timeout error")
	}
}

func TestPublish_ChannelPublishErrorReturnsError(t *testing.T) {
	fc := newFakeChannel()
	fc.publishErr = context.DeadlineExceeded
	pub, err := NewPublisher(fc, time.Second)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	err = pub.Publish(context.Background(), WorkExchange, "match", []byte(`{}`), nil)
	if err == nil {
		t.Fatal("Publish succeeded despite the underlying channel publish failing, want error")
	}
}
