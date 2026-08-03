# Phase 0 Research: Certifications as a Configurable Resume Category

**Feature**: 032-certifications-shape-config
**Date**: 2026-08-03

## Existing surface (established by code reading, not assumption)

The projects category — added by feature 031 — is the reference implementation this
feature mirrors. Its full surface, end to end:

| Layer | File | What is there |
|---|---|---|
| Migration | `apps/api/internal/db/migrations/00034_resume_shape_setting.sql` | `ResumeShapeSetting` singleton table, `projectsEnabled/Min/Max`, `projectBulletsMax` columns with `CHECK` ranges |
| Query | `apps/api/internal/db/queries/resumeshapesetting.sql` | `GetResumeShapeSetting`, `UpdateResumeShapeSetting` (positional `$1..$10`) |
| Domain value | `apps/api/internal/generation/domain/rendercv_shape.go` | `ShapeConfig` struct, `DefaultShapeConfig()`, `ProjectsLimited()`, `Validate()`, `ApplySectionToggles()`, `ApplyHardLimits()`, `Shortfall`, `ShapeReport` |
| Settings service | `apps/api/internal/resumeshape/service.go` | `rowToConfig`, `configToParams`, cached `Get`/`Update`/`Reset` |
| HTTP | `apps/api/internal/resumeshape/interfaces/http/resume_shape.go` | `configToDto`, `dtoToConfig`, GET/PUT/DELETE `/v1/settings/resume-shape` |
| DTO | `apps/api/internal/dto/settings.go` | `ResumeShapeConfigDto` |
| Generated TS | `packages/shared/src/generated.ts` | `ResumeShapeConfigDto` interface (tygo output) |
| Dashboard | `apps/dashboard/src/features/settings/ResumeShapeCard.tsx` | `NumericKey` type, `NUMERIC_FIELDS` table, toggle checkboxes, `dirty` computation |
| Activity meta | `apps/api/internal/generation/application/service.go` | `shapeConfigMeta()` — every knob echoed into the activity record |

Certifications, by contrast, exist in exactly **one** place today:
`apps/api/internal/generation/domain/prepare_marshal.go:45`, as the string
`"certifications"` in `defaultSectionOrder`, positioned between `education` and
`publications`. There is no DTO field, no column, no UI, no fixture, and no test data.

### D1: Certifications section key and entry shape

**Decision**: Target the section key `"certifications"` exactly as it appears in
`defaultSectionOrder`. Treat certification entries as opaque — read their count, never
their fields.

**Rationale**: `RemoveSection(sections, key)` and the `sections[key].([]any)` truncation
pattern both operate purely on the section key and slice length. Neither needs to know
what a certification entry contains. This is what makes the feature entry-shape-agnostic
and is why D2 below costs nothing to defer.

**Alternatives considered**: Matching several aliases (`"licenses"`,
`"licenses_and_certifications"`). Rejected — `defaultSectionOrder` already establishes a
single canonical key, and alias matching would silently capture user-authored custom
sections the user did not intend to put under these controls.

### D2: No per-certification detail cap (resolves spec Q1)

**Decision**: Ship three knobs — `certificationsEnabled`, `certificationsMin`,
`certificationsMax`. No `certificationBulletsMax`.

**Rationale**: `projectBulletsMax` exists because `normal` entries carry a `highlights`
array. The repo contains no certifications data at all, so there is no evidence
certification entries carry a sub-list. RenderCV certifications are conventionally
`one_line` entries (`label` + `details`), which have no bullet list to cap. Adding a
fourth knob speculatively means a column, a validation rule, a DTO field and a UI row
that may do nothing.

**Alternatives considered**: Full four-knob parity with projects. Rejected as
speculative. Adding it later is a purely additive migration — a new column with a
behaviour-preserving default of 0 — so deferring costs nothing.

### D3: Deterministic truncation, not relevance selection (resolves spec Q2)

**Decision**: A certifications cap keeps the first N in the master profile's authored
order. Certifications never enter the tailoring prompt (`buildSelectPrompt`), never
appear in the `TailoredSections` schema, and gain no grounding rule.

**Rationale**: Three reinforcing reasons.

1. **Grounding (Constitution II) is satisfied for free.** Content that never reaches the
   model cannot be fabricated. The projects path needs `rendercv_grounding.go` to check
   returned project names against the master precisely *because* projects are model-
   selected. Truncation needs no such check — the retained entries are the master's own,
   byte for byte.
