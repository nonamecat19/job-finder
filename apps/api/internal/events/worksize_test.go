package events

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestPublishWork_OversizedMessageFailsExplicitly guards E3-5: an oversized
// snapshot is a publish error naming the work id and the size, never a
// silently truncated message, and it never reaches the channel.
func TestPublishWork_OversizedMessageFailsExplicitly(t *testing.T) {
	fc := newFakeChannel()
	pub, err := NewPublisher(fc, time.Second)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	body := make([]byte, MaxMessageSize+1)
	err = PublishWork(context.Background(), pub, "ghost", "job_01H", body, nil)
	if err == nil {
		t.Fatal("PublishWork: want error for oversized body, got nil")
	}
	if !strings.Contains(err.Error(), "job_01H") {
		t.Errorf("error = %q, want it to name the work id", err.Error())
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error = %q, want it to name the work type", err.Error())
	}
	if len(fc.published) != 0 {
		t.Errorf("published %d messages, want 0 (oversized body must never reach the channel)", len(fc.published))
	}
}

// TestPublishWork_WithinLimitPublishes guards the happy path: a body at or
// under MaxMessageSize is published unchanged.
func TestPublishWork_WithinLimitPublishes(t *testing.T) {
	fc := newFakeChannel()
	pub, err := NewPublisher(fc, time.Second)
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	body := []byte(`{"event_type":"ghost.requested"}`)
	if err := PublishWork(context.Background(), pub, "ghost", "job_01H", body, nil); err != nil {
		t.Fatalf("PublishWork: %v", err)
	}
	if len(fc.published) != 1 {
		t.Fatalf("published %d messages, want 1", len(fc.published))
	}
}
