package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/roster"
)

// RosterProvider is the interface RosterHandler needs from roster.Service.
type RosterProvider interface {
	List(ctx context.Context) ([]sqlcgen.EmployerBoard, error)
	RegisterFromURL(ctx context.Context, rawURL, displayName string) (sqlcgen.EmployerBoard, error)
	Remove(ctx context.Context, id string) error
	ListCandidates(ctx context.Context) ([]sqlcgen.BoardCandidate, error)
	Accept(ctx context.Context, candidateID string) (sqlcgen.EmployerBoard, error)
	Reject(ctx context.Context, candidateID string) error
	Discover(ctx context.Context) (int, error)
}

// RosterHandler wires /api/roster, per contracts/roster-api.md (013).
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

func toEmployerBoardDto(e sqlcgen.EmployerBoard) dto.EmployerBoardDto {
	return dto.EmployerBoardDto{
		ID:                 dbutil.UUIDString(e.ID),
		Vendor:             e.Vendor,
		EmployerIdentifier: e.EmployerIdentifier,
		DisplayName:        e.DisplayName,
		AddedVia:           e.AddedVia,
		Enabled:            e.Enabled,
		LastSuccessAt:      dbutil.TimestampPtr(e.LastSuccessAt),
		LastPostingCount:   int(e.LastPostingCount),
		Stale:              roster.Stale(e),
	}
}

func toBoardCandidateDto(c sqlcgen.BoardCandidate) dto.BoardCandidateDto {
	return dto.BoardCandidateDto{
		ID:                 dbutil.UUIDString(c.ID),
		Vendor:             c.Vendor,
		EmployerIdentifier: c.EmployerIdentifier,
		DisplayName:        c.EmployerIdentifier,
		InferredFromJobID:  dbutil.UUIDStringPtr(c.InferredFromJobId),
		State:              c.State,
	}
}

func (h *RosterHandler) list(w http.ResponseWriter, r *http.Request) {
	boards, err := h.Roster.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]dto.EmployerBoardDto, 0, len(boards))
	for _, b := range boards {
		out = append(out, toEmployerBoardDto(b))
	}
	writeJSON(w, http.StatusOK, map[string]any{"employers": out})
}

type registerBoardBody struct {
	URL string `json:"url"`
}

func (h *RosterHandler) register(w http.ResponseWriter, r *http.Request) {
	var body registerBoardBody
	if err := decodeJSON(r, &body); err != nil || body.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	board, err := h.Roster.RegisterFromURL(r.Context(), body.URL, "")
	if err != nil {
		var unsupported *roster.UnsupportedVendorError
		var unreadable *roster.UnreadableError
		switch {
		case errors.As(err, &unsupported):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": "unsupported_vendor", "message": err.Error(), "supportedVendors": roster.SupportedVendors,
			})
		case errors.As(err, &unreadable):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "unreadable", "message": err.Error()})
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, toEmployerBoardDto(board))
}

func (h *RosterHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Roster.Remove(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RosterHandler) listCandidates(w http.ResponseWriter, r *http.Request) {
	candidates, err := h.Roster.ListCandidates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]dto.BoardCandidateDto, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, toBoardCandidateDto(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": out})
}

func (h *RosterHandler) accept(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	board, err := h.Roster.Accept(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toEmployerBoardDto(board))
}

func (h *RosterHandler) reject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Roster.Reject(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RosterHandler) discover(w http.ResponseWriter, r *http.Request) {
	created, err := h.Roster.Discover(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"newCandidates": created})
}
