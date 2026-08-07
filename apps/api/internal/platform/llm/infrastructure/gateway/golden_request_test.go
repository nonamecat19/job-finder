package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// This file is the behavioural baseline for 037 (SC-001, contracts C1-2).
//
// `Complete` and `CompleteJSON` become shims onto `CompleteChat`. Fourteen
// structured call sites depend on their current wire output, and the way a shim
// breaks them is not by failing — it is by quietly normalising a difference
// between the two entry points that was load-bearing. These goldens exist to
// make that impossible to do silently: they were captured against the
// unmodified adapter and must be byte-identical afterwards.
//
// A failure here after the shim lands is a defect in the shim, never a stale
// golden. Do not re-record.

// assertGolden compares the captured body against an expected object, field by
// field, and — critically — also asserts the *key set*, so a shim that adds
// `tools: null` or `tool_choice: ""` fails rather than passing on the fields
// somebody thought to check.
func assertGolden(t *testing.T, got map[string]any, want map[string]any) {
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

func TestGoldenCompleteWithoutSystemPrompt(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hello", nil)
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		// C1-2 row 2: Complete never sets response_format. C2-1: no tools key.
		"temperature": 0.3,
	})
}

func TestGoldenCompleteWithSystemPrompt(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hello", &domain.CompleteOptions{System: "be terse"})
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "system", "content": "be terse"},
			map[string]any{"role": "user", "content": "hello"},
		},
		"temperature": 0.3,
	})
}

func TestGoldenCompleteJSONNonStrict(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "give me json", &domain.CompleteOptions{})
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "give me json"},
		},
		"temperature":     0.1,
		"response_format": map[string]any{"type": "json_object"},
	})
}

// C1-2 row 2, the nil-options case: CompleteJSON sets response_format even when
// there are no options at all. cmd/llmsmoke calls exactly this way, so the nil
// path is live, not hypothetical (row 7).
func TestGoldenCompleteJSONWithNilOptions(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "give me json", nil)
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "give me json"},
		},
		"temperature":     0.1,
		"response_format": map[string]any{"type": "json_object"},
	})
}

func TestGoldenCompleteJSONStrict(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "give me json", &domain.CompleteOptions{
			ResponseMode: domain.ResponseModeStrict,
			JSONSchema:   `{"type":"object","properties":{"a":{"type":"string"}},"required":["a"],"additionalProperties":false}`,
		})
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "give me json"},
		},
		"temperature": 0.1,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "tailored_output",
				"strict": true,
				"schema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"a": map[string]any{"type": "string"}},
					"required":             []any{"a"},
					"additionalProperties": false,
				},
			},
		},
	})
}

// C1-2 row 5: an unparseable schema downgrades to json_object rather than
// sending a broken json_schema — and, per the retry test, skips the retry.
func TestGoldenCompleteJSONStrictWithUnparseableSchema(t *testing.T) {
	body := captureBody(t, func(p *Provider) error {
		_, err := p.CompleteJSON(context.Background(), "give me json", &domain.CompleteOptions{
			ResponseMode: domain.ResponseModeStrict,
			JSONSchema:   "{not json",
		})
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "give me json"},
		},
		"temperature":     0.1,
		"response_format": map[string]any{"type": "json_object"},
	})
}

func TestGoldenMaxTokensBecomesMaxCompletionTokens(t *testing.T) {
	limit := 512
	body := captureBody(t, func(p *Provider) error {
		_, err := p.Complete(context.Background(), "hello", &domain.CompleteOptions{MaxTokens: &limit})
		return err
	})
	assertGolden(t, body, map[string]any{
		"model":  "gateway",
		"stream": false,
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
		},
		"temperature":           0.3,
		"max_completion_tokens": float64(512),
	})
}

// C2-1, stated on its own rather than left implicit in the key-set assertions
// above: declaring no tools must leave `tools` and `tool_choice` **absent**.
// `"tools": null` or `"tools": []` is a different request, and a proxy that
// receives one is not receiving what it received yesterday.
func TestToolKeysAbsentWhenNoToolsDeclared(t *testing.T) {
	for name, opts := range map[string]*domain.CompleteOptions{
		"nil opts":                      nil,
		"empty opts":                    {},
		"tool choice set without tools": {ToolChoice: "required"},
	} {
		t.Run(name, func(t *testing.T) {
			body := captureBody(t, func(p *Provider) error {
				_, err := p.Complete(context.Background(), "hello", opts)
				return err
			})
			for _, key := range []string{"tools", "tool_choice"} {
				if _, present := body[key]; present {
					t.Errorf("key %q present (%v) with no tools declared; it must be absent", key, body[key])
				}
			}
		})
	}
}
