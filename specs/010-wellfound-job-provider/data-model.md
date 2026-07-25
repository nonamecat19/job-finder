# Phase 1 Data Model: Wellfound Job Source

No schema migration. This feature reuses existing tables/types unchanged; only new *values*
(a `wellfound` source key, its rows/records) are introduced, not new structures.

## Entities

### Job Source (existing table `job_sources`, existing Go type `sqlcgen.JobSource`)

One row identifies Wellfound, keyed exactly like every other source:

| Field | Value for Wellfound |
|---|---|
| `key` | `"wellfound"` |
| `kind` | `dto.SourceKindScrape` (`"scrape"`) — matches Indeed/DOU/Djinni/Glassdoor, not `"api"` (RemoteOK) |
| `enabled` | operator-controlled, defaults to disabled until a subscription is saved (existing convention) |
| `healthy` | set by `HealthCheck` / run outcomes |
| `config` | encrypted JSON blob; unused by Wellfound beyond what the registry already provides (no API key/credentials — see research.md R1) |

Row is upserted at startup via the existing `UpsertJobSource` call pattern in
`composeJobSources` (`cmd/server/compose.go`) — no manual migration or seed script needed to
make the source exist, matching Glassdoor/RemoteOK precedent.

### Source Subscription (existing table, existing `Subscription` flow)

| Field | Wellfound-specific behavior |
|---|---|
| `source_key` | `"wellfound"` |
| `url` | operator-pasted Wellfound search-results URL; validated by `validateWellfoundSubscriptionURL` (research.md R6) — rejects non-`wellfound.com`/`angel.co` hosts and single-job-posting URL shapes |

No new columns. Validation is Go-side logic keyed by `source_key`, same switch statement
already handling `"indeed"`/`"remoteok"`/`"glassdoor"`/`"jobleads"`.

### Normalized Job Listing (existing Go type `dto.NormalizedJob`, existing `jobs` table)

Fields populated by `WellfoundAdapter.Search`/`FetchDetail`, using only existing
`NormalizedJob` fields — no new columns (mapping detail: research.md R4):

| `NormalizedJob` field | Source |
|---|---|
| `SourceKey` | `"wellfound"` (constant) |
| `ExternalID` | stable per-listing identifier parsed from the card/detail-page URL or a data attribute (FR-003) |
| `Title` | listing title text; required — card skipped without it (protects SC-004) |
| `Company` | company name text; present even when the company has no public profile page, per the spec's corresponding edge case |
| `Location` | location text, `nil` when Wellfound doesn't publish one |
| `Remote` | derived from listing text (remote/hybrid/onsite markers), matching `glassdoorRemoteRe`-style detection |
| `SalaryRaw` | raw salary and/or equity text; `nil` when neither is published, populated with whichever of the two is present when only one is (spec edge case) |
| `URL` | canonical Wellfound listing detail-page URL |
| `Description` | summary text at ingestion, full text + qualifications after `FetchDetail` enrichment |
| `PostedAt` | resolved during enrichment (FR-009); best-effort at ingestion if present on the card |
| `Raw` | free-form map carrying an equity-vs-salary distinction flag and any other Wellfound-specific fields not worth promoting to typed columns |

### Source Run (existing table/type, existing run-recording mechanism)

No new fields. Wellfound runs use the existing outcome vocabulary (`succeeded`/`failed`/
`partial`), extended in *meaning* only insofar as `"failed"` now also covers the "blocked by
Wellfound" case for this source (FR-011) and a distinguishable "content returned but
unparseable" case (spec edge case "page structure changes upstream"), carrying a
human-readable reason string — same mechanism already used for Glassdoor's block/
parse-failure distinction.

## State Transitions

None beyond what already exists for every source: `enabled` toggled by operator action;
`healthy` flips based on `HealthCheck`/run outcomes; a run moves through
not-started → running → (succeeded | partial | failed), identical to Glassdoor/Indeed. The one
behavior this feature adds within that lifecycle: a listing enriched from a session-gated or
removed detail page transitions to `Available: false` while its already-ingested summary data
is preserved rather than discarded (FR-009's second acceptance scenario), mirroring Glassdoor's
existing "listing no longer available" enrichment outcome.

## Validation Rules

- A saved Wellfound subscription URL MUST resolve to host `wellfound.com` (or a
  `*.wellfound.com` subdomain), or the legacy `angel.co` host (FR-015).
- A saved Wellfound subscription URL MUST NOT match a single-job-posting path shape — it must
  look like a search-results page (FR-015, mirrors Glassdoor/Indeed's equivalent check).
- Ingested listings with an empty `Title` or `URL` are dropped at parse time, matching every
  existing adapter's defensive-parsing convention (protects SC-004).
- A listing with neither salary nor equity text present is still ingested with `SalaryRaw`
  `nil`, not dropped (spec edge case).
