package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/interviewprep"
)

// InterviewPrepProvider is the interface InterviewPrepHandler needs from the
// interview-prep pack service (013).
type InterviewPrepProvider interface {
	Get(ctx context.Context, jobID string) (dto.InterviewPrepPackDto, error)
}

// InterviewPrepHandler wires GET /jobs/{id}/interview-prep (spec 013).
type InterviewPrepHandler struct {
	InterviewPrep InterviewPrepProvider
}

func (h *InterviewPrepHandler) Mount(r chi.Router) {
	r.Get("/jobs/{id}/interview-prep", h.get)
}

func (h *InterviewPrepHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.InterviewPrep.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, interviewprep.ErrNoDiff) {
			httpx.WriteError(w, http.StatusNotFound, "no keyword diff for job "+id+" — run the keyword diff first")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
