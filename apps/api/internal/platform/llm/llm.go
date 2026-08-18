package llm

import (
	"context"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/platform/llm/application"
	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/gateway"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

type (
	Provider        = domain.Provider
	Validator       = domain.Validator
	CompleteOptions = domain.CompleteOptions
	ResponseMode    = domain.ResponseMode
	Usage           = domain.Usage

	Message    = domain.Message
	Role       = domain.Role
	ToolCall   = domain.ToolCall
	ChatResult = domain.ChatResult
	ToolDef    = domain.ToolDef

	Router        = application.Router
	ProviderClass = application.ProviderClass

	GatewayProvider = gateway.Provider
)

const (
	ProviderClassLocal  = application.ProviderClassLocal
	ProviderClassHosted = application.ProviderClassHosted

	ResponseModeJSON   = domain.ResponseModeJSON
	ResponseModeStrict = domain.ResponseModeStrict

	RoleSystem    = domain.RoleSystem
	RoleUser      = domain.RoleUser
	RoleAssistant = domain.RoleAssistant
	RoleTool      = domain.RoleTool
)

var (
	NewRouter = application.NewRouter

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
	WithUsageCapture       = domain.WithUsageCapture
	ReportUsage            = domain.ReportUsage
	ReportServedModel      = domain.ReportServedModel

	WithTraceID = domain.WithTraceID
	TraceIDFrom = domain.TraceIDFrom

	PromptMessages = domain.PromptMessages
)

func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	return domain.CompleteStructured[T](ctx, p, prompt, opts)
}

func NewTool[T any](name, description string, handler func(ctx context.Context, args T) (string, error)) ToolDef {
	return domain.NewTool[T](name, description, handler)
}

func CompleteStructuredChat[T any](ctx context.Context, p Provider, msgs []Message, opts *CompleteOptions) (T, error) {
	return domain.CompleteStructuredChat[T](ctx, p, msgs, opts)
}

func NewProviders(cfg *config.Config) (*GatewayProvider, error) {
	return gateway.New(cfg.GatewayURL, cfg.LiteLLMMasterKey, cfg.EmbedDims)
}
