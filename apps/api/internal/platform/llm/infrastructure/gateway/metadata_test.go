package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

func captureBody(t *testing.T, call func(p *Provider) error) map[string]any {
	t.Helper()
	var raw []byte
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		raw = buf
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"{}"}}]}`))
	})
	if err := call(p); err != nil {
		t.Fatalf("call: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal request body %q: %v", raw, err)
	}
	return body
}

func TestMetadataAbsentWhenUnset(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts *domain.CompleteOptions
	}{
		{"nil opts", nil},
		{"empty opts", &domain.CompleteOptions{}},
		{"opts with unrelated fields", &domain.CompleteOptions{System: "sys", Model: "match"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := captureBody(t, func(p *Provider) error {
				_, err := p.Complete(context.Background(), "hi", tc.opts)
				return err
			})
			if _, present := body["metadata"]; present {
				t.Errorf("metadata key present with unset options; want absent. body=%v", body)
			}
		})
	}
}

func TestMetadataUsesExistingTraceID(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hi", &domain.CompleteOptions{
			TraceID: "run-123",
		})
		return err
	})

	md, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing or not an object: %v", body)
	}
	if _, wrong := md["trace_id"]; wrong {
		t.Error("metadata carries trace_id, which OVERWRITES the trace on every call; want existing_trace_id")
	}
	if got := md["existing_trace_id"]; got != "run-123" {
		t.Errorf("existing_trace_id = %v, want run-123", got)
	}
}

func TestMetadataCarriesTaskKeyAsGenerationName(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "hi", &domain.CompleteOptions{
			TaskKey: "generation-summary",
		})
		return err
	})

	md, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing or not an object: %v", body)
	}
	if got := md["generation_name"]; got != "generation-summary" {
		t.Errorf("generation_name = %v, want generation-summary", got)
	}
	tags, ok := md["tags"].([]any)
	if !ok || len(tags) != 1 || tags[0] != "generation-summary" {
		t.Errorf("tags = %v, want [generation-summary]", md["tags"])
	}
}

func TestMetadataCarriesNothingElse(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hi", &domain.CompleteOptions{
			TraceID: "run-123",
			TaskKey: "match",
		})
		return err
	})

	md := body["metadata"].(map[string]any)
	allowed := map[string]bool{"existing_trace_id": true, "generation_name": true, "tags": true}
	for k := range md {
		if !allowed[k] {
			t.Errorf("unexpected metadata key %q — metadata must carry only correlation and grouping", k)
		}
	}
}

func TestTraceAndTaskNeverReachTheMessages(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "summarise this", &domain.CompleteOptions{
			System:  "you are helpful",
			TraceID: "run-secret-123",
			TaskKey: "generation-summary",
		})
		return err
	})

	msgs, _ := json.Marshal(body["messages"])
	for _, leaked := range []string{"run-secret-123", "generation-summary"} {
		if bytesContain(msgs, leaked) {
			t.Errorf("%q leaked into messages: %s", leaked, msgs)
		}
	}
}

func bytesContain(b []byte, s string) bool {
	return len(s) > 0 && len(b) >= len(s) && stringIndex(string(b), s) >= 0
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestTraceAndTaskAccessorsAreNilSafe(t *testing.T) {
	var opts *domain.CompleteOptions
	if got := opts.Trace(); got != "" {
		t.Errorf("nil opts Trace() = %q, want empty", got)
	}
	if got := opts.Task(); got != "" {
		t.Errorf("nil opts Task() = %q, want empty", got)
	}
	empty := &domain.CompleteOptions{}
	if got := empty.Trace(); got != "" {
		t.Errorf("empty opts Trace() = %q, want empty", got)
	}
	if got := empty.Task(); got != "" {
		t.Errorf("empty opts Task() = %q, want empty", got)
	}
}

func TestTraceIDFromContext(t *testing.T) {
	ctx := domain.WithTraceID(context.Background(), "run-from-ctx")
	var raw []byte
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		raw = buf
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"{}"}}]}`))
	})
	if _, err := p.Complete(ctx, "hi", nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	md, ok := body["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing: %v", body)
	}
	if got := md["existing_trace_id"]; got != "run-from-ctx" {
		t.Errorf("existing_trace_id = %v, want run-from-ctx", got)
	}
}

func TestNoTraceIDOnPlainContext(t *testing.T) {
	if got := domain.TraceIDFrom(context.Background()); got != "" {
		t.Errorf("TraceIDFrom(plain ctx) = %q, want empty", got)
	}
}

func TestExplicitTraceIDOverridesContext(t *testing.T) {
	ctx := domain.WithTraceID(context.Background(), "ctx-run")
	body := captureBodyCtx(t, ctx, func(p *Provider) error {
		_, err := p.Complete(ctx, "hi", &domain.CompleteOptions{TraceID: "explicit-run"})
		return err
	})
	md := body["metadata"].(map[string]any)
	if got := md["existing_trace_id"]; got != "explicit-run" {
		t.Errorf("existing_trace_id = %v, want explicit-run", got)
	}
}

func captureBodyCtx(t *testing.T, ctx context.Context, call func(p *Provider) error) map[string]any {
	t.Helper()
	var raw []byte
	p := newTestGateway(t, func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		raw = buf
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"{}"}}]}`))
	})
	if err := call(p); err != nil {
		t.Fatalf("call: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body
}
