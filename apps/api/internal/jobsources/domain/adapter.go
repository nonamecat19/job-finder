package domain

import (
	"context"

	"github.com/job-finder/api/internal/dto"
)

type Adapter interface {
	Key() string
	Kind() dto.SourceKind
	Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
	HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}

type DetailNeeder interface {
	NeedsDetail() bool
}

type Credentialed interface {
	UsesUserAccount() bool
}

func NeedsDetail(a Adapter) bool {
	dn, ok := a.(DetailNeeder)
	return ok && dn.NeedsDetail()
}

func IsCredentialed(a Adapter) bool {
	c, ok := a.(Credentialed)
	return ok && c.UsesUserAccount()
}

type EmployerOutcome string

const (
	EmployerOutcomeRead       EmployerOutcome = "read"
	EmployerOutcomeNotFound   EmployerOutcome = "not_found"
	EmployerOutcomeUnreadable EmployerOutcome = "unreadable"
	EmployerOutcomeRefused    EmployerOutcome = "refused"
	EmployerOutcomeNoPostings EmployerOutcome = "no_postings"
)

type EmployerRunOutcome struct {
	EmployerIdentifier string          `json:"employerIdentifier"`
	Outcome            EmployerOutcome `json:"outcome"`
	PostingsFound      int             `json:"postingsFound"`
}

type EmployerReporter interface {
	LastRunDetail() []EmployerRunOutcome
}

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
