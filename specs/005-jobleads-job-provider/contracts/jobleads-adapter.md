# Contract: JobLeadsAdapter (Go interfaces)

This feature adds no new HTTP endpoints — JobLeads is consumed entirely through the existing
`/api/sources`, `/api/subscriptions`, `/api/sources/{key}/run`, `/api/sources/{key}/test`,
and `/api/sources/{key}/enrich` routes, unchanged, with `key="jobleads"` as a new valid value.
The contract this feature actually introduces is the Go-level adapter + session surface
below, which every caller in `apps/api` (registry, ingestion handler, enrichment handler)
depends on.

## `jobsources.Adapter` (existing interface, `internal/jobsources/adapter.go`)

`JobLeadsAdapter` MUST implement:

```go
type Adapter interface {
    Key() string
    Kind() dto.SourceKind
    Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

- `Key()` → `"jobleads"`, constant, no receiver state.
- `Kind()` → `dto.SourceKindScrape` (see [research.md#R3](../research.md#r3-fetch-mechanism--html-scrape-vs-api)).
- `Search`:
  - **Precondition**: `query.SubscriptionURL != ""` (a JobLeads saved-search URL). If empty,
    return an error (`fmt.Errorf("jobleads keyword search not implemented — use subscription
    URL instead")`), mirroring `IndeedAdapter.Search`'s message exactly — keyword search is
    out of scope (FR-014).
  - **Precondition (credentials)**: if neither `JOBLEADS_EMAIL` nor `JOBLEADS_PASSWORD` is
    configured, return a distinguishable error ("jobleads requires login but no credentials
    configured: set JOBLEADS_EMAIL and JOBLEADS_PASSWORD") without attempting a request —
    unlike `DjinniAdapter`, JobLeads does NOT degrade to anonymous access (see
    [research.md#R1](../research.md#r1-access-model--login-gated-vs-public)).
  - **Postcondition (success)**: returns `[]dto.NormalizedJob` with `SourceKey: "jobleads"`
    on every element; empty slice + nil error is a valid "zero matching listings" result
    (FR-011), distinct from a non-nil parse-failure error and distinct from a non-nil
    authentication error (see below).
  - **Postcondition (session expired)**: if the login-page redirect is detected after one
    retry via `Session.Refresh` (see
    [research.md#R4](../research.md#r4-login-page--session-expiry-detection)), returns a
    non-nil error whose message is distinguishable as an authentication failure (e.g. wraps
    `"jobleads still at login after re-login"`), so callers/health state can report
    "access to the source is not authorized" distinctly from "content could not be
    interpreted" (FR-011).
  - **Pacing**: paginated fetches MUST wait at least 500ms between requests and MUST NOT
    exceed a fixed page cap per call (FR-010, see
    [research.md#R8](../research.md#r8-rate-limiting--request-pacing)).
- `HealthCheck`: performs a lightweight authenticated reachability check (fetch the
  saved-search root or account page) and returns `(false, nil)` for a normal
  "unreachable"/"not authorized" case — never a non-nil error for that case, matching
  `IndeedAdapter.HealthCheck`'s convention; the returned bool alone doesn't distinguish
  "unreachable" from "unauthorized", so callers needing that distinction use `Search`'s error
  message.

## `JobLeadsSession` + `JobLeadsSessionProvider` (new, mirrors `DjinniSession`)

```go
type JobLeadsSessionProvider interface {
    Ensure(ctx context.Context) (string, error)
    Refresh(ctx context.Context) (string, error)
}

type JobLeadsConfigStore interface {
    Config(ctx context.Context, key string) (map[string]any, error)
    Update(ctx context.Context, key string, enabled *bool, configPatch map[string]any) (*dto.JobSourceDto, error)
}

type JobLeadsSession struct {
    Sources  JobLeadsConfigStore
    Email    string
    Password string
    Key      string // "jobleads"
    Base     string // override base URL for tests
}
```

- `Ensure`: returns the stored cookie from `Sources.Config(ctx, "jobleads")["sessionCookie"]`
  if present; if absent AND `Email`/`Password` are both set, calls `Refresh`; if absent and no
  credentials are configured, returns `("", nil)` — callers (the adapter) are responsible for
  turning an empty cookie into the "credentials not configured" error at the `Search`/
  `HealthCheck` call site, not `Ensure` itself (mirrors `DjinniSession.Ensure`'s degrade
  behavior, but the *adapter* layer, not the session layer, is where JobLeads diverges from
  Djinni by refusing anonymous access).
- `Refresh`: performs the login POST, persists the resulting cookie via
  `Sources.Update(ctx, "jobleads", nil, map[string]any{"sessionCookie": cookie})`, serialized
  by an internal mutex so concurrent workers don't stampede login (mirrors
  `DjinniSession.Refresh` exactly).

## `JobLeadsDetailPatch` + `FetchDetail` (new, adapter-specific — not part of `Adapter`)

Called directly by `enrichment.Handler`, same shape as `IndeedDetailPatch`/`DouDetailPatch`:

```go
type JobLeadsDetailPatch struct {
    Description string
    SalaryRaw   *string
    PostedAt    *string
    Available   bool
    Raw         map[string]any
}

func (d JobLeadsAdapter) FetchDetail(ctx context.Context, jobURL string, config map[string]any) (JobLeadsDetailPatch, error)
```

- **Precondition**: `jobURL` is a URL previously returned in a `Search` result's
  `dto.NormalizedJob.URL`.
- **Postcondition (success)**: `Available: true`, `Description` populated from the fetched
  detail page (authenticated, with the same login-retry logic as `Search`).
- **Postcondition (listing gone)**: if the detail page reports the listing as removed/expired,
  returns `JobLeadsDetailPatch{Available: false}, nil` (not an error) — caller
  (`enrichment.Handler.enrichJobLeads`, mirroring `enrichIndeed`) marks the listing unavailable
  and leaves the summary-level fields already captured at ingestion untouched (FR-009 edge
  case).

## Consumers this contract must satisfy (no signature changes to these)

- `jobsources.Registry.Get("jobleads")` — resolves via `compose.go` registration.
- `ingestion.Handler.persistIfNew` — the enrich-eligibility check
  (`j.SourceKey == "djinni" || ... || j.SourceKey == "remoteok" || j.SourceKey == "glassdoor"`)
  gains `|| j.SourceKey == "jobleads"`.
- `enrichment.Handler.ProcessTask`'s `switch job.SourceKey` — gains a `case "jobleads":`
  calling a new `enrichJobLeads` method, structurally identical to `enrichDjinni`.
- `enrichment.NewHandler(...)` constructor — gains a `jobleads adapters.JobLeadsAdapter`
  parameter, threaded from `compose.go`.
- `subscriptions.Service.validateSubscriptionURL` — gains a `jobleads` case accepting hosts
  `jobleads.com` / `*.jobleads.com`, mirroring the existing `remoteok` case.
- `config.Config` — gains `JobLeadsEmail`/`JobLeadsPassword` fields (mapstructure-tagged
  `JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD`) and a secret-list entry, mirroring
  `DjinniEmail`/`DjinniPassword`.

## Reused REST contract (unchanged request/response shapes, `key`/`sourceKey` = `"jobleads"`)

| Method | Path | Behavior for JobLeads |
|---|---|---|
| `GET` | `/api/sources` | JobLeads appears once its `JobSource` row exists (lazy) or with registry defaults |
| `PUT` | `/api/sources/jobleads` | enable/disable, config patch (session cookie field is system-managed, not operator-editable) |
| `POST` | `/api/sources/jobleads/test` | invokes `HealthCheck` |
| `POST` | `/api/sources/jobleads/run` | invokes `Search` with no subscription (keyword path) — expected to error per FR-014; operators use subscription run instead |
| `POST` | `/api/sources/jobleads/enrich` | backfill sweep, same as existing sources |
| `POST` | `/api/subscriptions` `{sourceKey:"jobleads", url}` | create; MUST validate `url` per data-model.md's Subscription validation rule (FR-015) |
| `POST` | `/api/subscriptions/{id}/run` | runs `Search` with that subscription's URL; may fail with an "authentication required" reason if the session is expired and re-login fails |
