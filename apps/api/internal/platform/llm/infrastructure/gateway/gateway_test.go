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
	"github.com/job-finder/api/internal/platform/llm/infrastructure/shared"
)

func newTestGateway(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := New(srv.URL, "sk-test-master", wantEmbedDims)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestNewGatewayRequiresBaseURLAndKey(t *testing.T) {
	if _, err := New("", "key", wantEmbedDims); err == nil {
		t.Error("expected error when baseURL is empty")
	}
	if _, err := New("http://litellm:4000", "", wantEmbedDims); err == nil {
		t.Error("expected error when apiKey is empty")
	}
}

func TestGatewayModelName(t *testing.T) {
	p, err := New("http://litellm:4000", "key", wantEmbedDims)
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
			Choices: []chatChoice{{Message: chatChoiceMessage{Content: "hello"}}},
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
			Choices: []chatChoice{{Message: chatChoiceMessage{Content: "ok"}}},
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
			Choices: []chatChoice{{Message: chatChoiceMessage{Content: "{}"}}},
		})
	})
	if _, err := p.CompleteJSON(context.Background(), "hi", &domain.CompleteOptions{Model: "generation"}); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if gotBody.Model != "generation" {
		t.Errorf("request model = %q, want generation", gotBody.Model)
	}
	if gotBody.ResponseFormat == nil || gotBody.ResponseFormat.Type != "json_object" {
		t.Errorf("response_format = %+v, want json_object", gotBody.ResponseFormat)
	}
}

func TestGatewayCompleteJSONStrictSendsJsonSchema(t *testing.T) {
	var gotBody chatRequest
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(chatResponse{
			Choices: []chatChoice{{Message: chatChoiceMessage{Content: "{}"}}},
		})
	})
	opts := &domain.CompleteOptions{
		Model:        "generation",
		ResponseMode: domain.ResponseModeStrict,
		JSONSchema:   `{"type":"object","properties":{"summary":{"type":"string"}},"additionalProperties":false}`,
	}
	if _, err := p.CompleteJSON(context.Background(), "hi", opts); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if gotBody.ResponseFormat == nil || gotBody.ResponseFormat.Type != "json_schema" {
		t.Fatalf("response_format = %+v, want json_schema", gotBody.ResponseFormat)
	}
	if gotBody.ResponseFormat.JSONSchema == nil {
		t.Fatal("json_schema block missing")
	}
	if !gotBody.ResponseFormat.JSONSchema.Strict {
		t.Error("json_schema.strict = false, want true")
	}
	schema := gotBody.ResponseFormat.JSONSchema.Schema
	if schema == nil {
		t.Fatal("json_schema.schema is nil")
	}
	if ap, ok := schema["additionalProperties"].(bool); !ok || ap {
		t.Errorf("json_schema.schema additionalProperties = %v, want false", schema["additionalProperties"])
	}
}

func TestGatewayCompleteJSONSendsMaxCompletionTokens(t *testing.T) {
	var raw map[string]any
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&raw)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	})
	maxTokens := 8192
	opts := &domain.CompleteOptions{Model: "generation-analyze", MaxTokens: &maxTokens}
	if _, err := p.CompleteJSON(context.Background(), "hi", opts); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	got, ok := raw["max_completion_tokens"].(float64)
	if !ok {
		t.Fatalf("max_completion_tokens missing from request body: %v", raw)
	}
	if int(got) != maxTokens {
		t.Errorf("max_completion_tokens = %d, want %d", int(got), maxTokens)
	}
}

func TestGatewayOmitsMaxCompletionTokensWhenUnset(t *testing.T) {
	var raw map[string]any
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&raw)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	})
	if _, err := p.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, present := raw["max_completion_tokens"]; present {
		t.Errorf("max_completion_tokens sent with no cap configured: %v", raw["max_completion_tokens"])
	}
}

