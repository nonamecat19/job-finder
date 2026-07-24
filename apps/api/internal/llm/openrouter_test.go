package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestOpenRouter(t *testing.T, handler http.HandlerFunc) *OpenRouterProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := NewOpenRouter(srv.URL, "test-key", "", "https://job.finder.test", "job-finder", nil)
	if err != nil {
		t.Fatalf("NewOpenRouter: %v", err)
	}
	return p
}

// openRouterChoices builds the awkward anonymous-struct choices slice of
// openRouterResponse for a single assistant message.
func openRouterChoices(content string) []struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
} {
	return []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{Content: content}}}
}

func TestNewOpenRouterRequiresAPIKey(t *testing.T) {
	if _, err := NewOpenRouter("", "", "", "", "", nil); err == nil {
		t.Fatal("expected error when apiKey is empty")
	}
}

func TestNewOpenRouterDefaults(t *testing.T) {
	p, err := NewOpenRouter("", "key", "", "", "", nil)
	if err != nil {
		t.Fatalf("NewOpenRouter: %v", err)
	}
	if p.baseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("baseURL default = %q", p.baseURL)
	}
	if p.ModelName() != DefaultOpenRouterModel {
		t.Errorf("ModelName() = %q, want %q", p.ModelName(), DefaultOpenRouterModel)
	}
}

func TestOpenRouterCompleteRequestShape(t *testing.T) {
	var gotPath, gotAuth, gotReferer, gotTitle string
	var gotBody openRouterRequest
	p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(openRouterResponse{Choices: openRouterChoices("hello")})
	})

	out, err := p.Complete(context.Background(), "hi", &CompleteOptions{Model: "deepseek/deepseek-r1:free", System: "be brief"})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if out != "hello" {
		t.Errorf("Complete() = %q, want hello", out)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotReferer != "https://job.finder.test" || gotTitle != "job-finder" {
		t.Errorf("attribution headers = %q / %q", gotReferer, gotTitle)
	}
	if gotBody.Model != "deepseek/deepseek-r1:free" {
		t.Errorf("request model = %q, want the per-call override", gotBody.Model)
	}
	if len(gotBody.Messages) != 2 || gotBody.Messages[0].Role != "system" {
		t.Errorf("messages = %+v, want system+user", gotBody.Messages)
	}
}

func TestOpenRouterCompleteJSONSetsResponseFormat(t *testing.T) {
	var gotBody openRouterRequest
	p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(openRouterResponse{Choices: openRouterChoices("{}")})
	})
	if _, err := p.CompleteJSON(context.Background(), "hi", nil); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if gotBody.ResponseFormat["type"] != "json_object" {
		t.Errorf("response_format = %+v, want json_object", gotBody.ResponseFormat)
	}
	if gotBody.Model != DefaultOpenRouterModel {
		t.Errorf("request model = %q, want provider default", gotBody.Model)
	}
}

func TestOpenRouterCompleteEmptyChoices(t *testing.T) {
	p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openRouterResponse{})
	})
	if _, err := p.Complete(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected error on empty choices")
	}
}

func TestOpenRouterErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`, "credential rejected"},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"blocked"}}`, "credential rejected"},
		{"payment required", http.StatusPaymentRequired, `{"error":{"message":"no credits"}}`, "insufficient credits"},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"quota exceeded"}}`, "rate limit or quota exceeded"},
		{"other", http.StatusInternalServerError, `{"error":{"message":"boom"}}`, "returned 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := p.Complete(context.Background(), "hi", nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if strings.Contains(err.Error(), "test-key") {
				t.Error("error message must never contain the API key")
			}
		})
	}
}

func TestOpenRouterRateLimitTripsBreaker(t *testing.T) {
	var calls int
	p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded"}}`))
	})

	if _, err := p.Complete(context.Background(), "hi", nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("first call err = %v, want ErrRateLimited", err)
	}
	if _, err := p.Complete(context.Background(), "hi", nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second call err = %v, want ErrRateLimited", err)
	}
	if calls != 1 {
		t.Errorf("upstream calls = %d, want 1 (breaker should short-circuit the second)", calls)
	}
}

func TestOpenRouterRateLimitHonoursRetryAfter(t *testing.T) {
	p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"daily quota"}}`))
	})
	if _, err := p.Complete(context.Background(), "hi", nil); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
	// The breaker must hold for the advertised window, not the 60s default.
	held := time.Until(time.Unix(0, p.breaker.until.Load()))
	if held < 4*time.Minute {
		t.Errorf("breaker held for %v, want ~5m from Retry-After", held)
	}
}

func TestOpenRouterErrorClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrCredentialRejected},
		{"payment required", http.StatusPaymentRequired, ErrInsufficientCredits},
		{"unknown model", http.StatusNotFound, ErrModelUnavailable},
		{"upstream 5xx", http.StatusBadGateway, ErrProviderUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
			})
			_, err := p.Complete(context.Background(), "hi", nil)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestOpenRouterTransportFailureIsRetryable(t *testing.T) {
	p, err := NewOpenRouter("http://127.0.0.1:1", "key", "", "", "", nil)
	if err != nil {
		t.Fatalf("NewOpenRouter: %v", err)
	}
	_, err = p.Complete(context.Background(), "hi", nil)
	if !errors.Is(err, ErrProviderUnavailable) || !Retryable(err) {
		t.Errorf("err = %v, want a retryable ErrProviderUnavailable", err)
	}
}

// OpenRouter reports some upstream failures as HTTP 200 with an `error`
// object in the body; a 429 code there is the same exhausted-quota signal.
func TestOpenRouterErrorInBodyOn200(t *testing.T) {
	p := newTestOpenRouter(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"rate-limited upstream"}}`))
	})
	_, err := p.Complete(context.Background(), "hi", nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}

func TestOpenRouterEmbedDelegatesToOllama(t *testing.T) {
	var embedHit bool
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{0.1, 0.2}})
	}))
	t.Cleanup(ollamaSrv.Close)
	ollama := NewOllama(ollamaSrv.URL, "", "", "", "")

	p, err := NewOpenRouter("https://unused.example", "key", "", "", "", ollama)
	if err != nil {
		t.Fatalf("NewOpenRouter: %v", err)
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

func TestOpenRouterModelsCatalog(t *testing.T) {
	defaults := 0
	for _, m := range OpenRouterModels {
		if m.IsDefault {
			defaults++
			if m.ID != DefaultOpenRouterModel {
				t.Errorf("default entry = %q, want %q", m.ID, DefaultOpenRouterModel)
			}
		}
		if !IsSupportedOpenRouterModel(m.ID) {
			t.Errorf("catalog model %q is not reported as supported", m.ID)
		}
	}
	if defaults != 1 {
		t.Errorf("catalog has %d default entries, want exactly 1", defaults)
	}
	if !IsSupportedOpenRouterModel("") {
		t.Error(`"" must be supported (means "use the provider default")`)
	}
	if IsSupportedOpenRouterModel("gpt-oss-120b") {
		t.Error("a Cerebras model id must not validate as an OpenRouter model")
	}
}
