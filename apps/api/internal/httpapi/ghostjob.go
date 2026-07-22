package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/ghostjob"
)

// GhostJobProvider is the interface GhostJobHandler needs from the
// ghost-job service. *ghostjob.Service satisfies it structurally.
type GhostJobProvider interface {
	ScoreJob(ctx context.Context, jobID string) (dto.JobSignalDto, error)
}

// GhostJobHandler wires the manual re-score endpoint (US3, FR-014): scoring
// is triggered by ingestion and by this endpoint only — no scheduled or
// background re-scoring path exists.
type GhostJobHandler struct {
	Ghost GhostJobProvider
}

func (h *GhostJobHandler) Mount(r chi.Router) {
	r.Post("/jobs/{id}/ghost-score", h.score)
}

func (h *GhostJobHandler) score(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Ghost.ScoreJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, ghostjob.ErrDeclinedToScore) {
			writeError(w, http.StatusUnprocessableEntity, "insufficient signal to score this job yet")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
