package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
)

// PostAgeProvider is the interface PostAgeHandler needs from the postage service.
type PostAgeProvider interface {
	Compute(ctx context.Context) (dto.PostAgeResponseDto, error)
}

// PostAgeHandler serves GET /api/postage-response-rate — the post-age-at-apply
// vs response-rate signal (spec 010).
type PostAgeHandler struct {
	PostAge PostAgeProvider
}

func (h *PostAgeHandler) Mount(r chi.Router) {
	r.Get("/postage-response-rate", h.compute)
}

func (h *PostAgeHandler) compute(w http.ResponseWriter, r *http.Request) {
	out, err := h.PostAge.Compute(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
