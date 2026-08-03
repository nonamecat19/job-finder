# Phase 1 Data Model: Certifications as a Configurable Resume Category

**Feature**: 032-certifications-shape-config

This feature adds three attributes to one existing entity. It creates no new tables and
no new entities.

## Modified entity: `ResumeShapeSetting` (singleton row, `id = 'default'`)

### New columns

| Column | Type | Null | Default | CHECK | Meaning |
|---|---|---|---|---|---|
| `certificationsEnabled` | `boolean` | NOT NULL | `true` | — | Certifications section renders in generated resumes |
| `certificationsMin` | `integer` | NOT NULL | `0` | `BETWEEN 0 AND 20` | Target floor for certifications kept. `0` = no minimum |
| `certificationsMax` | `integer` | NOT NULL | `0` | `BETWEEN 0 AND 20` | Hard cap on certifications kept. `0` = unlimited |

`0` means "unlimited / no minimum", matching the convention the table's header comment
already documents for `skillsMaxGroups`, `projectsMin`, `projectsMax` and
`projectBulletsMax`.

Defaults are chosen so that an upgraded install behaves exactly as before the migration:
section rendered, no cap, no minimum (FR-010, SC-004).

### Migration

`apps/api/internal/db/migrations/00035_certifications_shape.sql`

- **Up**: three `ALTER TABLE "ResumeShapeSetting" ADD COLUMN` statements with the defaults
  and CHECKs above. The defaults backfill the existing singleton row in place.
- **Down**: three `DROP COLUMN` statements.

Goose version `00035` — next sequential after `00034`, per the constitution's
uniqueness/sequencing rule.

### Query change

`apps/api/internal/db/queries/resumeshapesetting.sql` — `UpdateResumeShapeSetting` grows
three assignments. Parameters are positional, so the new fields take `$11`, `$12`, `$13`
and `updatedAt = now()` stays last:

```
    "certificationsEnabled" = $11,
    "certificationsMin"     = $12,
    "certificationsMax"     = $13,
```

`GetResumeShapeSetting` is `SELECT *` and needs no edit.

Regenerate with `make sqlc-generate`; `make sqlc-check` gates drift.

## Modified value type: `domain.ShapeConfig`

`apps/api/internal/generation/domain/rendercv_shape.go`

### New fields

```go
// CertificationsEnabled renders the certifications section; false removes it
// entirely.
CertificationsEnabled bool
// CertificationsMin is the target floor for how many certifications are kept.
// 0 = no minimum. Never satisfied by inventing certifications.
CertificationsMin int
// CertificationsMax is the hard cap on how many certifications are kept.
// 0 = unlimited.
CertificationsMax int
```

### `DefaultShapeConfig()` additions

```go
CertificationsEnabled: true,
CertificationsMin:     0,
CertificationsMax:     0,
```

These must stay identical to the column defaults — the doc comment already names this
function the single source of truth for defaults.

### Validation rules (`Validate()`)

Two new entries in the data-driven `ranges` table:

| Field | Min | Max |
|---|---|---|
| `certificationsMin` | 0 | 20 |
| `certificationsMax` | 0 | 20 |

Two new cross-field rules, mirroring the projects rules:

| Rule | Error message |
|---|---|
| `CertificationsMax > 0 && CertificationsMin > CertificationsMax` | `certificationsMin must be <= certificationsMax` |
| `CertificationsMin > 0 && !CertificationsEnabled` | `certificationsMin > 0 requires certificationsEnabled` |

Note the asymmetry, inherited deliberately from projects: the min/max ordering rule is
only enforced when a max is configured, because `0` means unlimited and every min is
therefore satisfiable.

### No `CertificationsLimited()` helper

`ProjectsLimited()` exists solely to gate the projects prompt block. Per research D3
certifications never enter the prompt, so no analogous helper is needed. Truncation is
gated inline by `CertificationsMax > 0`.

## Behaviour changes in generation

### `ApplySectionToggles(master, cfg)`

Gains a third removal, alongside the existing skills and projects removals:

```go
if !cfg.CertificationsEnabled {
    RemoveSection(sections, "certifications")
}
```

`RemoveSection` already deletes the section from `cv.sections` *and* drops its key from
the captured `_order` list, which is what satisfies FR-004's "and from the enforced
section order" requirement with no extra work.

The function's doc comment currently reads "Only skills and projects can be disabled" —
this must be updated to include certifications.

### `ApplyHardLimits(merged, cfg)`

Gains a certifications block modelled on the existing projects block, minus the
per-entry bullet loop:

- If `CertificationsMax > 0` and the section holds more than that, keep the first N
  (`sections["certifications"] = certs[:cfg.CertificationsMax]`). Order is preserved —
  never re-sorted.
- If `CertificationsMin > 0` and fewer are available, append a `Shortfall`. Nothing is
  padded.

### New `Shortfall` path value

| Path | Emitted when |
|---|---|
| `cv.sections.certifications` | fewer certifications available than `CertificationsMin` |

Section-level, matching the existing `cv.sections.projects` form rather than the
per-entry `cv.sections.experience[Company].highlights` form. No change to the `Shortfall`
or `ShapeReport` struct is required — only a new value flowing through them.

## Contract propagation

| Layer | File | Change |
|---|---|---|
| DTO | `apps/api/internal/dto/settings.go` | three fields on `ResumeShapeConfigDto`: `certificationsEnabled`, `certificationsMin`, `certificationsMax` |
| HTTP mapping | `.../interfaces/http/resume_shape.go` | three lines each in `configToDto` and `dtoToConfig` |
| Service mapping | `apps/api/internal/resumeshape/service.go` | three lines each in `rowToConfig` and `configToParams` (with `int`/`int32` conversion) |
| Activity meta | `apps/api/internal/generation/application/service.go` | three keys in `shapeConfigMeta()` |
| Generated TS | `packages/shared/src/generated.ts` | regenerated via `make tygo-generate` — **never hand-edited** |

## Dashboard model

`apps/dashboard/src/features/settings/ResumeShapeCard.tsx`

- `NumericKey` exclusion union gains `'certificationsEnabled'`. Without this the new
  boolean key flows into `NumericKey` and `NUMERIC_FIELDS` fails to typecheck — a
  compiler-enforced reminder, not a silent bug.
- `NUMERIC_FIELDS` gains two rows: `certificationsMin` (0–20) and `certificationsMax`
  (0–20), with descriptions matching the projects rows' phrasing, including the "kept in
  the order they appear in your master resume" note that is now literally accurate.
- A third checkbox for `certificationsEnabled`.
- `dirty` gains `|| draft.certificationsEnabled !== config.certificationsEnabled`. This
  is the one change the TypeScript compiler will **not** catch — omitting it silently
  disables the save button for a toggle-only edit. It needs an explicit test.
