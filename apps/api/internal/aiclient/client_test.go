package aiclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/job-finder/api/internal/events"
)

func TestInvoke_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/capabilities/rephrase/invoke" {
			t.Errorf("path = %q, want /v1/capabilities/rephrase/invoke", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", got)
		}
		if r.Header.Get("X-Request-Id") == "" {
			t.Error("X-Request-Id header not set")
		}
		var req wireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response{
			Status:  events.ResultSucceeded,
			Result:  json.RawMessage(`{"keywords":["go"]}`),
			TraceID: "trace-123",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", nil, 0, nil)
	resp, err := c.Invoke(t.Context(), "rephrase", map[string]string{"a": "b"}, RequestContext{UserID: "u1", WorkID: "w1"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if resp.Status != events.ResultSucceeded {
		t.Errorf("Status = %q, want succeeded", resp.Status)
	}
	if resp.TraceID != "trace-123" {
		t.Errorf("TraceID = %q, want trace-123", resp.TraceID)
	}
}

func TestInvoke_UnknownCapability404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", nil, 0, nil)
	_, err := c.Invoke(t.Context(), "no-such-capability", struct{}{}, RequestContext{})
	if err == nil {
		t.Fatal("Invoke: want error for 404, got nil")
	}
}

func TestInvoke_NonRetryableFailureNotRetried(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(Response{
			Status: events.ResultFailed,
			Failure: &events.Failure{
				Category:  events.FailureInvalidInput,
				Retryable: false,
				Message:   "bad input",
			},
			TraceID: "trace-1",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", nil, 0, nil)
	resp, err := c.Invoke(t.Context(), "rephrase", struct{}{}, RequestContext{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if resp.Failure == nil || resp.Failure.Category != events.FailureInvalidInput {
		t.Fatalf("Failure = %+v, want invalid_input", resp.Failure)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want exactly 1 (no retry for a non-retryable category)", calls)
	}
}

func TestInvoke_RateLimitedRetriesOnceHonouringRetryAfter(t *testing.T) {
	calls := 0
	var firstCallAt, secondCallAt time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			firstCallAt = time.Now()
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(Response{
				Status: events.ResultFailed,
				Failure: &events.Failure{
					Category:  events.FailureRateLimited,
					Retryable: true,
					Message:   "slow down",
				},
			})
			return
		}
		secondCallAt = time.Now()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response{Status: events.ResultSucceeded, Result: json.RawMessage(`{}`)})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", nil, 0, nil)
	resp, err := c.Invoke(t.Context(), "rephrase", struct{}{}, RequestContext{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want exactly 2 (one retry)", calls)
	}
	if resp.Status != events.ResultSucceeded {
		t.Errorf("Status = %q, want succeeded after retry", resp.Status)
	}
	if secondCallAt.Before(firstCallAt) {
		t.Error("retry happened before the original call")
	}
}

func TestInvoke_PerCapabilityTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Response{Status: events.ResultSucceeded})
	}))
	defer srv.Close()

	c := New(srv.URL, "secret", map[string]time.Duration{"slow": 5 * time.Millisecond}, time.Second, nil)
	_, err := c.Invoke(t.Context(), "slow", struct{}{}, RequestContext{})
	if err == nil {
		t.Fatal("Invoke: want timeout error, got nil")
	}
}
