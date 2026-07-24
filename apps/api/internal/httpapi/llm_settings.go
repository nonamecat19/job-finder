package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/llmsettings"
)

// LlmSettingsProvider is the interface LlmSettingsHandler needs from the
// settings service. *llmsettings.Service satisfies it structurally.
type LlmSettingsProvider interface {
	Get() llmsettings.State
	Update(ctx context.Context, updates []llmsettings.TaskUpdate) (llmsettings.State, error)
}

// LlmSettingsHandler exposes the Cerebras free-tier model toggle
// (001-cerebras-model-toggle): per-task provider/model assignment plus the
// curated Cerebras model list, mounted at /v1/settings/llm.
type LlmSettingsHandler struct {
	Settings LlmSettingsProvider
}

func (h *LlmSettingsHandler) Mount(r chi.Router) {
	r.Get("/v1/settings/llm", h.get)
	r.Put("/v1/settings/llm", h.update)
	r.Get("/v1/settings/llm/models", h.models)
}

func stateToDto(s llmsettings.State) dto.LlmSettingsResponseDto {
	tasks := make([]dto.LlmTaskSettingDto, 0, len(s.Tasks))
	for _, t := range s.Tasks {
		tasks = append(tasks, dto.LlmTaskSettingDto{TaskKey: t.TaskKey, Provider: t.Provider, Model: t.Model})
	}
	return dto.LlmSettingsResponseDto{
		CredentialConfigured:           s.CredentialConfigured,
		OpenRouterCredentialConfigured: s.OpenRouterCredentialConfigured,
		Tasks:                          tasks,
	}
}

func (h *LlmSettingsHandler) get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, stateToDto(h.Settings.Get()))
}

func (h *LlmSettingsHandler) update(w http.ResponseWriter, r *http.Request) {
	var body dto.UpdateLlmSettingsRequestDto
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	updates := make([]llmsettings.TaskUpdate, 0, len(body.Tasks))
	for _, t := range body.Tasks {
		updates = append(updates, llmsettings.TaskUpdate{TaskKey: t.TaskKey, Provider: t.Provider, Model: t.Model})
	}
	state, err := h.Settings.Update(r.Context(), updates)
	if err != nil {
		if errors.Is(err, llmsettings.ErrUnknownTaskKey) ||
			errors.Is(err, llmsettings.ErrInvalidProvider) ||
			errors.Is(err, llmsettings.ErrInvalidModel) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stateToDto(state))
}

func (h *LlmSettingsHandler) models(w http.ResponseWriter, r *http.Request) {
	cerebras := make([]dto.CerebrasModelDto, 0, len(llm.CerebrasModels))
	for _, m := range llm.CerebrasModels {
		cerebras = append(cerebras, dto.CerebrasModelDto{ID: m.ID, Label: m.Label, IsDefault: m.IsDefault})
	}
	openrouter := make([]dto.OpenRouterModelDto, 0, len(llm.OpenRouterModels))
	for _, m := range llm.OpenRouterModels {
		openrouter = append(openrouter, dto.OpenRouterModelDto{ID: m.ID, Label: m.Label, IsDefault: m.IsDefault})
	}
	writeJSON(w, http.StatusOK, dto.LlmModelsResponseDto{Cerebras: cerebras, OpenRouter: openrouter})
}
