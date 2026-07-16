# Phase 1 Data Model: work.ua Adapter

**No new persisted entities. No schema change. No migration.** This feature maps work.ua HTML onto types that already exist. This document is the field-level mapping.

## Reused existing types

### `dto.NormalizedJob` (`apps/api/internal/dto/dto.go`)

What the adapter emits per listing. Unchanged.

| Field | Type | work.ua source | Notes |
|---|---|---|---|
| `SourceKey` | `string` | literal `"workua"` | Adapter's `Key()`. |
| `ExternalID` | `*string` | numeric segment of `/jobs/{id}/`, fallback card `data-id` | Drives dedup (FR-004). `nil` if unparseable. |
| `Title` | `string` | list: `h2 a`; detail: `h1#h1-name` | Card skipped entirely if empty (matches djinni). |
| `Company` | `string` | `a[href^="/jobs/by-company/"]` | Falls back to `"Unknown"` per FR-006 / edge case. |
| `Location` | `*string` | card location text | `nil` when absent. |
| `Remote` | `bool` | regex `remote\|віддалено\|дистанційно` over card text | See research Decision 3: the board's `/jobs-remote/` filter drives *search*, this regex drives the *per-job flag*. |
| `SalaryRaw` | `*string` | `li` containing `span.glyphicon-hryvnia-fill` | Raw string only; no min/max parse. |
| `URL` | `string` | `href` resolved against `https://www.work.ua` | Absolute. |
| `Description` | `string` | list: card teaser; detail: `#job-description` | Teaser at discovery, full text after enrichment (FR-010). |
| `PostedAt` | `*string` | detail page `time[datetime]` | **Must be normalised, not passed through.** work.ua emits `datetime="2026-07-16 02:29:02"` — space separator, no timezone. `dbutil.TimestampFromPtr` accepts only RFC3339 or date-only `2006-01-02`, and returns a zero timestamp **silently** on anything else. Parse with layout `"2006-01-02 15:04:05"`, emit RFC3339. `nil` when absent or unparseable. |
| `Raw` | `any` | `map[string]string{"html": ...}` | Truncated with `strutil.Truncate` — rune-safe, never a byte slice (FR-005). |

### `dto.SearchQuery` — consumed, unchanged

| Field | Used? | Behavior |
|---|---|---|
| `Keywords` | Yes | → `?search={urlencoded}` |
| `Remote` | Yes | when `true` → `/jobs-remote/` path |
| `SubscriptionURL` | Yes | when set, paginate that URL instead of keyword search (FR-012) |
| `Location`, `SalaryMin`, `Country`, `Site`, `Sources` | No | Not supported by this adapter; ignored. |

### `dto.SourceKind` — reused value

`SourceKindScrape` (`"scrape"`). No new enum value, so no tygo regeneration (Principle III).

## New type (in-process only, not persisted)

### `WorkUaDetailPatch`

Mirrors `DjinniDetailPatch`. Returned by `FetchDetail`, consumed only by the enrichment handler. Not part of the `Adapter` interface — adapter-specific, called directly, exactly as djinni/dou do it.

| Field | Type | Maps to `UpdateJobDetailParams` |
|---|---|---|
| `Description` | `string` | `Description` |
| `SalaryRaw` | `*string` | `SalaryRaw` |
| `Location` | `*string` | `Location` |
| `Remote` | `bool` | `Remote` |
| `PostedAt` | `*string` | `PostedAt` (via `dbutil.TimestampFromPtr`) — RFC3339 only; see normalisation note above |
| `Raw` | `map[string]string` | `Raw` (JSON-marshalled) |

The existing `UpdateJobDetail` query (`apps/api/internal/db/queries/job.sql:29`) already accepts precisely this set — no query change.

## Persisted rows touched (all pre-existing)

| Table | Interaction |
|---|---|
| `job_source` | One row, key `workua`, created lazily on first `GetByKey` — same path as every other adapter (see `apps/api/cmd/seed/main.go`). |
| `job` | Insert on discovery; `UpdateJobDetail` on enrichment. `DetailScrapedAt` guards re-enrichment. |
| `subscription` | Reused as-is for Story 4; rows carry `sourceKey = "workua"`. |
| `source_run` | Reused as-is; records per-run success/failure for FR-008/FR-009. |

## Validation rules (from spec requirements)

- **FR-004**: `ExternalID` must be stable across runs → derive from the URL's numeric id, never from list position.
- **FR-005**: any truncation of Cyrillic content must be rune-safe → `strutil.Truncate` only.
- **FR-006**: a missing `SalaryRaw`/`Location`/`PostedAt` yields `nil`, never a run failure; a missing `Company` yields `"Unknown"`.
- **FR-010**: `PostedAt` must round-trip to a `Valid: true` `pgtype.Timestamp`. Assert this explicitly — `dbutil.TimestampFromPtr` fails silently, so a broken date is invisible without a direct test.
- **FR-007**: zero parsed cards → return `(nil, nil)` plus a `slog.Warn` distinguishing "no matches" from "markup may have changed". Never an error.
- **FR-013**: a malformed `SubscriptionURL` → error naming the offending URL (mirrors `djinni.go:86`).
