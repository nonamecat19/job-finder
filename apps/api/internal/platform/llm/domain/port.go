// Package domain defines the LLM Provider abstraction and the shared
// structured-output retry loop used by matching, generation and profile
// import. Mirrors apps/api/src/modules/llm/*.
package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/invopop/jsonschema"
)

var ErrInvalidResponse = errors.New("llm: structured output failed after all retries")

// ResponseMode controls how the provider constrains structured output.
// The zero value (ResponseModeJSON) preserves the pre-033 behaviour: the
// request carries response_format {"type":"json_object"} and the model is
// trusted to produce JSON, with CompleteStructured's parse-and-retry loop
// catching malformed output. ResponseModeStrict sends a strict JSON Schema
// (response_format {"type":"json_schema","json_schema":{...,"strict":true}})
// so the provider rejects extra fields at the API level rather than relying
// on prose alone — the first line of defense against fabrication (033 FR-005).
type ResponseMode int

const (
	// ResponseModeJSON is the legacy mode: json_object, no schema enforcement.
	// Every existing caller that does not set ResponseMode gets this.
	ResponseModeJSON ResponseMode = iota
	// ResponseModeStrict sends a strict JSON Schema derived from the target
	// type so the model cannot emit unexpected fields. Only meaningful for
	// CompleteJSON (structured calls); ignored by Complete (plain text).
	ResponseModeStrict
)

// CompleteOptions mirrors llm.types.ts CompleteOptions. Temperature/MaxTokens
// are pointers so "unset" (use provider default) is distinguishable from an
// explicit zero, matching the TS `opts?: { temperature?: number }` semantics.
type CompleteOptions struct {
	System      string
	Temperature *float64
	MaxTokens   *int
	// Model overrides the provider's default model for this call (per-task
	// model selection). Empty uses the provider default.
	Model string
	// ResponseMode controls structured-output strictness (033). Zero value
	// (ResponseModeJSON) is backward-compatible — existing callers that do
	// not set it keep the json_object behaviour exactly.
	ResponseMode ResponseMode
	// JSONSchema carries the marshalled JSON Schema string for
	// ResponseModeStrict calls, set by CompleteStructured so the provider
	// adapter can embed it in the request without re-generating it. Empty
	// for ResponseModeJSON calls.
	JSONSchema string
}

// ModelOr returns the per-call model override, or def if opts is nil/unset.
func (o *CompleteOptions) ModelOr(def string) string {
	if o != nil && o.Model != "" {
		return o.Model
	}
	return def
}

// Temp resolves the effective temperature: explicit value if set, else def.
func (o *CompleteOptions) Temp(def float64) float64 {
	if o != nil && o.Temperature != nil {
		return *o.Temperature
	}
	return def
}

// System returns the configured system prompt, or "" if opts is nil.
func (o *CompleteOptions) SystemPrompt() string {
	if o == nil {
		return ""
	}
	return o.System
}

type servedModelKey struct{}

// WithServedModelCapture returns a context a Provider can use to report which
// model actually served a call (e.g. the gateway's LiteLLM fallback chain may
// serve a different model than the requested task-routing group). Callers
// that need to know the real served model — not just the task key they asked
// for — pass this context in, then read *ptr after the call returns.
func WithServedModelCapture(ctx context.Context) (context.Context, *string) {
	ptr := new(string)
	return context.WithValue(ctx, servedModelKey{}, ptr), ptr
}

// ReportServedModel writes model into the capture pointer stashed in ctx by
// WithServedModelCapture, if any. Providers call this after a successful
// request; a no-op when the caller didn't ask to capture it.
func ReportServedModel(ctx context.Context, model string) {
	if ptr, ok := ctx.Value(servedModelKey{}).(*string); ok {
		*ptr = model
	}
}

// Usage is the measured economics of a single provider call. CostUSD is the
// provider's own reported figure — measured, not estimated — and stays 0 when
// the deployment does not price the call (the local Ollama model, for instance).
type Usage struct {
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int
	// ServedGroup is the deployment tier that actually served the call, which
	// differs from the requested task key once the fallback chain advances.
	// It is a group name, not a model identity: the application still learns
	// nothing about providers (030-FR-004).
	ServedGroup string
	// AttemptedFallbacks is how far down the chain the proxy had to go, 0 when
	// tier 1 served. Substituted is the derived signal the dashboard marker
	// keys off (035 FR-012).
	AttemptedFallbacks int
	Substituted        bool
}

type usageKey struct{}

// WithUsageCapture is the cost-and-token counterpart of
// WithServedModelCapture: callers that need the economics of a call pass the
// returned context in, then read *ptr after the call returns.
func WithUsageCapture(ctx context.Context) (context.Context, *Usage) {
	ptr := new(Usage)
	return context.WithValue(ctx, usageKey{}, ptr), ptr
}

// ReportUsage writes u into the capture pointer stashed in ctx by
// WithUsageCapture, if any. A no-op when the caller didn't ask to capture it.
func ReportUsage(ctx context.Context, u Usage) {
	if ptr, ok := ctx.Value(usageKey{}).(*Usage); ok {
		*ptr = u
	}
}

