package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/ollama"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

func newTestGateway(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := New(srv.URL, "sk-test-master", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestNewGatewayRequiresBaseURLAndKey(t *testing.T) {
	if _, err := New("", "key", nil); err == nil {
		t.Error("expected error when baseURL is empty")
	}
	if _, err := New("http://litellm:4000", "", nil); err == nil {
		t.Error("expected error when apiKey is empty")
	}
}

func TestGatewayModelName(t *testing.T) {
	p, err := New("http://litellm:4000", "key", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.ModelName() != "gateway" {
		t.Errorf("ModelName() = %q, want gateway", p.ModelName())
	}
}

func TestGatewayCompleteRequestShape(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody chatRequest
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "hello"}}},
		})
	})

	out, err := p.Complete(context.Background(), "hi", &domain.CompleteOptions{Model: "match"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "hello" {
		t.Errorf("Complete() = %q, want hello", out)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test-master" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotBody.Model != "match" {
		t.Errorf("request model = %q, want match (task key via per-call override)", gotBody.Model)
	}
}

func TestGatewayCompleteSendsTaskKeyByDefault(t *testing.T) {
	var gotBody chatRequest
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "ok"}}},
		})
	})
	if _, err := p.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gotBody.Model != "gateway" {
		t.Errorf("request model with no override = %q, want gateway (ModelName fallback)", gotBody.Model)
	}
}

func TestGatewayCompleteJSONSetsResponseFormat(t *testing.T) {
	var gotBody chatRequest
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{{Message: struct {
				Content string `json:"content"`
			}{Content: "{}"}}},
		})
	})
	if _, err := p.CompleteJSON(context.Background(), "hi", &domain.CompleteOptions{Model: "generation"}); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if gotBody.Model != "generation" {
		t.Errorf("request model = %q, want generation", gotBody.Model)
	}
	if gotBody.ResponseFormat["type"] != "json_object" {
		t.Errorf("response_format = %+v, want json_object", gotBody.ResponseFormat)
	}
}

func TestGatewayCompleteEmptyChoices(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatResponse{})
	})
	if _, err := p.Complete(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected error on empty choices")
	}
}

func TestGatewayErrorClassification(t *testing.T) {
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
		{"unknown model", http.StatusBadRequest, `{"error":{"message":"model not found: match"}}`, shared.ErrModelUnavailable},
		{"provider 500", http.StatusInternalServerError, `{"error":{"message":"upstream boom"}}`, shared.ErrProviderUnavailable},
		{"proxy 502", http.StatusBadGateway, `bad gateway`, shared.ErrProviderUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := p.Complete(context.Background(), "hi", nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want %v", err, tc.want)
			}
			if strings.Contains(err.Error(), "sk-test-master") {
				t.Error("error message must never contain the API key")
			}
		})
	}
}

func TestGatewayConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now refuses connections
	p, err := New(url, "key", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = p.Complete(context.Background(), "hi", nil)
	if err == nil {
		t.Fatal("expected error on connection refused")
	}
	if !errors.Is(err, shared.ErrProviderUnavailable) {
		t.Errorf("connection refused error = %v, want ErrProviderUnavailable", err)
	}
	if !shared.Retryable(err) {
		t.Errorf("connection refused should be Retryable, got %v", err)
	}
}

// --- Served-model logging (030-litellm-model-routing, contracts/task-router.md §C4) ---

func TestServedModelPrefersHeaderOverBody(t *testing.T) {
	h := http.Header{}
	h.Set("x-litellm-model-name", "cerebras/gpt-oss-120b")
	got := servedModel(h, chatResponse{Model: "match"})
	if got != "cerebras/gpt-oss-120b" {
		t.Errorf("servedModel() = %q, want header value", got)
	}
}

func TestServedModelFallsBackToBodyWhenHeaderAbsent(t *testing.T) {
	got := servedModel(http.Header{}, chatResponse{Model: "llama-3.3-70b-versatile"})
	if got != "llama-3.3-70b-versatile" {
		t.Errorf("servedModel() = %q, want body model field", got)
	}
}

func TestServedModelUnknownWhenBothAbsent(t *testing.T) {
	got := servedModel(http.Header{}, chatResponse{})
	if got != "unknown" {
		t.Errorf("servedModel() = %q, want unknown", got)
	}
}

func TestGatewayCompleteSucceedsWhenModelFieldMissing(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		// No x-litellm-model-name header and no body "model" field.
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	})
	out, err := p.Complete(context.Background(), "hi", nil)
	if err != nil {
		t.Fatalf("Complete: %v (a missing served-model field must never be an error)", err)
	}
	if out != "hi" {
		t.Errorf("Complete() = %q, want hi", out)
	}
}

func TestGatewayErrorClassificationUnaffectedByServedModelLogging(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded"}}`))
	})
	_, err := p.Complete(context.Background(), "hi", nil)
	if !errors.Is(err, shared.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited (logging must not change classification)", err)
	}
}

func TestGatewayEmbedDelegatesToOllama(t *testing.T) {
	var embedHit bool
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{0.1, 0.2}})
	}))
	t.Cleanup(ollamaSrv.Close)
	ollamaProvider := ollama.New(ollamaSrv.URL, "", "", "", "")

	p, err := New("https://unused.example", "key", ollamaProvider)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	vec, err := p.Embed(context.Background(), "text")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if !embedHit {
		t.Error("Embed did not reach the Ollama provider")
	}
	if len(vec) != 2 {
		t.Errorf("Embed() len = %d, want 2", len(vec))
	}
}

// The regression this guards: an http.Client timeout is absolute and
// overrides the caller's context, so a value below the proxy's worst-case
// fallback chain truncates long tasks locally. Generation runs carrying a
// 900s budget once died at exactly 120.0s under a 120s client cap while the
// proxy was still working. gateway/config.yaml bounds its chain to
// tiers x (1+num_retries) x request_timeout = 5 x 2 x 60 = 600s; this must
// stay above that so the proxy is what times out first and the caller gets a
// real upstream error instead of a local deadline.
func TestClientTimeoutExceedsProxyWorstCaseChain(t *testing.T) {
	const proxyWorstCase = 600 * time.Second

	g, err := New("http://gateway.invalid", "key", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if g.http.Timeout != safetyNetTimeout {
		t.Fatalf("client timeout = %s, want the safety net %s", g.http.Timeout, safetyNetTimeout)
	}
	if safetyNetTimeout <= proxyWorstCase {
		t.Fatalf("safetyNetTimeout %s must exceed the proxy's worst-case chain %s, "+
			"or the app aborts mid-chain and reports a local deadline", safetyNetTimeout, proxyWorstCase)
	}
}
