package cerebras

import (
	"net/http"
	"testing"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/retrieval"
)

// TestNewProviders_NeverUsesPacedTransport guards FR-003 (research.md R1):
// AI provider traffic must never pick up the scraper's paced transport, so a
// future "wrap everything in retrieval.DefaultTransport" change can't
// silently throttle AI calls.
func TestNewProviders_NeverUsesPacedTransport(t *testing.T) {
	cfg := &config.Config{
		OllamaURL:              "http://localhost:11434",
		CerebrasAPIKey:         "cerebras-key",
		CerebrasBaseURL:        "https://api.cerebras.ai/v1",
		LLMMaxIdleConnsPerHost: 4,
	}
	ollama, cerebras, err := NewProviders(cfg)
	if err != nil {
		t.Fatalf("NewProviders: %v", err)
	}

	if ollama.http.Transport == retrieval.DefaultTransport {
		t.Error("ollama provider must not use retrieval.DefaultTransport")
	}
	if cerebras.http.Transport == retrieval.DefaultTransport {
		t.Error("cerebras provider must not use retrieval.DefaultTransport")
	}
}

func TestNewProviders_TunesMaxIdleConnsPerHost(t *testing.T) {
	cfg := &config.Config{
		OllamaURL:              "http://localhost:11434",
		LLMMaxIdleConnsPerHost: 7,
	}
	ollama, _, err := NewProviders(cfg)
	if err != nil {
		t.Fatalf("NewProviders: %v", err)
	}
	transport, ok := ollama.http.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", ollama.http.Transport)
	}
	if transport.MaxIdleConnsPerHost != 7 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 7", transport.MaxIdleConnsPerHost)
	}
}
