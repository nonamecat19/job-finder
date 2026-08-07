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
	lastOpts *domain.CompleteOptions
}

func (s *stubProvider) ModelName() string { return s.name }
func (s *stubProvider) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	s.gotModel = opts.ModelOr("")
	s.lastOpts = opts
	return s.name, nil
}
func (s *stubProvider) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	s.gotModel = opts.ModelOr("")
	s.lastOpts = opts
	return s.name, nil
}

func (s *stubProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1}, nil
}

func TestRouterNilGatewayRoutesToLocalWithLocalModel(t *testing.T) {
	local := &stubProvider{name: "ollama"}
	r := NewRouter("match", nil, local, "gpt-oss:20b-cloud")

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "ollama" {
		t.Errorf("Complete() = %q, want ollama", out)
	}
	if local.gotModel != "gpt-oss:20b-cloud" {
		t.Errorf("model passed to local = %q, want gpt-oss:20b-cloud", local.gotModel)
	}
}

func TestRouterNonNilGatewayRoutesWithTaskKeyAsModel(t *testing.T) {
	local := &stubProvider{name: "ollama"}
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("generation", gateway, local, "gpt-oss:120b-cloud")

	out, err := r.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "gateway" {
		t.Errorf("Complete() = %q, want gateway", out)
	}
	if gateway.gotModel != "generation" {
		t.Errorf("model passed to gateway = %q, want generation (task key)", gateway.gotModel)
	}
}

func TestRouterEmbedAlwaysUsesLocal(t *testing.T) {
	local := &stubProvider{name: "ollama"}
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("ghost", gateway, local, "")

	vec, err := r.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("Embed should delegate to local stub, got %v", vec)
	}
}

func TestRouterEmbedAlwaysUsesLocalWithNilGateway(t *testing.T) {
	local := &stubProvider{name: "ollama"}
	r := NewRouter("default", nil, local, "")

	vec, err := r.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 1 {
		t.Errorf("Embed should delegate to local stub, got %v", vec)
	}
}

func TestRouterExplicitCallerModelIsNotOverwritten(t *testing.T) {
	local := &stubProvider{name: "ollama"}
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("match", gateway, local, "")

	if _, err := r.Complete(context.Background(), "hi", &domain.CompleteOptions{Model: "caller-override"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gateway.gotModel != "caller-override" {
		t.Errorf("gotModel = %q, want caller-override (explicit override wins)", gateway.gotModel)
	}
}

func TestRouterEmptyLocalModelUsesProviderDefault(t *testing.T) {
	local := &stubProvider{name: "ollama"}
	r := NewRouter("rephrase", nil, local, "")

	if _, err := r.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if local.gotModel != "" {
		t.Errorf("gotModel = %q, want empty (provider default)", local.gotModel)
	}
}

func TestProviderClass_NilGatewayLoopbackIsLocal(t *testing.T) {
	local := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	r := NewRouter("match", nil, local, "")
	if got := r.ProviderClass(); got != ProviderClassLocal {
		t.Errorf("expected local, got %v", got)
	}
}

func TestProviderClass_NilGatewayLoopbackWithKeySetIsHosted(t *testing.T) {
	local := ollamaprovider.New("http://127.0.0.1:11434", "some-key", "", "", "")
	r := NewRouter("match", nil, local, "")
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted (key set on loopback), got %v", got)
	}
}

func TestProviderClass_NilGatewayCloudURLIsHosted(t *testing.T) {
	local := ollamaprovider.New("https://ollama.com", "cloud-key", "", "", "")
	r := NewRouter("match", nil, local, "")
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted, got %v", got)
	}
}

func TestProviderClass_ConfiguredGatewayIsAlwaysHosted(t *testing.T) {
	local := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("default", gateway, local, "")
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("expected hosted, got %v", got)
	}
}

func TestRouterModelNameGatewayIsTaskKey(t *testing.T) {
	local := ollamaprovider.New("http://localhost:11434", "", "", "", "")
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("ghost", gateway, local, "")
	if got := r.ModelName(); got != "ghost" {
		t.Errorf("ModelName() = %q, want ghost (task key)", got)
	}
}

func TestRouterModelNameLocalFallsBackToProviderDefault(t *testing.T) {
	local := &stubProvider{name: "ollama-default-model"}
	r := NewRouter("match", nil, local, "")
	if got := r.ModelName(); got != "ollama-default-model" {
		t.Errorf("ModelName() = %q, want ollama-default-model", got)
	}
}

// 036 FR-012: the router stamps its task key onto every call, so the collector
// can group by task rather than by serving deployment. Setting it here rather
// than at each call site is what makes coverage complete by construction —
// every task routed through a Router gets it, including ones added later.
func TestRouterStampsTaskKey(t *testing.T) {
	stub := &stubProvider{}
	r := NewRouter("generation-summary", stub, nil, "")

	if _, err := r.CompleteJSON(context.Background(), "p", nil); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if got := stub.lastOpts.Task(); got != "generation-summary" {
		t.Errorf("TaskKey = %q, want generation-summary", got)
	}

	if _, err := r.Complete(context.Background(), "p", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got := stub.lastOpts.Task(); got != "generation-summary" {
		t.Errorf("TaskKey = %q, want generation-summary", got)
	}
}

// An explicit TaskKey from the caller wins, matching how Model already behaves.
func TestRouterDoesNotOverrideExplicitTaskKey(t *testing.T) {
	stub := &stubProvider{}
	r := NewRouter("generation-summary", stub, nil, "")

	opts := &domain.CompleteOptions{TaskKey: "caller-supplied"}
	if _, err := r.CompleteJSON(context.Background(), "p", opts); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if got := stub.lastOpts.Task(); got != "caller-supplied" {
		t.Errorf("TaskKey = %q, want caller-supplied", got)
	}
}

// The caller's options must not be mutated — the router copies.
func TestRouterDoesNotMutateCallerOptions(t *testing.T) {
	stub := &stubProvider{}
	r := NewRouter("match", stub, nil, "")

	opts := &domain.CompleteOptions{}
	if _, err := r.CompleteJSON(context.Background(), "p", opts); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if opts.TaskKey != "" {
		t.Errorf("caller's opts.TaskKey mutated to %q; router must copy", opts.TaskKey)
	}
}
