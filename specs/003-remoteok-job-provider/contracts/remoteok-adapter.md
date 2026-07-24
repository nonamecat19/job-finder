# Contract: RemoteOKAdapter (Go interfaces)

This feature adds no new HTTP endpoints — RemoteOK is consumed entirely through the
existing `/api/sources`, `/api/subscriptions`, `/api/sources/{key}/run`,
`/api/sources/{key}/test`, and `/api/sources/{key}/enrich` routes, unchanged, with
`key="remoteok"` as a new valid value. The contract this feature actually introduces is the
Go-level adapter surface below, which every caller in `apps/api` (registry, ingestion
handler, enrichment handler) depends on.

## `jobsources.Adapter` (existing interface, `internal/jobsources/adapter.go`)

`RemoteOKAdapter` MUST implement:

```go
type Adapter interface {
    Key() string
    Kind() dto.SourceKind
    Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

- `Key()` → `"remoteok"`, constant, no receiver state.
- `Kind()` → `dto.SourceKindAPI` (see [research.md#R1](../research.md#r1-fetch-mechanism--json-api-vs-html-scrape)).
- `Search`:
  - **Precondition**: `query.SubscriptionURL != ""` (a RemoteOK tag/category URL or the
    bare API root). If empty, return an error
    (`fmt.Errorf("remoteok keyword search not implemented — use subscription URL instead")`),
    mirroring `IndeedAdapter.Search`'s message exactly — keyword search is out of scope
    (FR-014).
  - **Postcondition (success)**: returns `[]dto.NormalizedJob` with `SourceKey: "remoteok"`
    and `Remote: true` on every element; empty slice + nil error is a valid "zero matching
    listings" result (FR-011), distinct from a non-nil error ("could not be interpreted" —
    e.g. the response isn't valid JSON or has no recognizable job records per
    [research.md#R3](../research.md#r3-response-shape-and-the-leading-legal-notice-element)).
  - **Pacing**: a single call issues one HTTP request in the common case; no pagination
    loop exists to pace (FR-010 is trivially satisfied — see
    [research.md#R7](../research.md#r7-rate-limiting--request-pacing)).
  - **Request bound**: MUST NOT retry/loop against the API beyond the single fetch per
    `Search` invocation.
- `HealthCheck`: performs a lightweight reachability check (fetch the API root) and returns
  `(false, nil)` on failure — never a non-nil error for a normal "unreachable" case,
  matching `IndeedAdapter.HealthCheck`'s convention.

## `RemoteOKDetailPatch` + `FetchDetail` (new, adapter-specific — not part of `Adapter`)

Called directly by `enrichment.Handler`, same shape as `IndeedDetailPatch`/`DouDetailPatch`:

```go
type RemoteOKDetailPatch struct {
    Description string
    Tags        []string
    SalaryRaw   *string
    PostedAt    *string
    Available   bool
    Raw         map[string]any
}

func (d RemoteOKAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (RemoteOKDetailPatch, error)
```

- **Precondition**: `jobURL` is a URL previously returned in a `Search` result's
  `dto.NormalizedJob.URL`.
- **Postcondition (success)**: `Available: true`, `Description` populated from the
  re-fetched API payload's matching record (see
  [research.md#R5](../research.md#r5-fetchdetail--enrichment-shape)).
- **Postcondition (listing rotated out)**: if no record in the current API payload matches
  `jobURL`/its ID, returns `RemoteOKDetailPatch{Available: false}, nil` (not an error) —
  caller (`enrichment.Handler.enrichRemoteOK`, mirroring `enrichIndeed`) marks the listing
  unavailable and leaves the summary-level fields already captured at ingestion untouched
  (FR-009 edge case: "listing whose detail page is no longer available... summary data
  preserved").

## Consumers this contract must satisfy (no signature changes to these)

- `jobsources.Registry.Get("remoteok")` — resolves via `compose.go` registration.
- `ingestion.Handler.persistIfNew` — the
  `if j.SourceKey == "djinni" || j.SourceKey == "dou" || j.SourceKey == "indeed"`
  enrich-eligibility check gains `|| j.SourceKey == "remoteok"`.
- `enrichment.Handler.ProcessTask`'s `switch job.SourceKey` — gains a `case "remoteok":`
  calling a new `enrichRemoteOK` method, structurally identical to `enrichIndeed`.
- `enrichment.NewHandler(...)` constructor — gains a `remoteok adapters.RemoteOKAdapter`
  parameter, threaded from `compose.go`.
- `subscriptions.Service.validateSubscriptionURL` — gains a `remoteok` case accepting hosts
  `remoteok.com`/`remoteok.io`, mirroring the existing `indeed` case.

## Reused REST contract (unchanged request/response shapes, `key`/`sourceKey` = `"remoteok"`)

| Method | Path | Behavior for RemoteOK |
|---|---|---|
| `GET` | `/api/sources` | RemoteOK appears once its `JobSource` row exists (lazy) or with registry defaults |
| `PUT` | `/api/sources/remoteok` | enable/disable, config patch (unused fields today) |
| `POST` | `/api/sources/remoteok/test` | invokes `HealthCheck` |
| `POST` | `/api/sources/remoteok/run` | invokes `Search` with no subscription (keyword path) — expected to error per FR-014; operators use subscription run instead |
| `POST` | `/api/sources/remoteok/enrich` | backfill sweep, same as existing sources |
| `POST` | `/api/subscriptions` `{sourceKey:"remoteok", url}` | create; MUST validate `url` per data-model.md's Subscription validation rule (FR-015) |
| `POST` | `/api/subscriptions/{id}/run` | runs `Search` with that subscription's URL |
