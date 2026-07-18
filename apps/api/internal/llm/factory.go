package llm

import (
	"github.com/job-finder/api/internal/config"
)

// New builds the Ollama Provider from config. Ollama is the sole provider;
// OllamaKey authenticates to Ollama Cloud when set, and EmbedURL lets
// embeddings run on a different (e.g. local) endpoint than chat.
func New(cfg *config.Config) (Provider, error) {
	return NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL), nil
}
