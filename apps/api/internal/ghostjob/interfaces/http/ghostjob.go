package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/ghostjob/application"
	"github.com/job-finder/api/internal/httpx"
)

const scoreTimeout = 115 * time.Second

type GhostJobProvider interface {
	ScoreJob(ctx context.Context, jobID string) (dto.JobSignalDto, error)
}

type GhostJobHandler struct {
	Ghost GhostJobProvider
}

func (h *GhostJobHandler) Mount(r chi.Router) {
	r.Post("/jobs/{id}/ghost-score", h.score)
}

func (h *GhostJobHandler) score(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), scoreTimeout)
	defer cancel()

	out, err := h.Ghost.ScoreJob(ctx, id)
	if err != nil {
		if errors.Is(err, application.ErrDeclinedToScore) {
			httpx.WriteError(w, http.StatusUnprocessableEntity, "insufficient signal to score this job yet")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
