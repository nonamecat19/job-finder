// Package jobsources defines the JobSourceAdapter extensibility point and the
// registry of all adapters, mirroring modules/job-sources/adapter.interface.ts
// and job-source.registry.ts. Adding a job site = one adapter implementing
// Adapter + one entry in the registry's constructor list.
package jobsources

import (
	"context"
	"fmt"

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

// DetailNeeder is an optional capability an Adapter implements when its
// Search returns list-only rows — title/company/URL with a teaser or empty
// description — so the full posting has to be fetched by a separate enrich
// pass before the job text is worth matching or ghost-scoring.
//
// Declared as an optional interface (like the registry's HealthCheck
// fallback) rather than a method on Adapter, so adapters returning complete
// rows from Search need no change.
type DetailNeeder interface {
	NeedsDetail() bool
}

// NeedsDetail reports whether the adapter's Search rows are list-only and
// must be enriched before downstream analysis. Adapters that don't implement
// DetailNeeder return complete rows, so the answer is false.
func NeedsDetail(a Adapter) bool {
	dn, ok := a.(DetailNeeder)
	return ok && dn.NeedsDetail()
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
		return nil, fmt.Errorf("no job source adapter registered for key '%s'", key)
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
