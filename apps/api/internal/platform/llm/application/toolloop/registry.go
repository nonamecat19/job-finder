package toolloop

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/job-finder/api/internal/platform/llm/domain"
)

// Toolset is the fixed set of lookups an exchange may use.
//
// It is immutable after construction, deliberately (FR-026, C4-20). Nothing in
// a conversation — not a model turn, not a tool result — can add, remove or
// alter a tool. A registry that could be extended mid-exchange would make the
// read-only fence meaningless, since the fence reasons about a package's
// imports at build time and cannot see a handler that appeared at runtime.
type Toolset struct {
	order []string
	tools map[string]domain.ToolDef
}

// NewToolset builds a toolset, rejecting duplicate names at construction rather
// than at call time. A duplicate discovered at call time is discovered during a
// model exchange, in production, on whichever of the two the map happened to
// keep.
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

// Declarations renders the toolset for a provider call, in registration order.
// The slice is freshly built each time so a caller cannot reach back through it
// into the toolset.
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

// Names returns the declared tool names, for error messages.
func (ts *Toolset) Names() []string {
	if ts == nil {
		return nil
	}
	return append([]string(nil), ts.order...)
}

// refusal is a call that was rejected before dispatch. It is not an error: it
// is a result the model gets to react to, which is what keeps a hallucinated
// tool name from ending an exchange.
type refusal struct{ reason string }

func (r refusal) Error() string { return r.reason }

// Dispatch validates and runs one call.
//
// Refusals — an unknown name, arguments that are not a JSON object, arguments
// the handler cannot decode — happen **without dispatch** and come back as a
// refusal rather than an error, so the exchange continues.
//
// The validation is deliberately done against the *decoded* argument object.
// The gateway wire format encodes arguments as a JSON string containing JSON;
// validating that quoted string against an object schema would refuse every
// well-formed call while looking exactly like this function working correctly.
// The adapter unquotes before we ever see it (C2-12), and this is the code that
// would otherwise mask the bug.
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
		// A decode failure inside the handler is an argument problem, which
		// the model can fix; anything else is a genuine failure of the lookup.
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
