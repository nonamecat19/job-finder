// Package application holds the activity-tracking use-case: list active/
// recent/failed runs and re-enqueue failed ones, shaping sqlcgen.ActivityRun
// rows into DTOs so the HTTP layer never sees a raw persistence row. Mirrors
// activities.controller.ts's list/retry endpoints.
package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/queue"
)

// Repository is the outbound persistence port for the activity-tracking
// use-case. *sqlcgen.Queries satisfies it structurally. It embeds
// activity.Store since Retry starts a fresh Recorder for each re-enqueued run.
type Repository interface {
	activity.Store
	ListActiveActivityRuns(ctx context.Context) ([]sqlcgen.ActivityRun, error)
	ListRecentActivityRuns(ctx context.Context, limit int32) ([]sqlcgen.ActivityRun, error)
	ListFailedActivityRuns(ctx context.Context, op *string) ([]sqlcgen.ActivityRun, error)
}

// Enqueuer is the outbound task-queue port. *asynq.Client satisfies it.
type Enqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Service is the activity-tracking use-case: list + retry, mirroring
// activities.controller.ts.
type Service struct {
	q      Repository
	client Enqueuer
}

func NewService(q Repository, client Enqueuer) *Service {
	return &Service{q: q, client: client}
}

// List returns the active runs plus up to limit recent runs, DTO-shaped.
func (s *Service) List(ctx context.Context, limit int32) (dto.ActivityListResponse, error) {
	active, err := s.q.ListActiveActivityRuns(ctx)
	if err != nil {
		return dto.ActivityListResponse{}, err
	}
	recent, err := s.q.ListRecentActivityRuns(ctx, limit)
	if err != nil {
		return dto.ActivityListResponse{}, err
	}

	resp := dto.ActivityListResponse{
		Active: make([]dto.ActivityRunDto, 0, len(active)),
		Recent: make([]dto.ActivityRunDto, 0, len(recent)),
	}
	for _, row := range active {
		resp.Active = append(resp.Active, rowToDto(row))
	}
	for _, row := range recent {
		resp.Recent = append(resp.Recent, rowToDto(row))
	}
	return resp, nil
}

// Retry re-enqueues every failed ActivityRun, or only those matching opFilter
// when given. Each retry creates a fresh ActivityRun for tracking — the
// original failed row is left as-is for history. Rows that carry no jobId, or
// a "generate" row from before docType/profileId started being persisted to
// meta (jobs.Service.EnqueueGeneration), are skipped and counted separately.
func (s *Service) Retry(ctx context.Context, opFilter *string) (retried, skipped int, err error) {
	rows, err := s.q.ListFailedActivityRuns(ctx, opFilter)
	if err != nil {
		return 0, 0, err
	}

	for _, row := range rows {
		if s.retryOne(ctx, row) {
			retried++
		} else {
			skipped++
		}
	}
	return retried, skipped, nil
}

