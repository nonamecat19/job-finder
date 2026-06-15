package llm

import (
	"fmt"

	"github.com/job-finder/api/internal/config"
)

// New selects the active Provider based on cfg.LLMProvider ("ollama" | "cerebras"),
// mirroring the useFactory switch in llm.module.ts.
func New(cfg *config.Config) (Provider, error) {
	ollama := NewOllama(cfg.OllamaURL, cfg.LLMModel, cfg.EmbedModel)
	switch cfg.LLMProvider {
	case "", "ollama":
		return ollama, nil
	case "cerebras":
		return NewCerebras(cfg.CerebrasURL, cfg.CerebrasAPIKey, cfg.CerebrasModel, ollama)
	default:
		return nil, fmt.Errorf("unknown LLM_PROVIDER '%s' — expected 'ollama' or 'cerebras'", cfg.LLMProvider)
	}
}
