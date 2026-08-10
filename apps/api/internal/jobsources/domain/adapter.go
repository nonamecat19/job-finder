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

// PostingReader is implemented by adapters that can read a single posting page
// into a complete NormalizedJob. Optional: an adapter without it cannot serve
// manual add, and its hosts degrade to the fill-in path.
//
// Implementors must honour six rules:
//
//  1. MatchesPostingURL does no I/O and never panics on malformed input — it is
//     called for every registered adapter on every add, in registry order, and
//     the first claim wins.
//  2. It returns false for search URLs on its own host. A Djinni preset-search
//     URL is a Djinni URL that is not a posting, and the caller reports
//     not_a_posting precisely because the host was claimed but the shape was
//     rejected.
//  3. ReadPosting returns partial results rather than erroring when the page
//     loads but some fields are absent — that partial result is what feeds the
//     fill-in draft. It errors only when the page could not be read at all.
//  4. It honours the context deadline and returns context.DeadlineExceeded
//     wrapped no more deeply than errors.Is can see.
//  5. It sets SourceKey to its own Key() and resolves URL to an absolute,
//     canonical form.
//  6. It uses the same retrieval path as the adapter's other methods, so pacing
//     and ladder escalation apply unchanged (041 FR-003c).
type PostingReader interface {
	// MatchesPostingURL reports whether rawURL is a single posting on this
	// adapter's host. False for search pages, listings, and other hosts.
	// Must not perform I/O.
	MatchesPostingURL(rawURL string) bool

	// ReadPosting reads one posting page into a NormalizedJob. Title, Company,
	// URL and Description are filled where the page provides them; the caller
	// decides whether the result is complete enough to store.
	ReadPosting(ctx context.Context, rawURL string, config map[string]any) (dto.NormalizedJob, error)
}

// AsPostingReader returns the adapter's PostingReader, if it has one.
func AsPostingReader(a Adapter) (PostingReader, bool) {
	pr, ok := a.(PostingReader)
	return pr, ok
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
