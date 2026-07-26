package main

import (
	"context"

	"github.com/job-finder/api/internal/httpapi"
	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/llmsettings"
)

func composeLLM(ctx context.Context, p *Platform) (*llmHandles, error) {
	ollamaProvider, cerebrasProvider, openRouterProvider, err := llm.NewProviders(p.Config)
	if err != nil {
		return nil, err
	}
	var cerebrasIface, openRouterIface llm.Provider
	if cerebrasProvider != nil {
		cerebrasIface = cerebrasProvider
	}
	if openRouterProvider != nil {
		openRouterIface = openRouterProvider
	}
	settingsSvc, err := llmsettings.NewService(
		ctx, p.DB.Queries, p.Config.CerebrasAPIKey != "", p.Config.OpenRouterAPIKey != "",
	)
	if err != nil {
		return nil, err
	}
	holder := settingsSvc.Holder()
	return &llmHandles{
		Ollama:           ollamaProvider,
		MatchRouter:      llm.NewRouter("match", holder, ollamaProvider, cerebrasIface, openRouterIface),
		GenerationRouter: llm.NewRouter("generation", holder, ollamaProvider, cerebrasIface, openRouterIface),
		RephraseRouter:   llm.NewRouter("rephrase", holder, ollamaProvider, cerebrasIface, openRouterIface),
		GhostRouter:      llm.NewRouter("ghost", holder, ollamaProvider, cerebrasIface, openRouterIface),
		DefaultRouter:    llm.NewRouter("default", holder, ollamaProvider, cerebrasIface, openRouterIface),
		Settings:         settingsSvc,
		SettingsHandler:  &httpapi.LlmSettingsHandler{Settings: settingsSvc},
	}, nil
}
