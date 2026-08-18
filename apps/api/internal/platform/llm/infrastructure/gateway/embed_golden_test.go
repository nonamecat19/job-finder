package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func captureEmbedBody(t *testing.T, text string) map[string]any {
	t.Helper()
	var raw []byte
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("request path = %q, want /embeddings", r.URL.Path)
		}
		hit = true
		b, _ := io.ReadAll(r.Body)
		raw = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"embed","data":[{"index":0,"embedding":[0.1,0.2,0.3]}],` +
			`"usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	t.Cleanup(srv.Close)

	p, err := New(srv.URL, "sk-test-master", wantEmbedDims)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Embed(context.Background(), text); err != nil {
		t.Logf("Embed returned an error: %v", err)
	}
	if !hit {
		t.Fatal("POST /embeddings was never called")
	}

	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body %q: %v", raw, err)
	}
	return body
}

func TestEmbedGoldenRequestBodyKeySet(t *testing.T) {
	body := captureEmbedBody(t, "senior go engineer, remote, distributed systems")
	assertGolden(t, body, map[string]any{
		"model": "embed",
		"input": []any{"senior go engineer, remote, distributed systems"},
	})
}

func TestEmbedGoldenRequestTruncatesTo8000Chars(t *testing.T) {
	long := make([]byte, 9000)
	for i := range long {
		long[i] = 'a'
	}
	body := captureEmbedBody(t, string(long))
	input, ok := body["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %v, want a one-element array", body["input"])
	}
	got, ok := input[0].(string)
	if !ok {
		t.Fatalf("input[0] = %v (%T), want string", input[0], input[0])
	}
	if len(got) > 8000 {
		t.Errorf("input[0] length = %d, want <= 8000 (E1-3 backstop truncation)", len(got))
	}
}
