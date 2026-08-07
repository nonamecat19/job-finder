// Package llm is the facade for the LLM platform kernel: the Provider port
// and structured-output retry loop live in domain/, the static task-routing
// policy in application/, and the Ollama/gateway adapters in infrastructure/.
// This file re-exports the shape callers already depend on (matching,
// generation, profile, ...) so relocating the package into
// internal/platform/llm required no changes at call sites beyond the import
// path.
package llm

import (
	"context"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/platform/llm/application"
	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/gateway"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/ollama"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

type (
	Provider        = domain.Provider
	Validator       = domain.Validator
	CompleteOptions = domain.CompleteOptions
	ResponseMode    = domain.ResponseMode

	Router        = application.Router
	ProviderClass = application.ProviderClass

	OllamaProvider  = ollama.Provider
	GatewayProvider = gateway.Provider
)

const (
	ProviderClassLocal  = application.ProviderClassLocal
	ProviderClassHosted = application.ProviderClassHosted

	// 033: structured-output strictness modes.
	ResponseModeJSON   = domain.ResponseModeJSON
	ResponseModeStrict = domain.ResponseModeStrict
)

var (
	NewRouter = application.NewRouter

	NewOllama  = ollama.New
	NewGateway = gateway.New

	ErrRateLimited         = shared.ErrRateLimited
	ErrCredentialRejected  = shared.ErrCredentialRejected
	ErrInsufficientCredits = shared.ErrInsufficientCredits
	ErrModelUnavailable    = shared.ErrModelUnavailable
	ErrProviderUnavailable = shared.ErrProviderUnavailable
	ErrInvalidResponse     = shared.ErrInvalidResponse

	Terminal  = shared.Terminal
	Retryable = shared.Retryable

	WithServedModelCapture = domain.WithServedModelCapture
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

// NewProviders builds the Ollama provider plus, when configured, the gateway
// provider. The gateway is nil when GATEWAY_URL is absent — cmd/server then
// wires each task Router with a nil gateway leg, which makes the Router talk
// to Ollama directly (FR-008/FR-009).
func NewProviders(cfg *config.Config) (*OllamaProvider, *GatewayProvider, error) {
	o := ollama.New(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL)

	var gw *GatewayProvider
	if cfg.GatewayURL != "" {
		gwp, err := gateway.New(cfg.GatewayURL, cfg.LiteLLMMasterKey, o)
		if err != nil {
			return nil, nil, err
		}
		gw = gwp
	}

	return o, gw, nil
}
