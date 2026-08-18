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

// Kind is match's work-type/capability name, matching queue.TypeMatch and
// the capability name AI_CAPABILITY_ROUTING keys on.
const Kind = "match"

// MatchSnapshot is match's complete grounding input for the LLM fit-analysis
// step only (E3-3): embedding and similarity prefiltering stay in Go
// (MatchJob, data-model.md § 2) and only run once, regardless of routing —
// this is the same profile/job text the Go prompt builds from today, field
// names mirroring match.py's MatchSnapshot exactly (snake_case).
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

// Publisher publishes match.requested once MatchJob's own embedding and
// similarity-prefilter work has passed the threshold and AI_CAPABILITY_ROUTING
// routes "match" to python. Unlike ghostjob/salary's SnapshotEnqueuer, this
// is not the whole task: MatchJob keeps running its own DB-backed steps
// (embedding, prefilter, initial persistence) and only defers the LLM call
// itself, so publishing happens from inside MatchJob rather than by
// intercepting queue.Enqueuer at the work item's entry.
type Publisher struct {
	Pub *events.Publisher
}

// PublishRequested publishes one match.requested event carrying snapshot.
// activityID is the current run's correlation id (MatchJob's rec.ID(), same
// value llm.WithTraceID stamps on the go path's LLM call) so a python-routed
// run is traceable back to the same activity record.
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
