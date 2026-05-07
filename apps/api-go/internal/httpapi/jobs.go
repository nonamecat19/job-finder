package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api-go/internal/generation"
	"github.com/job-finder/api-go/internal/jobs"
)

// JobsHandler wires /api/jobs, mirroring jobs.controller.ts.
type JobsHandler struct {
	Jobs       *jobs.Service
	Generation *generation.Service
}

func (h *JobsHandler) Mount(r chi.Router) {
	r.Get("/jobs", h.list)
	r.Get("/jobs/{id}", h.get)
	r.Post("/jobs/{id}/shortlist", h.shortlist)
	r.Post("/jobs/{id}/hide", h.hide)
	r.Post("/jobs/{id}/generate", h.generate)
	r.Get("/jobs/{id}/documents", h.documents)
}

func (h *JobsHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := jobs.ListParams{Sort: q.Get("sort")}
	if v := q.Get("source"); v != "" {
		params.Source = &v
	}
	if v := q.Get("minScore"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			params.MinScore = &n
		}
	}
	if v := q.Get("status"); v != "" {
		params.Status = &v
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

	out, err := h.Jobs.List(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) shortlist(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Shortlist(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *JobsHandler) hide(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Jobs.Hide(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type generateBody struct {
	Type      string  `json:"type"`
	ProfileID *string `json:"profileId,omitempty"`
}

func (h *JobsHandler) generate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body generateBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Jobs.EnqueueGeneration(r.Context(), id, body.Type, body.ProfileID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

func (h *JobsHandler) documents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Generation.ListDocuments(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
