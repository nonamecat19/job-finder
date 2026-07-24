package llm

import (
	"context"
	"testing"
)

type stubProvider struct {
	name        string
	gotModel    string
	returnModel string
}

func (s *stubProvider) ModelName() string { return s.name }
func (s *stubProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	s.gotModel = opts.ModelOr("")
	return s.name, nil
}
func (s *stubProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	s.gotModel = opts.ModelOr("")
	return s.name, nil
}
func (s *stubProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1}, nil
}

func snapshotWith(tasks map[string]TaskSetting, credentialConfigured bool) RouterSnapshot {
	return RouterSnapshot{
		Tasks:                          tasks,
		CredentialConfigured:           credentialConfigured,
		OpenRouterCredentialConfigured: credentialConfigured,
	}
}

func TestRouterResolvesOllamaByDefault(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	cerebras := &stubProvider{name: "cerebras"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderOllama},
	}, true))
	r := NewRouter("match", holder, ollama, cerebras, nil)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ollama" {
		t.Errorf("Complete() = %q, want ollama", out)
	}
}

func TestRouterResolvesCerebrasWhenSelectedAndConfigured(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	cerebras := &stubProvider{name: "cerebras"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"generation": {Provider: TaskProviderCerebras, Model: "llama-3.3-70b"},
	}, true))
	r := NewRouter("generation", holder, ollama, cerebras, nil)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "cerebras" {
		t.Errorf("Complete() = %q, want cerebras", out)
	}
	if cerebras.gotModel != "llama-3.3-70b" {
		t.Errorf("model passed to provider = %q, want llama-3.3-70b", cerebras.gotModel)
	}
}

func TestRouterFallsBackToOllamaWhenCredentialMissing(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"ghost": {Provider: TaskProviderCerebras, Model: "gpt-oss-120b"},
	}, false))
	// cerebras provider is nil, exactly as main.go would construct it when
	// CEREBRAS_API_KEY is unset.
	r := NewRouter("ghost", holder, ollama, nil, nil)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ollama" {
		t.Errorf("Complete() = %q, want ollama (fallback)", out)
	}
}

func TestRouterResolvesOpenRouterWhenSelectedAndConfigured(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	cerebras := &stubProvider{name: "cerebras"}
	openrouter := &stubProvider{name: "openrouter"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"generation": {Provider: TaskProviderOpenRouter, Model: "deepseek/deepseek-r1:free"},
	}, true))
	r := NewRouter("generation", holder, ollama, cerebras, openrouter)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "openrouter" {
		t.Errorf("Complete() = %q, want openrouter", out)
	}
	if openrouter.gotModel != "deepseek/deepseek-r1:free" {
		t.Errorf("model passed to provider = %q, want deepseek/deepseek-r1:free", openrouter.gotModel)
	}
}

func TestRouterFallsBackToOllamaWhenOpenRouterCredentialMissing(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	cerebras := &stubProvider{name: "cerebras"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"ghost": {Provider: TaskProviderOpenRouter, Model: "deepseek/deepseek-r1:free"},
	}, false))
	// openrouter provider is nil, as main.go constructs it when
	// OPENROUTER_API_KEY is unset.
	r := NewRouter("ghost", holder, ollama, cerebras, nil)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ollama" {
		t.Errorf("Complete() = %q, want ollama (fallback)", out)
	}
	// The persisted OpenRouter model must not leak onto Ollama, which has no
	// such model — the fallback uses Ollama's own default.
	if ollama.gotModel != "" {
		t.Errorf("gotModel = %q, want empty on fallback", ollama.gotModel)
	}
}

func TestRouterEmbedAlwaysUsesOllamaForOpenRouterTask(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	openrouter := &stubProvider{name: "openrouter"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"default": {Provider: TaskProviderOpenRouter},
	}, true))
	r := NewRouter("default", holder, ollama, nil, openrouter)

	vec, err := r.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("Embed should delegate to ollama stub, got %v", vec)
	}
}

func TestRouterEmptyModelUsesProviderDefault(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"rephrase": {Provider: TaskProviderOllama, Model: ""},
	}, true))
	r := NewRouter("rephrase", holder, ollama, nil, nil)

	if _, err := r.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if ollama.gotModel != "" {
		t.Errorf("gotModel = %q, want empty (provider default)", ollama.gotModel)
	}
}

func TestRouterEmbedAlwaysUsesOllama(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	cerebras := &stubProvider{name: "cerebras"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"default": {Provider: TaskProviderCerebras},
	}, true))
	r := NewRouter("default", holder, ollama, cerebras, nil)

	vec, err := r.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("Embed should delegate to ollama stub, got %v", vec)
	}
}

func TestRouterSnapshotSwapIsLive(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	cerebras := &stubProvider{name: "cerebras"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderOllama},
	}, true))
	r := NewRouter("match", holder, ollama, cerebras, nil)

	out, _ := r.Complete(context.Background(), "hi", nil)
	if out != "ollama" {
		t.Fatalf("before update, Complete() = %q, want ollama", out)
	}

	holder.Store(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderCerebras},
	}, true))

	out, _ = r.Complete(context.Background(), "hi", nil)
	if out != "cerebras" {
		t.Errorf("after update, Complete() = %q, want cerebras", out)
	}
}
