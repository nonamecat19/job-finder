package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/queue"
)

// ActivityProvider is the inbound port the activity handler reads through.
// *sqlcgen.Queries satisfies it structurally.
type ActivityProvider interface {
	activity.Store
	ListActiveActivityRuns(ctx context.Context) ([]sqlcgen.ActivityRun, error)
	ListRecentActivityRuns(ctx context.Context, limit int32) ([]sqlcgen.ActivityRun, error)
	ListFailedActivityRuns(ctx context.Context, op *string) ([]sqlcgen.ActivityRun, error)
}

// ActivityEnqueuer re-enqueues asynq tasks for retry. *asynq.Client satisfies it.
type ActivityEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

type ActivityHandler struct {
	q      ActivityProvider
	client ActivityEnqueuer
}

func NewActivityHandler(q ActivityProvider, client ActivityEnqueuer) *ActivityHandler {
	return &ActivityHandler{q: q, client: client}
}

func (h *ActivityHandler) Mount(r chi.Router) {
	r.Get("/activity", h.list)
	r.Post("/activity/retry", h.retry)
}

func (h *ActivityHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := int32(100)
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = int32(v)
		}
	}

	active, err := h.q.ListActiveActivityRuns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list active activity runs: "+err.Error())
		return
	}

	recent, err := h.q.ListRecentActivityRuns(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list recent activity runs: "+err.Error())
		return
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

	writeJSON(w, http.StatusOK, resp)
}

// retry re-enqueues every failed ActivityRun, or only those matching
// ?op=<op> when given. Each retry creates a fresh ActivityRun for tracking —
// the original failed row is left as-is for history. Rows that carry no
// jobId, or a "generate" row from before docType/profileId started being
// persisted to meta (jobs.Service.EnqueueGeneration), are skipped and
// counted separately.
func (h *ActivityHandler) retry(w http.ResponseWriter, r *http.Request) {
	var opFilter *string
	if op := r.URL.Query().Get("op"); op != "" {
		opFilter = &op
	}

	rows, err := h.q.ListFailedActivityRuns(r.Context(), opFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed activity runs: "+err.Error())
		return
	}

	retried, skipped := 0, 0
	for _, row := range rows {
		if h.retryOne(r.Context(), row) {
			retried++
		} else {
			skipped++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"retried": retried, "skipped": skipped})
}

// retryOne re-enqueues a single failed run based on its op. Returns false
// when the op/row doesn't carry enough data to retry (e.g. no jobId).
func (h *ActivityHandler) retryOne(ctx context.Context, row sqlcgen.ActivityRun) bool {
	jobID := dbutil.UUIDStringPtr(row.JobId)

	switch row.Op {
	case "match", "enrich", "ghost_score", "salary_infer":
		if jobID == nil {
			return false
		}
		rec := activity.New(ctx, h.q, row.Op, row.Label, jobID, row.SourceKey, "")
		var actID *string
		if rec != nil {
			id := dbutil.UUIDString(rec.ID())
			actID = &id
		}
		return h.enqueueJobTask(ctx, row.Op, *jobID, actID)
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
		rec := activity.New(ctx, h.q, "generate", row.Label, jobID, nil, "")
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
		_, err = h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGenerate, payload),
			asynq.MaxRetry(0), asynq.Queue(queue.QueueGenerate))
		return err == nil
	case "ingest":
		if row.SourceKey == nil {
			return false
		}
		rec := activity.New(ctx, h.q, "ingest", row.Label, nil, row.SourceKey, "")
		var actID *string
		if rec != nil {
			id := dbutil.UUIDString(rec.ID())
			actID = &id
		}
		payload, err := json.Marshal(queue.IngestPayload{SourceKey: *row.SourceKey, ActivityID: actID})
		if err != nil {
			return false
		}
		_, err = h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload),
			asynq.MaxRetry(0), asynq.Queue(queue.QueueIngest))
		return err == nil
	default:
		return false
	}
}

func (h *ActivityHandler) enqueueJobTask(ctx context.Context, op, jobID string, actID *string) bool {
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
	_, err := h.client.EnqueueContext(ctx, task, asynq.MaxRetry(0), asynq.Queue(q))
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
