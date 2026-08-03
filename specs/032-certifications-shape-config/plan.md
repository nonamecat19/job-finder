# Implementation Plan: Certifications as a Configurable Resume Category

**Branch**: `032-certifications-shape-config` | **Date**: 2026-08-03 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/032-certifications-shape-config/spec.md`

## Summary

Certifications are the only recognised resume section with no user controls. They hold a
fixed slot in the enforced section order (`prepare_marshal.go:45`, between education and
publications) but pass through generation verbatim — they cannot be turned off or capped.

This feature gives certifications the same controls the projects category got in feature
031: an enable toggle, a minimum, and a maximum. The change is a vertical slice through
an existing, well-established path — migration → sqlc → domain value type → settings
service → HTTP → DTO → tygo → dashboard — adding three fields at each layer.

**The defining decision is what is left out.** Projects are LLM-selected for vacancy
relevance, which is why they need a prompt block, a schema field and a grounding rule.
Certifications are instead truncated deterministically in authored order (research D3).
That keeps `buildSelectPrompt`, the merge in `rendercv.go`, and `rendercv_grounding.go`
entirely untouched, adds zero generation tokens, and satisfies Constitution II for free —
content that never reaches the model cannot be fabricated.

## Decisions taken without user confirmation

The spec was written with two open `[NEEDS CLARIFICATION]` markers. `/speckit-plan` was
invoked before they were answered, so both were resolved to the recommended defaults
rather than blocking. **Both are reversible; flag if either is wrong.**

| Question | Resolved to | Impact if overturned |
|---|---|---|
| Q1 — per-certification detail cap? | **No.** Three knobs only (research D2) | Purely additive: one column, one validation range, one DTO field, one UI row |
| Q2 — relevance selection or truncation? | **Truncation** in authored order (research D3) | Substantial: adds a prompt block, a `TailoredSections` schema field, and a grounding rule. Rest of the plan unaffected |

Q1 was resolved this way because the repo contains **no certifications data anywhere** —
no fixture, no DTO, no form — so there is no evidence certification entries carry a
sub-list to cap. `projectBulletsMax` exists only because `normal` entries have
`highlights`.

## Technical Context

**Language/Version**: Go 1.x (`apps/api`), TypeScript + React (`apps/dashboard`)

**Primary Dependencies**: chi router, sqlc (typed DB access), goose (migrations), tygo
(Go DTO → TS), TanStack Query, Tailwind, yaml.v3, RenderCV (external renderer)

**Storage**: PostgreSQL — singleton `ResumeShapeSetting` row (`id = 'default'`)

**Testing**: `go test` (`make test-go`), vitest (`make test-react`), Docker-backed
integration (`make test-integration`)

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Web application — Go API + React dashboard + shared TS types package

**Performance Goals**: No regression. Settings are served from the resumeshape service's
in-memory cache, so generation resolves the config with no DB round trip. Truncation is
O(n) on a slice with no added LLM tokens (research D3).

**Constraints**: Default settings must produce identical resumes to pre-feature (SC-004).
Goose versions must be unique and sequential → `00035`. Generated files (`sqlcgen`,
`generated.ts`) must never be hand-edited.

**Scale/Scope**: Single-user self-hosted deployment; one settings row. Roughly 10 files
touched, ~3 fields each, plus one new migration and one new test fixture.

## Constitution Check

*GATE: evaluated before Phase 0 and re-evaluated after Phase 1 design.*

| Principle | Assessment | Verdict |
|---|---|---|
| **I. No Auto-Apply, Ever** | Touches only resume shape settings and rendering. No submission path involved. | N/A |
| **II. Grounded Generation** | The decisive point. Certifications never enter a prompt (D3); a cap truncates the master's own entries in place. No content is generated, so nothing can be fabricated. Configured minima are reported as `Shortfall`s, never padded — FR-006 makes this explicit, and `ApplyHardLimits`' existing doc comment already states padding would be an invented bullet. | Pass, strengthened |
| **III. Typed Contracts** | Go DTO → tygo → `packages/shared/src/generated.ts`; DB → sqlc → `sqlcgen`. No hand-maintained duplicate types. `make tygo-check` and `make sqlc-check` gate drift. | Pass |
| **IV. Test Discipline Per Language** | `go test` for domain/service/HTTP, vitest for the settings card, Docker-backed `make test-integration` for the generation path. Change crosses `apps/api` + `apps/dashboard` + `packages/shared`, so `make test-lint` is the done bar. | Pass |
| **V. Local-First, Self-Hosted** | Adds no external calls. D3 removes the only change that would have added LLM traffic. | Pass, strengthened |

**Technology & Architecture Constraints**:

- Goose version `00035` — unique and sequential after `00034`.
- sqlc for DB access, no hand-written SQL in Go.
- `packages/shared` as the single cross-app TS type source.
- `pnpm --filter @job-finder/shared build` before dashboard tooling (quickstart step 1).
- `make` targets as canonical entry points.

**Post-Phase-1 re-evaluation**: No new violations. The design touches no new architectural
boundary — every layer it modifies already exists and already carries the projects
equivalent of each field.

**Result: PASS. No entries in Complexity Tracking.**

## Project Structure

### Documentation (this feature)

```text
specs/032-certifications-shape-config/
├── plan.md                              # This file
├── spec.md                              # Feature specification
├── research.md                          # Phase 0 — decisions D1-D7
├── data-model.md                        # Phase 1 — entity + field changes
├── quickstart.md                        # Phase 1 — validation guide
├── contracts/
│   └── resume-shape-api.md              # Phase 1 — endpoint contract delta
├── checklists/
│   └── requirements.md                  # Spec quality checklist (all pass)
└── tasks.md                             # Phase 2 — NOT created by /speckit-plan
```

### Source Code (repository root)

```text
apps/api/
├── internal/db/
│   ├── migrations/00035_certifications_shape.sql    # NEW — 3 ALTER TABLE ADD COLUMN
│   ├── queries/resumeshapesetting.sql               # 3 assignments ($11-$13)
│   └── sqlcgen/                                     # REGENERATED — never hand-edited
├── internal/dto/settings.go                         # 3 fields on ResumeShapeConfigDto
├── internal/generation/
│   ├── domain/rendercv_shape.go                     # ShapeConfig fields, defaults,
│   │                                                #   Validate, ApplySectionToggles,
│   │                                                #   ApplyHardLimits
│   └── application/service.go                       # shapeConfigMeta 3 keys
└── internal/resumeshape/
    ├── service.go                                   # rowToConfig / configToParams
    └── interfaces/http/resume_shape.go              # configToDto / dtoToConfig

