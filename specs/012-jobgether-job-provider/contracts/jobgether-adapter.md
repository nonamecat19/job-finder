# Contract: JobgetherAdapter (Go interfaces)

This feature adds no new HTTP endpoints — Jobgether is consumed entirely through the existing
`/api/sources`, `/api/subscriptions`, `/api/sources/{key}/run`, `/api/sources/{key}/test`,
and `/api/sources/{key}/enrich` routes, unchanged, with `key="jobgether"` as a new valid value.
The contract this feature actually introduces is the Go-level adapter surface below, which
every caller in `apps/api` (registry, ingestion handler, enrichment handler) depends on.

## `jobsources.Adapter` (existing interface, `internal/jobsources/adapter.go`)

`JobgetherAdapter` MUST implement:

```go
type Adapter interface {
    Key() string
    Kind() dto.SourceKind
    Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

- `Key()` → `"jobgether"`, constant, no receiver state.
- `Kind()` → `dto.SourceKindScrape` (see [research.md#R2](../research.md#r2-fetch-mechanism--html-scrape-vs-api)).
- `Search`:
  - **Precondition**: `query.SubscriptionURL != ""` (a Jobgether search-results URL). If empty,
    return an error (`fmt.Errorf("jobgether keyword search not implemented — use subscription
    URL instead")`), mirroring `GlassdoorAdapter.Search`'s message exactly — keyword search is
    out of scope (FR-014).
  - **Postcondition (success)**: returns `[]dto.NormalizedJob` with `SourceKey: "jobgether"`
    on every element; empty slice + nil error is a valid "zero matching listings" result
    (FR-011), distinct from a non-nil parse-failure error and a non-nil blocked/throttled
    error (see below).
  - **Postcondition (blocked/throttled)**: if the response looks like a rate-limit or
    bot-challenge interstitial (see
    [research.md#R3](../research.md#r3-blockedthrottled-response-handling)), page 1 returns a
    non-nil, distinguishable "blocked" error; a later page hitting the same condition ends
    pagination with whatever was already collected, logged as a warning, not a fatal error
    (mirrors `GlassdoorAdapter.scrapeSubscription`).
  - **Postcondition (unparsable)**: if a fetched page returns content that fails to parse into
    any recognizable listing card, this is reported distinctly from "zero results" (FR-011) —
    page 1 failing to parse is fatal; a later page failing to parse ends pagination with
    whatever was already collected.
  - **Pacing**: paginated fetches MUST wait at least 500ms between requests and MUST NOT
    exceed a fixed page cap per call (FR-010, see
    [research.md#R7](../research.md#r7-rate-limiting--request-pacing)).
- `HealthCheck`: performs a lightweight, unauthenticated reachability check (fetch the
  Jobgether homepage or search root) and returns `(false, nil)` for a normal
  "unreachable"/"blocked" case — never a non-nil error for that case, matching
  `GlassdoorAdapter.HealthCheck`'s convention.

## `JobgetherDetailPatch` + `FetchDetail` (new, adapter-specific — not part of `Adapter`)

Called directly by `enrichment.Handler`, same shape as `GlassdoorDetailPatch`/
`IndeedDetailPatch`:

```go
type JobgetherDetailPatch struct {
    Description string
    SalaryRaw   *string
    PostedAt    *string
    Available   bool
    Raw         map[string]any
}

func (d JobgetherAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (JobgetherDetailPatch, error)
```

- **Precondition**: `jobURL` is a URL previously returned in a `Search` result's
  `dto.NormalizedJob.URL`.
- **Postcondition (success)**: `Available: true`, `Description` populated from the fetched
  detail page; `Raw` may include `jobgetherMatchScore` if present on the detail page and not
  already captured at list time.
- **Postcondition (listing gone)**: if the detail page has no recognizable description (the
  listing has rotated out / is gone — FR-009 edge case), returns
  `JobgetherDetailPatch{Available: false}, nil` (not an error) — caller
  (`enrichment.Handler.enrichJobgether`, mirroring `enrichGlassdoor`) marks the listing
  unavailable and leaves the summary-level fields already captured at ingestion untouched.
- Returns a non-nil error only on fetch failure or a blocked/challenge response.

## Consumers this contract must satisfy (no signature changes to these)

- `jobsources.Registry.Get("jobgether")` — resolves via `compose.go` registration.
- `ingestion.Handler.persistIfNew` — the enrich-eligibility check
  (`j.SourceKey == "djinni" || ... || j.SourceKey == "glassdoor" || j.SourceKey ==
  "jobleads"`) gains `|| j.SourceKey == "jobgether"`.
- `enrichment.Handler.ProcessTask`'s `switch job.SourceKey` — gains a `case "jobgether":`
  calling a new `enrichJobgether` method, structurally identical to `enrichGlassdoor`.
- `enrichment.NewHandler(...)` constructor — gains a `jobgether adapters.JobgetherAdapter`
  parameter, threaded from `compose.go`.
- `subscriptions.Service.validateSubscriptionURL` — gains a `jobgether` case accepting hosts
  `jobgether.com` / `*.jobgether.com`, mirroring the existing `glassdoor` case.

## Reused REST contract (unchanged request/response shapes, `key`/`sourceKey` = `"jobgether"`)

| Method | Path | Behavior for Jobgether |
|---|---|---|
| `GET` | `/api/sources` | Jobgether appears once its `JobSource` row exists (lazy) or with registry defaults |
| `PUT` | `/api/sources/jobgether` | enable/disable, config patch (no operator-editable secrets — Jobgether has no credentials) |
| `POST` | `/api/sources/jobgether/test` | invokes `HealthCheck` |
| `POST` | `/api/sources/jobgether/run` | invokes `Search` with no subscription (keyword path) — expected to error per FR-014; operators use subscription run instead |
| `POST` | `/api/sources/jobgether/enrich` | backfill sweep, same as existing sources |
| `POST` | `/api/subscriptions` `{sourceKey:"jobgether", url}` | create; MUST validate `url` per data-model.md's Subscription validation rule (FR-015) |
| `POST` | `/api/subscriptions/{id}/run` | runs `Search` with that subscription's URL; may fail with a "blocked/rate-limited" reason distinct from "no matching listings" or "could not be interpreted" (FR-011) |
