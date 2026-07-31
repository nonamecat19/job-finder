package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/platform/llm"
	"github.com/job-finder/api/internal/queue"
)

// ActivityProvider is the inbound port the activity handler reads through.
// *sqlcgen.Queries satisfies it structurally.
type ActivityProvider interface {
	activity.Store
	GetActivityRun(ctx context.Context, id pgtype.UUID) (sqlcgen.ActivityRun, error)
	ListActiveActivityRuns(ctx context.Context) ([]sqlcgen.ActivityRun, error)
	ListRecentActivityRuns(ctx context.Context, limit int32) ([]sqlcgen.ActivityRun, error)
	ListFailedActivityRuns(ctx context.Context, op *string) ([]sqlcgen.ActivityRun, error)
}

// ActivityEnqueuer re-enqueues asynq tasks for retry. *asynq.Client satisfies it.
type ActivityEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// ActivityInspector cancels an in-flight or still-queued asynq task.
// *asynq.Inspector satisfies it. Every enqueue call across the codebase sets
// asynq.TaskID(<activity run id>), so the ActivityRun's own id doubles as the
// asynq task id — no extra column lookup needed.
type ActivityInspector interface {
	CancelProcessing(id string) error
	DeleteTask(queue, id string) error
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
}

// queueOrder is the fixed display order for GET /activity/queues
// (contracts/activity-api.md §2).
var queueOrder = []string{
	queue.QueueIngest, queue.QueueMatch, queue.QueueGenerate,
	queue.QueueEnrich, queue.QueueSalaryInfer, queue.QueueGhostScore,
}

// queueForOp maps an ActivityRun.Op back to the asynq queue it was enqueued
// on, so a pending (not yet running) task can be found and deleted.
var queueForOp = map[string]string{
	"ingest":       queue.QueueIngest,
	"match":        queue.QueueMatch,
	"generate":     queue.QueueGenerate,
	"enrich":       queue.QueueEnrich,
	"ghost_score":  queue.QueueGhostScore,
	"salary_infer": queue.QueueSalaryInfer,
}

type ActivityHandler struct {
	q         ActivityProvider
	client    ActivityEnqueuer
	inspector ActivityInspector
	policies  []queue.TaskPolicy
	resolvers map[string]queue.ClassResolver // keyed by queue name; nil for non-LLM queues
}

func NewActivityHandler(q ActivityProvider, client ActivityEnqueuer, inspector ActivityInspector, policies []queue.TaskPolicy, resolvers map[string]queue.ClassResolver) *ActivityHandler {
	return &ActivityHandler{q: q, client: client, inspector: inspector, policies: policies, resolvers: resolvers}
}

func (h *ActivityHandler) Mount(r chi.Router) {
	r.Get("/activity", h.list)
	r.Get("/activity/queues", h.queues)
	r.Post("/activity/retry", h.retry)
	r.Post("/activity/cancel-all", h.cancelAll)
	r.Post("/activity/{id}/cancel", h.cancel)
}

// cancel stops an active (queued or running) run: it asks asynq to cancel a
// running task via its context (best-effort — the handler must still notice
// ctx.Done() and return), or deletes a still-pending one outright, then marks
// the ActivityRun row "cancelled" regardless of whether asynq's side
// succeeded, so the UI reflects the user's intent immediately.
func (h *ActivityHandler) cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	uid, err := dbutil.ParseUUID(id)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid activity id")
		return
	}

	row, err := h.q.GetActivityRun(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "activity run not found: "+id)
		return
	}
	if row.State != "queued" && row.State != "running" {
		httpx.WriteError(w, http.StatusConflict, "activity run is not active")
		return
	}

	h.cancelOne(r.Context(), id, row.Op)
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}

// cancelAll stops every currently active (queued or running) run in one
// batch request, rather than making the client fire one cancel call per row.
func (h *ActivityHandler) cancelAll(w http.ResponseWriter, r *http.Request) {
	active, err := h.q.ListActiveActivityRuns(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list active activity runs: "+err.Error())
		return
	}

	for _, row := range active {
		h.cancelOne(r.Context(), dbutil.UUIDString(row.ID), row.Op)
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]int{"cancelled": len(active)})
}

// cancelOne asks asynq to stop the run's task (best-effort) and marks the
// ActivityRun row "cancelled" regardless of whether asynq's side succeeded,
// so the UI reflects the user's intent immediately.
//
// It tries both CancelProcessing (running) and DeleteTask (still queued)
// unconditionally rather than branching on a state snapshot: a queued row
// can flip to running between being listed and being cancelled here (the
// worker may grab it in that window), and gating on the stale state left
// the task to run to completion untouched while the row was marked
// cancelled underneath it. Each call is a no-op/logged-warning when it
// doesn't apply to the task's actual current state.
func (h *ActivityHandler) cancelOne(ctx context.Context, id string, op string) {
	if h.inspector != nil {
		if err := h.inspector.CancelProcessing(id); err != nil {
			slog.Warn("activity: cancel processing failed", "id", id, "error", err)
		}
		if qname, ok := queueForOp[op]; ok {
			if err := h.inspector.DeleteTask(qname, id); err != nil {
				slog.Warn("activity: delete pending task failed", "id", id, "queue", qname, "error", err)
			}
		}
	}

	rec := activity.FromID(h.q, id)
	if rec != nil {
		rec.Cancel(ctx, "cancelled by user")
	}
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
		httpx.WriteError(w, http.StatusInternalServerError, "list active activity runs: "+err.Error())
		return
	}

	recent, err := h.q.ListRecentActivityRuns(r.Context(), limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list recent activity runs: "+err.Error())
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

	httpx.WriteJSON(w, http.StatusOK, resp)
}

