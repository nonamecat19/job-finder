package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/jobsources/roster"
)

// RosterProvider is the interface RosterHandler needs from roster.Service.
type RosterProvider interface {
	List(ctx context.Context) ([]dto.EmployerBoardDto, error)
	RegisterFromURL(ctx context.Context, rawURL, displayName string) (dto.EmployerBoardDto, error)
	Remove(ctx context.Context, id string) error
	ListCandidates(ctx context.Context) ([]dto.BoardCandidateDto, error)
	Accept(ctx context.Context, candidateID string) (dto.EmployerBoardDto, error)
	Reject(ctx context.Context, candidateID string) error
	Discover(ctx context.Context) (int, error)
}

// RosterHandler wires /api/roster, per specs/domains/job-sources.md § 3.
type RosterHandler struct {
	Roster RosterProvider
}

func (h *RosterHandler) Mount(r chi.Router) {
	r.Get("/roster", h.list)
	r.Post("/roster", h.register)
	r.Delete("/roster/{id}", h.remove)
	r.Get("/roster/candidates", h.listCandidates)
	r.Post("/roster/candidates/{id}/accept", h.accept)
	r.Post("/roster/candidates/{id}/reject", h.reject)
	r.Post("/roster/discover", h.discover)
}

func (h *RosterHandler) list(w http.ResponseWriter, r *http.Request) {
	boards, err := h.Roster.List(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"employers": boards})
}

type registerBoardBody struct {
	URL string `json:"url"`
}

func (h *RosterHandler) register(w http.ResponseWriter, r *http.Request) {
	var body registerBoardBody
	if err := httpx.DecodeJSON(r, &body); err != nil || body.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "url is required")
		return
	}
	board, err := h.Roster.RegisterFromURL(r.Context(), body.URL, "")
	if err != nil {
		var unsupported *roster.UnsupportedVendorError
		var unreadable *roster.UnreadableError
		switch {
		case errors.As(err, &unsupported):
			httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": "unsupported_vendor", "message": err.Error(), "supportedVendors": roster.SupportedVendors,
			})
		case errors.As(err, &unreadable):
			httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "unreadable", "message": err.Error()})
		default:
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, board)
}

func (h *RosterHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Roster.Remove(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "candidate not found: "+id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RosterHandler) listCandidates(w http.ResponseWriter, r *http.Request) {
	candidates, err := h.Roster.ListCandidates(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

func (h *RosterHandler) accept(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	board, err := h.Roster.Accept(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "candidate not found: "+id)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, board)
}

func (h *RosterHandler) reject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Roster.Reject(r.Context(), id); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "candidate not found: "+id)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RosterHandler) discover(w http.ResponseWriter, r *http.Request) {
	created, err := h.Roster.Discover(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"newCandidates": created})
}