packages/shared/src/
└── generated.ts                                     # REGENERATED via make tygo-generate

apps/dashboard/src/features/settings/
└── ResumeShapeCard.tsx                              # NumericKey, NUMERIC_FIELDS,
                                                     #   checkbox, dirty
```

**Structure Decision**: Web application — the existing Go API / React dashboard / shared
types split. This feature introduces no new directory, package or module. It extends the
`resumeshape` bounded context and the `generation/domain` value type, following the
one-way dependency the `resumeshape` package doc already establishes: the shape *value
type* lives in `generation/domain`, `resumeshape` imports it, and nothing in `generation`
imports back.

## Implementation Sequence

Ordered so each step compiles and the type system does as much of the work as possible.

1. **Migration** — `00035_certifications_shape.sql`, up and down. Verify the singleton row
   backfills to `true, 0, 0`.
2. **Query + sqlc** — extend `UpdateResumeShapeSetting`, run `make sqlc-generate`.
3. **Domain** — `ShapeConfig` fields, `DefaultShapeConfig()`, `Validate()` ranges and
   cross-field rules. Update `ApplySectionToggles`' doc comment, which currently claims
   "Only skills and projects can be disabled".
4. **Domain behaviour** — certifications removal in `ApplySectionToggles`, truncation +
   shortfall in `ApplyHardLimits`. Unit tests alongside.
5. **Service + HTTP + DTO** — four mapping functions, three lines each, plus
   `shapeConfigMeta`.
6. **tygo** — `make tygo-generate`, then `pnpm --filter @job-finder/shared build`.
7. **Dashboard** — `NumericKey` exclusion (the compiler will demand this once step 6
   lands), `NUMERIC_FIELDS` rows, checkbox, and the `dirty` term.
8. **Fixture + integration tests** — a profile fixture with a certifications section does
   not exist yet and must be created.
9. **`make test-lint`** — the cross-app done bar.

### Risk notes for implementation

- **The `dirty` computation in `ResumeShapeCard.tsx` is the one change TypeScript will
  not catch.** It enumerates booleans by hand; omitting the certifications term silently
  disables the save button for a toggle-only edit. Needs an explicit vitest case.
- **`UpdateResumeShapeSetting` uses positional parameters.** New assignments must be
  appended as `$11`-`$13` with `updatedAt = now()` kept last, or every field shifts.
- **Whole-config PUT + missing boolean = section silently disabled.** An external client
  PUTting a hand-written body without `certificationsEnabled` gets Go's zero value
  (`false`). This already applies identically to `skillsEnabled` and `projectsEnabled`, so
  the plan accepts it for consistency rather than making certifications behave uniquely.
  Documented in [contracts/resume-shape-api.md](contracts/resume-shape-api.md).
- **No certifications test data exists anywhere in the repo.** Step 8 is genuinely new
  work, not a copy-paste of an existing fixture.

## Complexity Tracking

No constitutional violations. Table intentionally empty.
