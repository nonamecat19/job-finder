package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api-go/internal/jobsources"
)

// SourcesHandler wires /api/sources, mirroring job-sources.controller.ts.
type SourcesHandler struct {
	Sources *jobsources.Service
}

func (h *SourcesHandler) Mount(r chi.Router) {
	r.Get("/sources", h.list)
	r.Put("/sources/{key}", h.update)
	r.Post("/sources/{key}/test", h.test)
}

func (h *SourcesHandler) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.Sources.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type updateSourceBody struct {
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (h *SourcesHandler) update(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var body updateSourceBody
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	out, err := h.Sources.Update(r.Context(), key, body.Enabled, body.Config)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *SourcesHandler) test(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	ok, errMsg := h.Sources.Test(r.Context(), key)
	resp := map[string]any{"ok": ok}
	if errMsg != "" {
		resp["error"] = errMsg
	}
	writeJSON(w, http.StatusOK, resp)
}
