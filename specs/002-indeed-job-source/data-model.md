# Phase 1 Data Model: Indeed Job Source

No schema migration. This feature reuses four existing tables/entities exactly as DOU and
Djinni already do, adding no columns and no new tables. Entities below describe the shape
each plays for Indeed specifically, mapped to existing Go/SQL types.

## JobSource (existing table `"JobSource"`, migration 00001)

Row identity is the adapter's `Key()` string. No row is seeded upfront — created lazily on
first real use (`jobsources.Service.List`/`GetByKey`), same as every other source.

| Field | Type | Indeed-specific value |
|---|---|---|
| `key` | `text` (unique) | `"indeed"` |
| `kind` | enum (`api`/`scrape`/`sidecar`) | `"scrape"` (`dto.SourceKindScrape`) |
| `enabled` | `bool` | defaults `true` until operator disables |
| `healthy` | `bool` | set by `HealthCheck` / run outcome, same as DOU |
| `config` | encrypted JSON | unused by Indeed (no credentials); `{}` |

**Validation rules**: none beyond what `jobsources.Service` already enforces (key must
resolve via the code-defined `Registry`, not a free-form string).

## Subscription (existing table `"Subscription"`, migration 00002)

One row per operator-pasted Indeed search URL. FK `sourceKey` → `JobSource.key`.

| Field | Type | Indeed-specific value |
|---|---|---|
| `sourceKey` | `text` (FK) | `"indeed"` |
| `url` | `text` | a pasted `indeed.com/jobs?...` (or country-domain equivalent) search URL |
| `name` | `text?` | operator-supplied label |
| `enabled` | `bool` | gates whether scheduled runs use it |
| `lastRunAt` | `timestamp?` | touched by `TouchSubscriptionLastRun` after each run |

**Validation rules** (FR-016): at `Create` time, reject a URL that does not parse as an
Indeed search URL (host is an Indeed domain, e.g. `indeed.com`/`*.indeed.com`, and path is a
job-search listing path, not an individual job posting or an unrelated page) — mirrors the
existing `sourceKey`-must-resolve-in-registry check in `subscriptions.Service.Create`, just
one more precondition before insert. No format validation happens at run time; an invalid
URL never reaches a run.

**State transitions**: none beyond existing enable/disable and `lastRunAt` touch — no new
subscription lifecycle.

## NormalizedJob → Job (existing table `"Job"`, produced via `dto.NormalizedJob`)

Produced by `IndeedAdapter.Search`/`FetchDetail`, persisted via the existing
`ingestion.Handler.persistIfNew` → `InsertJob`/`UpdateJobDetail` path, unchanged.

| `dto.NormalizedJob` field | Populated from | Notes |
|---|---|---|
| `SourceKey` | constant | `"indeed"` |
| `ExternalID` | listing URL's job-id fragment or Indeed's `jk=` param | best-effort; nil if not resolvable |
| `Title` | card `<h3><a>` text | required — card skipped if empty |
| `Company` | card text near title | may be empty (FR: listing still ingested) |
| `Location` | card text | may be empty |
| `Remote` | derived from location/description text match ("Remote"/"Hybrid") | boolean, defensive regex like `douRemoteRe`/`djinniRemoteRe` |
| `SalaryRaw` | card free-text salary line | nilable |
| `URL` | resolved absolute job/search-result URL | required — card skipped if empty; must be an openable page (SC-004) |
| `Description` | card snippet (list) → full body (detail) | short at ingestion, full after enrichment (US3) |
| `PostedAt` | relative/absolute date text on card or detail page, when present | nilable; parsed same defensive way as `parseRelativeDOUDate` |
| `Raw` | raw HTML/text snapshot | for debugging, same convention as other adapters |

**Dedupe key**: unchanged — `sha256(lower(company)|lower(title)|canonicalURL)` via existing
`DedupeKey` helper in `ingestion/handler.go`; Indeed listings dedupe against each other and
against other sources' listings for the same job identically to how DOU/Djinni already do
(spec edge case: "same job posted on Indeed and on another source").

## SourceRun (existing table `"SourceRun"`, migration 00001)

One row per Indeed run execution, created/finished by the existing `ingestion.Handler`
(`FinishSourceRunOk`/`FinishSourceRunError`) — no Indeed-specific change to this table's
shape.

| Field | Type | Indeed-specific value |
|---|---|---|
| `sourceId` | FK → `JobSource.id` | the lazily-created Indeed `JobSource` row |
| `found` | `int32` | count of cards parsed across all paginated pages in the run |
| `new` | `int32` | count that passed `persistIfNew` as newly inserted |
| `error` | `text?` | set on failure (blocked, unparseable response, network error) |

**Partial-run semantics** (FR-008): if pagination is interrupted after collecting N pages
successfully and the N+1th fails, the run still calls `FinishSourceRunOk` with whatever was
collected (mirrors `DjinniAdapter.scrapeSubscription`'s "later page failing just ends
pagination with whatever was collected" behavior) rather than discarding it via
`FinishSourceRunError`.
