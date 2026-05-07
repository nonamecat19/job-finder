package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api-go/internal/applications"
	"github.com/job-finder/api-go/internal/dto"
)

// ApplicationsHandler wires /api/applications and /api/stats, mirroring
// applications.controller.ts.
type ApplicationsHandler struct {
	Applications *applications.Service
}

func (h *ApplicationsHandler) Mount(r chi.Router) {
	r.Get("/applications", h.list)
	r.Patch("/applications/{id}", h.update)
	r.Get("/stats", h.stats)
}

func (h *ApplicationsHandler) list(w http.ResponseWriter, r *http.Request) {
	var status *string
	if v := r.URL.Query().Get("status"); v != "" {
		status = &v
	}
	out, err := h.Applications.List(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ApplicationsHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var raw map[string]any
	if err := decodeJSON(r, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	in := applications.UpdateInput{}
	if v, ok := raw["status"]; ok {
		if s, ok := v.(string); ok {
			status := dto.ApplicationStatus(s)
			in.Status = &status
		}
	}
	if v, ok := raw["notes"]; ok {
		var notes *string
		if s, ok := v.(string); ok {
			notes = &s
		}
		in.Notes = &notes
	}
	out, err := h.Applications.Update(r.Context(), id, in)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ApplicationsHandler) stats(w http.ResponseWriter, r *http.Request) {
	out, err := h.Applications.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
