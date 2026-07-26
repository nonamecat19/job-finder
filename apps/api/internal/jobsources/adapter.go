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

// Credentialed is an optional interface an Adapter implements when it uses
// user-account credentials (login session) to access the source. The
// retrieval ladder never escalates past the direct rung for credentialed
// adapters, since browser/FlareSolverr rungs can't carry session cookies
// and would land on a login page instead of the intended content.
type Credentialed interface {
	UsesUserAccount() bool
}

// NeedsDetail reports whether the adapter's Search rows are list-only and
// must be enriched before downstream analysis. Adapters that don't implement
// DetailNeeder return complete rows, so the answer is false.
func NeedsDetail(a Adapter) bool {
	dn, ok := a.(DetailNeeder)
	return ok && dn.NeedsDetail()
}

// IsCredentialed reports whether the adapter uses a user account.
func IsCredentialed(a Adapter) bool {
	c, ok := a.(Credentialed)
	return ok && c.UsesUserAccount()
}

// EmployerOutcome is one employer's result within a single fan-out run.
type EmployerOutcome string

const (
	EmployerOutcomeRead       EmployerOutcome = "read"
	EmployerOutcomeNotFound   EmployerOutcome = "not_found"
	EmployerOutcomeUnreadable EmployerOutcome = "unreadable"
	EmployerOutcomeRefused    EmployerOutcome = "refused"
	EmployerOutcomeNoPostings EmployerOutcome = "no_postings"
)

// EmployerRunOutcome is one roster employer's result from a single Search
// call, keyed by the vendor-scoped identifier used in EmployerBoard.
type EmployerRunOutcome struct {
	EmployerIdentifier string          `json:"employerIdentifier"`
	Outcome            EmployerOutcome `json:"outcome"`
	PostingsFound      int             `json:"postingsFound"`
}

// EmployerReporter is implemented by adapters whose one Search call fans out
// over multiple roster employers (the ATS board sources, 013) and can report
// a per-employer outcome distinct from the aggregate found/new counts
// (FR-019, FR-020). The ingestion handler type-asserts for it the same way it
// already does for DetailNeeder.
type EmployerReporter interface {
	LastRunDetail() []EmployerRunOutcome
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
