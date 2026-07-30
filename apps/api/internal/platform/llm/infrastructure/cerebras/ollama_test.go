package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func newTestOllama(t *testing.T, handler http.HandlerFunc) *OllamaProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewOllama(srv.URL, "", "", "", "")
}

func TestOllamaChat_KeepAliveSetWhenConfigured(t *testing.T) {
	var got ollamaChatRequest
	p := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{Message: struct {
			Content string `json:"content"`
		}{Content: "hi"}})
	})
	p.keepAlive = "30m"

	if _, err := p.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got.KeepAlive != "30m" {
		t.Errorf("keep_alive = %q, want 30m", got.KeepAlive)
	}
}

func TestOllamaChat_KeepAliveOmittedWhenEmpty(t *testing.T) {
	var raw map[string]any
	p := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&raw)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{Message: struct {
			Content string `json:"content"`
		}{Content: "hi"}})
	})
	// p.keepAlive left empty

	if _, err := p.Complete(context.Background(), "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, present := raw["keep_alive"]; present {
		t.Errorf("keep_alive should be omitted entirely, got %v", raw["keep_alive"])
	}
}

func TestOllamaEmbed_KeepAliveSetWhenConfigured(t *testing.T) {
	var got ollamaEmbedRequest
	p := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_ = json.NewEncoder(w).Encode(ollamaEmbedResponse{Embedding: []float32{1, 2}})
	})
	p.keepAlive = "30m"

	if _, err := p.Embed(context.Background(), "text"); err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got.KeepAlive != "30m" {
		t.Errorf("keep_alive = %q, want 30m", got.KeepAlive)
	}
}

// TestOllamaChat_ConcurrentRateLimitAllBackOff exercises FR-015/Scenario 7:
// three concurrent calls hitting a 429+Retry-After all trip the shared
// breaker and surface a retryable error, none permanent.
func TestOllamaChat_ConcurrentRateLimitAllBackOff(t *testing.T) {
	var hits int32
	p := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	})

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := p.Complete(context.Background(), "hi", nil)
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err == nil {
			t.Errorf("call %d: expected error, got nil", i)
			continue
		}
		// Rate limiting is deliberately not permanent (Terminal == false):
		// the quota can refill later, unlike a bad credential or model.
		if Terminal(err) {
			t.Errorf("call %d: expected non-terminal error, got terminal: %v", i, err)
		}
	}
	if !p.breaker.tripped() {
		t.Error("expected breaker to be tripped after 429")
	}
}

func TestOllamaProvider_IsHosted(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
		want    bool
	}{
		{"local no key", "http://localhost:11434", "", false},
		{"loopback IP no key", "http://127.0.0.1:11434", "", false},
		{"loopback IP with key", "http://127.0.0.1:11434", "some-key", true},
		{"private network no key", "http://192.168.1.50:11434", "", false},
		{"cloud URL no key", "https://ollama.com", "", true},
		{"cloud URL with key", "https://ollama.com", "cloud-key", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOllama(tt.baseURL, tt.apiKey, "", "", "")
			if got := p.IsHosted(); got != tt.want {
				t.Errorf("IsHosted() = %v, want %v", got, tt.want)
			}
		})
	}
}
