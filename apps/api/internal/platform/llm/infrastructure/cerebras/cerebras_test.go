package cerebras

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
	"github.com/job-finder/api/internal/platform/llm/infrastructure/ollama"
)

func newTestCerebras(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := New(srv.URL, "test-key", "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestNewCerebrasRequiresAPIKey(t *testing.T) {
	if _, err := New("", "", "", nil); err == nil {
		t.Fatal("expected error when apiKey is empty")
	}
}

func TestNewCerebrasDefaults(t *testing.T) {
	p, err := New("", "key", "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.baseURL != "https://api.cerebras.ai/v1" {
		t.Errorf("baseURL default = %q", p.baseURL)
	}
	if p.ModelName() != DefaultModel {
		t.Errorf("ModelName() = %q, want %q", p.ModelName(), DefaultModel)
	}
}

func TestCerebrasCompleteRequestShape(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody chatRequest
	p := newTestCerebras(t, func(w http.ResponseWriter, r *http.Request) {
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

	out, err := p.Complete(context.Background(), "hi", &domain.CompleteOptions{Model: "llama-3.3-70b"})
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
	if gotBody.Model != "llama-3.3-70b" {
		t.Errorf("request model = %q, want llama-3.3-70b (per-call override)", gotBody.Model)
	}
}

func TestCerebrasCompleteJSONSetsResponseFormat(t *testing.T) {
	var gotBody chatRequest
	p := newTestCerebras(t, func(w http.ResponseWriter, r *http.Request) {
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
	if _, err := p.CompleteJSON(context.Background(), "hi", nil); err != nil {
		t.Fatalf("CompleteJSON: %v", err)
	}
	if gotBody.ResponseFormat["type"] != "json_object" {
		t.Errorf("response_format = %+v, want json_object", gotBody.ResponseFormat)
	}
}

func TestCerebrasCompleteEmptyChoices(t *testing.T) {
	p := newTestCerebras(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(chatResponse{})
	})
	if _, err := p.Complete(context.Background(), "hi", nil); err == nil {
		t.Fatal("expected error on empty choices")
	}
}

func TestCerebrasErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, `{"error":{"message":"invalid api key"}}`, "credential rejected"},
		{"forbidden", http.StatusForbidden, `{"error":{"message":"blocked"}}`, "credential rejected"},
		{"rate limited", http.StatusTooManyRequests, `{"error":{"message":"quota exceeded"}}`, "rate limit or quota exceeded"},
		{"other", http.StatusInternalServerError, `{"error":{"message":"boom"}}`, "returned 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestCerebras(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestCerebrasEmbedDelegatesToOllama(t *testing.T) {
	var embedHit bool
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		embedHit = true
		_ = json.NewEncoder(w).Encode(map[string]any{"embedding": []float32{0.1, 0.2}})
	}))
	t.Cleanup(ollamaSrv.Close)
	ollamaProvider := ollama.New(ollamaSrv.URL, "", "", "", "")

	p, err := New("https://unused.example", "key", "", ollamaProvider)
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
