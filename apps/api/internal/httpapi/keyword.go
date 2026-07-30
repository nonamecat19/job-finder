package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/keyword/application"
)

// KeywordDiffProvider is the interface KeywordHandler needs from the keyword
// diff service.
type KeywordDiffProvider interface {
	KeywordDiff(ctx context.Context, jobID string) (dto.KeywordDiffDto, error)
}

// KeywordHandler wires the JD-ATS keyword-diff endpoint (008-6).
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
			writeError(w, http.StatusNotFound, "keyword diff not found for job")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}
