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
	// TraceID groups this call with the other calls of the same logical run
	// (036). The value is the run's activity-run id, so an observability trace
	// cross-references the platform's own history without a lookup table.
	// Empty means uncorrelated — the pre-036 behaviour, and valid.
	TraceID string
	// TaskKey is the requested routing key, carried as request metadata so the
	// collector can group by task rather than by serving deployment (036
	// FR-012). Without it two stages served by the same model collapse into
	// one reporting bucket. Empty means ungrouped.
	TaskKey string
	// Tools declares the lookups available for this call (037). Empty means no
	// tool declaration is sent at all — the key must be absent from the wire
	// body, not null and not [], so every pre-037 caller's request stays
	// byte-identical.
	Tools []ToolDef
	// ToolChoice is "" (omit entirely, provider default stands), "auto",
	// "none" or "required". The loop sends "required" on its first round and
	// "auto" afterwards; that asymmetry is the whole of its ability to tell a
	// model that chose not to look anything up from a model whose tools array
	// was silently dropped in transit (FR-017).
	ToolChoice string
	// JSONOutput asks CompleteChat to constrain the answer to JSON, the way
	// CompleteJSON always has (037).
	//
	// It exists because the shims cannot be told apart any other way: the
	// difference between Complete and CompleteJSON is exactly "does this call
	// carry response_format", and ResponseMode's zero value already means
	// "json_object" for the structured path. Leaving this false is what keeps a
	// plain Complete byte-identical to its pre-037 request.
	JSONOutput bool
}

// ShimOptions returns a copy of these options carrying an explicit temperature
// and the given JSON-output setting, for use by the Complete/CompleteJSON shims
// over CompleteChat (037).
//
// It copies rather than mutating because the options pointer belongs to the
// caller and is frequently reused across a structured retry loop; writing a
// resolved default through it would make the second attempt differ from the
// first for reasons nobody wrote down.
func (o *CompleteOptions) ShimOptions(temp float64, jsonOutput bool) *CompleteOptions {
	var cp CompleteOptions
	if o != nil {
		cp = *o
	}
	if cp.Temperature == nil {
		t := temp
		cp.Temperature = &t
	}
	cp.JSONOutput = jsonOutput
	return &cp
}

// PromptMessages is the one-shot conversation a prompt string becomes:
// [system?, user], with the system turn omitted entirely when there is no
// system prompt rather than sent empty (C1-3).
func PromptMessages(system, prompt string) []Message {
	msgs := make([]Message, 0, 2)
	if system != "" {
		msgs = append(msgs, Message{Role: string(RoleSystem), Content: system})
	}
	return append(msgs, Message{Role: string(RoleUser), Content: prompt})
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

// Trace returns the correlation id grouping this call with its run, or "" if
// opts is nil or unset (036 FR-009).
func (o *CompleteOptions) Trace() string {
	if o == nil {
		return ""
	}
	return o.TraceID
}

// Task returns the requested routing key for observability grouping, or "" if
// opts is nil or unset (036 FR-012).
func (o *CompleteOptions) Task() string {
	if o == nil {
		return ""
	}
	return o.TaskKey
}

type traceIDKey struct{}

// WithTraceID marks a context as belonging to one logical run, so every LLM
// call made under it is grouped into a single observability trace (036 FR-009).
// The value is the run's activity-run id, which makes the trace cross-reference
// the platform's own history without a lookup table (FR-010).
//
// This is a context value rather than a parameter threaded through call sites
// because the requirement is that *every* call of a run carries it — including
// retries, re-prompts and escalations, which are emitted from inside helper
// functions several frames below where the run id is known. A parameter would
// make FR-009 depend on each of those remembering; a context makes it
// structural. Concurrent runs are naturally isolated (FR-011), since each has
// its own context.
func WithTraceID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceIDFrom returns the run id stamped by WithTraceID, or "" when the context
// is not part of a correlated run.
func TraceIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
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
	// CompleteChat sends a conversation and returns the assistant's turn,
	// including any lookups it asked for (037). Complete and CompleteJSON are
	// shims onto it.
	//
	// It MUST NOT mutate, reorder, merge or drop the caller's messages. The
	// slice is the caller's; an adapter that appends to it in place will
	// corrupt a retry that reuses the same conversation.
	CompleteChat(ctx context.Context, msgs []Message, opts *CompleteOptions) (ChatResult, error)
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
	return CompleteStructuredChat[T](ctx, p, PromptMessages(opts.SystemPrompt(), prompt), opts)
}

// CompleteStructuredChat is CompleteStructured over a conversation (037 FR-023).
//
// It is the same function: same schema cache, same strictification, same fence
// stripping, same two extra attempts, same Validator assertion, same immediate
// propagation of provider-level errors. Only the place the schema instruction
// and the retry correction are appended differs — a trailing user message
// instead of the tail of a prompt string. CompleteStructured is now this called
// with [system?, user], because two independent structured paths would drift
// exactly the way two schema generators would.
//
// It exists because a tool exchange has to end in a typed value. Every
// structured call site in this codebase goes through CompleteStructured, so a
// loop that terminated in prose would have had no caller here at all.
func CompleteStructuredChat[T any](ctx context.Context, p Provider, msgs []Message, opts *CompleteOptions) (T, error) {
	var zero T
	schema := schemaFor(reflect.TypeOf(zero))

	if opts == nil {
		opts = &CompleteOptions{}
	}
	// When the caller opts into strict mode, attach the schema so the
	// provider adapter can embed it in response_format.json_schema. A
	// caller that leaves the zero value (ResponseModeJSON) keeps the
	// legacy behaviour: the schema is still appended to the conversation as
	// text below, but the wire request stays json_object.
	if opts.ResponseMode == ResponseModeStrict && opts.JSONSchema == "" {
		opts.JSONSchema = schema
	}
	// The system prompt has already been rendered into msgs by the caller (or
	// by CompleteStructured's shim), so it must not be sent a second time as a
	// separate field.
	callOpts := opts.ShimOptions(0.1, true)
	callOpts.System = ""

	if len(msgs) == 0 {
		return zero, fmt.Errorf("%w: structured output requires at least one message", ErrInvalidResponse)
	}

	lastErr := ""
	for attempt := 0; attempt <= structuredRetries; attempt++ {
		// The instruction rides on a copy of the final turn rather than a new
		// message, so a one-shot call produces the identical single user
		// message it always has (SC-001).
		turn := "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema
		if lastErr != "" {
			turn += "\nYour previous answer was invalid: " + lastErr + "\nFix it and answer again with valid JSON only."
		}
		full := make([]Message, len(msgs))
		copy(full, msgs)
		full[len(full)-1].Content += turn

		res, err := p.CompleteChat(ctx, full, callOpts)
		if err != nil {
			// Provider-level failures (rate limit, bad credential, model
			// gone, provider down) are already classified and are not
			// fixable by re-prompting, so they propagate immediately —
			// only malformed *content* is worth another attempt.
			return zero, err
		}

		var result T
		if err := json.Unmarshal([]byte(stripFences(res.Content)), &result); err != nil {
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
