package toolloop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

type Toolset struct {
	order []string
	tools map[string]domain.ToolDef
}

func NewToolset(tools ...domain.ToolDef) (*Toolset, error) {
	ts := &Toolset{tools: make(map[string]domain.ToolDef, len(tools))}
	for _, t := range tools {
		if t.Name == "" {
			return nil, fmt.Errorf("toolloop: a tool has no name")
		}
		if _, dup := ts.tools[t.Name]; dup {
			return nil, fmt.Errorf("toolloop: duplicate tool name %q", t.Name)
		}
		if t.Handler == nil {
			return nil, fmt.Errorf("toolloop: tool %q has no handler", t.Name)
		}
		ts.tools[t.Name] = t
		ts.order = append(ts.order, t.Name)
	}
	return ts, nil
}

func (ts *Toolset) Declarations() []domain.ToolDef {
	if ts == nil {
		return nil
	}
	out := make([]domain.ToolDef, 0, len(ts.order))
	for _, name := range ts.order {
		out = append(out, ts.tools[name])
	}
	return out
}

func (ts *Toolset) Names() []string {
	if ts == nil {
		return nil
	}
	return append([]string(nil), ts.order...)
}

type refusal struct{ reason string }

func (r refusal) Error() string { return r.reason }

func (ts *Toolset) Dispatch(ctx context.Context, call domain.ToolCall) (string, error) {
	tool, ok := ts.tools[call.Name]
	if !ok {
		return "", refusal{fmt.Sprintf("no such tool %q. Available tools: %v", call.Name, ts.Names())}
	}
	if !isJSONObject(call.Arguments) {
		return "", refusal{fmt.Sprintf("arguments for %q must be a JSON object matching its declared schema, got: %s", call.Name, preview(call.Arguments))}
	}
	out, err := tool.Handler(ctx, call.Arguments)
	if err != nil {

		var syntax *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		if asJSONError(err, &syntax, &typeErr) {
			return "", refusal{fmt.Sprintf("arguments for %q did not match its declared schema: %s", call.Name, err.Error())}
		}
		return "", err
	}
	return out, nil
}

func isJSONObject(raw json.RawMessage) bool {
	var probe map[string]any
	return json.Unmarshal(raw, &probe) == nil
}

func asJSONError(err error, syntax **json.SyntaxError, typeErr **json.UnmarshalTypeError) bool {
	if e, ok := err.(*json.SyntaxError); ok {
		*syntax = e
		return true
	}
	if e, ok := err.(*json.UnmarshalTypeError); ok {
		*typeErr = e
		return true
	}
	return false
}

func preview(raw json.RawMessage) string {
	const max = 200
	if len(raw) > max {
		return string(raw[:max]) + "…"
	}
	if len(raw) == 0 {
		return "(empty)"
	}
	return string(raw)
}
