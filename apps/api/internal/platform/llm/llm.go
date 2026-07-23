// Package llm is the facade for the LLM platform kernel: the Provider port
// and structured-output retry loop live in domain/, the per-task routing
// policy in application/, and the Ollama/Cerebras adapters in
// infrastructure/. This file re-exports the shape callers already depend on
// (matching, generation, profile, llmsettings, ...) so relocating the
// package into internal/platform/llm required no changes at call sites
// beyond the import path.
package llm

import (
	"context"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/platform/llm/application"
	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/cerebras"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/ollama"
)

type (
	Provider        = domain.Provider
	Validator       = domain.Validator
	CompleteOptions = domain.CompleteOptions

	Router         = application.Router
	TaskProvider   = application.TaskProvider
	TaskSetting    = application.TaskSetting
	RouterSnapshot = application.RouterSnapshot
	SnapshotHolder = application.SnapshotHolder

	OllamaProvider   = ollama.Provider
	CerebrasProvider = cerebras.Provider
	CerebrasModel    = cerebras.Model
)

const (
	TaskProviderOllama   = application.TaskProviderOllama
	TaskProviderCerebras = application.TaskProviderCerebras

	DefaultCerebrasModel = cerebras.DefaultModel
)

var (
	NewRouter         = application.NewRouter
	NewSnapshotHolder = application.NewSnapshotHolder

	NewOllama   = ollama.New
	NewCerebras = cerebras.New

	CerebrasModels           = cerebras.Models
	IsSupportedCerebrasModel = cerebras.IsSupportedModel

	ErrRateLimited = cerebras.ErrRateLimited
)

// CompleteStructured is the Go equivalent of `completeStructured<T>`; see
// domain.CompleteStructured for the retry-loop implementation.
func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	return domain.CompleteStructured[T](ctx, p, prompt, opts)
}

// New builds the Ollama Provider from config. Used by callers that only ever
// need the local provider directly (cmd/llmsmoke, live smoke tests) and
// don't participate in the Cerebras toggle / per-task routing.
func New(cfg *config.Config) (Provider, error) {
	return ollama.New(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL), nil
}

// NewProviders builds the Ollama provider plus, when CerebrasAPIKey is
// configured, the Cerebras provider (001-cerebras-model-toggle). Cerebras is
// nil when no key is set — cmd/server then wires every llm.Router with a nil
// Cerebras leg, which makes the Router fall back to Ollama regardless of a
// task's persisted setting (FR-008).
func NewProviders(cfg *config.Config) (*OllamaProvider, *CerebrasProvider, error) {
	o := ollama.New(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)
	if cfg.CerebrasAPIKey == "" {
		return o, nil, nil
	}
	c, err := cerebras.New(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", o)
	if err != nil {
		return nil, nil, err
	}
	return o, c, nil
}
