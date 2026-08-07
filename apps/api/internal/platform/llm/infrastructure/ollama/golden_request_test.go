package ollama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// The Ollama half of 037's behavioural baseline (SC-001, contracts C1-2 rows 1,
// 6, 7, C2-9).
//
// This adapter is where the highest-risk difference lives, and it is not the
// temperature split: `Complete` forwards MaxTokens as `options.num_predict`
// while `CompleteJSON` sends **no** num_predict at all, ever. A shared shim that
// "helpfully" forwards MaxTokens on both paths would cap every structured
// generation in the system — on the terminal tier every routing chain ends at,
// which is exactly where a regression is least visible and most damaging.
//
// A gateway-only golden test could not have seen it. That is why this file
// exists.

func captureOllamaBody(t *testing.T, fn func(p *Provider) error) map[string]any {
	t.Helper()
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"ok"}}`))
	}))
	t.Cleanup(srv.Close)

	p := New(srv.URL, "", "test-model", "", "")
	if err := fn(p); err != nil {
		t.Fatalf("call: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("request body is not JSON: %v\n%s", err, raw)
	}
	return body
}

func assertOllamaGolden(t *testing.T, got, want map[string]any) {
	t.Helper()
	for k, wantV := range want {
		gotV, ok := got[k]
		if !ok {
			t.Errorf("key %q missing from request body", k)
			continue
		}
		wj, _ := json.Marshal(wantV)
		gj, _ := json.Marshal(gotV)
		if string(wj) != string(gj) {
			t.Errorf("key %q = %s, want %s", k, gj, wj)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected key %q in request body (value %v); the body must stay byte-identical", k, got[k])
		}
	}
}

// C1-2 row 1, first half, and row 6's negative: Complete forwards MaxTokens as
// num_predict and sets no `format`.
func TestGoldenOllamaCompleteSendsNumPredict(t *testing.T) {
	limit := 256
	body := captureOllamaBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hello", &domain.CompleteOptions{MaxTokens: &limit})
		return err
	})
	assertOllamaGolden(t, body, map[string]any{
		"model":  "test-model",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"options": map[string]any{"temperature": 0.3, "num_predict": float64(256)},
	})
}

// C1-2 row 1, second half — the load-bearing one. CompleteJSON must send NO
// num_predict even when MaxTokens is set, and must set format: "json" (row 6).
func TestGoldenOllamaCompleteJSONOmitsNumPredictAndSetsJSONFormat(t *testing.T) {
	limit := 256
	body := captureOllamaBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "give me json", &domain.CompleteOptions{MaxTokens: &limit})
		return err
	})
	assertOllamaGolden(t, body, map[string]any{
		"model":  "test-model",
		"stream": false,
		"format": "json",
		"messages": []any{
			map[string]any{"role": "user", "content": "give me json"},
		},
		"options": map[string]any{"temperature": 0.1},
	})
	if opts, ok := body["options"].(map[string]any); ok {
		if _, present := opts["num_predict"]; present {
			t.Error("CompleteJSON sent num_predict; it must never do so, on any path (C1-2 row 1)")
		}
	}
}

func TestGoldenOllamaCompleteWithSystemPrompt(t *testing.T) {
	body := captureOllamaBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hello", &domain.CompleteOptions{System: "be terse"})
		return err
	})
	assertOllamaGolden(t, body, map[string]any{
		"model":  "test-model",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "system", "content": "be terse"},
			map[string]any{"role": "user", "content": "hello"},
		},
		"options": map[string]any{"temperature": 0.3},
	})
}

// C1-2 row 7: cmd/llmsmoke calls with nil options, so this path is live.
func TestGoldenOllamaNilOptionsDoesNotPanic(t *testing.T) {
	body := captureOllamaBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hello", nil)
		return err
	})
	assertOllamaGolden(t, body, map[string]any{
		"model":  "test-model",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"options": map[string]any{"temperature": 0.3},
	})

	jsonBody := captureOllamaBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "hello", nil)
		return err
	})
	assertOllamaGolden(t, jsonBody, map[string]any{
		"model":  "test-model",
		"stream": false,
		"format": "json",
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"options": map[string]any{"temperature": 0.1},
	})
}

// C2-7: the same absent-when-empty rule on the local path.
func TestOllamaToolKeyAbsentWhenNoToolsDeclared(t *testing.T) {
	for name, opts := range map[string]*domain.CompleteOptions{
		"nil opts":                      nil,
		"empty opts":                    {},
		"tool choice set without tools": {ToolChoice: "required"},
	} {
		t.Run(name, func(t *testing.T) {
			body := captureOllamaBody(t, func(p *Provider) error {
				_, err := p.Complete(context.Background(), "hello", opts)
				return err
			})
			if _, present := body["tools"]; present {
				t.Errorf("key \"tools\" present (%v) with no tools declared; it must be absent", body["tools"])
			}
		})
	}
}
