# Phase 1 Data Model: Jobgether Job Source

No new tables or columns. This feature reuses the existing job-source data model exactly as
Glassdoor does — Jobgether is a new *value* in existing entities, not a new entity. Unlike
JobLeads/Djinni, Jobgether's `config` JSON stays empty (no session/credentials), matching
Glassdoor/RemoteOK/Indeed.

## Job Source (existing `job_sources` table)

A new row with `key = 'jobgether'`, `kind = 'scrape'` (see
[research.md#R2](./research.md#r2-fetch-mechanism--html-scrape-vs-api)), following the same
shape as the `glassdoor`/`indeed`/`dou` rows: enabled flag, health state, last-run summary. No
credentials, no session cookie — `config` is empty JSON (`{}`) unless/until an operator patches
it, mirroring Glassdoor exactly.

## Source Subscription (existing `subscriptions` table)

A subscription row with `source_key = 'jobgether'` and `url` holding a Jobgether
search-results URL (e.g. `https://jobgether.com/remote-jobs?...`). Validated at save time by
`validateSubscriptionURL` (host must be `jobgether.com` or a `.jobgether.com` subdomain, and
the path must look like a search-results page rather than a single job-detail page); rejected
otherwise with a stated reason (FR-015, SC-008, see
[research.md#R6](./research.md#r6-subscription-configuration-shape)).

## Normalized Job Listing (existing `jobs` table, via `dto.NormalizedJob`)

Field mapping from Jobgether's HTML is documented in
[research.md#R4](./research.md#r4-job-field-mapping). Key points that affect stored data:

- `ExternalID` is derived from the listing's detail-page URL/path, used for cross-run dedup
  (FR-004) exactly like the other sources' `ExternalID` usage.
- `Description` starts as a list-level summary at ingestion and is completed to full text by
  enrichment (`FetchDetail`), matching Glassdoor/Indeed's summary→full-description split.
- `Remote` is best-effort from listing text; Jobgether is remote-focused so most listings are
  expected to resolve `true`, but the field is still derived per-listing rather than hardcoded,
  since Jobgether may include hybrid/onsite-tagged postings.
- `SalaryRaw` is nil/empty when Jobgether does not publish a salary for a listing (spec's "no
  salary range published" edge case) — the listing is still ingested, never dropped for this
  reason alone.
- `Raw` carries Jobgether-specific fields not promoted to first-class `NormalizedJob` fields,
  most notably `jobgetherMatchScore` (Jobgether's own AI match-percentage score, when present)
  — stored as descriptive metadata only and never read by this product's matching/scoring
  pipeline (FR-012, edge case: "Jobgether's own AI match-percentage score ... MUST NOT replace
  or influence this product's own scoring").

## Source Run (existing `source_runs` table)

A run record with `source_key = 'jobgether'`, populated identically to other sources: outcome
(`succeeded`/`failed`/`partial`), listings found, listings newly added, failure reason. The
failure-reason vocabulary gains Jobgether-relevant values in practice — "blocked/rate-limited"
and "response could not be interpreted" (FR-011) — as string values within the existing
`failure_reason` field, not new columns or states.

## State transitions

None beyond the existing source/subscription/run lifecycle already implemented for
Glassdoor/Indeed/DOU/RemoteOK/Djinni/JobLeads (enable ↔ disable; run outcome
succeeded/failed/partial; listing ingested → optionally enriched → optionally marked
unavailable). This feature introduces no new state machine.
