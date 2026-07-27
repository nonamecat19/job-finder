# Data Model: Djinni Preset-Search Rewrite

**Date**: 2026-07-28
**Spec**: [spec.md](./spec.md)

This feature changes **no schema column** in the existing `Subscription`
or `JobSource` tables — it deletes data and adds one audit table. The
entities below are the only data-model-touching pieces of the rewrite.

---

## Entities

### 1. `Subscription` (existing — behavior change only, no schema edit)

A row in the `Subscription` table (`apps/api/internal/db/migrations/
00002_subscriptions.sql`) representing one saved Djinni preset-search
subscription. **After the rewrite**, the only Djinni rows that may exist
are preset-search URLs; the validator at
`apps/api/internal/subscriptions/service.go` rejects every other shape
at save time.

| Field | Constraint change | Notes |
|---|---|---|
| `sourceKey` | unchanged (`"djinni"` for Djinni) | Registry key still `"djinni"`. |
| `url` | **must** match the preset shape (`host= djinni.co\|www.djinni.co`, `path=/jobs\|/jobs/`, `search_type=basic-search` query present) | Enforced at save by `validateDjinniSubscriptionURL`. Other djinni.co URLs — including `/my/dashboard/subs/{id}/` — are **rejected** with a stated reason. |
| `name`, `cron`, `createdAt`, etc. | unchanged | Generic subscription fields. |

**State transitions** (states are the existing `Subscription` lifecycle;
the rewrite adds no new state):

1. `created` — operator pastes a preset URL into the New Subscription
   form → validator accepts → row inserted (generic flow, unchanged).
2. `running` — scheduler picks the subscription → ingestion handler
   sets `query.SubscriptionURL = sub.url` → adapter's `Search`
   paginates `page=1..50`. Unchanged from today's basic-search run.
3. `ok` / `unhealthy` — verdict computed by
   `ingestion/handler.go:computeVerdict/flagIfUnhealthy`. Unchanged
   (the block-detection path is generic).

The **deleted transitions** are the `subs/{id}/` save/transition path and
the session-refresh path (no longer reachable; the validator branch and
the adapter branch are both removed).

**Validation rules implemented by the rewrite**:

- URL host ∈ `{djinni.co, www.djinni.co}`.
- URL path ∈ `{/jobs, /jobs/}`.
- URL query carries `search_type=basic-search`.
- Unknown query params are **preserved verbatim** (e.g.
  `&exp_level=1y&exp_level=2y&exp_level=3y`, or unrecognized extras)
  and re-issued by the run, NEVER interpreted or stripped except
  `page=` which is forced to `1` at run start (FR-002, spec edge
  "`page=N` already present").
- `exp_level` duplicates are collapsed **for display only**; the run
  issues them as-saved (spec edge "`3y&1y&2y`").

### 2. `JobSource` (existing — no schema edit; behavior change only)

The `JobSource` row with `key="djinni"` (created by seed at
`apps/api/internal/seed/`) stays — the adapter still runs preset
searches against it. Its `config` JSONB column may still carry an
encrypted `{"sessionCookie":"…"}` blob from before the rewrite; that
blob becomes **orphaned** (no code reads it). Per R6 of
[research.md](./research.md), it is left inert: the masking path
(`subscriptions/service.go:66-76` equivalent in `jobsources/service.go`)
still masks it on read, and it churns out on the next operator save.

No field changes. No new validation rule beyond "preset-runs do not
consult `JobSource.config` for credentials".

### 3. `DjinniLegacySubAudit` (NEW — added by migration `00027_…`)

A new audit table recording which `subs/{id}/` subscriptions migration
`00027_drop_djinni_dashboard_subs.sql` deleted, so the operator can
recreate them as preset URLs. Required by FR-009 / SC-009 ("a recorded
list of the deleted subscriptions is available").

| Field | Type | Notes |
|---|---|---|
| `id` | UUID (PK) | Synthetic audit-row id. |
| `subscriptionId` | UUID (FK → `Subscription.id`, **not enforced** — the row is deleted) | The id of the deleted subscription. Intentionally not a real FK; the referenced row is gone. |
| `name` | TEXT | The deleted subscription's `name`. |
| `url` | TEXT | The deleted subscription's `url` (a `/my/dashboard/subs/{id}/` URL). |
| `deletedAt` | TIMESTAMPTZ | When the migration ran (clock-time, not the sub's `createdAt`). |

**Validation rules**:

- The migration is the only writer.
- The audit row is **insert-only** — no update, no delete.
- The migration's `-- +goose Down` is a **no-op** (cannot recover
  deleted rows; documented in the migration file).

### 4. `NormalizedJob` (existing — no schema edit; behavior change only)

The `dto.NormalizedJob` that `parseDjinniCards` produces is unchanged.
`SourceKey="djinni"`, `ExternalID` from the card's path tail, `URL` via
`ResolveReference`. The only difference post-rewrite: the ingestion
originates from a preset-search page instead of a dashboard page. The
shape the downstream enrich/match/generate pipeline sees is identical
(R3 of research.md), so no entity change.

---

## Relationships

```text
Subscription (djinni, preset-search URL only)
  └── produces ──► NormalizedJob (SourceKey="djinni")
                     │
                     └── enriches via ──► (JobSource key="djinni" config: orphaned, unread)

DjinniLegacySubAudit (NEW)
  └── records every Subscription row the 00027 migration deleted
      (one audit row per deleted subscription)
```

---

## What does NOT change (data-model-wise)

- `Subscription` columns (`sourceKey`, `url`, `name`, `cron`, …) — no
  add/drop/alter.
- `JobSource` columns (`key`, `config` encrypted JSONB, …) — no
  add/drop/alter. The orphaned `sessionCookie` key sits in `config`
  inert; documented non-action.
- `NormalizedJob` / `Job` columns — no change (preset listings produce
  the same shape as before; dedup, scoring, generation are generic).
- `JobSourceRun` / verdict / health-flag columns — no change.
- No new `packages/shared` DTO field (no `tygo` regeneration needed —
  confirmed by grep finding no Djinni reference in `packages/shared/src`).
- No new `sqlc` query (the migration is plain SQL DELETE; the validator
  uses the existing `CreateSubscription` code path).