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

	// 037: conversation and tool-declaration types.
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

	// 033: structured-output strictness modes.
	ResponseModeJSON   = domain.ResponseModeJSON
	ResponseModeStrict = domain.ResponseModeStrict

	// 037: conversation roles.
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

	// 036: run correlation for observability. WithTraceID is stamped once per
	// logical run; every LLM call made under that context joins the same trace,
	// including retries and escalations emitted from helpers further down.
	WithTraceID = domain.WithTraceID
	TraceIDFrom = domain.TraceIDFrom

	// 037: the one-shot conversation a prompt string becomes.
	PromptMessages = domain.PromptMessages
)

func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	return domain.CompleteStructured[T](ctx, p, prompt, opts)
}

// NewTool declares a lookup whose arguments are the type parameter T, with its
// schema derived from the same path structured output uses (037).
func NewTool[T any](name, description string, handler func(ctx context.Context, args T) (string, error)) ToolDef {
	return domain.NewTool[T](name, description, handler)
}

// CompleteStructuredChat is CompleteStructured over a conversation — the typed
// terminal a tool exchange ends in (037 FR-023).
func CompleteStructuredChat[T any](ctx context.Context, p Provider, msgs []Message, opts *CompleteOptions) (T, error) {
	return domain.CompleteStructuredChat[T](ctx, p, msgs, opts)
}

// NewProviders builds the single inference path (044). GATEWAY_URL and
// LITELLM_MASTER_KEY are required application configuration (K1) — config.Load
// already refuses to boot without them — so this always constructs a gateway.
func NewProviders(cfg *config.Config) (*GatewayProvider, error) {
	return gateway.New(cfg.GatewayURL, cfg.LiteLLMMasterKey, cfg.EmbedDims)
}
