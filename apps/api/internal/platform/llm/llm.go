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
	"github.com/job-finder/api/internal/platform/llm/infrastructure/gateway"
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
	ProviderClass  = application.ProviderClass

	OllamaProvider   = ollama.Provider
	CerebrasProvider = cerebras.Provider
	CerebrasModel    = cerebras.Model
	GatewayProvider  = gateway.Provider
)

const (
	TaskProviderOllama   = application.TaskProviderOllama
	TaskProviderCerebras = application.TaskProviderCerebras
	TaskProviderGateway  = application.TaskProviderGateway

	ProviderClassLocal  = application.ProviderClassLocal
	ProviderClassHosted = application.ProviderClassHosted

	DefaultCerebrasModel = cerebras.DefaultModel
)

var (
	NewRouter         = application.NewRouter
	NewSnapshotHolder = application.NewSnapshotHolder

	NewOllama   = ollama.New
	NewCerebras = cerebras.New
	NewGateway  = gateway.New

	CerebrasModels           = cerebras.Models
	IsSupportedCerebrasModel = cerebras.IsSupportedModel

	ErrRateLimited         = cerebras.ErrRateLimited
	ErrCredentialRejected  = cerebras.ErrCredentialRejected
	ErrInsufficientCredits = cerebras.ErrInsufficientCredits
	ErrModelUnavailable    = cerebras.ErrModelUnavailable
	ErrProviderUnavailable = cerebras.ErrProviderUnavailable
	ErrInvalidResponse     = cerebras.ErrInvalidResponse

	Terminal  = cerebras.Terminal
	Retryable = cerebras.Retryable
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

// NewProviders builds the Ollama provider plus, when configured, the Cerebras
// and gateway providers. Each is nil when its credential/URL is absent —
// cmd/server then wires the Router with a nil leg for that provider, which
// makes the Router fall back to Ollama regardless of a task's persisted
// setting (FR-008).
func NewProviders(cfg *config.Config) (*OllamaProvider, *CerebrasProvider, *GatewayProvider, error) {
	o := ollama.New(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)

	var c *CerebrasProvider
	if cfg.CerebrasAPIKey != "" {
		cp, err := cerebras.New(cfg.CerebrasBaseURL, cfg.CerebrasAPIKey, "", o)
		if err != nil {
			return nil, nil, nil, err
		}
		c = cp
	}

	var gw *GatewayProvider
	if cfg.GatewayURL != "" {
		gwp, err := gateway.New(cfg.GatewayURL, cfg.LiteLLMMasterKey, o)
		if err != nil {
			return nil, nil, nil, err
		}
		gw = gwp
	}

	return o, c, gw, nil
}
