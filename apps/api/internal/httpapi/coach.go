package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/coach"
	"github.com/job-finder/api/internal/dto"
)

// CoachProvider is the interface CoachHandler needs from the fit-gap coach
// assessment service (009).
type CoachProvider interface {
	Assess(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error)
	CachedAssessment(ctx context.Context, jobID string) (dto.FitGapAssessmentDto, error)
}

// CoachHandler wires the fit-gap coach endpoints (009-wiring):
//
//	POST /jobs/{id}/coach/assess      — run a fresh assessment
//	GET  /jobs/{id}/coach/assessment  — return the last assessment for this job
//
// Assess is a POST (not a GET) because it generates a grounded rephrase via a
// live LLM call per missing must-have, mirroring 008-6's keyword-diff
// suggestions — that cost never runs inline on a GET.
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
		if errors.Is(err, coach.ErrNoDiff) {
			writeError(w, http.StatusNotFound, "no keyword diff for job "+id+" — run the keyword diff first")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *CoachHandler) cached(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Coach.CachedAssessment(r.Context(), id)
	if err != nil {
		if errors.Is(err, coach.ErrNotAssessed) {
			writeError(w, http.StatusNotFound, "no fit-gap assessment yet for job "+id+" — assess this job first")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
