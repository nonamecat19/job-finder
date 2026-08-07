package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var (
	ErrRateLimited         = errors.New("llm: rate limited")
	ErrCredentialRejected  = errors.New("llm: provider credential rejected")
	ErrInsufficientCredits = errors.New("llm: provider account out of credits")
	ErrModelUnavailable    = errors.New("llm: model unavailable")
	ErrProviderUnavailable = errors.New("llm: provider unavailable")
	ErrInvalidResponse     = errors.New("llm: invalid provider response")
)

func Terminal(err error) bool {
	return errors.Is(err, ErrCredentialRejected) ||
		errors.Is(err, ErrInsufficientCredits) ||
		errors.Is(err, ErrModelUnavailable)
}

func Retryable(err error) bool {
	return errors.Is(err, ErrProviderUnavailable) || errors.Is(err, ErrInvalidResponse)
}

func ProviderErrMessage(body []byte) string {
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

func ClassifyProviderError(provider string, status int, body []byte) error {
	msg := ProviderErrMessage(body)
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

const RateLimitCooldown = 60 * time.Second

const MaxRateLimitCooldown = 15 * time.Minute

func RetryAfter(h http.Header) time.Duration {
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
		case n > 1e12:
			return time.Until(time.UnixMilli(n))
		case n > 1e9:
			return time.Until(time.Unix(n, 0))
		default:
			_ = now
			return time.Duration(n) * time.Second
		}
	}
	return 0
}
