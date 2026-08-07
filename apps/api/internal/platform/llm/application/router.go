package application

import (
	"context"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

type ProviderClass string

const (
	ProviderClassLocal  ProviderClass = "local"
	ProviderClassHosted ProviderClass = "hosted"
)

type hostedChecker interface {
	IsHosted() bool
}

type Router struct {
	taskKey    string
	gateway    domain.Provider
	local      domain.Provider
	localModel string
}

func NewRouter(taskKey string, gateway, local domain.Provider, localModel string) *Router {
	return &Router{taskKey: taskKey, gateway: gateway, local: local, localModel: localModel}
}

func (r *Router) resolve() (domain.Provider, string) {
	if r.gateway != nil {
		return r.gateway, r.taskKey
	}
	return r.local, r.localModel
}

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

func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) {
	return r.local.Embed(ctx, text)
}

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
