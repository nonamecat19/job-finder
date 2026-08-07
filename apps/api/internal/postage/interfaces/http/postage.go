package http

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
)

type PostAgeProvider interface {
	Compute(ctx context.Context) (dto.PostAgeResponseDto, error)
}

type PostAgeHandler struct {
	PostAge PostAgeProvider
}

func (h *PostAgeHandler) Mount(r chi.Router) {
	r.Get("/postage-response-rate", h.compute)
}

func (h *PostAgeHandler) compute(w http.ResponseWriter, r *http.Request) {
	out, err := h.PostAge.Compute(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
