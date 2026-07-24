# Phase 1 Data Model: Glassdoor Job Source

No schema migration. This feature reuses existing tables/types unchanged; only new *values*
(a `glassdoor` source key, its rows/records) are introduced, not new structures.

## Entities

### Job Source (existing table `job_sources`, existing Go type `sqlcgen.JobSource`)

One row identifies Glassdoor, keyed exactly like every other source:

| Field | Value for Glassdoor |
|---|---|
| `key` | `"glassdoor"` |
| `kind` | `dto.SourceKindScrape` (`"scrape"`) — matches Indeed/DOU/Djinni, not `"api"` (RemoteOK) or `"sidecar"` (JobSpy) |
| `enabled` | operator-controlled, defaults to disabled until a subscription is saved (existing convention) |
| `healthy` | set by `HealthCheck` / run outcomes |
| `config` | encrypted JSON blob; unused by Glassdoor beyond what the registry already provides (no API key/credentials) |

Row is upserted at startup via the existing `UpsertJobSource` call pattern in
`composeJobSources` (`cmd/server/compose.go`) — no manual migration or seed script needed to
make the source exist, matching RemoteOK precedent.

### Source Subscription (existing table, existing `Subscription` flow)

| Field | Glassdoor-specific behavior |
|---|---|
| `source_key` | `"glassdoor"` |
| `url` | operator-pasted Glassdoor search-results URL; validated by `validateGlassdoorSubscriptionURL` (research.md R6) — rejects non-`glassdoor.com` hosts and single-job-posting URL shapes |

No new columns. Validation is Go-side logic keyed by `source_key`, same switch statement
already handling `"indeed"`/`"remoteok"`.

### Normalized Job Listing (existing Go type `dto.NormalizedJob`, existing `jobs` table)

Fields populated by `GlassdoorAdapter.Search`/`FetchDetail`, using only existing
`NormalizedJob` fields — no new columns:

| `NormalizedJob` field | Source |
|---|---|
| `SourceKey` | `"glassdoor"` (constant) |
| `ExternalID` | the card's `data-jobid` attribute (a stable numeric ID Glassdoor attaches directly to each `[data-test="jobListing"]` element — confirmed live, research.md R3) |
| `Title` | `[data-test="job-title"]` text |
| `Company` | employer name text (near `[data-test="job-title"]`, company-profile block) |
| `Location` | `[data-test="emp-location"]` text, `nil` when Glassdoor doesn't publish one |
| `Remote` | derived from listing text (e.g. "Remote"/"Work from home" markers), matching `indeedRemoteRe`-style detection — not always-true like RemoteOK, since Glassdoor lists both remote and on-site roles |
| `SalaryRaw` | `[data-test="detailSalary"]` text, `nil` when absent (research.md R4) |
| `URL` | canonical detail-page URL, shape `https://www.glassdoor.com/job-listing/<slug>-JV_<...>.htm?jl=<data-jobid>` (confirmed live) |
| `Description` | summary text at ingestion, full text after `FetchDetail` enrichment |
| `PostedAt` | resolved from `[data-test="job-age"]` relative-time text (e.g. "13d"), parsed to a timestamp when possible |
| `Raw` | free-form map carrying the estimate-vs-employer-stated salary flag, the employer star rating (a `[class*="RatingText"]` element, when present — informational only, distinct from and not a substitute for `companyintel`'s `KindGlassdoorRating`, research.md R4), and any other Glassdoor-specific fields not worth promoting to typed columns |

### Source Run (existing table/type, existing run-recording mechanism)

No new fields. Glassdoor runs use the existing outcome vocabulary (`succeeded`/`failed`/
`partial`), extended in *meaning* only insofar as `"failed"` now also covers the "blocked by
Glassdoor" case for this source, carrying a human-readable reason string (FR-011, FR-018) —
same mechanism already used for Indeed's block/parse-failure distinction.

## State Transitions

None beyond what already exists for every source: `enabled` toggled by operator action;
`healthy` flips based on `HealthCheck`/run outcomes; a run moves through
not-started → running → (succeeded | partial | failed), identical to Indeed/RemoteOK.

## Validation Rules

- A saved Glassdoor subscription URL MUST resolve to host `glassdoor.com` or a
  `*.glassdoor.com` subdomain (FR-015).
- A saved Glassdoor subscription URL MUST NOT match a single-job-posting path shape — it must
  look like a search-results page (FR-015, mirrors Indeed's equivalent check).
- Ingested listings with an empty `Title` or `URL` are dropped at parse time, matching every
  existing adapter's defensive-parsing convention (protects SC-004).