2. **Zero token cost.** `ProjectsLimited()` gates a whole prompt block for exactly this
   reason. Truncation keeps that cost at zero unconditionally.
3. **Low value.** Certifications are short, atomic, and few. A user orders them once by
   hand and the ordering holds across every vacancy — unlike projects, where the relevant
   subset genuinely shifts per posting.

**Alternatives considered**: Mirroring the projects LLM-selection path
(`CertificationsLimited()` → prompt block → `TailoredCertification` schema field →
grounding rule). Rejected for this iteration; it is a strictly additive change if user
feedback later justifies it. Also considered keyword-match ranking against the vacancy
analysis without an LLM call — rejected as a bespoke ranking mechanism with no precedent
in the codebase, added before any evidence it is needed.

**Consequence**: `ApplyHardLimits` handles certifications; `buildSelectPrompt`,
`rendercv.go` merge, and `rendercv_grounding.go` are untouched. This is the single
largest scope reduction in the plan.

### D4: Migration strategy

**Decision**: New goose migration `00035_certifications_shape.sql` adding three columns
via `ALTER TABLE`, with defaults that preserve current behaviour:
`certificationsEnabled` default `true`, `certificationsMin` default `0`,
`certificationsMax` default `0`.

**Rationale**: Constitution requires unique, sequential goose versions; `00034` is the
current maximum, so `00035` is next. `ALTER TABLE ... ADD COLUMN ... DEFAULT` backfills
the existing singleton row in place, so an upgraded install and a fresh install hold
identical values — satisfying FR-010 and SC-004. Editing `00034` in place is not an
option: it has shipped.

**Range CHECKs**: `certificationsMin`/`certificationsMax` both `BETWEEN 0 AND 20`,
matching `projectsMin`/`projectsMax` exactly per the spec's Assumptions.

**Down migration**: drops the three columns.

### D5: Validation rules

**Decision**: Extend `ShapeConfig.Validate()` with two range entries
(`certificationsMin`, `certificationsMax`, both 0–20) and two cross-field rules that
mirror the projects rules verbatim:

- `certificationsMax > 0 && certificationsMin > certificationsMax` → error
- `certificationsMin > 0 && !certificationsEnabled` → error

**Rationale**: FR-009 states these directly, and the projects precedent already
establishes the exact error-message form (`"projectsMin must be <= projectsMax"`). The
`ranges` table in `Validate()` is data-driven, so the range checks are two struct
literals. Validation lives in the domain and is called from both the HTTP handler (for a
400) and the service (for write atomicity) — no new call sites needed.

### D6: Type contract propagation (Constitution III)

**Decision**: Add fields to the Go `dto.ResumeShapeConfigDto`, then regenerate
`packages/shared/src/generated.ts` via `make tygo-generate`. Never hand-edit the
generated file.

**Rationale**: Constitution III forbids hand-maintained duplicate types and mandates
regeneration. `make tygo-check` is a CI gate that fails if the committed generated file
does not match a fresh generation, so hand-editing would be caught but wastes a cycle.
`packages/shared` must be rebuilt (`pnpm --filter @job-finder/shared build`) before the
dashboard typechecks against the new fields.

### D7: Dashboard integration

**Decision**: Add `certificationsEnabled` to the `NumericKey` exclusion union, add a
third checkbox, add `certificationsMin`/`certificationsMax` rows to `NUMERIC_FIELDS`, and
extend the `dirty` computation with the new boolean.

**Rationale**: `NumericKey` is
`Exclude<keyof ResumeShapeConfigDto & string, 'skillsEnabled' | 'projectsEnabled'>`. Once
the DTO gains a boolean `certificationsEnabled`, that key flows into `NumericKey` and
`NUMERIC_FIELDS` starts type-erroring — the compiler enforces this step, which is the
desired failure mode. The `dirty` check enumerates booleans explicitly and will silently
miss the new toggle otherwise; this is the one place the compiler will *not* help, so it
needs a deliberate test.

## Resolved unknowns

| Unknown | Resolution |
|---|---|
| Which section key identifies certifications | `"certifications"`, per `defaultSectionOrder` (D1) |
| Whether a per-entry detail cap is needed | No (D2) |
| Whether a cap uses LLM relevance selection | No — deterministic truncation (D3) |
| Next goose migration number | `00035` (D4) |
| Permitted ranges for min/max | 0–20, matching projects (D4) |
| How TS types stay in sync | tygo regeneration, `make tygo-check` gate (D6) |

No NEEDS CLARIFICATION markers remain.
