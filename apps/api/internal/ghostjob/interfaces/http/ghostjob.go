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

// scoreTimeout bounds the manual re-score call. It must clear the LLM
// gateway's own fallback-chain budget (gateway/config.yaml
// litellm_settings.request_timeout: 110s across Cerebras -> Groq -> Cohere ->
// OpenRouter -> local) with headroom, so a slow tier doesn't get cut off by
// this handler before the gateway itself gives up.
const scoreTimeout = 115 * time.Second

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

	// acquireDeadline (httpapi) caps every route's context at
	// DB_ACQUIRE_TIMEOUT (default 5s) for DB-pool-capacity reasons
	// (026-db-pool-capacity), which is far shorter than this handler's LLM
	// call can legitimately take. Detach from that inherited deadline and
	// apply this handler's own bound instead.
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
