package application

import (
	"context"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

type stubProvider struct {
	name     string
	gotModel string
	lastOpts *domain.CompleteOptions

	embedHit  bool
	embedText string
	embedVec  []float32
	embedErr  error
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

func (s *stubProvider) CompleteChat(ctx context.Context, msgs []domain.Message, opts *domain.CompleteOptions) (domain.ChatResult, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	text, err := s.CompleteJSON(ctx, prompt, opts)
	return domain.ChatResult{Content: text}, err
}

func (s *stubProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	s.embedHit = true
	s.embedText = text
	if s.embedErr != nil {
		return nil, s.embedErr
	}
	if s.embedVec != nil {
		return s.embedVec, nil
	}
	return []float32{1}, nil
}

func TestRouterRoutesToGatewayWithTaskKeyAsModel(t *testing.T) {
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("generation", gateway)

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

func TestRouterEmbedRoutesToGateway(t *testing.T) {
	gateway := &stubProvider{name: "gateway", embedVec: []float32{0.1, 0.2, 0.3}}
	r := NewRouter("embed", gateway)

	vec, err := r.Embed(context.Background(), "senior go engineer")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if !gateway.embedHit {
		t.Fatal("Embed did not reach the configured gateway")
	}
	if gateway.embedText != "senior go engineer" {
		t.Errorf("embed text = %q, want %q", gateway.embedText, "senior go engineer")
	}
	if len(vec) != 3 {
		t.Errorf("Embed() = %v, want the gateway's 3-element vector", vec)
	}
}

func TestRouterEmbedPropagatesGatewayError(t *testing.T) {
	wantErr := context.DeadlineExceeded
	gateway := &stubProvider{name: "gateway", embedErr: wantErr}
	r := NewRouter("embed", gateway)

	if _, err := r.Embed(context.Background(), "text"); err != wantErr {
		t.Errorf("Embed() error = %v, want %v (no fallback vector, E3-1)", err, wantErr)
	}
}

func TestProviderClassIsAlwaysHosted(t *testing.T) {
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("match", gateway)
	if got := r.ProviderClass(); got != ProviderClassHosted {
		t.Errorf("ProviderClass() = %v, want %v", got, ProviderClassHosted)
	}
}

func TestRouterExplicitCallerModelIsNotOverwritten(t *testing.T) {
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("match", gateway)

	if _, err := r.Complete(context.Background(), "hi", &domain.CompleteOptions{Model: "caller-override"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gateway.gotModel != "caller-override" {
		t.Errorf("gotModel = %q, want caller-override (explicit override wins)", gateway.gotModel)
	}
}

func TestRouterModelNameIsTaskKey(t *testing.T) {
	gateway := &stubProvider{name: "gateway"}
	r := NewRouter("ghost", gateway)
	if got := r.ModelName(); got != "ghost" {
		t.Errorf("ModelName() = %q, want ghost (task key)", got)
	}
}

func TestRouterStampsTaskKey(t *testing.T) {
	stub := &stubProvider{}
	r := NewRouter("generation-summary", stub)

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

func TestRouterDoesNotOverrideExplicitTaskKey(t *testing.T) {
	stub := &stubProvider{}
	r := NewRouter("generation-summary", stub)

	opts := &domain.CompleteOptions{TaskKey: "caller-supplied"}
	if _, err := r.CompleteJSON(context.Background(), "p", opts); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if got := stub.lastOpts.Task(); got != "caller-supplied" {
		t.Errorf("TaskKey = %q, want caller-supplied", got)
	}
}

func TestRouterDoesNotMutateCallerOptions(t *testing.T) {
	stub := &stubProvider{}
	r := NewRouter("match", stub)

	opts := &domain.CompleteOptions{}
	if _, err := r.CompleteJSON(context.Background(), "p", opts); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if opts.TaskKey != "" {
		t.Errorf("caller's opts.TaskKey mutated to %q; router must copy", opts.TaskKey)
	}
}
