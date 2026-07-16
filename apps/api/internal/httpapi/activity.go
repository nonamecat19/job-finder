package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

type ActivityHandler struct {
	q *sqlcgen.Queries
}

func NewActivityHandler(q *sqlcgen.Queries) *ActivityHandler {
	return &ActivityHandler{q: q}
}

func (h *ActivityHandler) Mount(r chi.Router) {
	r.Get("/activity", h.list)
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
