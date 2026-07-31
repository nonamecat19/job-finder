package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
	ollamaprovider "github.com/job-finder/api/internal/platform/llm/infrastructure/ollama"
)

type stubProvider struct {
	name     string
	gotModel string
}

func (s *stubProvider) ModelName() string { return s.name }
func (s *stubProvider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	s.gotModel = opts.ModelOr("")
	return s.name, nil
}
func (s *stubProvider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	s.gotModel = opts.ModelOr("")
	return s.name, nil
}
func (s *stubProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1}, nil
}

func snapshotWith(tasks map[string]TaskSetting, credentialConfigured bool) RouterSnapshot {
	return RouterSnapshot{
		Tasks:                tasks,
		CredentialConfigured: credentialConfigured,
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
	r := NewRouter("ghost", holder, ollama, nil, nil)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ollama" {
		t.Errorf("Complete() = %q, want ollama (fallback)", out)
	}
}

func TestRouterEmbedAlwaysUsesOllamaForCerebrasTask(t *testing.T) {
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

func TestProviderClass_LoopbackIsLocal(t *testing.T) {
	ollama := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderOllama},
	}, false))
	r := NewRouter("match", holder, ollama, nil, nil)
	if got := r.ProviderClass(); got != ProviderClassLocal {
		t.Errorf("expected local, got %v", got)
	}
}

func TestProviderClass_LoopbackWithKeySetIsHosted(t *testing.T) {
	ollama := ollamaprovider.New("http://127.0.0.1:11434", "some-key", "", "", "")
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderOllama},
	}, false))
	r := NewRouter("match", holder, ollama, nil, nil)
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted (key set on loopback), got %v", got)
	}
}

func TestProviderClass_CloudURLIsHosted(t *testing.T) {
	ollama := ollamaprovider.New("https://ollama.com", "cloud-key", "", "", "")
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderOllama},
	}, false))
	r := NewRouter("match", holder, ollama, nil, nil)
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted, got %v", got)
	}
}

func TestProviderClass_CerebrasWithCredentialIsHosted(t *testing.T) {
	ollama := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	cerebras := &stubProvider{name: "cerebras"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderCerebras},
	}, true))
	r := NewRouter("match", holder, ollama, cerebras, nil)
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted, got %v", got)
	}
}

func TestProviderClass_CerebrasWithoutCredentialFollowsOllamaFallback(t *testing.T) {
	ollama := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderCerebras},
	}, false))
	r := NewRouter("match", holder, ollama, nil, nil)
	if got := r.ProviderClass(); got != ProviderClassLocal {
		t.Errorf("expected local (fallback to local ollama), got %v", got)
	}
}

// --- Gateway routing tests (029-litellm-proxy-gateway) ---

func TestRouterResolvesGatewayWhenSelectedAndConfigured(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	gateway := &stubProvider{name: "gateway"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"match": {Provider: TaskProviderGateway, Model: "match"},
	}, true))
	r := NewRouter("match", holder, ollama, nil, gateway)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "gateway" {
		t.Errorf("Complete() = %q, want gateway", out)
	}
	if gateway.gotModel != "match" {
		t.Errorf("model passed to gateway = %q, want match (task key)", gateway.gotModel)
	}
}

func TestRouterFallsBackToOllamaWhenGatewayNotConfigured(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"generation": {Provider: TaskProviderGateway, Model: "generation"},
	}, false))
	r := NewRouter("generation", holder, ollama, nil, nil)

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ollama" {
		t.Errorf("Complete() = %q, want ollama (gateway nil fallback)", out)
	}
}

func TestRouterEmbedAlwaysUsesOllamaForGatewayTask(t *testing.T) {
	ollama := &stubProvider{name: "ollama"}
	gateway := &stubProvider{name: "gateway"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"ghost": {Provider: TaskProviderGateway},
	}, true))
	r := NewRouter("ghost", holder, ollama, nil, gateway)

	vec, err := r.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("Embed should delegate to ollama stub, got %v", vec)
	}
}

func TestProviderClass_GatewayWithConfigIsHosted(t *testing.T) {
	ollama := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	gateway := &stubProvider{name: "gateway"}
	holder := NewSnapshotHolder(snapshotWith(map[string]TaskSetting{
		"default": {Provider: TaskProviderGateway},
	}, true))
	r := NewRouter("default", holder, ollama, nil, gateway)
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted, got %v", got)
	}
}
