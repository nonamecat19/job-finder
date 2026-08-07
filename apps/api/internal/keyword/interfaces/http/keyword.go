package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/httpx"
	"github.com/job-finder/api/internal/keyword/application"
)

type KeywordDiffProvider interface {
	KeywordDiff(ctx context.Context, jobID string) (dto.KeywordDiffDto, error)
}

type KeywordHandler struct {
	Diff KeywordDiffProvider
}

func (h *KeywordHandler) Mount(r chi.Router) {
	r.Get("/jobs/{id}/keyword-diff", h.get)
}

func (h *KeywordHandler) get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	out, err := h.Diff.KeywordDiff(r.Context(), id)
	if err != nil {
		if errors.Is(err, application.ErrDiffNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "keyword diff not found for job")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}
