# Phase 1 Data Model: JobLeads Job Source

No new tables or columns. This feature reuses the existing job-source data model exactly as
DOU, Djinni, Indeed, and RemoteOK do — JobLeads is a new *value* in existing entities, not a
new entity. The only nuance versus RemoteOK is that JobLeads's `config` JSON is not empty: it
carries the login-derived session cookie, exactly like Djinni's.

## Job Source (existing `job_sources` table)

A new row with `key = 'jobleads'`, `kind = 'scrape'` (see
[research.md#R3](./research.md#r3-fetch-mechanism--html-scrape-vs-api)), following the same
shape as the `djinni`/`indeed`/`dou` rows: enabled flag, health state, last-run summary. The
`config` JSON column holds `{"sessionCookie": "<value>"}` once a login has succeeded —
identical in mechanism to Djinni's config, and encrypted at rest by the existing
`jobsources.Service` encrypt-on-write path (no new crypto, see
[research.md#R2](./research.md#r2-credential-storage)). Raw email/password are never stored
in this row or any DB row; they live only in `JOBLEADS_EMAIL`/`JOBLEADS_PASSWORD` env vars.

## Source Subscription (existing `subscriptions` table)

A subscription row with `source_key = 'jobleads'` and `url` holding a JobLeads saved-search
URL (e.g. `https://www.jobleads.com/...`). Validated at save time by
`validateSubscriptionURL` (host must be `jobleads.com` or a `.jobleads.com` subdomain);
rejected otherwise with a stated reason (FR-015, FR-016).

## Source Credentials (env vars, not a DB table)

`JOBLEADS_EMAIL` / `JOBLEADS_PASSWORD` — operator-supplied, read once at process start via
`config.go` (mapstructure), passed into the `JobLeadsSession` at construction time in
`compose.go`, and registered in `config.go`'s secret-list so they're redacted from any config
dump/logging path, mirroring `DJINNI_EMAIL`/`DJINNI_PASSWORD`. Not persisted to Postgres; only
the *derived* session cookie is persisted (see Job Source above), satisfying FR-018's "stored
securely, not exposed in logs or UI" without introducing a new secrets mechanism.

## Normalized Job Listing (existing `jobs` table, via `dto.NormalizedJob`)

Field mapping from JobLeads's HTML is documented in
[research.md#R5](./research.md#r5-job-field-mapping). Key points that affect stored data:

- `ExternalID` is derived from the listing's detail-page URL/path, used for cross-run dedup
  (FR-004) exactly like the other sources' `ExternalID` usage.
- `Description` starts as a list-level summary at ingestion and is completed to full text by
  enrichment (`FetchDetail`), matching Indeed's summary→full-description split rather than
  RemoteOK's all-at-once population.
- `Remote` is best-effort from listing text (JobLeads is not remote-only, unlike RemoteOK),
  following Djinni's `djinniRemoteRe`-style convention.
- `Raw` carries any JobLeads-specific fields not yet promoted to first-class `NormalizedJob`
  fields, matching how other adapters stash source-specific extras.

## Source Run (existing `source_runs` table)

A run record with `source_key = 'jobleads'`, populated identically to other sources: outcome
(`succeeded`/`failed`/`partial`), listings found, listings newly added, failure reason. The
failure reason vocabulary gains one JobLeads-relevant value in practice —
"authentication required" (session expired/credentials invalid, see
[research.md#R4](./research.md#r4-login-page--session-expiry-detection)) — but this is a
string value within the existing `failure_reason` field, not a new column or state (FR-011).

## State transitions

None beyond the existing source/subscription/run lifecycle already implemented for
DOU/Djinni/Indeed/RemoteOK (enable ↔ disable; run outcome succeeded/failed/partial; listing
ingested → optionally enriched → optionally marked unavailable). The one behavior this
feature adds within that lifecycle: a run transitioning to `failed` specifically because the
stored session is no longer valid, and a subsequent successful `Session.Refresh` clearing that
state on the next run — mirroring Djinni's existing session-refresh behavior, not a new state
machine.
