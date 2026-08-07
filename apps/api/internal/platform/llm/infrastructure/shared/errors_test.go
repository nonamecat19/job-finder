package shared

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClassifyProviderError(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"quota"}}`, ErrRateLimited},
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"bad key"}}`, ErrCredentialRejected},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"blocked"}}`, ErrCredentialRejected},
		{"payment required", http.StatusPaymentRequired, `{"error":{"message":"broke"}}`, ErrInsufficientCredits},
		{"not found", http.StatusNotFound, `{"error":{"message":"no such model"}}`, ErrModelUnavailable},
		{"bad request", http.StatusBadRequest, `{"error":{"message":"bad model"}}`, ErrModelUnavailable},
		{"unprocessable", http.StatusUnprocessableEntity, `{"error":{"message":"nope"}}`, ErrModelUnavailable},
		{"server error", http.StatusInternalServerError, `{"error":{"message":"boom"}}`, ErrProviderUnavailable},
		{"bad gateway", http.StatusBadGateway, `upstream down`, ErrProviderUnavailable},
		{"no status", 0, `{"error":{"message":"upstream blew up"}}`, ErrProviderUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ClassifyProviderError("testprov", tc.status, []byte(tc.body))
			if !errors.Is(err, tc.want) {
				t.Fatalf("classify(%d) = %v, want %v", tc.status, err, tc.want)
			}
			if !strings.Contains(err.Error(), "testprov") {
				t.Errorf("error %q should name the provider", err)
			}
		})
	}
}

func TestTerminalAndRetryable(t *testing.T) {
	terminal := []error{ErrCredentialRejected, ErrInsufficientCredits, ErrModelUnavailable}
	for _, e := range terminal {
		if !Terminal(e) {
			t.Errorf("Terminal(%v) = false, want true", e)
		}
		if Retryable(e) {
			t.Errorf("Retryable(%v) = true, want false", e)
		}
	}
	retryable := []error{ErrProviderUnavailable, ErrInvalidResponse}
	for _, e := range retryable {
		if !Retryable(e) {
			t.Errorf("Retryable(%v) = false, want true", e)
		}
		if Terminal(e) {
			t.Errorf("Terminal(%v) = true, want false", e)
		}
	}
	if Terminal(ErrRateLimited) || Retryable(ErrRateLimited) {
		t.Error("ErrRateLimited should be neither Terminal nor Retryable")
	}
	if Terminal(errors.New("plain")) || Retryable(errors.New("plain")) {
		t.Error("an unclassified error should be neither Terminal nor Retryable")
	}
}

func TestProviderErrMessageForms(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"error":{"message":"structured"}}`, "structured"},
		{`{"error":"plain string"}`, "plain string"},
		{`not json at all`, "not json at all"},
	}
	for _, tc := range cases {
		if got := ProviderErrMessage([]byte(tc.body)); got != tc.want {
			t.Errorf("ProviderErrMessage(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestRetryAfterParsing(t *testing.T) {
	cases := []struct {
		name   string
		header http.Header
		min    time.Duration
		max    time.Duration
	}{
		{"delta seconds", http.Header{"Retry-After": {"30"}}, 30 * time.Second, 30 * time.Second},
		{"http date", http.Header{"Retry-After": {time.Now().Add(45 * time.Second).UTC().Format(http.TimeFormat)}}, 43 * time.Second, 46 * time.Second},
		{"reset millis", http.Header{"X-Ratelimit-Reset": {strconv.FormatInt(time.Now().Add(90*time.Second).UnixMilli(), 10)}}, 88 * time.Second, 91 * time.Second},
		{"reset seconds", http.Header{"X-Ratelimit-Reset": {strconv.FormatInt(time.Now().Add(120*time.Second).Unix(), 10)}}, 118 * time.Second, 121 * time.Second},
		{"reset duration", http.Header{"X-Ratelimit-Reset": {"20"}}, 20 * time.Second, 20 * time.Second},
		{"nothing usable", http.Header{}, 0, 0},
		{"garbage", http.Header{"Retry-After": {"soon"}}, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RetryAfter(tc.header)
			if got < tc.min || got > tc.max {
				t.Errorf("RetryAfter() = %v, want within [%v, %v]", got, tc.min, tc.max)
			}
		})
	}
}
