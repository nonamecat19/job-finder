package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	applications "github.com/job-finder/api/internal/applications/application"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

// ApplicationProvider is the interface ApplicationsHandler needs from the applications service.
type ApplicationProvider interface {
	List(ctx context.Context, status *string) ([]dto.ApplicationDto, error)
	Update(ctx context.Context, id string, in applications.UpdateInput) (dto.ApplicationDto, error)
	Stats(ctx context.Context) (dto.StatsDto, error)
}

// ApplicationsHandler wires /api/applications and /api/stats, mirroring
// applications.controller.ts.
type ApplicationsHandler struct {
	Applications ApplicationProvider
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
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *ApplicationsHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var raw map[string]any
	if err := httpx.DecodeJSON(r, &raw); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid body")
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
		// Not-found (bad/missing id) is 404; anything else (invalid status
		// value) is 400 — mirrors NotFoundException vs BadRequestException
		// in applications.service.ts.
		if errors.Is(err, applications.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "application not found: "+id)
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *ApplicationsHandler) stats(w http.ResponseWriter, r *http.Request) {
	out, err := h.Applications.Stats(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
