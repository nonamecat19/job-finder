package http

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/jobs"
)

type JobsProvider interface {
	List(ctx context.Context, params jobs.ListParams) (dto.JobListResponse, error)
	Get(ctx context.Context, id string) (dto.JobDto, error)
	Shortlist(ctx context.Context, id string) (dto.JobDto, error)
	Hide(ctx context.Context, id string) (dto.JobDto, error)
	Unhide(ctx context.Context, id string) (dto.JobDto, error)
	DeleteAll(ctx context.Context) (int64, error)
	EnqueueGeneration(ctx context.Context, id, docType string, profileID *string) (map[string]any, error)
}

type DocumentLister interface {
	ListDocuments(ctx context.Context, jobID string) ([]dto.GeneratedDocumentDto, error)
	ListDocumentStatuses(ctx context.Context, jobID string) ([]dto.DocumentStatusDto, error)
}

type Reenricher interface {
	RescrapeOne(ctx context.Context, jobID string) error
}

type JobsHandler struct {
	Jobs       JobsProvider
	Generation DocumentLister
	Enrichment Reenricher
}

func (h *JobsHandler) Mount(r chi.Router) {
	r.Get("/jobs", h.list)
	r.Delete("/jobs", h.deleteAll)
	r.Get("/jobs/{id}", h.get)
	r.Post("/jobs/{id}/shortlist", h.shortlist)
	r.Post("/jobs/{id}/hide", h.hide)
	r.Post("/jobs/{id}/unhide", h.unhide)
	r.Post("/jobs/{id}/generate", h.generate)
	r.Get("/jobs/{id}/documents", h.documents)
	r.Get("/jobs/{id}/documents/status", h.documentStatuses)
	if h.Enrichment != nil {
		r.Post("/jobs/{id}/re-enrich", h.reenrich)
	}
}

func (h *JobsHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := jobs.ListParams{Sort: q.Get("sort")}
	if v := q.Get("source"); v != "" {
		params.Source = &v
	}
	if v := q.Get("subscriptionId"); v != "" {
		params.SubscriptionID = &v
	}
	if v := q.Get("minScore"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.MinScore = &n
		}
	}
	if v := q.Get("status"); v != "" {
		params.Status = &v
	}
	if v := q.Get("includeHidden"); v != "" {
		params.IncludeHidden = v == "true"
	}
	if v := q.Get("includeApplied"); v != "" {
		params.IncludeApplied = v == "true"
	}
	if v := q.Get("remote"); v != "" {
		b := v == "true"
		params.Remote = &b
	}
	if v := q.Get("q"); v != "" {
		params.Q = &v
	}
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.Page = n
		}
	}
	if v := q.Get("pageSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.PageSize = n
		}
	}
	if v := q.Get("showBelowFloor"); v != "" {
		params.ShowBelowFloor = v == "true"
	}
	if v := q.Get("onlyHidden"); v != "" {
		params.OnlyHidden = v == "true"
	}
	if v := q.Get("onlyApplied"); v != "" {
		params.OnlyApplied = v == "true"
	}
	if v := q.Get("onlyBelowFloor"); v != "" {
		params.OnlyBelowFloor = v == "true"
	}
	if v := q.Get("onlyManual"); v != "" {
		params.OnlyManual = v == "true"
	}

	out, err := h.Jobs.List(r.Context(), params)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) deleteAll(w http.ResponseWriter, r *http.Request) {
	n, err := h.Jobs.DeleteAll(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": n})
}

func (h *JobsHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "job not found: "+id)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) shortlist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Shortlist(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "job not found: "+id)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) hide(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Hide(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "job not found: "+id)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) unhide(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Unhide(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "job not found: "+id)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type generateBody struct {
	Type      string  `json:"type"`
	ProfileID *string `json:"profileId,omitempty"`
}

func (h *JobsHandler) generate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body generateBody
	if err := httpx.DecodeJSON(r, &body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Jobs.EnqueueGeneration(r.Context(), id, body.Type, body.ProfileID)
	if err != nil {
		httpx.WriteAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, out)
}

func (h *JobsHandler) documents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Generation.ListDocuments(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) documentStatuses(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Generation.ListDocumentStatuses(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) reenrich(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Enrichment.RescrapeOne(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
