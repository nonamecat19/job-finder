// Package domain holds the jobsources bounded context's core model: the
// Adapter extensibility point, the adapter Registry, the persistence
// Repository port, the JobSource value object, and the typed errors the use
// case can return. It mirrors modules/job-sources/adapter.interface.ts and
// job-source.registry.ts. Adding a job site = one adapter implementing Adapter
// + one entry in the registry's constructor list.
package domain

import (
	"context"

	"github.com/job-finder/api/internal/dto"
)

// Adapter is the extensibility point every job source implements.
type Adapter interface {
	Key() string
	Kind() dto.SourceKind
	// Search receives the decrypted JobSource.config merged over env defaults.
	Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
	// HealthCheck is optional; nil means the registry falls back to a tiny search.
	HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}

// Registry holds every registered adapter keyed by its Key().
type Registry struct {
	byKey map[string]Adapter
	order []string
}

func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{byKey: make(map[string]Adapter, len(adapters))}
	for _, a := range adapters {
		r.byKey[a.Key()] = a
		r.order = append(r.order, a.Key())
	}
	return r
}

func (r *Registry) Get(key string) (Adapter, error) {
	a, ok := r.byKey[key]
	if !ok {
		return nil, AdapterNotRegisteredError{Key: key}
	}
	return a, nil
}

func (r *Registry) All() []Adapter {
	out := make([]Adapter, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.byKey[k])
	}
	return out
}

func (r *Registry) Keys() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}
