package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/activity"
	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/events"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/queue"
)

type ActivityProvider interface {
	activity.Store
	GetActivityRun(ctx context.Context, id pgtype.UUID) (sqlcgen.ActivityRun, error)
	ListActiveActivityRuns(ctx context.Context) ([]sqlcgen.ActivityRun, error)
	ListRecentActivityRuns(ctx context.Context, limit int32) ([]sqlcgen.ActivityRun, error)
	ListFailedActivityRuns(ctx context.Context, op *string) ([]sqlcgen.ActivityRun, error)
}

type ActivityEnqueuer interface {
	EnqueueContext(ctx context.Context, workType string, payload []byte) error
}

type ActivityInspector interface {
	QueueDepth(queueName string) (events.QueueInfo, error)
}

var queueOrder = []string{
	queue.QueueIngest, queue.QueueMatch, queue.QueueGenerate,
	queue.QueueEnrich, queue.QueueSalaryInfer, queue.QueueGhostScore,
}

type ActivityHandler struct {
	q         ActivityProvider
	client    ActivityEnqueuer
	inspector ActivityInspector
	policies  []queue.TaskPolicy
	resolvers map[string]queue.ClassResolver
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

func (h *ActivityHandler) cancelOne(ctx context.Context, id string, op string) {
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

	info, err := h.inspector.QueueDepth(qname)
	if err != nil {
		msg := err.Error()
		entry.Error = &msg
		return entry
	}

	entry.Pending = info.MessagesReady
	entry.Active = info.MessagesUnacked
	return entry
}

func (h *ActivityHandler) policyFor(qname string) (queue.TaskPolicy, bool) {
	for _, p := range h.policies {
		if p.Queue == qname {
			return p, true
		}
	}
	return queue.TaskPolicy{}, false
}

func (h *ActivityHandler) effectiveConcurrency(qname string, policy queue.TaskPolicy) int {
	return policy.Concurrency
}

func (h *ActivityHandler) providerClass(qname string) *string {
	resolver := h.resolvers[qname]
	if resolver == nil {
		return nil
	}
	class := string(resolver.ProviderClass())
	return &class
}

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
		return h.client.EnqueueContext(ctx, queue.TypeGenerate, payload) == nil
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
		return h.client.EnqueueContext(ctx, queue.TypeIngest, payload) == nil
	default:
		return false
	}
}

func (h *ActivityHandler) enqueueJobTask(ctx context.Context, op, jobID string, actID *string) bool {
	var workType string
	var payload []byte
	var err error
	switch op {
	case "match":
		workType = queue.TypeMatch
		payload, err = json.Marshal(queue.MatchPayload{JobID: jobID, ActivityID: actID})
	case "enrich":
		workType = queue.TypeEnrich
		payload, err = json.Marshal(queue.EnrichPayload{JobID: jobID, ActivityID: actID})
	case "ghost_score":
		workType = queue.TypeGhostScore
		payload, err = json.Marshal(queue.GhostScorePayload{JobID: jobID, ActivityID: actID})
	case "salary_infer":
		workType = queue.TypeSalaryInfer
		payload, err = json.Marshal(queue.SalaryInferPayload{JobID: jobID, ActivityID: actID})
	default:
		return false
	}
	if err != nil {
		return false
	}
	return h.client.EnqueueContext(ctx, workType, payload) == nil
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
