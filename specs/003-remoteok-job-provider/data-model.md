# Phase 1 Data Model: RemoteOK Job Source

No new tables or columns. This feature reuses the existing job-source data model exactly as
DOU, Djinni, and Indeed do — RemoteOK is a new *value* in existing entities, not a new
entity.

## Job Source (existing `job_sources` table)

A new row with `key = 'remoteok'`, `kind = 'api'` (see [research.md#R1](./research.md)),
following the same shape as the `indeed`/`dou`/`djinni` rows: enabled flag, health state,
last-run summary, optional config JSON (unused for now — configuration lives on
subscriptions, matching the Indeed precedent).

## Source Subscription (existing `subscriptions` table)

A subscription row with `source_key = 'remoteok'` and `url` holding either:
- `https://remoteok.com/remote-<tag>-jobs` (a RemoteOK tag/category listing page), or
- `https://remoteok.com/api` (the bare, untagged feed).

Validated at save time by `validateSubscriptionURL` (host must be `remoteok.com` or
`remoteok.io`); rejected otherwise with a stated reason (FR-015, FR-016).

## Normalized Job Listing (existing `jobs` table, via `dto.NormalizedJob`)

Field mapping from RemoteOK's JSON API is documented in
[research.md#R4](./research.md#r4-job-field-mapping). Key points that affect stored data:

- `Remote` is always `true` for every RemoteOK-sourced job (RemoteOK is remote-only).
- `ExternalID` is RemoteOK's own `id`, used for cross-run dedup (FR-004) exactly like the
  other sources' `ExternalID` usage.
- `Description` is populated in full at ingestion time (not summary-then-detail like
  Indeed), since RemoteOK's API returns full description text up front.
- `Raw` carries RemoteOK's `tags` array and any fields not yet promoted to first-class
  `NormalizedJob` fields, matching how other adapters stash source-specific extras.

## Source Run (existing `source_runs` table)

A run record with `source_key = 'remoteok'`, populated identically to other sources:
outcome (`succeeded`/`failed`/`partial`), listings found, listings newly added, failure
reason. No new fields required — FR-007, FR-008, FR-011 are satisfied by the existing run
record shape.

## State transitions

None beyond the existing source/subscription/run lifecycle already implemented for
DOU/Djinni/Indeed (enable ↔ disable; run outcome succeeded/failed/partial; listing
ingested → optionally enriched → optionally marked unavailable). This feature adds no new
states.
