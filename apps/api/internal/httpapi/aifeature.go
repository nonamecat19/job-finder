package httpapi

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/aifeature"
	"github.com/job-finder/api/internal/dto"
)

// AiFeatureProvider is the interface AiFeatureHandler needs from the
// aifeature service. *aifeature.Service satisfies it structurally.
type AiFeatureProvider interface {
	GetAll() []aifeature.State
	Update(ctx context.Context, feature string, enabled bool, threshold int) (aifeature.State, error)
}

// AiFeatureHandler exposes GET /v1/settings/ai-features and PUT
// /v1/settings/ai-features/{feature}: per-feature "run automatically when
// match score is high" toggle and threshold, for every AI feature except
// match scoring itself (resume generation, cover letter generation, salary
// inference).
type AiFeatureHandler struct {
	Settings AiFeatureProvider
}

func (h *AiFeatureHandler) Mount(r chi.Router) {
	r.Get("/settings/ai-features", h.list)
	r.Put("/settings/ai-features/{feature}", h.update)
}

func stateToAiFeatureDto(s aifeature.State) dto.AiFeatureSettingDto {
	return dto.AiFeatureSettingDto{Feature: s.Feature, Enabled: s.Enabled, Threshold: s.Threshold}
}

func (h *AiFeatureHandler) list(w http.ResponseWriter, r *http.Request) {
	states := h.Settings.GetAll()
	out := make([]dto.AiFeatureSettingDto, 0, len(states))
	for _, s := range states {
		out = append(out, stateToAiFeatureDto(s))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *AiFeatureHandler) update(w http.ResponseWriter, r *http.Request) {
	feature := chi.URLParam(r, "feature")
	valid := false
	for _, k := range aifeature.Keys {
		if k == feature {
			valid = true
			break
		}
	}
	if !valid {
		writeError(w, http.StatusNotFound, "unknown feature")
		return
	}

	var body dto.AiFeatureSettingDto
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Threshold < 0 || body.Threshold > 100 {
		writeError(w, http.StatusBadRequest, "threshold must be between 0 and 100")
		return
	}
	s, err := h.Settings.Update(r.Context(), feature, body.Enabled, body.Threshold)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stateToAiFeatureDto(s))
}
