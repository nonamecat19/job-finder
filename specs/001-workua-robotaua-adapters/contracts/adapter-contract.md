# Contract: `WorkUaAdapter`

The job-source extensibility point is this project's internal plugin interface, defined at `apps/api/internal/jobsources/adapter.go`. This document states what `WorkUaAdapter` must satisfy.

## Interface obligations

```go
type Adapter interface {
    Key() string
    Kind() dto.SourceKind
    Search(ctx context.Context, query dto.SearchQuery, config map[string]any) ([]dto.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

### `Key() string`

Returns `"workua"`. Constant, and stable forever — it is the `job_source.key` primary lookup and the discriminator in the enrichment handler's `switch job.SourceKey`. Changing it later orphans existing `job` rows.

### `Kind() dto.SourceKind`

Returns `dto.SourceKindScrape`. No new enum value.

### `Search(ctx, query, config) ([]dto.NormalizedJob, error)`

| Input condition | Required behavior |
|---|---|
| `query.SubscriptionURL != ""` | Paginate that URL; ignore `Keywords`/`Remote`. |
| `query.Remote != nil && *query.Remote` | Request `https://www.work.ua/jobs-remote/?search={kw}`. |
| otherwise | Request `https://www.work.ua/jobs/?search={urlencoded kw}`; follow redirect. |
| `query.Keywords == ""` and no `SubscriptionURL` | Return the board's unfiltered first page. Not an error. |

**Contract guarantees**:

- **Never errors on zero results.** Returns `(nil, nil)` + a `slog.Warn` naming the URL and stating markup may have changed (FR-007).
- **Errors only on transport failure** or a malformed `SubscriptionURL` (FR-013). A returned error confines failure to this source; the registry records the run failed and other sources proceed (FR-008).
- **Every returned job** has non-empty `Title`, non-empty `Company` (or `"Unknown"`), and an absolute `URL`. Cards failing this are skipped, not emitted half-built (SC-003).
- **Pacing**: ≥2s between successive HTTP requests within one `Search` call (research Decision 4).
- **Pagination terminates**: on an empty page, on a repeated first-card URL, or at a hard cap (`workuaMaxSubscriptionPages = 50`), mirroring `djinni.go:91`.
- **Context cancellation** aborts pagination and returns what was collected so far.
- **Read-only**: GET only. No POST, no form submission, no state change on work.ua (Principle I, FR-015).

### `HealthCheck(ctx, config) (bool, error)`

Fetches `https://www.work.ua/jobs/` and reports whether the response looks like work.ua. Follows the djinni shape: a fetch failure returns `(false, nil)` — not an error — so the Sources page shows the source as down rather than the health check itself blowing up (FR-009).

## Non-interface method (called directly by enrichment)

### `FetchDetail(ctx, jobURL string, config map[string]any) (WorkUaDetailPatch, error)`

Not part of `Adapter` — adapter-specific, exactly as `DjinniAdapter.FetchDetail` / `DouAdapter.FetchDetail` are. Called from `enrichment.Handler` via the `switch job.SourceKey` dispatch.

**Contract guarantees**:

- A missing field yields a zero value (`nil` / `""`), never an error (FR-006).
- Transport failure returns an error; the enrichment handler logs a warning and returns `nil`, leaving the teaser description intact and never blocking other jobs (spec Story 3 scenario 2).
- Retained raw HTML is truncated rune-safely (FR-005).
- Read-only GET.

## Configuration contract

| Key | Source | Default | Purpose |
|---|---|---|---|
| `WORKUA_DETAIL_DELAY_MS` | env → `config.Config` | `2000` | Pause before each detail fetch. Floor mandated by work.ua's published `Crawl-delay: 2`. |

work.ua requires **no credentials**. The adapter's `config map[string]any` is accepted for interface conformance but carries nothing today — hence no `CONFIG_FIELDS` entry in `SourcesPage.tsx`.

## Registration contract

Adding the source requires exactly two registry entries, per `adapter.go`'s stated rule:

1. `apps/api/cmd/server/main.go` — construct with the shared `scrapingSvc`, add to `jobsources.NewRegistry(...)`.
2. `apps/api/cmd/seed/main.go` — add to the seed registry so the lazy `GetByKey` materializes the `job_source` row that `SourceRun`/`Subscription` fixtures FK against.

Omitting (2) breaks `make seed` for any fixture referencing this source.
