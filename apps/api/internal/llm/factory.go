package llm

import (
	"net/http"

	"github.com/job-finder/api/internal/config"
)

// tunedTransport clones http.DefaultTransport with a raised
// MaxIdleConnsPerHost so hosted concurrency (default 3) doesn't force a
// fresh TLS handshake on the third simultaneous request (research.md R5).
// Deliberately distinct from retrieval.DefaultTransport (FR-003): AI
// provider traffic must never pick up the scraper's request pacing.
func tunedTransport(maxIdleConnsPerHost int) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if maxIdleConnsPerHost > 0 {
		t.MaxIdleConnsPerHost = maxIdleConnsPerHost
	}
	return t
}

// New builds the Ollama Provider from config. Used by callers that only ever
// need the Ollama provider directly (cmd/llmsmoke, live smoke tests) and
// don't participate in the Cerebras toggle / per-task routing.
func New(cfg *config.Config) (Provider, error) {
	o := NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	o.http.Transport = tunedTransport(cfg.LLMMaxIdleConnsPerHost)
	o.keepAlive = cfg.OllamaKeepAlive
	return o, nil
}

// NewProviders builds the Ollama provider plus, when the Cerebras API key
// is configured, the Cerebras provider (001-cerebras-model-toggle).
// Cerebras is nil when its key is unset — cmd/server then wires every
// llm.Router with that leg nil, which makes the Router fall back to Ollama
// regardless of a task's persisted setting (FR-008).
func NewProviders(cfg *config.Config) (ollama *OllamaProvider, cerebras *CerebrasProvider, err error) {
	transport := tunedTransport(cfg.LLMMaxIdleConnsPerHost)

	ollama = NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	ollama.http.Transport = transport
	ollama.keepAlive = cfg.OllamaKeepAlive
	if cfg.CerebrasAPIKey != "" {
		cerebras, err = NewCerebras(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", ollama)
		if err != nil {
			return nil, nil, err
		}
		cerebras.http.Transport = transport
	}
	return ollama, cerebras, nil
}
