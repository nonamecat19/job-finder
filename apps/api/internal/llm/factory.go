package llm

import (
	"github.com/job-finder/api/internal/config"
)

// New builds the Ollama Provider from config. Used by callers that only ever
// need the local provider directly (cmd/llmsmoke, live smoke tests) and
// don't participate in the Cerebras toggle / per-task routing.
func New(cfg *config.Config) (Provider, error) {
	return NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL), nil
}

// NewProviders builds the Ollama provider plus, when their respective API
// keys are configured, the Cerebras (001-cerebras-model-toggle) and
// OpenRouter providers. Either remote provider is nil when its key is unset —
// cmd/server then wires every llm.Router with that leg nil, which makes the
// Router fall back to Ollama regardless of a task's persisted setting
// (FR-008).
func NewProviders(cfg *config.Config) (ollama *OllamaProvider, cerebras *CerebrasProvider, openrouter *OpenRouterProvider, err error) {
	ollama = NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	if cfg.CerebrasAPIKey != "" {
		cerebras, err = NewCerebras(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", ollama)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	if cfg.OpenRouterAPIKey != "" {
		openrouter, err = NewOpenRouter(
			cfg.OpenRouterBaseURL, cfg.OpenRouterAPIKey, "",
			cfg.OpenRouterSiteURL, cfg.OpenRouterAppName, ollama,
		)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return ollama, cerebras, openrouter, nil
}
