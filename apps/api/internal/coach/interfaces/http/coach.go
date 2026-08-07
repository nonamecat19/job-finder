package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/coach/application"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

type CoachProvider interface {
	Assess(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error)
	CachedAssessment(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error)
}

type CoachHandler struct {
	Coach CoachProvider
}

func (h *CoachHandler) Mount(r chi.Router) {
	r.Post("/jobs/{id}/coach/assess", h.assess)
	r.Get("/jobs/{id}/coach/assessment", h.cached)
}

func (h *CoachHandler) assess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Coach.Assess(r.Context(), id)
	if err != nil {
		if errors.Is(err, application.ErrNoDiff) {
			httpx.WriteError(w, http.StatusNotFound, "no keyword diff for job — run the keyword diff first")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *CoachHandler) cached(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Coach.CachedAssessment(r.Context(), id)
	if err != nil {
		if errors.Is(err, application.ErrNotAssessed) {
			httpx.WriteError(w, http.StatusNotFound, "no fit-gap assessment yet — assess this job first")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