// retryOne re-enqueues a single failed run based on its op. Returns false
// when the op/row doesn't carry enough data to retry (e.g. no jobId).
func (s *Service) retryOne(ctx context.Context, row sqlcgen.ActivityRun) bool {
	jobID := dbutil.UUIDStringPtr(row.JobId)

	switch row.Op {
	case "match", "enrich", "ghost_score", "salary_infer":
		if jobID == nil {
			return false
		}
		rec := activity.New(ctx, s.q, row.Op, row.Label, jobID, row.SourceKey, "")
		var actID *string
		if rec != nil {
			id := dbutil.UUIDString(rec.ID())
			actID = &id
		}
		return s.enqueueJobTask(ctx, row.Op, *jobID, actID)
	case "generate":
		if jobID == nil {
			return false
		}
		var meta map[string]any
		if len(row.Meta) > 0 {
			_ = dbutil.UnmarshalJSONB(row.Meta, &meta)
		}
		docType, _ := meta["docType"].(string)
		if docType == "" {
			// Older run, from before docType/profileId started being
			// persisted (see jobs.Service.EnqueueGeneration) — nothing to
			// rebuild the payload from.
			return false
		}
		var profileID *string
		if pid, ok := meta["profileId"].(string); ok && pid != "" {
			profileID = &pid
		}
		rec := activity.New(ctx, s.q, "generate", row.Label, jobID, nil, "")
		var actID *string
		if rec != nil {
			id := dbutil.UUIDString(rec.ID())
			actID = &id
			ms := map[string]any{"docType": docType}
			if profileID != nil {
				ms["profileId"] = *profileID
			}
			rec.Step(ctx, "queued", ms)
		}
		payload, err := json.Marshal(queue.GeneratePayload{JobID: *jobID, Type: docType, ProfileID: profileID, ActivityID: actID})
		if err != nil {
			return false
		}
		_, err = s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGenerate, payload),
			asynq.MaxRetry(0), asynq.Queue(queue.QueueGenerate))
		return err == nil
	case "ingest":
		if row.SourceKey == nil {
			return false
		}
		rec := activity.New(ctx, s.q, "ingest", row.Label, nil, row.SourceKey, "")
		var actID *string
		if rec != nil {
			id := dbutil.UUIDString(rec.ID())
			actID = &id
		}
		payload, err := json.Marshal(queue.IngestPayload{SourceKey: *row.SourceKey, ActivityID: actID})
		if err != nil {
			return false
		}
		_, err = s.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload),
			asynq.MaxRetry(0), asynq.Queue(queue.QueueIngest))
		return err == nil
	default:
		return false
	}
}

func (s *Service) enqueueJobTask(ctx context.Context, op, jobID string, actID *string) bool {
	var task *asynq.Task
	var q string
	switch op {
	case "match":
		payload, err := json.Marshal(queue.MatchPayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			return false
		}
		task, q = asynq.NewTask(queue.TypeMatch, payload), queue.QueueMatch
	case "enrich":
		payload, err := json.Marshal(queue.EnrichPayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			return false
		}
		task, q = asynq.NewTask(queue.TypeEnrich, payload), queue.QueueEnrich
	case "ghost_score":
		payload, err := json.Marshal(queue.GhostScorePayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			return false
		}
		task, q = asynq.NewTask(queue.TypeGhostScore, payload), queue.QueueGhostScore
	case "salary_infer":
		payload, err := json.Marshal(queue.SalaryInferPayload{JobID: jobID, ActivityID: actID})
		if err != nil {
			return false
		}
		task, q = asynq.NewTask(queue.TypeSalaryInfer, payload), queue.QueueSalaryInfer
	default:
		return false
	}
	_, err := s.client.EnqueueContext(ctx, task, asynq.MaxRetry(0), asynq.Queue(q))
	return err == nil
}

func rowToDto(row sqlcgen.ActivityRun) dto.ActivityRunDto {
	var meta map[string]any
	if len(row.Meta) > 0 {
		_ = dbutil.UnmarshalJSONB(row.Meta, &meta)
	}
	if meta == nil {
		meta = map[string]any{}
	}

	now := time.Now().UTC()
	var elapsedMs *int64
	if row.StartedAt.Valid {
		var end time.Time
		if row.FinishedAt.Valid {
			end = row.FinishedAt.Time
		} else {
			end = now
		}
		ms := end.Sub(row.StartedAt.Time).Milliseconds()
		elapsedMs = &ms
	}

	return dto.ActivityRunDto{
		ID:         dbutil.UUIDString(row.ID),
		Op:         row.Op,
		State:      row.State,
		Label:      row.Label,
		Step:       row.Step,
		JobID:      dbutil.UUIDStringPtr(row.JobId),
		SourceKey:  row.SourceKey,
		RefID:      row.RefId,
		Error:      row.Error,
		Meta:       meta,
		CreatedAt:  dbutil.Timestamp(row.CreatedAt),
		StartedAt:  dbutil.TimestampPtr(row.StartedAt),
		FinishedAt: dbutil.TimestampPtr(row.FinishedAt),
		ElapsedMs:  elapsedMs,
	}
}
