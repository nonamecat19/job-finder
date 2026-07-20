# Feature Specification: Application Outcome Tracking + Post-Age Response Signal

**Feature Branch**: `010-application-outcome`

**Created**: 2026-07-20

**Status**: Draft

**Migration**: `00012_application_outcome.sql`

**Input**: Director task el-2qgn (010-1). Spec two coupled things: (a) capturing
application outcomes with timestamps, which the codebase does not track today,
and (b) a post-age vs response-rate signal built on top of that outcome data.
Be explicit that until enough outcome data accrues, the feature shows a
documented prior or "not enough data" rather than a fabricated rate.

**Downstream**: el-5s6h (010-2) implements the migration, sqlc queries, and the
status-change write path this spec defines. This document is spec-only — **no
code in this task**.

---

## Why this exists

Today an `"Application"` row carries a single mutable `"status"` string
(`shortlisted` → `applied` → …) and an untyped `"events"` jsonb blob
(`apps/api/internal/db/migrations/00001_init.sql`). Two problems:

1. **No queryable history with timestamps.** `"status"` holds only the *current*
   state; the previous states and *when* each transition happened are lost or
   buried in an unstructured jsonb array that no aggregate query can read. You
   cannot ask "how long after applying did a rejection arrive" or "how many
   applications got any response at all".
2. **No signal can be built on it.** Any response-rate or timing analysis needs
   an ordered, typed, timestamped event stream per application. The jsonb blob
   is not that.

This feature adds a first-class, append-only **outcome event log** and one
**derived timing signal** — post-age-at-apply vs response-rate — that tells the
user whether applying to fresher postings actually correlates with getting a
reply.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Record what happened to an application, and when (Priority: P1)

The user moves an application through its lifecycle: they apply, the employer
views it, a screen is scheduled, then an offer or a rejection lands. Each of
those transitions is recorded as a distinct, timestamped event — not just an
overwrite of a status field. The application's timeline can be read back in order.

**Why this priority**: Nothing else in this feature exists without the event log.
The timing signal (Story 2) and every future outcome analytic read from it. It
ships alone and is immediately useful: an honest, ordered history of each application.

**Independent Test**: Move an application `applied → screen → rejected`, then read
the application back and confirm three events exist, each with its own timestamp,
in the order they occurred.

**Acceptance Scenarios**:

1. **Given** an application in `shortlisted`, **When** its status changes to
   `applied`, **Then** an `applied` outcome event is written with the transition
   timestamp and the application's `"appliedAt"` is set.
2. **Given** an application that has already recorded `applied`, **When** the
   employer views it and status advances to a viewed state, **Then** a `viewed`
   event is appended without destroying the earlier `applied` event.
3. **Given** an application with several recorded events, **When** its timeline is
   read, **Then** events return ordered by their event timestamp, oldest first.
4. **Given** the same status change submitted twice with no new information,
   **When** the write path runs, **Then** a duplicate event is not created
   (idempotent on `(applicationId, eventType)` for the terminal-once events;
   see Outcome Event Model).
5. **Given** any status change, **When** the event is written, **Then** the
   `"Application"."status"` current-state field is still updated too — the log is
   additive, it does not replace the fast current-state read.

---

### User Story 2 — See whether applying to fresh postings gets more replies (Priority: P2)

On an insights view the user sees their applications bucketed by **how old the
posting was at the moment they applied** (post-age-at-apply), and the **response
rate** within each bucket. The user learns, from their own history, whether
jumping on a day-old posting beats applying to a three-week-old one.

**Why this priority**: This is the payoff signal, but it is worthless without
Story 1's event log and dangerous without Story 3's honesty rules. Ships after
both.

**Independent Test**: With a seeded set of applications spanning several post-age
buckets and known outcomes, load the signal and confirm each bucket's response
rate equals responses/applications for that bucket.

**Acceptance Scenarios**:

1. **Given** applications with recorded `applied` events and known posting ages,
   **When** the signal computes, **Then** each application is placed in a post-age
   bucket by `appliedAt − Job."postedAt"`.
2. **Given** a bucket with enough applications (≥ the minimum sample), **When** the
   signal renders, **Then** it shows that bucket's observed response rate =
   (applications with any response event) / (applications in bucket).
3. **Given** a bucket below the minimum sample, **When** the signal renders,
   **Then** that bucket shows "not enough data" — never a computed rate from a
   tiny sample (see Story 3).
4. **Given** a posting with no `"postedAt"` (null), **When** bucketing runs,
   **Then** the application is placed in an explicit "unknown age" bucket, never
   silently dropped and never assumed age-zero.
5. **Given** the signal, **When** it renders any rate, **Then** it also shows the
   sample size (n) behind that rate so the user can judge its weight.

---

### User Story 3 — Never be shown a fabricated rate (Priority: P1, cross-cutting)

Before the user has applied to enough jobs, the feature refuses to invent a
number. Instead it either shows a clearly-labelled **prior** (a documented
baseline expectation) or an explicit **"not enough data yet"** state — and it
tells the user which of the two they are looking at.

