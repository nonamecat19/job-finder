# Phase 1 Data Model: Employer ATS Board Sources

## EmployerBoard (the roster)

Registered employer board — one row per (vendor, employer identifier).

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | |
| `vendor` | text | one of `greenhouse`, `lever`, `ashby`, `workable`, `smartrecruiters` |
| `employerIdentifier` | text | the vendor-scoped token/slug in the board URL |
| `displayName` | text | employer name, resolved from board response or user input |
| `addedVia` | text | `proposed` \| `pasted` |
| `enabled` | boolean default true | |
| `lastSuccessAt` | timestamp nullable | |
| `lastPostingCount` | integer default 0 | postings found on last successful read |
| `consecutiveEmptyRuns` | integer default 0 | drives stale flag |
| `stale` | boolean, generated/computed at read time from `consecutiveEmptyRuns >= threshold` | surfaced via query, not stored redundantly — implemented as a view column or computed in the service layer |
| `createdAt` | timestamp default now() | |

Constraints: `UNIQUE(vendor, employerIdentifier)` (FR-013 — no duplicate roster entry).

Relationships: none by FK — `Job.sourceKey` stays vendor-level (`greenhouse`, not
`greenhouse:acme`); which employer a job came from is recovered from `Job.raw`/`url`, matching how
every existing source already stores vendor-specific detail in `raw`.

## BoardCandidate

Proposed, not-yet-registered board inferred from an existing job's apply URL.

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | |
| `vendor` | text | |
| `employerIdentifier` | text | |
| `inferredFromJobId` | uuid FK → `Job.id` | the listing the candidate was derived from |
| `state` | text | `proposed` \| `accepted` \| `rejected` |
| `createdAt` | timestamp default now() | |
| `decidedAt` | timestamp nullable | |

Constraints: `UNIQUE(vendor, employerIdentifier)` — one candidate row per employer regardless of
how many listings reference it; re-running discovery updates `inferredFromJobId` only if still
`proposed`. A `rejected` row is never re-proposed (FR-010) because discovery checks existing
`BoardCandidate` rows before inserting, not just `EmployerBoard`.

State transitions: `proposed → accepted` (creates/enables the matching `EmployerBoard` row,
FR-010 scenario 2) or `proposed → rejected` (terminal, FR-010 scenario 3). No transition out of
`accepted`/`rejected` — removing the employer later is a delete on `EmployerBoard`, independent of
the candidate record's history.

## Job table extension (merge support)

| Column | Type | Notes |
|---|---|---|
| `seenOnSources` | text[] default `{}` | every `sourceKey` this opening has been observed on; appended to on merge, not replaced (FR-016) |

`sourceKey`/`url` on `Job` continue to reflect the *current* preferred source (board over
aggregator per FR-016); `seenOnSources` is the append-only history. No other `Job` column changes
— `dedupeKey` stays as the first-seen dedupe key; a merge is recognized by the new similarity
check (research.md §4), not by `dedupeKey` collision, since the board URL never canonicalizes to
the same string as the aggregator URL.

## SourceRun (existing table, reused, no schema change)

One row per vendor per run, as today. `found`/`new` aggregate across all employers read in that
run. Per-employer detail (FR-020's five outcome kinds, FR-023's counts) is carried in a new JSONB
column:

| Column | Type | Notes |
|---|---|---|
| `employerDetail` | jsonb nullable | array of `{employerIdentifier, outcome, postingsFound, postingsNew}`; `outcome` ∈ `read`, `not_found`, `unreadable`, `refused`, `no_postings` (FR-020) |

`ok` stays boolean, computed as `employersReadSuccessfully > 0` (FR-021 — never `ok=true` with
zero successful employers, even if `error` is empty).

## Board Vendor (code-level entity, not a DB table)

Represented purely in Go as the five new `jobsources.Adapter` implementations
(`greenhouse.go`, `lever.go`, `ashby.go`, `workable.go`, `smartrecruiters.go`) plus a shared
`roster/urlmatch.go` mapping a URL to `(vendor, employerIdentifier)` — used by both candidate
discovery and by the "paste a URL" registration path (FR-011), so vendor recognition logic exists
exactly once.

## Validation rules

- `EmployerBoard.vendor` and `BoardCandidate.vendor` MUST be one of the five supported vendor
  keys; a paste of an unsupported vendor's URL is rejected before insert (Edge Cases).
- `EmployerBoard` insert (via paste, FR-011) MUST pass a live health check (one read call) before
  the row is committed — validation happens synchronously in the HTTP handler, not deferred to the
  next scheduled run.
- `Job.seenOnSources` always includes `Job.sourceKey`'s value at minimum (invariant, not enforced
  by a DB constraint — maintained by the merge code path).
