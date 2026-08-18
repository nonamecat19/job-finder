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

type Router struct {
	taskKey string
	gateway domain.Provider
}

func NewRouter(taskKey string, gateway domain.Provider) *Router {
	return &Router{taskKey: taskKey, gateway: gateway}
}

func (r *Router) resolve() (domain.Provider, string) {
	return r.gateway, r.taskKey
}

func (r *Router) ProviderClass() ProviderClass {
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
	opts = withRouting(opts, model, r.taskKey)
	return p.Complete(ctx, prompt, opts)
}

func (r *Router) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	p, model := r.resolve()
	opts = withRouting(opts, model, r.taskKey)
	return p.CompleteJSON(ctx, prompt, opts)
}

func (r *Router) CompleteChat(ctx context.Context, msgs []domain.Message, opts *domain.CompleteOptions) (domain.ChatResult, error) {
	p, model := r.resolve()
	opts = withRouting(opts, model, r.taskKey)
	return p.CompleteChat(ctx, msgs, opts)
}

func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) {
	return r.gateway.Embed(ctx, text)
}

func withRouting(opts *domain.CompleteOptions, model, taskKey string) *domain.CompleteOptions {
	if model == "" && taskKey == "" {
		return opts
	}
	cp := domain.CompleteOptions{}
	if opts != nil {
		cp = *opts
	}
	if cp.Model == "" {
		cp.Model = model
	}
	if cp.TaskKey == "" {
		cp.TaskKey = taskKey
	}
	return &cp
}

var _ domain.Provider = (*Router)(nil)
