package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/queue"
)

const Kind = "match"

type MatchSnapshot struct {
	ProfileText string `json:"profile_text"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	Remote      bool   `json:"remote"`
	Description string `json:"description"`
}

type matchRequestedMessage struct {
	events.Envelope
	events.MatchWork
}

type Publisher struct {
	Pub *events.Publisher
}

func (p *Publisher) PublishRequested(ctx context.Context, jobID string, activityID *string, snapshot MatchSnapshot) error {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("matching: marshal snapshot: %w", err)
	}
	sum := sha256.Sum256(snapshotJSON)
	snapshotHash := "sha256:" + hex.EncodeToString(sum[:])

	correlationID := uuid.NewString()
	env := events.Envelope{
		EventID:        uuid.NewString(),
		EventType:      events.EventMatchRequested,
		SchemaVersion:  1,
		OccurredAt:     time.Now().UTC(),
		WorkID:         jobID,
		CorrelationID:  correlationID,
		IdempotencyKey: fmt.Sprintf("%s:%s:%s", Kind, jobID, correlationID),
		RunID:          uuid.NewString(),
		ActivityID:     activityID,
	}

	msg := matchRequestedMessage{
		Envelope: env,
		MatchWork: events.MatchWork{
			MatchPayload: queue.MatchPayload{JobID: jobID, ActivityID: activityID},
			Snapshot:     events.InputSnapshot(snapshotJSON),
			SnapshotHash: snapshotHash,
		},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("matching: marshal match.requested: %w", err)
	}

	headers := amqp.Table{
		events.HeaderAttempt:  int32(0),
		events.HeaderWorkType: Kind,
	}
	return events.PublishWork(ctx, p.Pub, Kind, jobID, body, headers)
}
