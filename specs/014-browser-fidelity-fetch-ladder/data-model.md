# Phase 1 Data Model: Browser-Fidelity Retrieval and Escalation Ladder

## BrowserIdentity (configured value, not persisted per-row)

Single process-wide configured value (FR-004), versioned so stored visitor state can be tied to
it (FR-006).

| Field | Type | Notes |
|---|---|---|
| `Version` | string | e.g. `"chrome-126"`; bumped whenever the underlying UA/header/TLS profile changes |
| `UserAgent` | string | Must agree with `Platform`/`TLSProfile` (FR-002) |
| `Platform` | string | e.g. `"Windows"` — drives `Sec-CH-UA-Platform` and related client hints |
| `Headers` | ordered []KV | Full header set + order a real instance of this browser sends |
| `TLSProfileID` | string | tls-client profile identifier matching `UserAgent`'s browser/version |

Validation: on process start, reject a `BrowserIdentity` whose `UserAgent` browser/version
doesn't match `TLSProfileID`'s browser/version (FR-002 "all agree").

## HostRetrievalState (new table, one row per host)

Per-host memory (FR-005, FR-013, FR-025, FR-026, FR-030), keyed by host, independent of any
single `JobSource` since a host can be shared by several sources (edge case) and a source can
span several hosts.

| Column | Type | Notes |
|---|---|---|
| `id` | uuid PK | |
| `host` | text, unique | e.g. `"www.glassdoor.com"` |
| `identityVersion` | text | `BrowserIdentity.Version` the stored cookies/rung pref were issued under |
| `currentRung` | text | one of the `RetrievalMethod` keys; last-succeeded method (FR-013) |
| `rungLastVerifiedAt` | timestamptz | drives periodic cheap-method re-test (FR-014) |
| `cookies` | jsonb, encrypted | serialized cookie jar + any challenge-clearance token (FR-005, FR-007) |
| `consecutiveBlocks` | integer, default 0 | reset to 0 on any successful read |
| `coolingOffUntil` | timestamptz, nullable | non-null while cooling off (FR-026) |
| `lastBlockAt` | timestamptz, nullable | (FR-025) |
| `lastBlockReason` | text, nullable | human-readable, matches `PageOutcome.Reason` (FR-025, FR-033) |
| `crawlDelaySeconds` | integer, nullable | parsed from the host's published crawl-delay/`Retry-After` (FR-028, FR-029) |
| `budgetPeriodStart` | timestamptz | start of the current daily budget window |
| `budgetUsed` | integer, default 0 | requests made against `budgetPeriodStart`'s window |
| `budgetLimit` | integer | configurable per host, default from a global setting (FR-030) |
| `updatedAt` | timestamptz | |

Relationships: none by foreign key — `host` is a free-standing key that adapters/sources
reference by the host portion of the URLs they fetch, not by joining to `JobSource`. This is
deliberate (edge case: "a block on one host does not silence the others" — a `JobSource` row
must never gate more than its own host).

State transitions:
- `currentRung` escalates (cheap → heavy) only after a `Challenged`/`Refused` outcome on the
  currently recorded rung; it is never escalated speculatively (FR-011).
- `currentRung` de-escalates only via the periodic re-test (FR-014) or an explicit operator
  clear (FR-015), never automatically mid-run.
- `coolingOffUntil` is set/extended on reaching a configurable `consecutiveBlocks` threshold
  (FR-026) and is left untouched by an operator override (FR-027) — the override bypasses the
  check for one on-demand run without resetting the stored expiry.
- `identityVersion` mismatch on read ⇒ `cookies` is treated as absent and discarded on next
  write (FR-006).

## RetrievalMethod (ladder rung — static/configured, not a DB entity)

| Key | Cost order | Availability |
|---|---|---|
| `direct` | 0 (cheapest) | always available |
| `browser` | 1 | always available (local chromedp) |
| `flaresolverr` | 2 (heaviest) | available iff `FLARESOLVERR_URL` is configured and the service responds to a health check |

Ordering is fixed and shared by every host; only the *starting point* (`HostRetrievalState.
currentRung`) and *ceiling reached this run* vary per host.

## PageOutcome (per-retrieval-attempt result, not persisted as its own table — carried in-run and folded into SourceRun reporting)

| Field | Type | Notes |
|---|---|---|
| `Status` | enum | `Read`, `Challenged`, `Refused`, `Unparseable`, `Deferred` (FR-022) |
| `Method` | RetrievalMethod key | which rung produced this outcome |
| `Reason` | string, nullable | populated for `Challenged`/`Refused`/`Deferred` (budget/cooling-off) |
| `URL` | string | the page attempted |

## RunVerdict (aggregate — extends existing `SourceRun`)

Existing `SourceRun` table (`apps/api/internal/db/migrations/00001_init.sql:96-105`) gains
columns rather than a new table, since it is already the per-run outcome log:

| New column | Type | Notes |
|---|---|---|
| `verdict` | text | `success` \| `partial` \| `blocked` (FR-021, FR-023) — distinct from `ok`/`found` which describe listing counts, not honesty |
| `blockedCount` | integer, default 0 | count of pages with a non-`Read` outcome this run (FR-023) |
| `blockReason` | text, nullable | set when `verdict = blocked` |

`ok` (existing boolean) becomes derived/redundant with `verdict != blocked` for backward
compatibility with existing dashboard reads, but `verdict` is the field new code and the
Sources-screen honesty surface reads (FR-021: "MUST NOT report a run as successful when its
pages were blocked").

## Zero-listings-without-block flag (FR-024)

No new table: computed at read time by comparing a source's last N `SourceRun.found` values
(N configurable) against `verdict`. Flagging state itself doesn't need persistence beyond what
`SourceRun` already stores — this is a derived signal, not new source-of-truth state.

## Dashboard-facing DTOs (packages/shared, generated/shared per Constitution III)

- `HostRetrievalStatusDto`: `{host, identityVersion, currentRung, lastBlockAt, lastBlockReason, coolingOffUntil, budgetUsed, budgetLimit, budgetResetsAt}` — read model for FR-033/FR-034.
- `PageOutcomeDto`: mirrors `PageOutcome` above, for any future per-page drill-down.
- `RunVerdictDto`: mirrors the new `SourceRun` columns, consumed by `RecentRunsPanel`.
