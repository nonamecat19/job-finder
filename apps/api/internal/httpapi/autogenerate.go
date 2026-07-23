package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/autogen"
	"github.com/job-finder/api/internal/dto"
)

// AutoGenerateProvider is the interface AutoGenerateHandler needs from the
// autogen service. *autogen.Service satisfies it structurally.
type AutoGenerateProvider interface {
	Get() autogen.State
	Update(ctx context.Context, enabled bool, threshold int) (autogen.State, error)
}

// AutoGenerateHandler exposes GET/PUT /v1/settings/autogenerate: the
// "auto-generate resume when match score is high" toggle and threshold.
type AutoGenerateHandler struct {
	Settings AutoGenerateProvider
}

func (h *AutoGenerateHandler) Mount(r chi.Router) {
	r.Get("/v1/settings/autogenerate", h.get)
	r.Put("/v1/settings/autogenerate", h.update)
}

func (h *AutoGenerateHandler) get(w http.ResponseWriter, r *http.Request) {
	s := h.Settings.Get()
	writeJSON(w, http.StatusOK, dto.AutoGenerateSettingDto{Enabled: s.Enabled, Threshold: s.Threshold})
}

func (h *AutoGenerateHandler) update(w http.ResponseWriter, r *http.Request) {
	var body dto.AutoGenerateSettingDto
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Threshold < 0 || body.Threshold > 100 {
		writeError(w, http.StatusBadRequest, "threshold must be between 0 and 100")
		return
	}
	s, err := h.Settings.Update(r.Context(), body.Enabled, body.Threshold)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dto.AutoGenerateSettingDto{Enabled: s.Enabled, Threshold: s.Threshold})
}
