package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

// This file is written against the TARGET embedding contract
// (specs/044-litellm-only-routing/contracts/embeddings.md E1-E3, E6 / tasks.md
// T015), before the real implementation (T019) lands. Today `Provider.Embed`
// still delegates to an injected `ollama domain.Provider` (gateway.go:466-468,
// E5-1 "not a delegation" — the thing this feature removes). These tests are
// expected to FAIL against that code, not to compile-fail and not to panic:
// every assertion below is reached through a normal error/value comparison,
// never through a nil-interface call, so a still-delegating Embed produces a
// clean t.Errorf/t.Fatalf rather than a crash of the whole test binary.
//
// Do not "fix" these tests by loosening them to pass today. They pin the
// contract T019 must satisfy.

// wantEmbedDims is the target EMBED_DIMS default once T025 lands
// (contracts/configuration.md K3: 768, read by nothing -> 1024, asserted
// against every returned vector). It is hardcoded here — rather than read
// from internal/config — because gateway.New has no embedDims parameter yet;
// wiring that is part of the implementation task, not this test-only one.
const wantEmbedDims = 1024

func newEmbedTestGateway(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := New(srv.URL, "sk-test-master", wantEmbedDims)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// TestEmbedHappyPath — E2-1: a valid response's data[0].embedding becomes the
// returned []float32.
func TestEmbedHappyPath(t *testing.T) {
	want := make([]float32, wantEmbedDims)
	for i := range want {
		want[i] = float32(i) / float32(wantEmbedDims)
	}
	p := newEmbedTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "embed",
			"data": []map[string]any{
				{"index": 0, "embedding": want},
			},
			"usage": map[string]any{"prompt_tokens": 3, "total_tokens": 3},
		})
	})

	got, err := p.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(got) != wantEmbedDims {
		t.Fatalf("Embed() len = %d, want %d", len(got), wantEmbedDims)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Embed()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestEmbedEmptyDataIsInvalidResponse — E2-1: an empty data array is
// ErrInvalidResponse, the same sentinel the chat path returns for zero choices
// (gateway.go send(): "no choices returned").
func TestEmbedEmptyDataIsInvalidResponse(t *testing.T) {
	p := newEmbedTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "embed", "data": []any{}})
	})

	_, err := p.Embed(context.Background(), "hello world")
	if !errors.Is(err, shared.ErrInvalidResponse) {
		t.Errorf("error = %v, want ErrInvalidResponse (empty data array, E2-1)", err)
	}
}

// TestEmbedWrongLengthVectorIsError — E2-2, the load-bearing check: the
// returned vector's length MUST equal the configured EMBED_DIMS, or it is an
// error, never a stored value.
func TestEmbedWrongLengthVectorIsError(t *testing.T) {
	wrong := make([]float32, wantEmbedDims-1)
	p := newEmbedTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "embed",
			"data":  []map[string]any{{"index": 0, "embedding": wrong}},
		})
	})

	_, err := p.Embed(context.Background(), "hello world")
	if err == nil {
		t.Fatal("expected an error for a vector whose length does not equal EMBED_DIMS (E2-2); " +
			"a mismatch must never be silently stored")
	}
}

// TestEmbedErrorClassification — E3: each proxy status maps to the same
// sentinel the chat path uses, through shared.ClassifyProviderError.
func TestEmbedErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"quota exceeded"}}`, shared.ErrRateLimited},
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"invalid master key"}}`, shared.ErrCredentialRejected},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"blocked"}}`, shared.ErrCredentialRejected},
		{"payment required", http.StatusPaymentRequired, `{"error":{"message":"insufficient credits"}}`, shared.ErrInsufficientCredits},
		{"not found", http.StatusNotFound, `{"error":{"message":"model not found: embed"}}`, shared.ErrModelUnavailable},
		{"bad request", http.StatusBadRequest, `{"error":{"message":"bad model"}}`, shared.ErrModelUnavailable},
		{"unprocessable", http.StatusUnprocessableEntity, `{"error":{"message":"nope"}}`, shared.ErrModelUnavailable},
		{"server error", http.StatusInternalServerError, `{"error":{"message":"upstream boom"}}`, shared.ErrProviderUnavailable},
		{"bad gateway", http.StatusBadGateway, `bad gateway`, shared.ErrProviderUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newEmbedTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := p.Embed(context.Background(), "hello world")
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestEmbedTransportFailureIsProviderUnavailable — E3: a transport failure
// (connection refused) classifies as ErrProviderUnavailable, same as chat's
// TestGatewayConnectionRefused.
func TestEmbedTransportFailureIsProviderUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	p, err := New(url, "sk-test-master", wantEmbedDims)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = p.Embed(context.Background(), "hello world")
	if !errors.Is(err, shared.ErrProviderUnavailable) {
		t.Errorf("error = %v, want ErrProviderUnavailable (transport failure)", err)
	}
}
