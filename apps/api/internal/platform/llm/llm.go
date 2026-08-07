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

func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	return domain.CompleteStructured[T](ctx, p, prompt, opts)
}

func New(cfg *config.Config) (Provider, error) {
	return ollama.New(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL), nil
}

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
