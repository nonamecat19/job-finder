package domain

import (
	"context"
	"encoding/json"
	"reflect"
)

// ToolDef is a lookup the model may request.
//
// Nothing here is executable by the platform layer on its own — the handler is
// carried alongside, and the loop package is what dispatches. What lives here is
// the declaration, because the declaration has to be derived from the same
// schema machinery structured output already uses.
type ToolDef struct {
	Name        string
	Description string
	// ArgsSchema is the marshalled strict JSON Schema for the arguments.
	ArgsSchema string
	// Handler runs the lookup. It takes the raw decoded argument object and
	// returns an opaque result string; the loop treats that result as
	// untrusted data, never as instruction (FR-024).
	Handler func(ctx context.Context, args json.RawMessage) (string, error)
}

// NewTool declares a lookup whose arguments are the type parameter T.
//
// The schema comes from the **existing** schemaFor + strictifySchema path — the
// same one CompleteStructured uses, cache included. That is the whole point of
// the generic constructor: a second schema generator would drift from the first,
// and the failure mode of a drifted tool schema is a model that keeps sending
// arguments the registry keeps refusing, which reads as a model problem.
//
// Because the declared schema and the decoded struct are the same Go type, they
// cannot disagree.
func NewTool[T any](name, description string, handler func(ctx context.Context, args T) (string, error)) ToolDef {
	var zero T
	return ToolDef{
		Name:        name,
		Description: description,
		ArgsSchema:  schemaFor(reflect.TypeOf(zero)),
		Handler: func(ctx context.Context, raw json.RawMessage) (string, error) {
			var args T
			if err := json.Unmarshal(raw, &args); err != nil {
				return "", err
			}
			return handler(ctx, args)
		},
	}
}
