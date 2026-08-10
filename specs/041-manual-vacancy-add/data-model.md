# Phase 1 Data Model: Manual Vacancy Add by URL

Schema deltas, entity rules, and the invariants that must hold after the migration.
Conventions follow the existing tree: PascalCase quoted identifiers, goose up/down,
sqlc-generated access.

---

## Migration `00041_manual_add.sql`

Next free goose version is **00041** (00040 is `document_summary_option`). Version numbers
are unique and sequential — never reused.

```sql
-- +goose Up
-- Manual vacancy add (041). Two changes, both additive:
--   1. Subscription gains a kind, so a non-crawling "Manual" row can exist alongside
--      the saved-search rows without ever being scheduled.
--   2. SourceRun gains a subscription link and a trigger, so a manual add is recorded
--      like any ingest run yet excluded from source-health accounting.
ALTER TABLE "Subscription"
    ADD COLUMN "kind" text NOT NULL DEFAULT 'crawl';

ALTER TABLE "Subscription"
    ADD CONSTRAINT "Subscription_kind_check" CHECK ("kind" IN ('crawl', 'manual'));

-- At most one manual subscription per source.
CREATE UNIQUE INDEX "Subscription_manual_per_source_idx"
    ON "Subscription" ("sourceKey") WHERE "kind" = 'manual';

ALTER TABLE "SourceRun"
    ADD COLUMN "subscriptionId" uuid,
    ADD COLUMN "trigger" text NOT NULL DEFAULT 'scheduled';

ALTER TABLE "SourceRun"
    ADD CONSTRAINT "SourceRun_subscriptionId_Subscription_id_fk"
    FOREIGN KEY ("subscriptionId") REFERENCES "public"."Subscription"("id")
    ON DELETE SET NULL ON UPDATE no action;

ALTER TABLE "SourceRun"
    ADD CONSTRAINT "SourceRun_trigger_check" CHECK ("trigger" IN ('scheduled', 'manual'));

CREATE INDEX "SourceRun_subscriptionId_idx" ON "SourceRun" ("subscriptionId");

-- The 'manual' JobSource backs fill-in vacancies on hosts with no reader (FR-012a).
-- Inserted here so the first fill-in save has a source to reference; the registered
-- ManualAdapter makes it visible in the sources list.
INSERT INTO "JobSource" ("key", "kind", "enabled", "config", "healthy")
VALUES ('manual', 'manual', true, '{}'::jsonb, true)
ON CONFLICT ("key") DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS "SourceRun_subscriptionId_idx";
ALTER TABLE "SourceRun" DROP CONSTRAINT IF EXISTS "SourceRun_trigger_check";
ALTER TABLE "SourceRun" DROP CONSTRAINT IF EXISTS "SourceRun_subscriptionId_Subscription_id_fk";
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "trigger";
ALTER TABLE "SourceRun" DROP COLUMN IF EXISTS "subscriptionId";
DROP INDEX IF EXISTS "Subscription_manual_per_source_idx";
ALTER TABLE "Subscription" DROP CONSTRAINT IF EXISTS "Subscription_kind_check";
ALTER TABLE "Subscription" DROP COLUMN IF EXISTS "kind";
-- The 'manual' JobSource row and any vacancies under it are intentionally left in
-- place: dropping the source would cascade-delete real vacancies the operator entered
-- by hand. Down is for rolling back the schema, not for discarding the operator's data.
```

**Idempotency and safety.** Every statement is additive with a default, so existing rows
become `kind = 'crawl'` / `trigger = 'scheduled'` — which is what they are. No existing
`Subscription` or `SourceRun` row changes meaning. No `Job` column is touched, so the dedupe
key and every existing index are untouched. The partial unique index can only fire on rows
this feature creates.

**Down is deliberately not symmetric.** It reverses the schema but keeps the `manual`
`JobSource` row, because `Job.sourceKey` → `JobSource.key` cascades on delete: dropping that
row would silently destroy hand-entered vacancies. Precedent: `00027`'s Down is also a no-op
where reversal would lose data.

---

## Entities

### `Job` — unchanged

No new column. Manual attribution rides entirely on existing fields:

| Field | Meaning for a manual add |
|---|---|
| `sourceKey` | The real host's source (`djinni`), or `manual` for a fill-in on an unknown host (FR-012, FR-012a) |
| `subscriptionId` | The Manual subscription for that source (FR-012b) |
| `ingestedAt` | The moment of the add — drives the 24-hour surfacing (FR-017c) |
| `postedAt` | As extracted from the posting, or null. Never the add time (FR-017a) |
| `dedupeKey` | `DedupeKey(company, title, url)` — the identical function a crawl uses (FR-007) |

### `Subscription` — gains `kind`

| Field | Crawl row | Manual row |
|---|---|---|
| `kind` | `'crawl'` | `'manual'` |
| `sourceKey` | FK to `JobSource.key` | same |
| `url` | The saved search URL, validated per source | `''` — nothing to crawl |
| `cron` | Operator's schedule | The default string, never read |
| `enabled` | Operator-controlled | `true`, not togglable (FR-016) |
| `lastRunAt` | Touched by the scheduler | Touched by each manual add |
| `name` | Operator's label | `'Manual'` |

**Invariants**

1. At most one `kind = 'manual'` row per `sourceKey` — enforced by the partial unique index.
2. A manual row is never returned by the scheduler's due-subscription query (FR-014).
3. A manual row is created only by `EnsureManualSubscription`, never by `POST /subscriptions`
   (FR-015).
