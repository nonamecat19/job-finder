---
description: "Task list for 032-certifications-shape-config"
---

# Tasks: Certifications as a Configurable Resume Category

**Input**: Design documents from `/specs/032-certifications-shape-config/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/resume-shape-api.md](contracts/resume-shape-api.md), [quickstart.md](quickstart.md)

**Tests**: INCLUDED. Constitution Principle IV ("Test Discipline Per Language, Enforced at
the Boundary") makes tests mandatory, not optional, for a change crossing `apps/api`,
`apps/dashboard` and `packages/shared`. `make test-lint` is the done bar.

**Organization**: Grouped by user story. Note that this feature's foundational phase is
unusually large relative to its stories — the three settings fields must be plumbed
through eight layers before any story delivers behaviour. That is inherent to a vertical
slice through an existing pipeline, not a planning artifact.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: US1, US2, US3 — maps to the user stories in spec.md
- Exact file paths included in every task

## Path Conventions

Web application per plan.md: Go API at `apps/api/`, React dashboard at
`apps/dashboard/`, shared TS types at `packages/shared/`. Go tests are colocated
(`*_test.go` beside the source); integration tests use the `integration` build tag.

---

## Phase 1: Setup

**Purpose**: Working tree ready and baseline green before any edits

- [X] T001 Run `pnpm install` and `pnpm --filter @job-finder/shared build` from the repo root — shared must build before dashboard/api tooling can typecheck
- [X] T002 Run `make up` to bring up Postgres and Redis via Docker Compose
- [X] T003 Run `make test-lint` and confirm a green baseline before any edits, so later failures are attributable to this feature

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Plumb `certificationsEnabled` / `certificationsMin` / `certificationsMax`
through every layer, carrying values but changing no generation behaviour. Every user
story depends on this.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

**Ordering note**: T004→T005→T006 and T009→T010 are strict chains (generated code depends
on its source). Do not parallelize within a chain.

- [X] T004 Create migration `apps/api/internal/db/migrations/00035_certifications_shape.sql` with three `ALTER TABLE "ResumeShapeSetting" ADD COLUMN` statements — `certificationsEnabled boolean NOT NULL DEFAULT true`, `certificationsMin integer NOT NULL DEFAULT 0 CHECK (BETWEEN 0 AND 20)`, `certificationsMax integer NOT NULL DEFAULT 0 CHECK (BETWEEN 0 AND 20)` — plus a `-- +goose Down` dropping all three. Goose version 00035 is the next sequential after 00034 per constitution
- [X] T005 Extend `UpdateResumeShapeSetting` in `apps/api/internal/db/queries/resumeshapesetting.sql` with `"certificationsEnabled" = $11`, `"certificationsMin" = $12`, `"certificationsMax" = $13`, keeping `"updatedAt" = now()` last — parameters are positional, so appending is mandatory
- [X] T006 Run `make sqlc-generate` then `make sqlc-check` to regenerate `apps/api/internal/db/sqlcgen/` — never hand-edit generated files (Constitution III)
- [X] T007 Add the three fields to `ShapeConfig` in `apps/api/internal/generation/domain/rendercv_shape.go` with doc comments matching the existing projects fields' style, and add `CertificationsEnabled: true, CertificationsMin: 0, CertificationsMax: 0` to `DefaultShapeConfig()` — these must stay identical to the T004 column defaults
- [X] T008 Map the three fields in `rowToConfig` and `configToParams` in `apps/api/internal/resumeshape/service.go`, with `int`/`int32` conversion matching the projects fields
- [X] T009 Add `CertificationsEnabled bool`, `CertificationsMin int`, `CertificationsMax int` to `ResumeShapeConfigDto` in `apps/api/internal/dto/settings.go` and update its doc comment so the "0 means unlimited" list names `certificationsMin` and `certificationsMax`
- [X] T010 Run `make tygo-generate`, then `pnpm --filter @job-finder/shared build`, then `make tygo-check` — regenerates `packages/shared/src/generated.ts`. `packages/shared/src/index.ts` already re-exports `ResumeShapeConfigDto` (commit `aee564a`), so no export change is needed
- [X] T011 Map the three fields in `configToDto` and `dtoToConfig` in `apps/api/internal/resumeshape/interfaces/http/resume_shape.go`
- [X] T012 [P] Add the three keys to `shapeConfigMeta()` in `apps/api/internal/generation/application/service.go` so a past document can be explained by the settings that produced it
- [X] T013 [P] Create a RenderCV master profile test fixture containing a `certifications` section with at least 8 entries, for use by the domain and integration tests. **No certifications test data exists anywhere in the repo today** — check how `apps/api/internal/generation/domain/rendercv_shape_test.go` builds its masters and follow that pattern
- [X] T014 Add a unit test in `apps/api/internal/generation/domain/rendercv_shape_test.go` asserting `DefaultShapeConfig()` returns `CertificationsEnabled: true`, `CertificationsMin: 0`, `CertificationsMax: 0` (FR-010, SC-004)

**Checkpoint**: `make test-go` and `make test-react` pass. The three settings round-trip
through DB → service → HTTP → TS with no behaviour change. Generated resumes are still
identical to pre-feature output.

---

## Phase 3: User Story 1 - Turn the certifications section off (Priority: P1) 🎯 MVP

**Goal**: A user can disable the certifications section and have it omitted from
generated resumes, without deleting anything from their master profile.

**Independent Test**: Set `certificationsEnabled: false`, generate a resume from a profile
with a certifications section, confirm the rendered document has no certifications section
while the master profile still lists them. Toggle back on, confirm the section returns.

### Tests for User Story 1

- [X] T015 [P] [US1] Add unit tests in `apps/api/internal/generation/domain/rendercv_shape_test.go` for `ApplySectionToggles`: certifications removed from **both** `cv.sections` and the `_order` list when disabled; left untouched when enabled; no panic or error when the master has no certifications section at all
- [X] T016 [P] [US1] Add an integration test (build tag `integration`) asserting that with `certificationsEnabled: false` the rendered resume contains no certifications section, the remaining sections keep their relative order, and the source master profile is unmodified (FR-004)

### Implementation for User Story 1

- [X] T017 [US1] Add `if !cfg.CertificationsEnabled { RemoveSection(sections, "certifications") }` to `ApplySectionToggles` in `apps/api/internal/generation/domain/rendercv_shape.go`. `RemoveSection` already drops the key from `_order` as well, which is what satisfies FR-004's section-order clause
- [X] T018 [US1] Update the `ApplySectionToggles` doc comment in the same file — it currently reads "Only skills and projects can be disabled", which this task makes false
- [X] T019 [US1] Verify via integration run that a profile with **no** certifications section generates successfully under both toggle states and renders no empty certifications heading

**Checkpoint**: US1 fully functional and independently testable. This is a shippable MVP —
the toggle alone reclaims page space on a length-constrained resume.

---

## Phase 4: User Story 2 - Cap how many certifications appear (Priority: P2)

**Goal**: A cap keeps the first N certifications in the master's authored order; a minimum
the profile cannot meet is reported as a shortfall, never padded.

**Independent Test**: Configure `certificationsMax: 3` against a profile holding 8,
generate, confirm exactly 3 appear and that they are the first 3 in authored order.

**Depends on**: Phase 2 only. Independent of US1.

### Tests for User Story 2

- [X] T020 [P] [US2] Add unit tests in `apps/api/internal/generation/domain/rendercv_shape_test.go` for `ApplyHardLimits` certifications truncation: 8 entries with `CertificationsMax: 3` keeps exactly the first 3 in authored order; `CertificationsMax: 0` keeps all; a cap larger than the available count keeps all and invents nothing
- [X] T021 [P] [US2] Add a unit test in the same file asserting that `CertificationsMin: 4` against 2 available appends exactly one `Shortfall{Path: "cv.sections.certifications", Requested: 4, Available: 2}` and that the section content is left untouched — nothing padded (FR-006, Constitution II)
- [X] T022 [P] [US2] Add an integration test asserting a run with an unmeetable `certificationsMin` still succeeds, renders the available certifications, and records the shortfall in the activity record (SC-007)

### Implementation for User Story 2

- [X] T023 [US2] Add the certifications block to `ApplyHardLimits` in `apps/api/internal/generation/domain/rendercv_shape.go`, modelled on the existing projects block but **without** a per-entry bullet loop (research D2): when `CertificationsMax > 0` and more are present, assign `sections["certifications"] = certs[:cfg.CertificationsMax]`; when `CertificationsMin > 0` and fewer are present, append a `Shortfall` with path `cv.sections.certifications`
- [X] T024 [US2] Confirm the retained subset is never re-sorted — truncation preserves authored order, matching the existing projects comment "the retained subset is never reordered" (FR-015)
- [X] T025 [US2] Confirm no changes were made to `buildSelectPrompt` in `apps/api/internal/generation/application/rendercv_llm.go`, to the merge in `apps/api/internal/generation/domain/rendercv.go`, or to `apps/api/internal/generation/domain/rendercv_grounding.go`. Certifications must never reach the tailoring model (research D3) — this task is a deliberate negative check, not a no-op

**Checkpoint**: US1 and US2 both work independently. Capping and disabling are both live.

---

## Phase 5: User Story 3 - Manage certifications settings alongside the other categories (Priority: P2)

**Goal**: Certifications controls appear, validate, persist and reset in the same place
and the same way as the existing resume shape controls.

**Independent Test**: Open resume shape settings, confirm certifications controls appear
grouped with the others, change them, save, reload, confirm persistence. Submit an
out-of-range value and confirm rejection with no stored change.

**Depends on**: Phase 2 only. Independent of US1 and US2.

### Tests for User Story 3

- [X] T026 [P] [US3] Add unit tests for `ShapeConfig.Validate()` in `apps/api/internal/generation/domain/rendercv_shape_test.go` covering each new bound: `certificationsMin`/`certificationsMax` below 0 and above 20 rejected with `"certificationsMin must be between 0 and 20"` / `"certificationsMax must be between 0 and 20"`
- [X] T027 [P] [US3] Add unit tests for both new cross-field rules: `certificationsMin > certificationsMax` (when max > 0) → `"certificationsMin must be <= certificationsMax"`; `certificationsMin > 0` with `certificationsEnabled: false` → `"certificationsMin > 0 requires certificationsEnabled"`. Also assert that `certificationsMax: 0` (unlimited) never triggers the ordering rule
- [X] T028 [P] [US3] Add HTTP handler tests in `apps/api/internal/resumeshape/interfaces/http/resume_shape_test.go`: GET returns the three new fields; an invalid PUT returns 400 with the exact message and a follow-up GET shows **unchanged** stored values (FR-008 atomicity); DELETE returns the new defaults
- [X] T029 [P] [US3] Extend the service test in `apps/api/internal/resumeshape/service_test.go` and the DB-backed `service_integration_test.go` so the new fields survive `Update` → cache → `Get`, and `Reset` restores their defaults (FR-011)
- [X] T030 [P] [US3] Add a vitest case in `apps/dashboard/src/features/settings/ResumeShapeCard.test.tsx` asserting the save button enables when **only** `certificationsEnabled` is toggled. **This is the one change TypeScript cannot catch** — the `dirty` computation enumerates booleans by hand, so omitting the new term silently breaks toggle-only saves
- [X] T031 [P] [US3] Update the fixture configs in `apps/dashboard/src/features/settings/ResumeShapeCard.test.tsx` and `SettingsPage.test.tsx` to include the three new fields, since `ResumeShapeConfigDto` now requires them

### Implementation for User Story 3

- [X] T032 [US3] Add the two range entries (`certificationsMin` 0-20, `certificationsMax` 0-20) to the data-driven `ranges` table in `ShapeConfig.Validate()` in `apps/api/internal/generation/domain/rendercv_shape.go`
- [X] T033 [US3] Add the two cross-field rules to the same function, mirroring the projects rules verbatim including the `CertificationsMax > 0` guard on the ordering check
- [X] T034 [US3] Add `'certificationsEnabled'` to the `NumericKey` exclusion union in `apps/dashboard/src/features/settings/ResumeShapeCard.tsx` — once T010 lands, the compiler will demand this before `NUMERIC_FIELDS` typechecks
- [X] T035 [US3] Add the "Include certifications section" checkbox to the toggle row in the same file, alongside the skills and projects checkboxes
- [X] T036 [US3] Add `certificationsMin` (0-20) and `certificationsMax` (0-20) entries to `NUMERIC_FIELDS` in the same file, with descriptions matching the projects rows' phrasing — including "kept in the order they appear in your master resume", which is now literally accurate given research D3
- [X] T037 [US3] Add `|| draft.certificationsEnabled !== config.certificationsEnabled` to the `dirty` computation in the same file, making T030 pass

**Checkpoint**: All three user stories independently functional. Feature is complete.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [ ] T038 Verify the migration's down path by rolling back and forward once, confirming the singleton row backfills to `true, 0, 0` (quickstart step 2)
- [ ] T039 [P] Run the full quickstart manual API sequence in [quickstart.md](quickstart.md) step 4 — GET, two rejected PUTs with atomicity re-checks, DELETE
- [ ] T040 [P] Run the manual UI check in [quickstart.md](quickstart.md) step 7 against `make dev`
- [ ] T041 [P] Update the `ResumeShapeSetting` table's header comment in the migration directory, or the `resumeshape` package doc in `apps/api/internal/resumeshape/service.go`, if either enumerates the configurable categories and now reads as incomplete
- [ ] T042 Run `make test-integration` and confirm every case in the [quickstart.md](quickstart.md) step 5 table passes
- [ ] T043 Run `make test-lint` — the constitution's done bar for a change crossing `apps/api`, `apps/dashboard` and `packages/shared`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies
- **Foundational (Phase 2)**: depends on Setup — **blocks all user stories**
- **User Stories (Phases 3-5)**: all depend only on Phase 2. Mutually independent — they
  touch disjoint concerns (section removal / truncation / settings surface)
- **Polish (Phase 6)**: depends on all desired stories

### Critical chains inside Phase 2

```
T004 → T005 → T006        (migration → query → sqlc regeneration)
T007 → T008 ─┐
T009 → T010 → T011        (DTO → tygo+shared build → HTTP mapping)
```

T012, T013, T014 are independent of those chains.

### User Story Dependencies

- **US1 (P1)**: after Phase 2. No dependency on US2 or US3
- **US2 (P2)**: after Phase 2. No dependency on US1 or US3
- **US3 (P2)**: after Phase 2. No dependency on US1 or US2

All three stories touch `rendercv_shape.go`, but different functions —
`ApplySectionToggles` (US1), `ApplyHardLimits` (US2), `Validate` (US3). Parallel work by
different people means merge conflicts in one file; sequential is safer for a solo run.

### Within Each User Story

- Tests written first and confirmed failing before implementation
- Domain behaviour before UI
- Story complete before moving to the next

---

## Parallel Opportunities

**Phase 2** — after the two chains complete:

```bash
Task: "T012 shapeConfigMeta keys in generation/application/service.go"
Task: "T013 certifications test fixture"
Task: "T014 DefaultShapeConfig unit test"
```

**Phase 3 (US1)** — both tests together:

```bash
Task: "T015 ApplySectionToggles unit tests"
Task: "T016 disabled-section integration test"
```

**Phase 4 (US2)** — all three tests together:

```bash
Task: "T020 truncation unit tests"
Task: "T021 shortfall unit test"
Task: "T022 shortfall integration test"
```

**Phase 5 (US3)** — six test tasks together, spanning Go and TS:

```bash
Task: "T026 Validate range tests"
Task: "T027 Validate cross-field tests"
Task: "T028 HTTP handler tests"
Task: "T029 service + service_integration tests"
Task: "T030 dirty-flag vitest case"
Task: "T031 dashboard test fixture updates"
```

**Cross-story**: with multiple developers, US1 / US2 / US3 can run concurrently after
Phase 2 — subject to the `rendercv_shape.go` conflict note above.

---

## Implementation Strategy

### MVP (User Story 1 only)

1. Phase 1: Setup
2. Phase 2: Foundational — the bulk of the work; blocks everything
3. Phase 3: US1
4. **STOP and VALIDATE** — toggle certifications off, confirm the section disappears and
   the master profile is untouched
5. Shippable: the toggle alone delivers page-space control

### Incremental Delivery

1. Setup + Foundational → three settings round-trip end to end, zero behaviour change
2. + US1 → section can be disabled → validate → demo (MVP)
3. + US2 → cap and shortfall reporting → validate → demo
4. + US3 → reachable and validated in the dashboard → validate → demo

US3 is what makes US1 and US2 usable by a non-developer; treat it as required for a real
release even though it is independently testable.

---

## Notes

- **Out of scope, deliberately**: vacancy-relevance selection of certifications (research
  D3, spec FR-015) and a per-certification detail cap (research D2, spec FR-016). T025 is
  an explicit guard against the first one creeping in. Both were resolved without user
  confirmation — see the "Decisions taken without user confirmation" table in
  [plan.md](plan.md). Overturning D3 would add a prompt block, a `TailoredSections` schema
  field and a grounding rule; overturning D2 is purely additive
- Generated files (`apps/api/internal/db/sqlcgen/`, `packages/shared/src/generated.ts`)
  are regenerated by T006 and T010, never hand-edited (Constitution III)
- `[P]` = different files, no dependencies
- Commit after each task or logical group
