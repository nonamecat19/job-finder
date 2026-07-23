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

// NewProviders builds the Ollama provider plus, when CerebrasAPIKey is
// configured, the Cerebras provider (001-cerebras-model-toggle). Cerebras is
// nil when no key is set — cmd/server then wires every llm.Router with a nil
// Cerebras leg, which makes the Router fall back to Ollama regardless of a
// task's persisted setting (FR-008).
func NewProviders(cfg *config.Config) (ollama *OllamaProvider, cerebras *CerebrasProvider, err error) {
	ollama = NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	if cfg.CerebrasAPIKey == "" {
		return ollama, nil, nil
	}
	cerebras, err = NewCerebras(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", ollama)
	if err != nil {
		return nil, nil, err
	}
	return ollama, cerebras, nil
}