**Why this priority**: Same rank as Story 1. A job-search tool that fabricates
"you have a 22% response rate" from four applications actively harms the user's
decisions and destroys trust. This is a correctness requirement, not polish.

**Independent Test**: With fewer than the global minimum recorded outcomes, load
the signal and confirm no per-bucket observed rate is presented as fact; the
prior or the "not enough data" state is shown and labelled as such.

**Acceptance Scenarios**:

1. **Given** total recorded outcomes below the global cold-start threshold,
   **When** the signal loads, **Then** it shows the documented prior labelled as
   a baseline (not the user's own rate) OR an explicit "not enough data yet"
   message — and states which.
2. **Given** a per-bucket sample below the bucket minimum, **When** that bucket
   renders, **Then** it shows "not enough data" for that bucket even if other
   buckets have enough.
3. **Given** any displayed rate, **When** it is a prior rather than observed,
   **Then** the UI never labels it as the user's personal/observed rate.
4. **Given** enough data accrues to cross a threshold, **When** the signal
   reloads, **Then** it switches from prior/"not enough" to the observed rate
   without any code change — purely data-driven.

---

### Edge Cases

- **Out-of-order / regressing status** (e.g. `offer` then back to `screen`, or a
  correction): every submitted transition is still appended as its own event with
  its own timestamp. The event log is the truth; `"status"` reflects the latest.
  History is never rewritten to look linear.
- **An application with zero response events** (applied, then silence) counts in
  its bucket's denominator as a *non-response* — silence is data. It is never
  excluded, which would inflate the rate.
- **"Ghosted"/no-response is derived, not stored.** There is no `ghosted` event;
  no-response = an application whose latest event after `applied` is still
  `applied` past a documented staleness window. Deriving it keeps the event log
  to real observed transitions only.
- **`postedAt` in the future or after `appliedAt`** (bad scrape data) → clamp to
  the "unknown age" bucket rather than producing a negative age.
- **Deleting an `"Application"`** cascades to its outcome events (they are
  meaningless without their application).

---

## Outcome Event Model

One new table, `"ApplicationOutcome"` — an append-only event log, one row per
outcome event. Migration `00012_application_outcome.sql` (goose, PostgreSQL,
quoted-PascalCase table + quoted-camelCase columns to match
`apps/api/internal/db/migrations/00001_init.sql`; `uuid` PK via
`gen_random_uuid()`; `timestamp (3)`).

### Why a table, not the existing `"events"` jsonb

The `"Application"."events"` jsonb column stays for any UI free-form annotation,
but it is **not** the outcome record. A jsonb blob cannot be indexed, ordered, or
aggregated across applications, which is exactly what the timing signal needs.
The outcome log is relational so `GROUP BY` bucket/response is a plain SQL query.

### `"ApplicationOutcome"`

| Column | Type | Null? | Meaning |
|---|---|---|---|
| `"id"` | `uuid` PK default `gen_random_uuid()` | no | |
| `"applicationId"` | `uuid` | no | FK → `"Application"."id"` ON DELETE cascade |
| `"eventType"` | `text` | no | `CHECK` in the enum below |
| `"occurredAt"` | `timestamp (3)` | no | when the real-world event happened (may be back-dated by the user) |
| `"recordedAt"` | `timestamp (3)` default `now()` | no | when the row was written (audit; never back-dated) |
| `"note"` | `text` | yes | optional free text for this event |
| `"createdAt"` | `timestamp (3)` default `now()` | no | |

**`"eventType"` enum** (matches the set el-5s6h implements):

| Value | Meaning | Counts as a "response"? |
|---|---|---|
| `applied` | user submitted the application | no (it is the denominator anchor) |
| `viewed` | employer/ATS viewed the application | yes |
| `screen` | recruiter screen / phone screen scheduled or held | yes |
| `offer` | offer extended | yes |
| `rejected` | application rejected | yes |

- A **"response"** = any event whose type ≠ `applied`. Silence (only `applied`
  exists) = non-response. This single definition drives the numerator in Story 2.
- **Terminal-once idempotency**: `offer` and `rejected` are terminal — at most one
  each per application. `UNIQUE ("applicationId", "eventType")` enforced for those
  via a partial unique index, so a double-submit (Story 1 AC-4) cannot duplicate
  them. `viewed`/`screen` may legitimately recur (multiple screens) — not unique.
- **`applied` exactly once** per application (partial unique index) — it anchors
  post-age and the denominator; a second `applied` is a no-op.

### Indexes

```
CREATE INDEX "ApplicationOutcome_applicationId_idx"
  ON "ApplicationOutcome" ("applicationId", "occurredAt");
```

Every read is "give me this application's events in time order", and the signal
scans by application; this composite covers both.

### Write path (specified here, built in el-5s6h)

- A status change on `"Application"` **also** appends the corresponding
  `"ApplicationOutcome"` row in the same transaction — status update and event
  insert never diverge.
- `applied` transition sets `"Application"."appliedAt"` = the event's
  `"occurredAt"` (the post-age signal reads `appliedAt`, so it must be the same
  instant as the `applied` event).
- The write is idempotent per the uniqueness rules above; a redundant transition
  updates nothing and inserts nothing.

---

## The Timing Signal: Post-Age-at-Apply vs Response Rate

A **deterministic SQL aggregation** — no model, no LLM in the signal path
(mirrors the ghost-job / salary-inference determinism convention).

### Definitions

- **Post-age-at-apply** for an application = `Application."appliedAt" −
  Job."postedAt"`, in days. Requires an `applied` event and a non-null
  `"postedAt"`; otherwise → **unknown-age bucket**.
- **Response** = the application has ≥ 1 `"ApplicationOutcome"` with
  `eventType ≠ 'applied'`.
- **Bucket response rate** = (applications in bucket with a response) /
  (applications in bucket). Denominator includes silent applications.

### Buckets (proposed; implementer may tune)

| Bucket | Post-age-at-apply |
|---|---|
| `fresh` | 0–2 days |
| `recent` | 3–7 days |
| `aging` | 8–21 days |
| `stale` | 22+ days |
| `unknown` | `postedAt` null / invalid |

The `unknown` bucket is always reported separately and never folded into a real
age bucket — mixing "we don't know the age" into "22+ days" would silently bias
the signal.

### Output contract

Per bucket: `{ bucket, n (applications), responses, rate | null, state }` where
`state ∈ { observed, prior, insufficient }`. `rate` is null unless
`state = observed`. The caller always receives `n`, so it can render sample size
alongside any rate (Story 2 AC-5).

---

## Cold-Start Honesty (mandatory)

The signal MUST NOT present a computed rate as the user's own until it is backed
by a real sample. Two thresholds, both documented and both configurable:

1. **Global cold-start threshold** — minimum *total* recorded applications (rows
   with an `applied` event) before the feature shows any personalised observed
   rate at all. **Proposed default: 30.** Below it, the feature shows the
   **documented prior** (below) for the overall view, explicitly labelled as a
   baseline, not the user's rate.
2. **Per-bucket minimum sample** — minimum applications *in a bucket* before that
   bucket shows its own observed rate. **Proposed default: 8.** Below it, the
   bucket's `state = insufficient` and it renders "not enough data" even when
   other buckets qualify.

### The documented prior

The prior is a **static, documented baseline response rate** used only for the
overall view during global cold-start. It is:

- a single fixed value with its source recorded in this spec (a conservative
  industry-baseline job-application response rate; **proposed placeholder: 20%**,
  to be confirmed by the implementer with a cited source, and stored as a named
  constant, not scattered magic numbers);
- always rendered **labelled as a baseline/prior** — never as "your response
  rate";
- **never** blended silently into observed per-bucket rates. Once a bucket has
  enough data it shows its own observed rate; the prior does not "smooth" it.

### The three states, exhaustively

| Condition | State shown | What the user sees |
|---|---|---|
| Total applications < global threshold | `prior` | The documented baseline, labelled "typical baseline — not yet your data", with a note on how many more applications until personalised. |
| Total ≥ global threshold, bucket < per-bucket min | `insufficient` (for that bucket) | "Not enough data in this bucket yet (n = X)". Other qualifying buckets still show observed rates. |
| Total ≥ global threshold, bucket ≥ per-bucket min | `observed` | The bucket's own response rate, with n. |

**Explicitly forbidden**: computing and displaying a per-bucket or overall rate
from a sample below its threshold; presenting the prior as the user's observed
rate; hiding the sample size behind a rate; dropping silent applications to make a
rate look better.

---

## Assumptions & Open Questions (for el-5s6h / implementer)

- Thresholds (30 global, 8 per-bucket) and bucket boundaries are **proposed
  defaults** — confirm against real data volume expectations; keep them named
  constants so they are tunable without touching query logic.
- The prior value (20% placeholder) needs a **cited source** before ship; record
  it in a companion decision-log entry if it drives product-visible numbers.
- `viewed` depends on ATS/employer view signals actually being observable; if the
  data source can't supply `viewed`, the enum value stays defined but simply never
  fires — the model does not depend on it existing.
- Identity of "response" is intentionally coarse (any non-`applied` event). A
  future task could weight offer > screen > viewed; out of scope here.

---

## Acceptance Criteria for this spec task (el-2qgn)

- [x] Spec-category doc covering the **outcome event model** (`"ApplicationOutcome"`,
      enum, timestamps, idempotency).
- [x] Covers the **post-age-at-apply vs response-rate timing signal** (definitions,
      buckets, deterministic SQL, output contract).
- [x] Covers **cold-start honesty** — documented prior vs "not enough data",
      thresholds, three exhaustive states, forbidden behaviours.
- [x] Migration named `00012_application_outcome.sql`.
- [x] No code.
