# Contract: HimalayasAdapter (Go interfaces)

This feature adds no new HTTP endpoints — Himalayas is consumed entirely through the existing
`/api/sources`, `/api/subscriptions`, `/api/sources/{key}/run`, and `/api/sources/{key}/test`
routes, unchanged, with `key="himalayas"` as a new valid value. Unlike JobLeads/Indeed/Glassdoor,
there is no `/api/sources/{key}/enrich` involvement — Himalayas is never enrich-eligible
(research.md R6). The contract this feature actually introduces is the Go-level adapter surface
below, which every caller in `apps/api` (registry, ingestion handler) depends on.

## `jobsources.Adapter` (existing interface, `internal/jobsources/adapter.go`)

`HimalayasAdapter` MUST implement:

```go
type Adapter interface {
    Key() string
    Kind() dto.SourceKind
    Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

- `Key()` → `"himalayas"`, constant, no receiver state.
- `Kind()` → `dto.SourceKindAPI` (see
  research.md#r2-fetch-mechanism--undocumented-public-json-feed-not-html-scrape (`research.md#r2-fetch-mechanism--undocumented-public-json-feed-not-html-scrape`, removed on merge — see git history)).
- `Search`:
  - **Precondition**: `query.SubscriptionURL != ""` (a Himalayas `/jobs?categories=...` search-page
    URL). If empty, return an error (`fmt.Errorf("himalayas keyword search not implemented — use
    subscription URL instead")`), mirroring `RemoteOKAdapter.Search`'s message exactly — keyword
    search is out of scope (FR-014).
  - **Precondition (shape)**: the `SubscriptionURL` MUST parse to a non-empty `categories` value
    (see data-model.md (`data-model.md#source-subscription-existing-subscriptions-table`, removed on merge — see git history)); an
    already-invalid URL reaching `Search` (bypassing save-time validation, e.g. stale data) returns
    a distinguishable error rather than silently fetching unfiltered results.
  - **Behavior**: pages through `https://himalayas.app/jobs/api?limit=20&offset=N`
    (`N = 0, 20, 40, ...`) up to `himalayasMaxSubscriptionPages`, waiting
    `himalayasRequestDelay` (500ms) between requests (FR-010), keeping only jobs whose `categories`
    or `parentCategories` contains the subscription's category slug(s) and, when `timezones` was
    given, whose `timezoneRestrictions` overlaps or is empty (research.md R3, R7). Stops early once
    a fetched page's `offset >= totalCount`.
  - **Postcondition (success)**: returns `[]dto.NormalizedJob` with `SourceKey: "himalayas"` and
    `Remote: true` on every element; empty slice + nil error is a valid "zero matching listings"
    result (FR-011), distinct from a non-nil parse-failure error.
  - **Postcondition (response shape changed)**: if a page's body fails to decode as the expected
    JSON shape, returns a non-nil error distinguishable as "could not be interpreted" (FR-011) — a
    page that decodes but has an empty `jobs` array is NOT an error (that's the zero-results case).
  - **Pacing**: MUST wait at least 500ms between paginated requests and MUST NOT exceed
    `himalayasMaxSubscriptionPages` per call (FR-010).
- `HealthCheck`: fetches `https://himalayas.app/jobs/api?limit=1&offset=0` and returns `(false,
  nil)` for a normal "unreachable"/"unparseable" case — never a non-nil error for that case,
  matching `RemoteOKAdapter.HealthCheck`'s convention; returns `(true, nil)` when the response
  decodes and its `jobs` field is non-nil (a `totalCount` of 0 or an empty page is still considered
  healthy — health means "the endpoint is reachable and returns the expected shape," not "has
  matches for any particular subscription").

No `FetchDetail` method, no session type, no config-store interface — Himalayas has no credentials
and no enrichment step (research.md R1, R6). `HimalayasAdapter` carries only:

```go
type HimalayasAdapter struct {
    Scraping *scraping.Service
    APIURL   string // override for tests; production code leaves empty
}
```

mirroring `RemoteOKAdapter`'s shape exactly minus the subscription-URL-to-query-param translation
(replaced here by local post-fetch filtering, per research.md R3).

## Consumers this contract must satisfy (no signature changes to these)

- `jobsources.Registry.Get("himalayas")` — resolves via `compose.go` registration.
- `ingestion.Handler.persistIfNew` — the enrich-eligibility check
  (`j.SourceKey == "djinni" || ... || j.SourceKey == "jobleads"`) is **NOT** extended for
  `"himalayas"` (research.md R6) — a Himalayas job is never enqueued for enrichment.
- `enrichment.Handler`'s `switch job.SourceKey` — **NOT** extended for `"himalayas"`; the existing
  `default: return nil` case already handles any source key with no enrichment case, so no code
  change is needed there at all.
- `subscriptions.Service.validateSubscriptionURL` — gains a `himalayas` case accepting host
  `himalayas.app` / `*.himalayas.app`, path `/jobs` (or `/jobs/...`), and requiring a non-empty
  `categories` query parameter, mirroring the existing `remoteok` case's host-suffix check plus an
  additional required-query-param check.
- `config.Config` — **NOT** extended; no env vars, no secrets (research.md R1).

## Reused REST contract (unchanged request/response shapes, `key`/`sourceKey` = `"himalayas"`)

| Method | Path | Behavior for Himalayas |
|---|---|---|
| `GET` | `/api/sources` | Himalayas appears once its `JobSource` row exists (lazy) or with registry defaults |
| `PUT` | `/api/sources/himalayas` | enable/disable only — no operator-editable config fields exist |
| `POST` | `/api/sources/himalayas/test` | invokes `HealthCheck` |
| `POST` | `/api/sources/himalayas/run` | invokes `Search` with no subscription (keyword path) — expected to error per FR-014; operators use subscription run instead |
| `POST` | `/api/subscriptions` `{sourceKey:"himalayas", url}` | create; MUST validate `url` per data-model.md's Subscription validation rule (FR-015) |
| `POST` | `/api/subscriptions/{id}/run` | runs `Search` with that subscription's category/timezone filter; may report zero new jobs for a narrow category within the bounded page sweep (research.md R7) without that being an error |
