# Contract: IndeedAdapter (Go interfaces)

This feature adds no new HTTP endpoints — Indeed is consumed entirely through the existing
`/api/sources`, `/api/subscriptions`, `/api/sources/{key}/run`, `/api/sources/{key}/test`,
and `/api/sources/{key}/enrich` routes, unchanged, with `key="indeed"` as a new valid value.
The contract this feature actually introduces is the Go-level adapter surface below, which
every caller in `apps/api` (registry, ingestion handler, enrichment handler) depends on.

## `jobsources.Adapter` (existing interface, `internal/jobsources/adapter.go`)

`IndeedAdapter` MUST implement:

```go
type Adapter interface {
    Key() string
    Kind() dto.SourceKind
    Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

- `Key()` → `"indeed"`, constant, no receiver state.
- `Kind()` → `dto.SourceKindScrape`.
- `Search`:
  - **Precondition**: `query.SubscriptionURL != ""` (an Indeed search URL). If empty,
    return an error (`fmt.Errorf("indeed keyword search not implemented — use subscription URL instead")`),
    mirroring `DouAdapter.Search`'s message exactly — keyword search is out of scope
    (FR-015).
  - **Postcondition (success)**: returns `[]dto.NormalizedJob` with `SourceKey: "indeed"` on
    every element; empty slice + nil error is a valid "zero matching listings" result
    (FR-011, edge case "zero results"), distinct from a non-nil error ("could not be
    interpreted").
  - **Postcondition (partial failure)**: if pagination succeeds for pages 1..N and page
    N+1 fails, returns the N pages' jobs with `nil` error (matches
    `DjinniAdapter.scrapeSubscription`'s convention) — never returns `nil, err` after
    partial progress unless page 1 itself failed.
  - **Pacing**: MUST NOT issue two HTTP requests less than 500ms apart within one call
    (FR-010).
  - **Pagination bound**: MUST stop after a fixed max-pages constant (mirrors
    `douMaxSubscriptionPages`/`djinniMaxSubscriptionPages = 50`).
- `HealthCheck`: performs a lightweight reachability check (fetch the search-results host)
  and returns `(false, nil)` on failure — never a non-nil error for a normal "unreachable"
  case, matching `DjinniAdapter.HealthCheck`'s convention (`return false, nil` on fetch
  error).

## `IndeedDetailPatch` + `FetchDetail` (new, adapter-specific — not part of `Adapter`)

Called directly by `enrichment.Handler`, same as `DjinniDetailPatch`/`DouDetailPatch`:

```go
type IndeedDetailPatch struct {
    Description string
    SalaryRaw   *string
    Location    *string
    Remote      bool
    PostedAt    *string
    Raw         map[string]string
}

func (d IndeedAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (IndeedDetailPatch, error)
```

- **Precondition**: `jobURL` is a URL previously returned in a `Search` result's
  `dto.NormalizedJob.URL`.
- **Postcondition (success)**: `Description` non-empty when the detail page is reachable
  and parseable.
- **Postcondition (listing gone)**: if the detail page 404s or is otherwise gone, returns a
  non-nil `error`; caller (`enrichment.Handler.enrichIndeed`, mirroring `enrichDjinni`)
  logs a warning and returns `nil` to the queue rather than retrying — the job's existing
  summary-level fields are left untouched (FR-009 edge case: "detail page no longer
  available... summary data preserved").

## Consumers this contract must satisfy (no signature changes to these)

- `jobsources.Registry.Get("indeed")` — resolves via `compose.go` registration.
- `ingestion.Handler.persistIfNew` — the `if j.SourceKey == "djinni" || j.SourceKey ==
  "dou"` enrich-eligibility check gains `|| j.SourceKey == "indeed"`.
- `enrichment.Handler.ProcessTask`'s `switch job.SourceKey` — gains a `case "indeed":`
  calling a new `enrichIndeed` method, structurally identical to `enrichDjinni`/`enrichDOU`.
- `enrichment.NewHandler(...)` constructor — gains an `indeed adapters.IndeedAdapter`
  parameter, threaded from `compose.go`.

## Reused REST contract (unchanged request/response shapes, `key`/`sourceKey` = `"indeed"`)

| Method | Path | Behavior for Indeed |
|---|---|---|
| `GET` | `/api/sources` | Indeed appears once its `JobSource` row exists (lazy) or with registry defaults |
| `PUT` | `/api/sources/indeed` | enable/disable, config patch (unused fields today) |
| `POST` | `/api/sources/indeed/test` | invokes `HealthCheck` |
| `POST` | `/api/sources/indeed/run` | invokes `Search` with no subscription (keyword path) — expected to error per FR-015; operators use subscription run instead |
| `POST` | `/api/sources/indeed/enrich` | backfill sweep, same as existing sources |
| `POST` | `/api/subscriptions` `{sourceKey:"indeed", url}` | create; MUST validate `url` per data-model.md's Subscription validation rule (FR-016) |
| `POST` | `/api/subscriptions/{id}/run` | runs `Search` with that subscription's URL |
