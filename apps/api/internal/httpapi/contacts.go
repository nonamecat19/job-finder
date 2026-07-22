package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
)

// ContactsProvider is the interface ContactsHandler needs from the
// recruiter service.
type ContactsProvider interface {
	ListContacts(ctx context.Context, jobID string) ([]dto.JobContactDto, error)
	Resolve(ctx context.Context, jobID string) ([]dto.JobContactDto, error)
}

// ContactsHandler wires /api/jobs/{id}/contacts and
// /api/jobs/{id}/contacts/refresh (spec 007). Read-only per Constitution
// Principle I: neither route ever sends anything to a resolved contact.
type ContactsHandler struct {
	Recruiter ContactsProvider
}

func (h *ContactsHandler) Mount(r chi.Router) {
	r.Get("/jobs/{id}/contacts", h.list)
	r.Post("/jobs/{id}/contacts/refresh", h.refresh)
}

func (h *ContactsHandler) list(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	out, err := h.Recruiter.ListContacts(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *ContactsHandler) refresh(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	out, err := h.Recruiter.Resolve(r.Context(), jobID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
