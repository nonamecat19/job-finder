# Phase 1 Data Model: Himalayas Job Source

No new tables or columns. This feature reuses the existing job-source data model exactly as
RemoteOK/Arbeitnow/Remotive do — Himalayas is a new *value* in existing entities, not a new entity.
Unlike JobLeads/Djinni, Himalayas's `config` JSON stays empty: there is no session cookie or any
other derived state to persist (research.md R1).

## Job Source (existing `job_sources` table)

A new row with `key = 'himalayas'`, `kind = 'api'` (see
[research.md#r2-fetch-mechanism--undocumented-public-json-feed-not-html-scrape](./research.md#r2-fetch-mechanism--undocumented-public-json-feed-not-html-scrape)),
following the same shape as the `remoteok`/`arbeitnow`/`remotive` rows: enabled flag, health state,
last-run summary. The `config` JSON column has no Himalayas-specific keys — no credentials, no
session, nothing to encrypt.

## Source Subscription (existing `subscriptions` table)

A subscription row with `source_key = 'himalayas'` and `url` holding a Himalayas `/jobs` search-page
URL (e.g. `https://www.himalayas.app/jobs?categories=Backend-Engineering&timezones=UTC-5,UTC-8`).
Validated at save time by `validateSubscriptionURL` (host must be `himalayas.app` or a
`.himalayas.app` subdomain, path must be `/jobs` or a `/jobs/...` prefix, and a non-empty
`categories` query parameter must be present); rejected otherwise with a stated reason (FR-015,
FR-016). At `Search` time, the adapter parses `categories` (comma-separated slugs) and the optional
`timezones` parameter out of this URL — it never fetches the URL itself (that URL renders an HTML
page); only `https://himalayas.app/jobs/api` is fetched (research.md R2, R4).

## Normalized Job Listing (existing `jobs` table, via `dto.NormalizedJob`)

Field mapping from Himalayas's `/jobs/api` JSON is documented in
[research.md#r5-job-field-mapping](./research.md#r5-job-field-mapping). Key points that affect
stored data:

- `ExternalID` and `URL` both come from `guid` (a canonical, stable per-listing URL), used for
  cross-run dedup (FR-004) exactly like the other sources' `ExternalID` usage.
- `Description` is populated **in full** directly from `Search` — Himalayas's feed does not have a
  separate summary/detail split at the data-model level the way Indeed/Glassdoor/JobLeads do; there
  is no later enrichment step to complete it (research.md R6). `Description` is therefore complete
  from the moment a Himalayas job is first ingested.
- `Remote` is always `true` — Himalayas is a remote-only board, per the spec's Assumptions; no
  text-based derivation like Djinni/Glassdoor's `*RemoteRe` patterns is needed.
- `Location` is derived from `locationRestrictions` (an array of permitted countries/regions, often
  empty meaning "worldwide"); empty array maps to `nil` (spec's "no explicit location" edge case is
  not really applicable here since Himalayas is remote-only, but the field can still be empty).
- `SalaryRaw` is derived from `minSalary`/`maxSalary`/`currency`/`salaryPeriod`; all-zero/absent maps
  to `nil` (spec's "no salary range published" edge case).
- `PostedAt` is derived from `pubDate`, a Unix-seconds integer (not ISO text like every other
  source) — converted to RFC3339 UTC.
- `Raw` carries `timezoneRestrictions` (folded into descriptive text per the spec's "listing
  restricts applicants to a specific timezone band" edge case), plus `categories`,
  `parentCategories`, `seniority`, and `employmentType` — source-specific extras not yet promoted to
  first-class `NormalizedJob` fields, matching how other adapters stash source-specific extras.

## Source Run (existing `source_runs` table)

A run record with `source_key = 'himalayas'`, populated identically to other sources: outcome
(`succeeded`/`failed`/`partial`), listings found, listings newly added, failure reason. The
"response shape changes upstream" edge case surfaces here as a distinguishable failure reason (JSON
decode failure on the `/jobs/api` body) — a string value within the existing `failure_reason`
field, not a new column or state (FR-011).

## State transitions

None beyond the existing source/subscription/run lifecycle already implemented for
RemoteOK/Arbeitnow/Remotive (enable ↔ disable; run outcome succeeded/failed/partial). Unlike
Djinni/JobLeads, there is no session-expiry state to transition through (research.md R1), and
unlike Indeed/Glassdoor/RemoteOK, there is no ingested-then-enriched transition for a Himalayas
job — a Himalayas `Job` row is created complete and never has a pending "detail not yet fetched"
state (research.md R6).