func TestGatewayCapturesUsage(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"openrouter/anthropic/claude-sonnet-5",
			"choices":[{"message":{"content":"hi"}}],
			"usage":{"cost":0.004213,"prompt_tokens":1820,"completion_tokens":355}}`))
	})
	ctx, usage := domain.WithUsageCapture(context.Background())
	if _, err := p.Complete(ctx, "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if usage.CostUSD != 0.004213 {
		t.Errorf("CostUSD = %v, want 0.004213", usage.CostUSD)
	}
	if usage.PromptTokens != 1820 {
		t.Errorf("PromptTokens = %d, want 1820", usage.PromptTokens)
	}
	if usage.CompletionTokens != 355 {
		t.Errorf("CompletionTokens = %d, want 355", usage.CompletionTokens)
	}
}

func TestGatewayUsageZeroWhenResponseOmitsIt(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
	})
	ctx, usage := domain.WithUsageCapture(context.Background())
	if _, err := p.Complete(ctx, "hi", nil); err != nil {
		t.Fatalf("Complete: %v (an unpriced deployment must never fail the call)", err)
	}
	if *usage != (domain.Usage{}) {
		t.Errorf("usage = %+v, want zero value", *usage)
	}
}

func TestGatewayCapturesSubstitutionFromHeaders(t *testing.T) {
	cases := []struct {
		name            string
		attemptedHeader string
		wantAttempted   int
		wantSubstituted bool
	}{
		{name: "tier 1 served", attemptedHeader: "0"},
		{name: "chain advanced once", attemptedHeader: "1", wantAttempted: 1, wantSubstituted: true},
		{name: "header absent", attemptedHeader: ""},
		{name: "header malformed", attemptedHeader: "yes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-litellm-model-group", "generation-analyze-fallback")
				if tc.attemptedHeader != "" {
					w.Header().Set("x-litellm-attempted-fallbacks", tc.attemptedHeader)
				}
				w.Header().Set("x-litellm-response-cost", "3.615e-05")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}]}`))
			})
			ctx, usage := domain.WithUsageCapture(context.Background())
			if _, err := p.Complete(ctx, "hi", nil); err != nil {
				t.Fatalf("Complete: %v (a malformed provenance header must never fail the call)", err)
			}
			if usage.ServedGroup != "generation-analyze-fallback" {
				t.Errorf("ServedGroup = %q, want generation-analyze-fallback", usage.ServedGroup)
			}
			if usage.AttemptedFallbacks != tc.wantAttempted {
				t.Errorf("AttemptedFallbacks = %d, want %d", usage.AttemptedFallbacks, tc.wantAttempted)
			}
			if usage.Substituted != tc.wantSubstituted {
				t.Errorf("Substituted = %v, want %v", usage.Substituted, tc.wantSubstituted)
			}
			if usage.CostUSD != 3.615e-05 {
				t.Errorf("CostUSD = %v, want the header value 3.615e-05 when the body has no usage block", usage.CostUSD)
			}
		})
	}
}

func TestGatewayBodyCostWinsOverHeader(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-litellm-response-cost", "0.5")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}],"usage":{"cost":0.0042}}`))
	})
	ctx, usage := domain.WithUsageCapture(context.Background())
	if _, err := p.Complete(ctx, "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if usage.CostUSD != 0.0042 {
		t.Errorf("CostUSD = %v, want the body's usage.cost 0.0042", usage.CostUSD)
	}
}

func TestGatewayUsageCaptureIsOptional(t *testing.T) {
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"}}],"usage":{"cost":0.01}}`))
	})
	if _, err := p.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete without capture sink: %v", err)
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
	srv.Close()
	p, err := New(url, "key", wantEmbedDims)
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

func TestServedModelPrefersHeaderOverBody(t *testing.T) {
	h := http.Header{}
	h.Set("x-litellm-model-name", "openrouter/anthropic/claude-haiku-4.5")
	got := servedModel(h, chatResponse{Model: "match"})
	if got != "openrouter/anthropic/claude-haiku-4.5" {
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

func TestClientTimeoutExceedsProxyWorstCaseChain(t *testing.T) {
	const proxyWorstCase = 600 * time.Second

	g, err := New("http://gateway.invalid", "key", wantEmbedDims)
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
