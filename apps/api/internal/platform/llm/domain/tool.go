package domain

import (
	"context"
	"encoding/json"
	"reflect"
)

type ToolDef struct {
	Name        string
	Description string

	ArgsSchema string

	Handler func(ctx context.Context, args json.RawMessage) (string, error)
}

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
