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