// queues reports a per-queue backlog snapshot (019-ai-job-throughput,
// contracts/activity-api.md §2): pending/active/scheduled/retry/archived
// counts, the effective admission concurrency for the currently resolved
// provider class, and a throughput-derived ETA. An Inspector failure for one
// queue is reported per-entry rather than failing the whole response; total
// Inspector unavailability (nil inspector) yields 503.
func (h *ActivityHandler) queues(w http.ResponseWriter, r *http.Request) {
	if h.inspector == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "queue inspector unavailable")
		return
	}

	entries := make([]dto.QueueBacklogDto, 0, len(queueOrder))
	for _, qname := range queueOrder {
		entries = append(entries, h.queueBacklog(qname))
	}
	httpx.WriteJSON(w, http.StatusOK, dto.QueueBacklogResponse{Queues: entries})
}

func (h *ActivityHandler) queueBacklog(qname string) dto.QueueBacklogDto {
	entry := dto.QueueBacklogDto{Queue: qname}

	policy, ok := h.policyFor(qname)
	if ok {
		entry.Concurrency = h.effectiveConcurrency(qname, policy)
		if class := h.providerClass(qname); class != nil {
			entry.ProviderClass = class
		}
	}

	info, err := h.inspector.GetQueueInfo(qname)
	if err != nil {
		msg := err.Error()
		entry.Error = &msg
		return entry
	}

	entry.Pending = info.Pending
	entry.Active = info.Active
	entry.Scheduled = info.Scheduled
	entry.Retry = info.Retry
	entry.Archived = info.Archived

	minutesToday := time.Since(startOfDay(info.Timestamp)).Minutes()
	if minutesToday < 1 {
		minutesToday = 1
	}
	entry.ProcessedPerMinute = float64(info.Processed) / minutesToday

	if entry.ProcessedPerMinute > 0 && entry.Pending > 0 {
		eta := int(float64(entry.Pending) / entry.ProcessedPerMinute * 60)
		entry.EtaSeconds = &eta
	}
	return entry
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func (h *ActivityHandler) policyFor(qname string) (queue.TaskPolicy, bool) {
	for _, p := range h.policies {
		if p.Queue == qname {
			return p, true
		}
	}
	return queue.TaskPolicy{}, false
}

// effectiveConcurrency is the admission capacity for the currently resolved
// provider class, not the pool size (contracts/activity-api.md §2).
func (h *ActivityHandler) effectiveConcurrency(qname string, policy queue.TaskPolicy) int {
	resolver := h.resolvers[qname]
	if resolver == nil {
		return policy.LocalConcurrency
	}
	if resolver.ProviderClass() == llm.ProviderClassHosted {
		return policy.HostedConcurrency
	}
	return policy.LocalConcurrency
}

func (h *ActivityHandler) providerClass(qname string) *string {
	resolver := h.resolvers[qname]
	if resolver == nil {
		return nil
	}
	class := string(resolver.ProviderClass())
	return &class
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
		httpx.WriteError(w, http.StatusInternalServerError, "list failed activity runs: "+err.Error())
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

	httpx.WriteJSON(w, http.StatusOK, map[string]int{"retried": retried, "skipped": skipped})
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
		opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueGenerate)}
		if actID != nil {
			opts = append(opts, asynq.TaskID(*actID))
		}
		_, err = h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeGenerate, payload), opts...)
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
		opts := []asynq.Option{asynq.MaxRetry(0), asynq.Queue(queue.QueueIngest)}
		if actID != nil {
			opts = append(opts, asynq.TaskID(*actID))
		}
		_, err = h.client.EnqueueContext(ctx, asynq.NewTask(queue.TypeIngest, payload), opts...)
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
		ID:          dbutil.UUIDString(row.ID),
		Op:          row.Op,
		State:       row.State,
		Label:       row.Label,
		Step:        row.Step,
		JobID:       dbutil.UUIDStringPtr(row.JobId),
		SourceKey:   row.SourceKey,
		RefID:       row.RefId,
		Error:       row.Error,
		Meta:        meta,
		CreatedAt:   dbutil.Timestamp(row.CreatedAt),
		StartedAt:   dbutil.TimestampPtr(row.StartedAt),
		FinishedAt:  dbutil.TimestampPtr(row.FinishedAt),
		ElapsedMs:   elapsedMs,
		HeartbeatAt: dbutil.TimestampPtr(row.HeartbeatAt),
		TimeoutMs:   row.TimeoutMs,
	}
}