// Provider is the interface the infrastructure/ollama and
// infrastructure/cerebras adapters implement. CompleteJSON is the low-level
// "ask for JSON, no retry" call; the retry loop (strip fences → parse →
// validate → retry with error) lives in CompleteStructured below.
type Provider interface {
	ModelName() string
	Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error)
	CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error)
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Validator is an optional interface structured-output target types can
// implement to add semantic validation beyond JSON structural typing
// (e.g. zod's `.min(0).max(100)` in matching.service.ts's fitSchema).
type Validator interface {
	Validate() error
}

const structuredRetries = 2 // max 2 EXTRA attempts after the first, same as cerebras.provider.ts

var fenceRe = regexp.MustCompile("(?s)^```(?:json)?\\s*(.*?)\\s*```$")

// stripFences removes a surrounding ```json ... ``` fence, same regex shape
// as the TS `stripFences` private method on both providers.
func stripFences(text string) string {
	t := strings.TrimSpace(text)
	if m := fenceRe.FindStringSubmatch(t); m != nil {
		return m[1]
	}
	return t
}

var schemaCache sync.Map // reflect.Type -> string

func schemaFor(t reflect.Type) string {
	if cached, ok := schemaCache.Load(t); ok {
		return cached.(string)
	}
	r := &jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true}
	schema := r.ReflectFromType(t)
	b, err := json.Marshal(schema)
	s := ""
	if err == nil {
		s = string(b)
	}
	if strict, serr := strictifySchema(s); serr == nil {
		s = strict
	}
	schemaCache.Store(t, s)
	return s
}

// strictifySchema rewrites a reflected JSON Schema into the dialect strict
// structured-output mode accepts. Providers that validate the schema (OpenAI
// and Azure behind OpenRouter) reject anything looser with a 400: every
// object must list *every* one of its properties in `required` and must set
// `additionalProperties: false`. A field the Go type marks omitempty is
// therefore made required-but-nullable instead, which unmarshals back to the
// zero value. Meta keywords ($schema/$id) are dropped — they are not part of
// the accepted dialect.
func strictifySchema(raw string) (string, error) {
	if raw == "" {
		return raw, nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return raw, err
	}
	strictifyNode(doc)
	delete(doc, "$schema")
	delete(doc, "$id")
	b, err := json.Marshal(doc)
	if err != nil {
		return raw, err
	}
	return string(b), nil
}

func strictifyNode(node map[string]any) {
	props, ok := node["properties"].(map[string]any)
	if ok {
		required := map[string]bool{}
		if existing, isList := node["required"].([]any); isList {
			for _, name := range existing {
				if s, isStr := name.(string); isStr {
					required[s] = true
				}
			}
		}
		names := make([]string, 0, len(props))
		for name, child := range props {
			names = append(names, name)
			childMap, isMap := child.(map[string]any)
			if !isMap {
				continue
			}
			if !required[name] {
				makeNullable(childMap)
			}
			strictifyNode(childMap)
		}
		sort.Strings(names)
		all := make([]any, len(names))
		for i, name := range names {
			all[i] = name
		}
		node["required"] = all
		node["additionalProperties"] = false
	}
	if items, isMap := node["items"].(map[string]any); isMap {
		strictifyNode(items)
	}
}

// makeNullable widens a property's declared type to include null, so a field
// the model has nothing to say about can be omitted in spirit while still
// satisfying the "every property is required" rule.
func makeNullable(node map[string]any) {
	switch t := node["type"].(type) {
	case string:
		if t != "null" {
			node["type"] = []any{t, "null"}
		}
	case []any:
		for _, v := range t {
			if s, ok := v.(string); ok && s == "null" {
				return
			}
		}
		node["type"] = append(t, "null")
	}
}

// CompleteStructured is the Go equivalent of `completeStructured<T>`: builds
// the schema-in-prompt request, parses+validates the JSON response, and on
// failure appends the validation error to the prompt and retries (max 2
// extra attempts), matching cerebras.provider.ts / ollama.provider.ts.
func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	var zero T
	schema := schemaFor(reflect.TypeOf(zero))

	if opts == nil {
		opts = &CompleteOptions{}
	}
	// When the caller opts into strict mode, attach the schema so the
	// provider adapter can embed it in response_format.json_schema. A
	// caller that leaves the zero value (ResponseModeJSON) keeps the
	// legacy behaviour: the schema is still appended to the prompt as
	// text below, but the wire request stays json_object.
	if opts.ResponseMode == ResponseModeStrict && opts.JSONSchema == "" {
		opts.JSONSchema = schema
	}

	lastErr := ""
	for attempt := 0; attempt <= structuredRetries; attempt++ {
		full := prompt + "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema
		if lastErr != "" {
			full += "\nYour previous answer was invalid: " + lastErr + "\nFix it and answer again with valid JSON only."
		}

		text, err := p.CompleteJSON(ctx, full, opts)
		if err != nil {
			// Provider-level failures (rate limit, bad credential, model
			// gone, provider down) are already classified and are not
			// fixable by re-prompting, so they propagate immediately —
			// only malformed *content* is worth another attempt.
			return zero, err
		}

		var result T
		if err := json.Unmarshal([]byte(stripFences(text)), &result); err != nil {
			lastErr = fmt.Sprintf("not valid JSON: %s", err.Error())
			continue
		}
		if v, ok := any(&result).(Validator); ok {
			if verr := v.Validate(); verr != nil {
				lastErr = verr.Error()
				continue
			}
		}
		return result, nil
	}
	return zero, fmt.Errorf("%w: structured output failed after %d attempts: %s", ErrInvalidResponse, structuredRetries+1, lastErr)
}
