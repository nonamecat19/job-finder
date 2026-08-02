// Package application holds the LLM routing policy: a static, task-bound
// Router that dispatches to the gateway when configured, else Ollama
// (030-litellm-model-routing).
package application

import (
	"context"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// ProviderClass identifies whether a resolved provider is local or hosted
// (019-ai-job-throughput), used to size the admission gate.
type ProviderClass string

const (
	ProviderClassLocal  ProviderClass = "local"
	ProviderClassHosted ProviderClass = "hosted"
)

// hostedChecker is implemented by ollama.Provider; a configured gateway is
// always hosted by definition (FR-013).
type hostedChecker interface {
	IsHosted() bool
}

// Router is a task-bound domain.Provider fixed at construction: it routes
// chat calls to gateway when non-nil (sending the task key as the model, for
// the proxy to resolve per gateway/config.yaml), else to local with
// localModel. There is no holder, no atomic swap, and no runtime
// reconfiguration — routing state lives entirely in gateway/config.yaml and
// environment variables (FR-004/FR-005).
type Router struct {
	taskKey    string
	gateway    domain.Provider // nil when GATEWAY_URL is empty
	local      domain.Provider // Ollama; never nil
	localModel string          // per-task LLM_MODEL_*, "" falls back to the provider default
}

// NewRouter builds a Router for taskKey. gateway is nil when GATEWAY_URL is
// empty; local (Ollama) is never nil. localModel is used only on the
// direct-Ollama path.
func NewRouter(taskKey string, gateway, local domain.Provider, localModel string) *Router {
	return &Router{taskKey: taskKey, gateway: gateway, local: local, localModel: localModel}
}

// resolve returns the underlying provider and effective model for this
// Router's fixed configuration.
func (r *Router) resolve() (domain.Provider, string) {
	if r.gateway != nil {
		return r.gateway, r.taskKey
	}
	return r.local, r.localModel
}

// ProviderClass is hosted whenever a gateway is configured (every hop from
// there is remote, even its own local tier), else it reflects the local
// Ollama provider's own loopback/private-host + API-key heuristic.
func (r *Router) ProviderClass() ProviderClass {
	if r.gateway != nil {
		return ProviderClassHosted
	}
	if hc, ok := r.local.(hostedChecker); ok {
		if hc.IsHosted() {
			return ProviderClassHosted
		}
		return ProviderClassLocal
	}
	return ProviderClassHosted
}

func (r *Router) ModelName() string {
	p, model := r.resolve()
	if model != "" {
		return model
	}
	return p.ModelName()
}

func (r *Router) Complete(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	p, model := r.resolve()
	opts = withModel(opts, model)
	return p.Complete(ctx, prompt, opts)
}

func (r *Router) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	p, model := r.resolve()
	opts = withModel(opts, model)
	return p.CompleteJSON(ctx, prompt, opts)
}

// Embed always uses the local Ollama provider regardless of chat routing —
// the gateway offers no embeddings endpoint (FR-014).
func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) {
	return r.local.Embed(ctx, text)
}

// withModel returns a copy of opts with Model set to model when the caller
// didn't already ask for a specific per-call override; the Router's resolved
// model wins over the task's own default but never overrides an explicit
// caller-supplied CompleteOptions.Model.
func withModel(opts *domain.CompleteOptions, model string) *domain.CompleteOptions {
	if model == "" {
		return opts
	}
	cp := domain.CompleteOptions{}
	if opts != nil {
		cp = *opts
	}
	if cp.Model == "" {
		cp.Model = model
	}
	return &cp
}

var _ domain.Provider = (*Router)(nil)
