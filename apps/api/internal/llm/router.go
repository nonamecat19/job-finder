package llm

import (
	"context"
	"sync/atomic"
)

// TaskProvider identifies which provider a task setting selects.
type TaskProvider string

const (
	TaskProviderOllama     TaskProvider = "ollama"
	TaskProviderCerebras   TaskProvider = "cerebras"
	TaskProviderOpenRouter TaskProvider = "openrouter"
)

// TaskSetting is the resolved {provider, model} for one chat task, as read
// from LlmTaskSetting (001-cerebras-model-toggle). Model empty means "use the
// provider's default model".
type TaskSetting struct {
	Provider TaskProvider
	Model    string
}

// RouterSnapshot is the full set of per-task settings plus whether each
// remote provider's credential is configured. It is swapped atomically
// whenever settings change, so in-flight Complete/CompleteJSON calls always
// see a consistent view (research.md R4/R5).
//
// CredentialConfigured is Cerebras's flag (kept unprefixed for wire
// compatibility with the existing /v1/settings/llm response);
// OpenRouterCredentialConfigured is OpenRouter's.
type RouterSnapshot struct {
	Tasks                          map[string]TaskSetting
	CredentialConfigured           bool
	OpenRouterCredentialConfigured bool
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

// Router is a task-bound llm.Provider: on every call it resolves the current
// provider/model for its task key from the shared snapshot and dispatches to
// the matching underlying provider. Services depend on Router exactly like
// any other Provider, so per-task runtime switching (FR-005/FR-014) needs no
// change to call sites beyond construction.
type Router struct {
	taskKey    string
	holder     *SnapshotHolder
	ollama     Provider
	cerebras   Provider // nil when no Cerebras credential is configured
	openrouter Provider // nil when no OpenRouter credential is configured
}

// NewRouter builds a Router for taskKey sharing holder (use the same holder
// across all task Routers so one settings update reaches every task).
func NewRouter(taskKey string, holder *SnapshotHolder, ollama, cerebras, openrouter Provider) *Router {
	return &Router{taskKey: taskKey, holder: holder, ollama: ollama, cerebras: cerebras, openrouter: openrouter}
}

// resolve returns the underlying provider and effective model for the
// current snapshot. A remote provider selected without a configured
// credential falls back to Ollama (FR-008) — the caller (httpapi layer) is
// responsible for surfacing the *CredentialConfigured flags to the operator;
// the Router just keeps the task working.
func (r *Router) resolve() (Provider, string) {
	snap := r.holder.Load()
	setting := snap.Tasks[r.taskKey]
	switch setting.Provider {
	case TaskProviderCerebras:
		if r.cerebras != nil {
			return r.cerebras, setting.Model
		}
		// Credential missing: the persisted model names a Cerebras model
		// Ollama has never heard of, so drop it and use Ollama's default.
		return r.ollama, ""
	case TaskProviderOpenRouter:
		if r.openrouter != nil {
			return r.openrouter, setting.Model
		}
		return r.ollama, ""
	}
	return r.ollama, setting.Model
}

func (r *Router) ModelName() string {
	p, model := r.resolve()
	if model != "" {
		return model
	}
	return p.ModelName()
}

func (r *Router) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	p, model := r.resolve()
	opts = withModel(opts, model)
	return p.Complete(ctx, prompt, opts)
}

func (r *Router) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	p, model := r.resolve()
	opts = withModel(opts, model)
	return p.CompleteJSON(ctx, prompt, opts)
}

// Embed always uses Ollama regardless of the task's chat provider — Cerebras
// has no embeddings endpoint (FR-006).
func (r *Router) Embed(ctx context.Context, text string) ([]float32, error) {
	return r.ollama.Embed(ctx, text)
}

// withModel returns a copy of opts with Model set to model when the caller
// didn't already ask for a specific per-call override; the Router's resolved
// model wins over the task's own default but never overrides an explicit
// caller-supplied CompleteOptions.Model.
func withModel(opts *CompleteOptions, model string) *CompleteOptions {
	if model == "" {
		return opts
	}
	cp := CompleteOptions{}
	if opts != nil {
		cp = *opts
	}
	if cp.Model == "" {
		cp.Model = model
	}
	return &cp
}

var _ Provider = (*Router)(nil)
