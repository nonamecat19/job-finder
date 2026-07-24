// Package llm defines the LLM Provider abstraction (Ollama) and the shared
// structured-output retry loop used by matching, generation and profile
// import. Mirrors apps/api/src/modules/llm/*.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/invopop/jsonschema"
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

// Provider is the interface ollama.go implements. CompleteJSON is the
// low-level "ask for JSON, no retry" call; the retry loop (strip fences →
// parse → validate → retry with error) lives in CompleteStructured below.
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
	schemaCache.Store(t, s)
	return s
}

// CompleteStructured is the Go equivalent of `completeStructured<T>`: builds
// the schema-in-prompt request, parses+validates the JSON response, and on
// failure appends the validation error to the prompt and retries (max 2
// extra attempts), matching cerebras.provider.ts / ollama.provider.ts.
func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	var zero T
	schema := schemaFor(reflect.TypeOf(zero))

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
