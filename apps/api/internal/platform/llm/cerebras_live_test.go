//go:build live

package llm_test

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/platform/llm"
)

// TestLive_CerebrasComplete makes one real call to the Cerebras API, guarded
// behind CEREBRAS_API_KEY (001-cerebras-model-toggle). Run with:
//
//	go test -tags live ./internal/platform/llm/... -run TestLive_Cerebras
func TestLive_CerebrasComplete(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.CerebrasAPIKey == "" {
		t.Skip("CEREBRAS_API_KEY not set")
	}

	ctx := context.Background()
	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	provider, err := llm.NewCerebras(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", ollama)
	if err != nil {
		t.Fatalf("NewCerebras: %v", err)
	}

	out, err := provider.Complete(ctx, "Reply with exactly the word: pong", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out == "" {
		t.Fatal("expected a non-empty completion from Cerebras")
	}
	t.Logf("cerebras live response: %q", out)
}

// TestLive_CerebrasEmbedDelegatesToOllama exercises the real embeddings
// delegation path (Cerebras has no embeddings endpoint — FR-006), still
// gated on CEREBRAS_API_KEY since the provider requires one to construct.
func TestLive_CerebrasEmbedDelegatesToOllama(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.CerebrasAPIKey == "" {
		t.Skip("CEREBRAS_API_KEY not set")
	}

	ctx := context.Background()
	ollama := llm.NewOllama(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	provider, err := llm.NewCerebras(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", ollama)
	if err != nil {
		t.Fatalf("NewCerebras: %v", err)
	}

	vec, err := provider.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) == 0 {
		t.Fatal("expected a non-empty embedding vector")
	}
}
