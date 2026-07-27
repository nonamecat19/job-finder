# Phase 1 Data Model: Throttle-Only Rate Control

**Feature**: `017-throttle-only-rate-control` | **Date**: 2026-07-28

## Overview

One persisted entity changes (`host_retrieval_state`, losing three columns), and one derived
entity is introduced (`HostPacing`, computed at request time, never stored). No new tables.

---

## Entity: `host_retrieval_state` (persisted, modified)

What the system remembers about one job-board host.

### Columns removed

| Column | Type | Why it goes |
|---|---|---|
| `budget_period_start` | `TIMESTAMPTZ` | Tracks the 24h window for the cap being removed (FR-003) |
| `budget_used` | `INTEGER NOT NULL DEFAULT 0` | Cumulative request counter for the cap being removed (FR-003) |
| `budget_limit` | `INTEGER NOT NULL DEFAULT 200` | The cap itself (FR-001) |

Dropped, not nullified. Historical usage counters carry no user value (spec Assumptions).
Migration `00029` drops them; its `Down` re-adds them with their original types and defaults
so a rollback restores a schema the previous binary can run against.

### Columns retained

| Column | Type | Role after this change |
|---|---|---|
| `id` | `UUID PK` | unchanged |
| `host` | `TEXT UNIQUE NOT NULL` | identity; the key pacing is enforced against (FR-008) |
| `identity_version` | `TEXT NOT NULL` | unchanged |
| `current_rung` | `TEXT NOT NULL` | unchanged; retrieval ladder is out of scope |
| `rung_last_verified_at` | `TIMESTAMPTZ` | unchanged |
| `cookies` | `JSONB` | unchanged; encrypted at rest |
| `consecutive_blocks` | `INTEGER NOT NULL` | unchanged; feeds cooling-off (FR-011) |
| `cooling_off_until` | `TIMESTAMPTZ` | unchanged; cooling-off survives (FR-011) |
| `last_block_at` | `TIMESTAMPTZ` | unchanged |
| `last_block_reason` | `TEXT` | unchanged |
| `crawl_delay_seconds` | `INTEGER` | **promoted from dead data to a pacing input** (FR-009) |
| `created_at` / `updated_at` | `TIMESTAMPTZ NOT NULL` | unchanged |

### `crawl_delay_seconds` semantics

This column changes meaning by finally acquiring a reader. Its tri-state must be handled
exactly:

| Value | Meaning | Pacing effect |
|---|---|---|
| `NULL` | never looked | triggers lazy discovery on first contact; default rate applies meanwhile |
| `0` | looked, host advertises nothing usable | **no signal** — default rate applies |
| `N > 0` | host advertises `Crawl-delay: N` | candidate rate `1/N`, applied only if slower than default |

The `0` case is the hazard: `parseCrawlDelay` returns `0` for absent, malformed, zero, and
negative delays alike, and the setter persists that `0`. It must never be read as "zero
seconds between requests."

### Validation rules

- `crawl_delay_seconds` is written only by the discovery path, and only when currently `NULL`.
- No column may express a request quota, allowance, or counting period (FR-003).
- Rows written before migration `00029` remain valid; dropping columns cannot orphan a host
  or make it unfetchable (FR-005).

---

## Entity: `HostPacing` (derived, not persisted)

The rate currently in force for a host and where it came from. Computed on demand; has no
table and no cache beyond the resolver's short TTL.

### Fields

| Field | Type | Description |
|---|---|---|
| `host` | string | destination host, keyed as `req.URL.Host` |
| `requestsPerSecond` | float | effective steady-state rate before jitter |
| `intervalSeconds` | float | `1 / requestsPerSecond`; the user-facing form (FR-016) |
| `source` | enum | provenance of the rate |

### `source` enum

| Value | Meaning |
|---|---|
| `default` | system-wide conservative rate; no host-specific signal |
| `site-requested` | derived from the host's advertised `Crawl-delay` |
| `override` | explicit operator-configured per-host rate |

The enum is what makes the display answer "why is this slow?" without support contact (SC-003).

### Resolution rules

Applied in order; first match wins.

1. `override` present for this host → use it.
2. `crawl_delay_seconds` is `N > 0` **and** `1/N < default rate` → `site-requested` at `1/N`.
3. Otherwise → `default`.

Rule 2's inequality is deliberate: a crawl delay *faster* than the default is ignored. A board
permitting one request per second has not asked to be hit that hard.

### Invariants

- Always resolvable. Every host yields a pacing value; there is no "unpaced" state for a
  third-party host (FR-007).
- Never a failure. Pacing produces a wait, never an outcome, error, or status (FR-010, FR-017).
- Per host, not per source. Sources sharing a host share one rate (FR-008).
- Loopback exempt. Local services are not third-party hosts and are not paced.

---

## Entity: `HostRetrievalStatusDto` (contract, modified)

Wire shape for the host status view. Full contract in
[`contracts/host-retrieval-status.md`](contracts/host-retrieval-status.md).

**Removed**: `budgetUsed`, `budgetLimit`, `budgetResetsAt` (FR-014).

**Added**: `pacing` — a `HostPacing` projection carrying `requestsPerSecond`,
`intervalSeconds`, and `source`.

**Retained**: `host`, `identityVersion`, `currentRung`, `lastBlockAt`, `lastBlockReason`,
`coolingOffUntil`, `crawlDelaySeconds`.

`crawlDelaySeconds` stays as a raw diagnostic even though `pacing.source` now conveys the same
fact in usable form; it is the unrounded input, and it costs nothing.

Per Constitution III this type is generated from `apps/api/internal/dto/jobs.go` via tygo.
The hand-maintained duplicate at `packages/shared/src/index.ts` must be edited in step — see
plan Complexity Tracking.

---

## Entity: `Run outcome` (semantics changed, shape unchanged)

No structural change. `PageDeferred` loses two of its three producers (budget exhaustion and
the deduct race), leaving cooling-off as the only one.

Consequence: a `deferred` outcome now unambiguously means "this host repeatedly refused us and
we are backing off" — a genuine block-family event that continues to be reported as one. A run
delayed purely by pacing produces no outcome marker and is reported as successful (FR-018).

---

## State transitions

### Pacing (per host, in-memory)

```
no limiter ──first request──> limiter created at resolved rate
     ^                                    │
     │                              TTL expiry
     └────rate re-resolved────────────────┘
```

Crawl delay discovered mid-session takes effect at the next TTL boundary; no restart needed.

### Crawl-delay discovery (per host, persisted)

```
NULL ──first contact──> robots.txt fetched ──> 0  (nothing advertised, default applies)
                                          └──> N>0 (advertised, may slow pacing)
```

Terminal once written. Discovery runs out of band; no user-facing fetch waits on it.

### Cooling-off (unchanged, documented for boundary clarity)

```
healthy ──consecutive_blocks >= threshold──> cooling off until T (doubling per extension)
   ^                                                  │
   └──────────────now() > T, or operator override─────┘
```

Driven entirely by observed blocks. No relationship to request volume, before or after this
change — which is precisely why it survives the removal.
