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
	"github.com/job-finder/api/internal/queue"
)

// ComparableBand is one row lookup_comparable_bands would have returned for
// the posting's own bucket at publish time — the same rows GetSalaryCacheByBucket
// serves the Go tool loop today, pre-fetched because FR-008 denies the
// orchestration service database access (E3-2).
type ComparableBand struct {
	Source   string `json:"source"`
	Min      int    `json:"min"`
	Max      int    `json:"max"`
	Currency string `json:"currency"`
}

// SalarySnapshot is salary's complete grounding input (E3-3): the posting
// fields llmInfer reads plus the comparable bands for its own bucket, using
// the same makeBucket logic the Go tool uses today (Python's make_bucket is a
// verbatim port, salary_tools.py).
type SalarySnapshot struct {
	JobID           string           `json:"job_id"`
	Title           string           `json:"title"`
	Company         string           `json:"company"`
	Location        *string          `json:"location,omitempty"`
	Remote          bool             `json:"remote"`
	Description     string           `json:"description"`
	ComparableBands []ComparableBand `json:"comparable_bands"`
}

type salaryRequestedMessage struct {
	events.Envelope
	events.SalaryWork
}

// SnapshotEnqueuer wraps a base queue.Enqueuer, intercepting the "salary" work
// type so that when AI_CAPABILITY_ROUTING routes salary to python, the
// dispatch is an envelope-wrapped salary.requested event carrying the
// snapshot instead of the bare SalaryInferPayload the Go consumer expects.
// Every other work type — and salary itself when routed to go — passes
// through to Base unchanged, mirroring ghostjob.SnapshotEnqueuer.
type SnapshotEnqueuer struct {
	Base    queue.Enqueuer
	Repo    Repository
	Pub     *events.Publisher
	Routing func(capability string) string
}

// EnqueueContext implements queue.Enqueuer.
func (e *SnapshotEnqueuer) EnqueueContext(ctx context.Context, workType string, payload []byte) error {
	if workType != Kind || e.Routing == nil || e.Routing(Kind) != "python" {
		return e.Base.EnqueueContext(ctx, workType, payload)
	}

	var p queue.SalaryInferPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("salary: invalid salary enqueue payload: %w", err)
	}
	return e.publishRequested(ctx, p)
}

func (e *SnapshotEnqueuer) publishRequested(ctx context.Context, p queue.SalaryInferPayload) error {
	uid, err := dbutil.ParseUUID(p.JobID)
	if err != nil {
		return fmt.Errorf("salary: publish snapshot: %w", err)
	}
	job, err := e.Repo.GetJobByID(ctx, uid)
	if err != nil {
		return fmt.Errorf("salary: publish snapshot: job %s: %w", p.JobID, err)
	}

	bucket := makeBucket(job.Title, ptrStr(job.Location), "")
	rows, err := e.Repo.GetSalaryCacheByBucket(ctx, bucket)
	if err != nil {
		return fmt.Errorf("salary: publish snapshot: comparable bands for %s: %w", p.JobID, err)
	}
	bands := make([]ComparableBand, 0, len(rows))
	for _, r := range rows {
		bands = append(bands, ComparableBand{
			Source:   r.Source,
			Min:      intVal(r.SalaryMin),
			Max:      intVal(r.SalaryMax),
			Currency: r.Currency,
		})
	}

	snapshot := SalarySnapshot{
		JobID:           p.JobID,
		Title:           job.Title,
		Company:         job.Company,
		Location:        job.Location,
		Remote:          job.Remote,
		Description:     job.Description,
		ComparableBands: bands,
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("salary: marshal snapshot: %w", err)
	}
	sum := sha256.Sum256(snapshotJSON)
	snapshotHash := "sha256:" + hex.EncodeToString(sum[:])

	correlationID := uuid.NewString()
	env := events.Envelope{
		EventID:        uuid.NewString(),
		EventType:      events.EventSalaryRequested,
		SchemaVersion:  1,
		OccurredAt:     time.Now().UTC(),
		WorkID:         p.JobID,
		CorrelationID:  correlationID,
		IdempotencyKey: fmt.Sprintf("%s:%s:%s", Kind, p.JobID, correlationID),
		RunID:          uuid.NewString(),
		ActivityID:     p.ActivityID,
	}

	msg := salaryRequestedMessage{
		Envelope: env,
		SalaryWork: events.SalaryWork{
			SalaryInferPayload: p,
			Snapshot:           events.InputSnapshot(snapshotJSON),
			SnapshotHash:       snapshotHash,
		},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("salary: marshal salary.requested: %w", err)
	}

	headers := amqp.Table{
		events.HeaderAttempt:  int32(0),
		events.HeaderWorkType: Kind,
	}
	return events.PublishWork(ctx, e.Pub, Kind, p.JobID, body, headers)
}
