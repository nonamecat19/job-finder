# Contract: Adapter Framework

The public surface of the library's adapter framework — the registry, the `Adapter`/`PostingReader` interfaces, and the supporting types that every site adapter implements. Consumers construct a `Registry` with the adapters they want and call `Search`/`ReadPosting`/`HealthCheck` against it.

## Interface

```go
// Package adapter
package adapter

import (
    "context"

    "github.com/job-finder/jobscraper/model"
)

type Adapter interface {
    Key() string
    Kind() model.SourceKind
    Search(ctx context.Context, query model.SearchQuery, config map[string]any) ([]model.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}

type DetailNeeder interface {
    NeedsDetail() bool
}

type Credentialed interface {
    UsesUserAccount() bool
}

type PostingReader interface {
    // MatchesPostingURL reports whether rawURL is a single posting on this
    // adapter's host. False for search pages, listings, and other hosts.
    // Must not perform I/O.
    MatchesPostingURL(rawURL string) bool

    // ReadPosting reads one posting page into a NormalizedJob. Partial results
    // are returned (not errors) when the page loads but some fields are absent.
    ReadPosting(ctx context.Context, rawURL string, config map[string]any) (model.NormalizedJob, error)
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

func NewRegistry(adapters ...Adapter) *Registry
func (r *Registry) Get(key string) (Adapter, error)
func (r *Registry) All() []Adapter
func (r *Registry) Keys() []string

func AsPostingReader(a Adapter) (PostingReader, bool)
func NeedsDetail(a Adapter) bool
func IsCredentialed(a Adapter) bool
```

## Errors

```go
type SourceNotFoundError struct{ Key string }
type AdapterNotRegisteredError struct{ Key string }
```

## Rules carried from the existing code (preserved verbatim)

A `PostingReader` must honour six rules (from the current `domain/adapter.go` doc block — they encode behavior the app depends on, so they move with the interface):

1. `MatchesPostingURL` does no I/O and never panics on malformed input.
2. `MatchesPostingURL` returns false for search URLs on its own host.
3. `ReadPosting` returns partial results rather than erroring when the page loads but some fields are absent; errors only when the page could not be read at all.
4. `ReadPosting` honours the context deadline; returns `context.DeadlineExceeded` wrapped no more deeply than `errors.Is` can see.
5. `ReadPosting` sets `SourceKey` to its own `Key()` and resolves URL to absolute canonical form.
6. `ReadPosting` uses the same retrieval path as the adapter's other methods (pacing + ladder apply).

## Board-adapter helpers (library-internal, in `adapters/`)

`adapters/atsboard.go` moves whole and exposes:

```go
type employerFetcher func(ctx context.Context, employer rosterport.EmployerBoard) (statusCode int, jobs []model.NormalizedJob, err error)

func runBoardVendor(ctx context.Context, roster rosterport.RosterPort, state *boardRunState, vendor string, fetch employerFetcher) ([]model.NormalizedJob, error)
func vendorHealthCheck(ctx context.Context, roster rosterport.RosterPort, vendor string, fetch employerFetcher) (bool, error)
func healthCheckEmployer(fetch employerFetcher) rosterport.EmployerHealthChecker
func classifyOutcome(status int, jobs []model.NormalizedJob, err error) adapter.EmployerOutcome

func NewBoardAdapters() (
    gh *GreenhouseAdapter, lv *LeverAdapter, as *AshbyAdapter,
    wk *WorkableAdapter, sr *SmartRecruitersAdapter,
    checkers map[string]rosterport.EmployerHealthChecker,
)
```

`NewBoardAdapters` returns the 5 board adapters plus the health-checker map; the app passes the map into `roster.NewService(q, checkers)` unchanged. ATSBoard is not a `NewBoardAdapters` returnee (it's constructed separately with a `Roster` field).

## Import graph guarantee

`jobscraper/adapter` imports only `jobscraper/model` + stdlib. It does NOT import `jobscraper/retrieval` or `jobscraper/adapters` — so a consumer can use the framework types without pulling in any rung or site code.

`jobscraper/adapters` imports `jobscraper/adapter`, `jobscraper/model`, `jobscraper/retrieval`, `jobscraper/scraping`, `jobscraper/rosterport`, `jobscraper/htmlutil`, `jobscraper/httpjson`, `jobscraper/strutil` + goquery/chromedp/bogdanfinn. This is the heavyweight package; consumers who want only the engine avoid it (FR-016).