4. A manual row's `url` and `cron` are immutable through the update path (FR-015).
5. A manual row cannot be deleted or disabled while any `Job` references it (FR-016).
6. `validateSubscriptionURL` is not applied to manual rows — there is no URL.

### `SourceRun` — gains `subscriptionId` and `trigger`

| Field | Manual add |
|---|---|
| `sourceId` | The resolved source (or `manual`) |
| `subscriptionId` | The Manual subscription |
| `trigger` | `'manual'` |
| `found` | 1 when the posting was read, 0 when it was not |
| `new` | 1 when a vacancy was created, 0 for duplicate or failure |
| `ok` | true for created and duplicate; false for failure |
| `error` | The failure reason, truncated to 1000 chars as the existing path does |
| `verdict` | Reuses the existing verdict vocabulary where it applies (`blocked` etc.) |

**Invariants**

7. Every manual add writes exactly one run — created, duplicate, or failed (FR-017e/f).
8. Runs with `trigger = 'manual'` are excluded from `RecentSourceRunsForSource`, so they can
   never flag a source unhealthy (FR-017g).
9. A duplicate outcome is `ok = true`, `found = 1`, `new = 0` — not an error.

### `JobSource` — one new row

`key = 'manual'`, `kind = 'manual'`, backed by a registered no-op `ManualAdapter` whose
`Search` fails permanently and whose `HealthCheck` returns healthy. It exists so fill-in
vacancies on unknown hosts have a valid source (research D4).

---

## Query changes

### `joblist.sql` — `ListJobsByScore`, `ListJobsByDate`, `CountJobs`

Add to all three:

```sql
LEFT JOIN "Subscription" s ON s."id" = j."subscriptionId"
```

and the filter predicate:

```sql
AND (NOT COALESCE(sqlc.narg('only_manual')::bool, false) OR s."kind" = 'manual')
```

`ListJobsByScore` (the default ordering) additionally gets the leading surfacing term:

```sql
ORDER BY (s."kind" = 'manual' AND j."ingestedAt" > now() - interval '24 hours') DESC,
         mr."score" DESC NULLS LAST, j."ingestedAt" DESC
```

`ListJobsByDate` — the explicitly chosen ordering — does **not** get the term (FR-017d).

### `subscription.sql`

- `CreateSubscription` takes `kind`.
- New `EnsureManualSubscription` — insert-or-return by `(sourceKey, kind='manual')`.
- New `GetManualSubscription(sourceKey)`.
- New `CountJobsForSubscription(id)` — the FR-016 deletion guard.
- New `ManualSubscriptionStats(id)` → `SUM("new")`, `MAX("startedAt")` over manual runs
  (FR-013, FR-017h).
- The scheduler's due query gains `AND "kind" = 'crawl'`.

### `sourcerun.sql`

- `InsertSourceRun` takes `subscriptionId` and `trigger`.
- `RecentSourceRunsForSource` gains `AND "trigger" <> 'manual'`.

Regenerate with `sqlc generate` — never hand-edit `db/sqlcgen/`.

---

## DTO changes

In `apps/api/internal/dto/jobs.go`, regenerated into `packages/shared/src/generated.ts`
via tygo (Constitution III):

```go
type SubscriptionDto struct {
    // ... existing fields
    Kind          string  `json:"kind"`                    // "crawl" | "manual"
    ManualCount   *int    `json:"manualCount,omitempty"`   // manual rows only
    LastAddedAt   *string `json:"lastAddedAt,omitempty"`   // manual rows only
}

type ManualAddResultDto struct {
    Outcome   string   `json:"outcome"` // created | duplicate | needs_fill_in | failed
    Job       *JobDto  `json:"job,omitempty"`      // created, duplicate
    Reason    *string  `json:"reason,omitempty"`   // failed, needs_fill_in
    Kind      *string  `json:"kind,omitempty"`     // the FR-018 failure kind
    Draft     *ManualVacancyDraftDto `json:"draft,omitempty"` // needs_fill_in
}

type ManualVacancyDraftDto struct {
    URL         string  `json:"url"`
    SourceKey   *string `json:"sourceKey,omitempty"`
    Title       *string `json:"title,omitempty"`
    Company     *string `json:"company,omitempty"`
    Location    *string `json:"location,omitempty"`
    Remote      bool    `json:"remote"`
    SalaryRaw   *string `json:"salaryRaw,omitempty"`
    Description *string `json:"description,omitempty"`
    PostedAt    *string `json:"postedAt,omitempty"`
}
```

`SourceRunDto` gains `subscriptionId` and `trigger`.

---

## State transitions

A manual add attempt is a straight line with four terminal states. No intermediate state is
persisted, so an interrupted attempt leaves nothing (FR-021).

```
submitted
  ├─ invalid URL (not http/https, unparseable) ──────────────► failed: invalid_url
  ├─ no PostingReader claims the host ───────────────────────► needs_fill_in: no_reader
  ├─ host claimed, but URL is a search page ─────────────────► failed: not_a_posting
  ├─ read attempted
  │    ├─ deadline exceeded (30 s) ──────────────────────────► failed: timed_out
  │    ├─ unreachable / 404 ─────────────────────────────────► failed: unreachable
  │    ├─ blocked / challenge / login wall ──────────────────► failed: blocked
  │    └─ read OK
  │         ├─ missing title, company or description ────────► needs_fill_in: incomplete
  │         └─ complete
  │              ├─ dedupeKey exists ───────────────────────► duplicate (ok, new=0)
  │              └─ new ────────────────────────────────────► created (ok, new=1)
  └─ every terminal state writes exactly one SourceRun
```

The fill-in save path is separate and shorter: validate required fields → persist → created,
or rejected with the missing field names (FR-020).
