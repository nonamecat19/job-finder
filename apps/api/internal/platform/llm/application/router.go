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
	opts = withRouting(opts, model, r.taskKey)
	return p.Complete(ctx, prompt, opts)
}

func (r *Router) CompleteJSON(ctx context.Context, prompt string, opts *domain.CompleteOptions) (string, error) {
	p, model := r.resolve()
	opts = withRouting(opts, model, r.taskKey)
	return p.CompleteJSON(ctx, prompt, opts)
}

// CompleteChat is a passthrough (037, contracts C3-1/C3-2/C3-3).
//
// It resolves provider and model exactly as Complete does and then gets out of
// the way. In particular it does not inspect, filter or rewrite tool
// declarations, and it does not choose a provider based on whether one is
// thought to support tools — the whole point of 030's routing model is that the
// application asks for a task and knows nothing about upstream capabilities.
// Picking a provider here on capability grounds would reintroduce exactly the
// knowledge that design removed.
func (r *Router) CompleteChat(ctx context.Context, msgs []domain.Message, opts *domain.CompleteOptions) (domain.ChatResult, error) {
	p, model := r.resolve()
	opts = withRouting(opts, model, r.taskKey)
	return p.CompleteChat(ctx, msgs, opts)
}

func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) {
	return r.local.Embed(ctx, text)
}

// withRouting stamps the resolved model and the router's task key onto a copy
// of the caller's options.
//
// TaskKey is set here rather than at each call site deliberately (036 FR-012).
// The router is the only place that knows the requested task key for every
// task in the system, so setting it once covers match, generation, ghost,
// salary, rephrase and everything added later — where threading it through
// call sites would cover only the ones somebody remembered. It is observability
// metadata: the collector records the *served deployment* as `model`, so
// without the requested key two stages served by one model are indistinguishable
// in reporting.
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
