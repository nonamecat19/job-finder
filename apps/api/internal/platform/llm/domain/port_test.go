package domain

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/invopop/jsonschema"
)

type fakeProvider struct {
	responses []string
	calls     int
	prompts   []string
}

func (f *fakeProvider) ModelName() string { return "fake" }
func (f *fakeProvider) Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	return "", nil
}
func (f *fakeProvider) CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.calls >= len(f.responses) {
		return "", errors.New("no more scripted responses")
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

// CompleteChat satisfies the 037 Provider interface. The fake's behaviour lives
// in CompleteJSON, so this delegates to it with the final turn as the prompt —
// which is what the real adapters do in reverse. Tool calls are never
// fabricated here: a fake that invented one would make a tool-loop test pass
// for the wrong reason.
func (f *fakeProvider) CompleteChat(ctx context.Context, msgs []Message, opts *CompleteOptions) (ChatResult, error) {
	prompt := ""
	if len(msgs) > 0 {
		prompt = msgs[len(msgs)-1].Content
	}
	text, err := f.CompleteJSON(ctx, prompt, opts)
	return ChatResult{Content: text}, err
}
func (f *fakeProvider) Embed(ctx context.Context, text string) ([]float32, error) { return nil, nil }

type fitResult struct {
	Score   int      `json:"score"`
	Summary string   `json:"summary"`
	Skills  []string `json:"skills"`
}

func (f *fitResult) Validate() error {
	if f.Score < 0 || f.Score > 100 {
		return errors.New("score must be between 0 and 100")
	}
	return nil
}

func TestCompleteStructured_MalformedThenValid(t *testing.T) {
	p := &fakeProvider{responses: []string{
		"not json at all",
		"```json\n{\"score\": 85, \"summary\": \"solid fit\", \"skills\": [\"go\"]}\n```",
	}}
	got, err := CompleteStructured[fitResult](context.Background(), p, "rate this candidate", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Score != 85 || got.Summary != "solid fit" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if p.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", p.calls)
	}
	if !strings.Contains(p.prompts[1], "previous answer was invalid") {
		t.Fatalf("retry prompt missing validation-error hint: %s", p.prompts[1])
	}
}

func TestCompleteStructured_FailsAfterMaxRetries(t *testing.T) {
	p := &fakeProvider{responses: []string{"nope", "still nope", "nope again"}}
	_, err := CompleteStructured[fitResult](context.Background(), p, "rate this candidate", nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if p.calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", p.calls)
	}
}

func TestCompleteStructured_SemanticValidationRetries(t *testing.T) {
	p := &fakeProvider{responses: []string{
		`{"score": 150, "summary": "x", "skills": []}`,
		`{"score": 50, "summary": "x", "skills": []}`,
	}}
	got, err := CompleteStructured[fitResult](context.Background(), p, "rate", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Score != 50 {
		t.Fatalf("expected corrected score 50, got %d", got.Score)
	}
}

func TestStripFences(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"a\":1}\n```": `{"a":1}`,
		"```\n{\"a\":1}\n```":     `{"a":1}`,
		`{"a":1}`:                 `{"a":1}`,
	}
	for in, want := range cases {
		if got := stripFences(in); got != want {
			t.Errorf("stripFences(%q) = %q, want %q", in, got, want)
		}
	}
}

type strictItem struct {
	Name  string `json:"name"`
	Score int    `json:"score,omitempty"`
}

type strictSample struct {
	Title string       `json:"title"`
	Notes string       `json:"notes,omitempty"`
	Items []strictItem `json:"items"`
}

// nodeAt walks doc down a slash-separated path where "[]" descends into an
// array's item schema and any other segment descends into a named property.
func nodeAt(t *testing.T, doc map[string]any, path string) map[string]any {
	t.Helper()
	node := doc
	if path == "" {
		return node
	}
	for _, seg := range strings.Split(path, "/") {
		var next any
		if seg == "[]" {
			next = node["items"]
		} else {
			props, ok := node["properties"].(map[string]any)
			if !ok {
				t.Fatalf("node %q has no properties while resolving %q", seg, path)
			}
			next = props[seg]
		}
		m, ok := next.(map[string]any)
		if !ok {
			t.Fatalf("segment %q of %q is not an object: %v", seg, path, next)
		}
		node = m
	}
	return node
}

func TestStrictifySchema(t *testing.T) {
	r := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true}
	rawBytes, err := json.Marshal(r.ReflectFromType(reflect.TypeOf(strictSample{})))
	if err != nil {
		t.Fatalf("marshal reflected schema: %v", err)
	}
	strict, err := strictifySchema(string(rawBytes))
	if err != nil {
		t.Fatalf("strictifySchema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(strict), &doc); err != nil {
		t.Fatalf("strictified schema is not JSON: %v", err)
	}

	if _, present := doc["$schema"]; present {
		t.Error("$schema survived strictify; the strict dialect rejects it")
	}
	if _, present := doc["$id"]; present {
		t.Error("$id survived strictify; the strict dialect rejects it")
	}

	cases := []struct {
		name     string
		path     string
		required []string
		nullable []string
	}{
		{
			name:     "root object lists every property as required",
			path:     "",
			required: []string{"items", "notes", "title"},
			nullable: []string{"notes"},
		},
		{
			name:     "nested array item object is strictified too",
			path:     "items/[]",
			required: []string{"name", "score"},
			nullable: []string{"score"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := nodeAt(t, doc, tc.path)

			got := []string{}
			list, ok := node["required"].([]any)
			if !ok {
				t.Fatalf("required is not a list: %v", node["required"])
			}
			for _, v := range list {
				got = append(got, v.(string))
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tc.required, ",") {
				t.Errorf("required = %v, want %v", got, tc.required)
			}

			props, _ := node["properties"].(map[string]any)
			if len(props) != len(tc.required) {
				t.Errorf("property count = %d, want %d — required must name every property", len(props), len(tc.required))
			}

			if ap, ok := node["additionalProperties"].(bool); !ok || ap {
				t.Errorf("additionalProperties = %v, want false", node["additionalProperties"])
			}

			for _, name := range tc.nullable {
				child, ok := props[name].(map[string]any)
				if !ok {
					t.Fatalf("property %q missing", name)
				}
				types, ok := child["type"].([]any)
				if !ok {
					t.Fatalf("optional property %q type = %v, want a list widened with null", name, child["type"])
				}
				var hasNull bool
				for _, v := range types {
					if v == "null" {
						hasNull = true
					}
				}
				if !hasNull {
					t.Errorf("optional property %q type = %v, want null included", name, types)
				}
			}
			for name := range props {
				if slices.Contains(tc.nullable, name) {
					continue
				}
				child := props[name].(map[string]any)
				if _, widened := child["type"].([]any); widened {
					t.Errorf("required property %q was widened to %v; only omitempty fields become nullable", name, child["type"])
				}
			}
		})
	}
}

func TestUsageCapture(t *testing.T) {
	ctx, usage := WithUsageCapture(context.Background())
	ReportUsage(ctx, Usage{CostUSD: 0.0042, PromptTokens: 1820, CompletionTokens: 355})
	want := Usage{CostUSD: 0.0042, PromptTokens: 1820, CompletionTokens: 355}
	if *usage != want {
		t.Errorf("captured usage = %+v, want %+v", *usage, want)
	}
}

func TestReportUsageWithoutSinkIsNoop(t *testing.T) {
	// Providers report unconditionally; a caller that never opted in must not panic.
	ReportUsage(context.Background(), Usage{CostUSD: 1})
}
