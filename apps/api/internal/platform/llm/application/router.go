// Package application holds the LLM routing policy: which provider/model a
// chat task uses, and the atomic-snapshot Router that dispatches to it.
package application

import (
	"context"
	"sync/atomic"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// TaskProvider identifies which provider a task setting selects.
type TaskProvider string

const (
	TaskProviderOllama   TaskProvider = "ollama"
	TaskProviderCerebras TaskProvider = "cerebras"
	TaskProviderGateway  TaskProvider = "gateway"
)

// TaskSetting is the resolved {provider, model} for one chat task, as read
// from LlmTaskSetting (001-cerebras-model-toggle). Model empty means "use the
// provider's default model".
type TaskSetting struct {
	Provider TaskProvider
	Model    string
}

// RouterSnapshot is the full set of per-task settings plus whether a hosted
// credential is configured. It is swapped atomically whenever settings
// change, so in-flight Complete/CompleteJSON calls always see a consistent
// view (research.md R4/R5). CredentialConfigured reflects only the Cerebras
// key (the gateway URL is env-only, not per-task, so it is not part of the
// snapshot — a nil gateway leg already makes such tasks fall back to Ollama).
type RouterSnapshot struct {
	Tasks                map[string]TaskSetting
	CredentialConfigured bool
}

// SnapshotHolder is shared by every task Router for one process; updating it
// once (via Store, called by the llmsettings service after a persisted
// change) updates what all Routers resolve on their next call.
type SnapshotHolder struct {
	v atomic.Value // RouterSnapshot
}

// NewSnapshotHolder builds a holder pre-populated with initial. Callers
// (llmsettings.Service) load the persisted settings before constructing
// this, so a holder never observes a zero-value snapshot.
func NewSnapshotHolder(initial RouterSnapshot) *SnapshotHolder {
	h := &SnapshotHolder{}
	h.v.Store(initial)
	return h
}

// Load returns the currently active snapshot.
func (h *SnapshotHolder) Load() RouterSnapshot {
	return h.v.Load().(RouterSnapshot)
}

// Store atomically replaces the active snapshot.
func (h *SnapshotHolder) Store(s RouterSnapshot) {
	h.v.Store(s)
}

// Router is a task-bound domain.Provider: on every call it resolves the
// current provider/model for its task key from the shared snapshot and
// dispatches to the matching underlying provider. Services depend on Router
// exactly like any other Provider, so per-task runtime switching
// (FR-005/FR-014) needs no change to call sites beyond construction.
type Router struct {
	taskKey  string
	holder   *SnapshotHolder
	ollama   domain.Provider
	cerebras domain.Provider // nil when no Cerebras credential is configured
	gateway  domain.Provider // nil when no GATEWAY_URL is configured
}

// NewRouter builds a Router for taskKey sharing holder (use the same holder
// across all task Routers so one settings update reaches every task).
func NewRouter(taskKey string, holder *SnapshotHolder, ollama, cerebras, gateway domain.Provider) *Router {
	return &Router{taskKey: taskKey, holder: holder, ollama: ollama, cerebras: cerebras, gateway: gateway}
}

// resolve returns the underlying provider and effective model for the
// current snapshot. A hosted provider selected without its configuration
// (Cerebras without a key, gateway without a URL) falls back to Ollama
// (FR-008) — the caller (httpapi layer) is responsible for surfacing
// CredentialConfigured to the operator; the Router just keeps the task
// working.
func (r *Router) resolve() (domain.Provider, string) {
	snap := r.holder.Load()
	setting, ok := snap.Tasks[r.taskKey]
	if !ok {
		return r.ollama, setting.Model
	}
	switch setting.Provider {
	case TaskProviderGateway:
		if r.gateway != nil {
			return r.gateway, setting.Model
		}
		return r.ollama, setting.Model // fall back when gateway unavailable
	case TaskProviderCerebras:
		if r.cerebras != nil {
			return r.cerebras, setting.Model
		}
		return r.ollama, setting.Model
	default:
		return r.ollama, setting.Model
	}
}

// ProviderClass identifies whether a resolved provider is local or hosted
// (019-ai-job-throughput), used to size the admission gate.
type ProviderClass string

const (
	ProviderClassLocal  ProviderClass = "local"
	ProviderClassHosted ProviderClass = "hosted"
)

// hostedChecker is implemented by ollama.Provider; Cerebras and the gateway
// are always hosted when selected (resolve() only returns them when their
// configuration is present).
type hostedChecker interface {
	IsHosted() bool
}

// ProviderClass resolves the current provider class from the live snapshot,
// reusing resolve() so a credential-less remote provider that falls back to
// Ollama reports the *effective* class (data-model.md §3).
func (r *Router) ProviderClass() ProviderClass {
	p, _ := r.resolve()
	if hc, ok := p.(hostedChecker); ok {
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

// Embed always uses Ollama regardless of the task's chat provider — Cerebras
// and the gateway offer no embeddings endpoint (FR-006).
func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) {
	return r.ollama.Embed(ctx, text)
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
