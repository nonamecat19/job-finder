package cerebras

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"
)

// The error taxonomy every remote chat provider (Cerebras) maps
// its HTTP failures onto, so callers can decide what to do without knowing
// which provider produced the error:
//
//	ErrRateLimited         — quota exhausted; a later attempt may work, but
//	                         not now. Callers cancel rather than retry.
//	ErrCredentialRejected  — bad/revoked API key. Terminal: retrying burns
//	                         requests and can never succeed.
//	ErrInsufficientCredits — account out of credit. Terminal.
//	ErrModelUnavailable    — the selected model id is unknown/withdrawn, or
//	                         the request was malformed. Terminal.
//	ErrProviderUnavailable — 5xx or a transport failure. Retryable.
//	ErrInvalidResponse     — 2xx whose body wasn't the expected shape.
//	                         Retryable (usually a transient upstream glitch).
var (
	// ErrRateLimited is returned (wrapped) whenever a provider responds 429,
	// or while that provider's breaker is tripped from a prior 429. Callers
	// (the asynq task handlers) treat it as "skip this task" rather than
	// "retry it" — see rateLimitCooldown.
	ErrRateLimited         = errors.New("llm: rate limited")
	ErrCredentialRejected  = errors.New("llm: provider credential rejected")
	ErrInsufficientCredits = errors.New("llm: provider account out of credits")
	ErrModelUnavailable    = errors.New("llm: model unavailable")
	ErrProviderUnavailable = errors.New("llm: provider unavailable")
	ErrInvalidResponse     = errors.New("llm: invalid provider response")
)

// Terminal reports whether err can never succeed on a retry with the same
// settings, so the operator has to change something (key, model, plan).
// Handlers use it to fail a task outright instead of handing it back to
// asynq for a retry that is guaranteed to fail the same way.
func Terminal(err error) bool {
	return errors.Is(err, ErrCredentialRejected) ||
		errors.Is(err, ErrInsufficientCredits) ||
		errors.Is(err, ErrModelUnavailable)
}

// Retryable reports whether a later attempt could plausibly succeed without
// any operator action. Rate limiting is deliberately NOT retryable here: the
// breaker already holds the whole process off, and handlers cancel those
// tasks rather than re-queueing them against an exhausted quota.
func Retryable(err error) bool {
	return errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrInvalidResponse)
}

// providerErrMessage extracts a human-readable message from an OpenAI-style
// {"error":{"message":...}} body, or Ollama's simpler {"error":"..."} string
// form, falling back to the raw body. It reads only the response, so an API
// key (sent solely in the Authorization header) can never leak into an error
// string derived from it.
func providerErrMessage(body []byte) string {
	var obj struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &obj); err == nil && obj.Error.Message != "" {
		return obj.Error.Message
	}
	var str struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &str); err == nil && str.Error != "" {
		return str.Error
	}
	return string(body)
}

// classifyProviderError maps one provider's HTTP status + body onto the
// taxonomy above. provider is the short name used in the message ("cerebras",
// "ollama"). status 0 means the failure was reported in the
// body rather than the status line and carried no usable code.
func classifyProviderError(provider string, status int, body []byte) error {
	msg := providerErrMessage(body)
	switch {
	case status <= 0:
		return fmt.Errorf("%w: %s: upstream error: %s", ErrProviderUnavailable, provider, msg)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %s: rate limit or quota exceeded: %s", ErrRateLimited, provider, msg)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: %s: credential rejected: %s", ErrCredentialRejected, provider, msg)
	case status == http.StatusPaymentRequired:
		return fmt.Errorf("%w: %s: insufficient credits: %s", ErrInsufficientCredits, provider, msg)
	case status == http.StatusNotFound || status == http.StatusBadRequest ||
		status == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s: returned %d: %s", ErrModelUnavailable, provider, status, msg)
	default:
		return fmt.Errorf("%w: %s: returned %d: %s", ErrProviderUnavailable, provider, status, msg)
	}
}

// rateLimitCooldown is the fallback cooldown after a 429 that carries no
// usable reset hint, matching a typical per-minute free-tier quota window.
const rateLimitCooldown = 60 * time.Second

// maxRateLimitCooldown caps whatever a provider asks for, so one hostile or
// mistaken header cannot park a task queue for hours.
const maxRateLimitCooldown = 15 * time.Minute

// rateLimitBreaker is a tiny per-provider circuit breaker: the first 429
// trips it, and every other in-flight/queued task sharing that provider (one
// instance per process, shared by every task Router) immediately fails fast
// with ErrRateLimited instead of also burning a request against the same
// exhausted quota.
type rateLimitBreaker struct {
	until atomic.Int64 // unix nano; 0 or in the past = not tripped
}

// tripFor holds the breaker for d, clamped to (0, maxRateLimitCooldown].
// A later reset never shortens an earlier, longer one.
func (b *rateLimitBreaker) tripFor(d time.Duration) {
	if d <= 0 {
		d = rateLimitCooldown
	}
	if d > maxRateLimitCooldown {
		d = maxRateLimitCooldown
	}
	until := time.Now().Add(d).UnixNano()
	for {
		cur := b.until.Load()
		if cur >= until {
			return
		}
		if b.until.CompareAndSwap(cur, until) {
			return
		}
	}
}

func (b *rateLimitBreaker) tripped() bool {
	u := b.until.Load()
	return u != 0 && time.Now().UnixNano() < u
}

// retryAfter reads how long a provider wants us to wait before the next
// request, from (in order) RFC 7231's Retry-After — both the delta-seconds
// and the HTTP-date form — then the de-facto X-RateLimit-Reset header, which
// providers send as either a unix timestamp (seconds or milliseconds) or a
// duration in seconds. Returns 0 when no usable hint is present, which makes
// callers fall back to rateLimitCooldown.
func retryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			return time.Until(t)
		}
	}
	for _, key := range []string{"X-RateLimit-Reset", "X-RateLimit-Reset-Requests", "X-RateLimit-Reset-Tokens"} {
		v := h.Get(key)
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			continue
		}
		now := time.Now()
		switch {
		case n > 1e12: // unix milliseconds
			return time.Until(time.UnixMilli(n))
		case n > 1e9: // unix seconds
			return time.Until(time.Unix(n, 0))
		default: // a plain duration in seconds
			_ = now
			return time.Duration(n) * time.Second
		}
	}
	return 0
}
