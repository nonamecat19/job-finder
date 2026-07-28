package main

import (
	"context"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/llmsettings"
)

func composeLLM(ctx context.Context, p *Platform) (*llmHandles, error) {
	ollamaProvider, cerebrasProvider, err := llm.NewProviders(p.Config)
	if err != nil {
		return nil, err
	}
	var cerebrasIface llm.Provider
	if cerebrasProvider != nil {
		cerebrasIface = cerebrasProvider
	}
	settingsSvc, err := llmsettings.NewService(
		ctx, p.DB.Queries, p.Config.CerebrasAPIKey != "",
	)
	if err != nil {
		return nil, err
	}
	holder := settingsSvc.Holder()
	return &llmHandles{
		Ollama:           ollamaProvider,
		MatchRouter:      llm.NewRouter("match", holder, ollamaProvider, cerebrasIface),
		GenerationRouter: llm.NewRouter("generation", holder, ollamaProvider, cerebrasIface),
		RephraseRouter:   llm.NewRouter("rephrase", holder, ollamaProvider, cerebrasIface),
		GhostRouter:      llm.NewRouter("ghost", holder, ollamaProvider, cerebrasIface),
		DefaultRouter:    llm.NewRouter("default", holder, ollamaProvider, cerebrasIface),
		Settings:         settingsSvc,
		SettingsHandler:  &httpapi.LlmSettingsHandler{Settings: settingsSvc},
	}, nil
}
