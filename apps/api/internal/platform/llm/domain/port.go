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

type ResponseMode int

const (
	ResponseModeJSON ResponseMode = iota

	ResponseModeStrict
)

type CompleteOptions struct {
	System      string
	Temperature *float64
	MaxTokens   *int

	Model string

	ResponseMode ResponseMode

	JSONSchema string

	TraceID string

	TaskKey string

	Tools []ToolDef

	ToolChoice string

	JSONOutput bool
}

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

func PromptMessages(system, prompt string) []Message {
	msgs := make([]Message, 0, 2)
	if system != "" {
		msgs = append(msgs, Message{Role: string(RoleSystem), Content: system})
	}
	return append(msgs, Message{Role: string(RoleUser), Content: prompt})
}

func (o *CompleteOptions) ModelOr(def string) string {
	if o != nil && o.Model != "" {
		return o.Model
	}
	return def
}

func (o *CompleteOptions) Temp(def float64) float64 {
	if o != nil && o.Temperature != nil {
		return *o.Temperature
	}
	return def
}

func (o *CompleteOptions) SystemPrompt() string {
	if o == nil {
		return ""
	}
	return o.System
}

func (o *CompleteOptions) Trace() string {
	if o == nil {
		return ""
	}
	return o.TraceID
}

func (o *CompleteOptions) Task() string {
	if o == nil {
		return ""
	}
	return o.TaskKey
}

type traceIDKey struct{}

func WithTraceID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, traceIDKey{}, id)
}

func TraceIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey{}).(string); ok {
		return v
	}
	return ""
}

type servedModelKey struct{}

func WithServedModelCapture(ctx context.Context) (context.Context, *string) {
	ptr := new(string)
	return context.WithValue(ctx, servedModelKey{}, ptr), ptr
}

func ReportServedModel(ctx context.Context, model string) {
	if ptr, ok := ctx.Value(servedModelKey{}).(*string); ok {
		*ptr = model
	}
}

type Usage struct {
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int

	ServedGroup string

	AttemptedFallbacks int
	Substituted        bool
}

type usageKey struct{}

func WithUsageCapture(ctx context.Context) (context.Context, *Usage) {
	ptr := new(Usage)
	return context.WithValue(ctx, usageKey{}, ptr), ptr
}

func ReportUsage(ctx context.Context, u Usage) {
	if ptr, ok := ctx.Value(usageKey{}).(*Usage); ok {
		*ptr = u
	}
}

type Provider interface {
	ModelName() string
	Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error)
	CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error)

	CompleteChat(ctx context.Context, msgs []Message, opts *CompleteOptions) (ChatResult, error)
	Embed(ctx context.Context, text string) ([]float32, error)
}

type Validator interface {
	Validate() error
}

const structuredRetries = 2

var fenceRe = regexp.MustCompile("(?s)^```(?:json)?\\s*(.*?)\\s*```$")

func stripFences(text string) string {
	t := strings.TrimSpace(text)
	if m := fenceRe.FindStringSubmatch(t); m != nil {
		return m[1]
	}
	return t
}

var schemaCache sync.Map

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

func CompleteStructured[T any](ctx context.Context, p Provider, prompt string, opts *CompleteOptions) (T, error) {
	return CompleteStructuredChat[T](ctx, p, PromptMessages(opts.SystemPrompt(), prompt), opts)
}

func CompleteStructuredChat[T any](ctx context.Context, p Provider, msgs []Message, opts *CompleteOptions) (T, error) {
	var zero T
	schema := schemaFor(reflect.TypeOf(zero))

	if opts == nil {
		opts = &CompleteOptions{}
	}

	if opts.ResponseMode == ResponseModeStrict && opts.JSONSchema == "" {
		opts.JSONSchema = schema
	}

	callOpts := opts.ShimOptions(0.1, true)
	callOpts.System = ""

	if len(msgs) == 0 {
		return zero, fmt.Errorf("%w: structured output requires at least one message", ErrInvalidResponse)
	}

	lastErr := ""
	for attempt := 0; attempt <= structuredRetries; attempt++ {

		turn := "\n\nRespond with a single JSON object matching this JSON Schema:\n" + schema
		if lastErr != "" {
			turn += "\nYour previous answer was invalid: " + lastErr + "\nFix it and answer again with valid JSON only."
		}
		full := make([]Message, len(msgs))
		copy(full, msgs)
		full[len(full)-1].Content += turn

		res, err := p.CompleteChat(ctx, full, callOpts)
		if err != nil {

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
