# Phase 1 Data Model: Configurable Resume Generation Shape

**Feature**: `031-resume-generation-config` | **Date**: 2026-08-02

---

## 1. `ResumeShapeSetting` (new table)

Singleton row, guarded the same way `AutoGenerateSetting` was (migration `00019`). Migration: `apps/api/internal/db/migrations/00034_resume_shape_setting.sql`.

| Column | Type | Default | Range guard | Maps to |
|--------|------|---------|-------------|---------|
| `id` | `text` PK | `'default'` | `CHECK ("id" = 'default')` | — |
| `summaryLines` | `integer` | `4` | `CHECK BETWEEN 1 AND 12` | FR-007 |
| `skillsEnabled` | `boolean` | `true` | — | FR-009 |
| `skillsMaxGroups` | `integer` | `0` | `CHECK BETWEEN 0 AND 20` (`0` = all) | FR-008 |
| `experienceBulletsMin` | `integer` | `8` | `CHECK BETWEEN 1 AND 20` | FR-010 |
| `experienceBulletsMax` | `integer` | `10` | `CHECK BETWEEN 1 AND 20` | FR-010 |
| `targetPages` | `integer` | `2` | `CHECK BETWEEN 1 AND 3` | FR-011 |
| `projectsEnabled` | `boolean` | `true` | — | FR-012 |
| `projectsMin` | `integer` | `0` | `CHECK BETWEEN 0 AND 20` (`0` = no minimum) | FR-012 |
| `projectsMax` | `integer` | `0` | `CHECK BETWEEN 0 AND 20` (`0` = all) | FR-012 |
| `projectBulletsMax` | `integer` | `0` | `CHECK BETWEEN 0 AND 10` (`0` = all) | FR-013 |
| `updatedAt` | `timestamp(3)` | `now()` | — | — |

The migration seeds the single `'default'` row so a fresh install and an upgraded install behave identically. `Down` drops the table.

**Cross-column rules** (enforced in Go, not SQL, so the error message can name the field — FR-004):

- `experienceBulletsMin <= experienceBulletsMax`
- `projectsMin <= projectsMax` whenever `projectsMax > 0`
- `projectsMin > 0` requires `projectsEnabled = true`

### Queries (`apps/api/internal/db/queries/resumeshapesetting.sql`)

- `GetResumeShapeSetting :one` — `SELECT * … WHERE "id" = 'default'`
- `UpdateResumeShapeSetting :one` — full-row `UPDATE … SET … , "updatedAt" = now() … RETURNING *`

Reset (FR-005) is an `UpdateResumeShapeSetting` call with the defaults struct — no third query needed.

---

## 2. `domain.ShapeConfig` (new value type)

`apps/api/internal/generation/domain/rendercv_shape.go`. Plain struct, no DB or HTTP awareness, so the generation pipeline depends on nothing outside its own domain package.

```text
ShapeConfig
  SummaryLines          int
  SkillsEnabled         bool
  SkillsMaxGroups       int   // 0 = all
  ExperienceBulletsMin  int
  ExperienceBulletsMax  int
  TargetPages           int
  ProjectsEnabled       bool
  ProjectsMin           int   // 0 = no minimum
  ProjectsMax           int   // 0 = all
  ProjectBulletsMax     int   // 0 = all
```

**Behaviour on the type**:

| Function | Purpose | Requirement |
|----------|---------|-------------|
| `DefaultShapeConfig() ShapeConfig` | Single source of truth for defaults; matches the table defaults exactly | FR-003, FR-005 |
| `(ShapeConfig) Validate() error` | Range + cross-field checks, error names the field and its valid range | FR-004 |
| `(ShapeConfig) ProjectsLimited() bool` | `ProjectsMax > 0 \|\| ProjectBulletsMax > 0` — gates whether projects enter the prompt at all | R7, keeps the default path inert |
| `ApplySectionToggles(master, cfg)` | Deletes disabled sections from `cv.sections` and from the `_order` list | FR-009, FR-012, FR-020 |
| `ApplyHardLimits(merged, cfg) ShapeReport` | Clamps experience bullets to `Max`, project count to `ProjectsMax` (master order), project bullets to `ProjectBulletsMax`; returns what it could not satisfy | FR-014, FR-017 |

**`ShapeReport`** — the shortfall/observability payload written to the activity record (FR-006, FR-017):

