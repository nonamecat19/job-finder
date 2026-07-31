package cerebras

import (
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

// Backward-compatible re-export of the shared provider error taxonomy
// (029-litellm-proxy-gateway): the classification logic now lives in
// infrastructure/shared so the gateway adapter can reuse it, but every
// existing caller still refers to it as cerebras.ErrRateLimited & co.

var (
	// ErrRateLimited is returned (wrapped) whenever a provider responds 429,
	// or while that provider's breaker is tripped from a prior 429. Callers
	// (the asynq task handlers) treat it as "skip this task" rather than
	// "retry it" — see RateLimitCooldown.
	ErrRateLimited         = shared.ErrRateLimited
	ErrCredentialRejected  = shared.ErrCredentialRejected
	ErrInsufficientCredits = shared.ErrInsufficientCredits
	ErrModelUnavailable    = shared.ErrModelUnavailable
	ErrProviderUnavailable = shared.ErrProviderUnavailable
	ErrInvalidResponse     = shared.ErrInvalidResponse
)

// Terminal reports whether err can never succeed on a retry with the same
// settings, so the operator has to change something (key, model, plan).
func Terminal(err error) bool { return shared.Terminal(err) }

// Retryable reports whether a later attempt could plausibly succeed without
// any operator action.
func Retryable(err error) bool { return shared.Retryable(err) }
