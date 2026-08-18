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

	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/ghostjob/domain"
	"github.com/job-finder/api/internal/queue"
)

// GhostSnapshot is ghost's complete grounding input (E3-3): the posting
// fields and the already-measured signals MeasureSignals computes today,
// since FR-008 denies the orchestration service database access and the
// signals themselves (repost count, cross-board count, ...) require it.
type GhostSnapshot struct {
	JobID             string            `json:"job_id"`
	Title             string            `json:"title"`
	Company           string            `json:"company"`
	RepostCount       int               `json:"repost_count"`
	DaysOpen          *int              `json:"days_open,omitempty"`
	CrossBoardCount   *int              `json:"cross_board_count,omitempty"`
	AlwaysHiringCount *int              `json:"always_hiring_count,omitempty"`
	Notes             map[string]string `json:"notes,omitempty"`
}

// ghostRequestedMessage is the wire body for ghost.requested: the shared
// envelope merged with the existing GhostScorePayload plus the snapshot
// (E2, E3-2), matching the flattening every work event uses so the AI
// service's generated Pydantic model can be a single flat class.
type ghostRequestedMessage struct {
	events.Envelope
	events.GhostWork
}

// SnapshotEnqueuer wraps a base queue.Enqueuer, intercepting the "ghost"
// work type so that when AI_CAPABILITY_ROUTING routes ghost to python, the
// dispatch is an envelope-wrapped ghost.requested event carrying the
// snapshot instead of the bare GhostScorePayload the Go consumer expects.
// Every other work type — and ghost itself when routed to go — passes
// through to Base unchanged (C8-3: reversible by configuration alone).
type SnapshotEnqueuer struct {
	Base    queue.Enqueuer
	Repo    domain.Repository
	Pub     *events.Publisher
	Routing func(capability string) string
}

// EnqueueContext implements queue.Enqueuer.
func (e *SnapshotEnqueuer) EnqueueContext(ctx context.Context, workType string, payload []byte) error {
	if workType != Kind || e.Routing == nil || e.Routing(Kind) != "python" {
		return e.Base.EnqueueContext(ctx, workType, payload)
	}

	var p queue.GhostScorePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("ghostjob: invalid ghost enqueue payload: %w", err)
	}
	return e.publishRequested(ctx, p)
}

func (e *SnapshotEnqueuer) publishRequested(ctx context.Context, p queue.GhostScorePayload) error {
	uid, err := dbutil.ParseUUID(p.JobID)
	if err != nil {
		return fmt.Errorf("ghostjob: publish snapshot: %w", err)
	}
	job, err := e.Repo.GetJobByID(ctx, uid)
	if err != nil {
		return fmt.Errorf("ghostjob: publish snapshot: job %s: %w", p.JobID, err)
	}
	signals := MeasureSignals(ctx, e.Repo, job)

	snapshot := GhostSnapshot{
		JobID:             p.JobID,
		Title:             job.Title,
		Company:           job.Company,
		RepostCount:       signals.RepostCount,
		DaysOpen:          signals.DaysOpen,
		CrossBoardCount:   signals.CrossBoardCount,
		AlwaysHiringCount: signals.AlwaysHiringCount,
		Notes:             signals.Notes,
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("ghostjob: marshal snapshot: %w", err)
	}
	sum := sha256.Sum256(snapshotJSON)
	snapshotHash := "sha256:" + hex.EncodeToString(sum[:])

	correlationID := uuid.NewString()
	env := events.Envelope{
		EventID:        uuid.NewString(),
		EventType:      events.EventGhostRequested,
		SchemaVersion:  1,
		OccurredAt:     time.Now().UTC(),
		WorkID:         p.JobID,
		CorrelationID:  correlationID,
		IdempotencyKey: fmt.Sprintf("%s:%s:%s", Kind, p.JobID, correlationID),
		RunID:          uuid.NewString(),
		ActivityID:     p.ActivityID,
	}

	msg := ghostRequestedMessage{
		Envelope: env,
		GhostWork: events.GhostWork{
			GhostScorePayload: p,
			Snapshot:          events.InputSnapshot(snapshotJSON),
			SnapshotHash:      snapshotHash,
		},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("ghostjob: marshal ghost.requested: %w", err)
	}

	headers := amqp.Table{
		events.HeaderAttempt:  int32(0),
		events.HeaderWorkType: Kind,
	}
	return events.PublishWork(ctx, e.Pub, Kind, p.JobID, body, headers)
}