```text
ShapeReport
  Config          ShapeConfig            // what the run used
  Shortfalls      []Shortfall            // target vs available, per path
  PageTarget      int
  PagesAchieved   int
  ConflictNoted   bool                   // page target overrode section lengths (FR-016)

Shortfall
  Path      string   // e.g. "cv.sections.experience[Acme].highlights"
  Requested int
  Available int
```

---

## 3. `TailoredSections` extension

`apps/api/internal/generation/domain/rendercv.go`. One new field and one new type; existing fields unchanged.

```text
TailoredProject
  Name        string     // copied EXACTLY from the master; used only as a lookup key
  Highlights  []string   // selected/rephrased from THIS project's master bullets only

TailoredSections
  Summary     string
  Skills      []TailoredSkillGroup
  Experience  []TailoredExperience
  Projects    []TailoredProject      // NEW — populated only when cfg.ProjectsLimited()
```

**Merge rules** (extend `MergeTailored`, mirroring the existing experience path at `rendercv.go:277-291`):

1. Index master projects by normalised `name`.
2. For each payload project with a matching master entry, replace **only** `highlights`.
3. `url`, `start_date`, `end_date`, `name` come from the deep-cloned master and are never read from the payload — the model structurally cannot corrupt them (FR-018).
4. Unmatched payload names are ignored by the merge and reported by grounding.
5. When `Projects` is empty (the default path), the master's projects survive the merge untouched — today's exact behaviour.

**Post-merge, in order**: `ApplySectionToggles` → `ApplyHardLimits` → `VerifyRendercvGrounding` → `VerifyStructureIntegrity`. Verification runs last so it sees exactly the document that will be rendered.

---

## 4. Grounding extension

`apps/api/internal/generation/domain/rendercv_grounding.go`, appended to `VerifyRendercvGrounding`:

| Check | Level | Violation message shape |
|-------|-------|-------------------------|
| Merged project names ⊆ master project names | all | `project "X" not in master profile` |
| Project highlight tokens ⊆ that project's own master bullet tokens | `GroundingStrict` | `project highlight token "t" (X) not in master profile (strict grounding)` |

The existing section-subset check (`rendercv_grounding.go:30-39`) already permits deletions, so disabled sections need no exemption (FR-020) — verified in R5.

---

## 5. `dto.ResumeShapeConfigDto`

`apps/api/internal/dto/settings.go`. Field-for-field mirror of `ShapeConfig` with JSON tags; flows to `packages/shared/src/generated.ts` via tygo (constitution III).

```text
ResumeShapeConfigDto
  SummaryLines          int   `json:"summaryLines"`
  SkillsEnabled         bool  `json:"skillsEnabled"`
  SkillsMaxGroups       int   `json:"skillsMaxGroups"`
  ExperienceBulletsMin  int   `json:"experienceBulletsMin"`
  ExperienceBulletsMax  int   `json:"experienceBulletsMax"`
  TargetPages           int   `json:"targetPages"`
  ProjectsEnabled       bool  `json:"projectsEnabled"`
  ProjectsMin           int   `json:"projectsMin"`
  ProjectsMax           int   `json:"projectsMax"`
  ProjectBulletsMax     int   `json:"projectBulletsMax"`
```

---

## 6. Entity mapping back to the spec

| Spec entity | Realised as |
|-------------|-------------|
| Resume Generation Configuration | `ResumeShapeSetting` row → `domain.ShapeConfig` → `dto.ResumeShapeConfigDto` |
| Section Setting | The per-section field group inside `ShapeConfig` (no separate type — a struct-per-section would add indirection with no behaviour) |
| Generation Record | Existing `activity` row, extended through `ShapeReport` written via `Recorder.Step` / `Recorder.Ok` meta |
| Project Entry | Existing `cv.sections.projects[]` in the master YAML; `TailoredProject` is the LLM-facing projection of its `highlights` only |

## 7. Data flow

```text
DB row ──sqlc──> resumeshape.Service (cached)
                        │ ShapeProvider port
                        ▼
        generation.Service.<run>  ── cfg resolved ONCE per run
                        │
      ┌─────────────────┼──────────────────────┐
      ▼                 ▼                      ▼
 prompt builders   MergeTailored +        renderResume
 (approx targets)  ApplySectionToggles    (targetPages loop)
                   + ApplyHardLimits
                        │                      │
                        └────► ShapeReport ◄───┘
                                   │
                                   ▼
                        activity.Recorder meta
```
