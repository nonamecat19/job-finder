---

description: "Task list for 031-resume-generation-config"
---

# Tasks: Configurable Resume Generation Shape

**Input**: Design documents from `/specs/031-resume-generation-config/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/settings-resume-shape.md](./contracts/settings-resume-shape.md), [quickstart.md](./quickstart.md)

**Tests**: Test tasks are **included and required**. Constitution principle IV ("Test Discipline Per Language, Enforced at the Boundary") mandates `go test` for `apps/api` and `vitest` for the dashboard, with `make test-lint` passing before a multi-app change is done. This is a project governance requirement, not an optional TDD preference.

**Organization**: Tasks are grouped by user story. Each story phase is an independently testable increment.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story the task belongs to (US1–US4)
- Exact file paths are included in every task

## Path Conventions

Monorepo (per plan.md): Go API under `apps/api/`, React dashboard under `apps/dashboard/src/`, shared TS contract in `packages/shared/src/`. Go tests are colocated as `*_test.go` beside the file under test.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Persistence layer for the config row. Nothing else can be wired until the table and its generated accessors exist.

- [X] T001 Create goose migration `apps/api/internal/db/migrations/00034_resume_shape_setting.sql` creating the singleton `ResumeShapeSetting` table with the columns, defaults and CHECK guards in data-model.md §1 (`id` text PK with `CHECK ("id" = 'default')`, `summaryLines`, `skillsEnabled`, `skillsMaxGroups`, `experienceBulletsMin`, `experienceBulletsMax`, `targetPages`, `projectsEnabled`, `projectsMin`, `projectsMax`, `projectBulletsMax`, `updatedAt`), seeding the single `'default'` row, with a `Down` that drops the table. Version 00034 is the next free sequential goose version after 00033 — do not reuse or duplicate it.
- [X] T002 Create `apps/api/internal/db/queries/resumeshapesetting.sql` with `GetResumeShapeSetting :one` (`WHERE "id" = 'default'`) and `UpdateResumeShapeSetting :one` (full-row UPDATE setting every config column plus `"updatedAt" = now()`, `RETURNING *`), following the style of `apps/api/internal/db/queries/aifeaturesetting.sql`.
- [X] T003 Run `make sqlc-generate` and commit the regenerated `apps/api/internal/db/sqlcgen/` output (new `resumeshapesetting.sql.go`, updated `models.go`). Do not hand-edit generated files (constitution III).

**Checkpoint**: Migration applies cleanly on a fresh DB and typed accessors compile.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The shape config value type, its persistence service, and the read/write API surface. Every user story reads a `ShapeConfig`, so nothing in Phase 3+ can start until this is done.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 [P] Create `apps/api/internal/generation/domain/rendercv_shape.go` with the `ShapeConfig` struct (10 fields per data-model.md §2), `DefaultShapeConfig()` returning the exact values in research.md R7 (`SummaryLines: 4`, `SkillsEnabled: true`, `SkillsMaxGroups: 0`, `ExperienceBulletsMin: 8`, `ExperienceBulletsMax: 10`, `TargetPages: 2`, `ProjectsEnabled: true`, `ProjectsMin: 0`, `ProjectsMax: 0`, `ProjectBulletsMax: 0`), and `(ShapeConfig) ProjectsLimited() bool` returning `ProjectsMax > 0 || ProjectBulletsMax > 0`. Document the `0 = unlimited` sentinel on each field.
- [X] T005 Add `(ShapeConfig) Validate() error` to `apps/api/internal/generation/domain/rendercv_shape.go` enforcing every range in data-model.md §1 plus the cross-field rules (`experienceBulletsMin <= experienceBulletsMax`; `projectsMin <= projectsMax` when `projectsMax > 0`; `projectsMin > 0` requires `projectsEnabled`). Each error message must name the offending field and its valid range (FR-004).
- [X] T006 [P] Write table-driven tests in `apps/api/internal/generation/domain/rendercv_shape_test.go` covering: `DefaultShapeConfig()` field-by-field, `Validate()` accepting defaults, `Validate()` rejecting each out-of-range field with a message naming that field, each cross-field rule, and `ProjectsLimited()` truth table including the all-zero default returning `false`.
- [X] T007 Create `apps/api/internal/resumeshape/service.go` mirroring `apps/api/internal/aifeature/service.go`: `NewService(ctx, queries)` loads the `'default'` row into an in-memory cache (falling back to `DefaultShapeConfig()` if the row is missing), `Get() domain.ShapeConfig` serves from cache, `Update(ctx, cfg) (domain.ShapeConfig, error)` calls `cfg.Validate()` **before** any write then persists and refreshes the cache, and `Reset(ctx) (domain.ShapeConfig, error)` persists `DefaultShapeConfig()`. Validation-before-write is what gives FR-004's all-or-nothing guarantee.
- [X] T008 [P] Write `apps/api/internal/resumeshape/service_test.go` asserting: cache serves without re-querying, `Update` with an invalid config returns an error and leaves the cached value untouched, `Update` with a valid config refreshes the cache, `Reset` restores defaults.
- [X] T009 [P] Add `ResumeShapeConfigDto` to `apps/api/internal/dto/settings.go` with the JSON tags in data-model.md §5 (field-for-field mirror of `ShapeConfig`).
- [X] T010 Run `make tygo-generate` and commit the regenerated `packages/shared/src/generated.ts` so `ResumeShapeConfigDto` crosses the Go↔TS boundary as a generated type (constitution III).
- [X] T011 Create `apps/api/internal/resumeshape/interfaces/http/resume_shape.go` modelled on `apps/api/internal/aifeature/interfaces/http/aifeature.go`: a `ShapeProvider` interface the handler needs, `Mount(r chi.Router)` registering `GET /settings/resume-shape` and `PUT /settings/resume-shape`, DTO↔domain mapping helpers, and `httpx.DecodeJSON`/`httpx.WriteJSON`/`httpx.WriteError` usage per contracts/settings-resume-shape.md. `PUT` returns 400 with the validation message on invalid input and 500 on persistence failure.
- [X] T012 [P] Write `apps/api/internal/resumeshape/interfaces/http/resume_shape_test.go` covering the contract test checklist in contracts/settings-resume-shape.md rows 1–2 and 4–7 (GET returns defaults; valid PUT round-trips; `targetPages: 4` → 400 naming the range with state unchanged; `experienceBulletsMin > Max` → 400; `projectsEnabled: false` with `projectsMin: 2` → 400; full non-default round-trip). The DELETE/reset rows belong to US4.
- [X] T013 Wire the service and handler in `apps/api/cmd/server/compose.go`: construct `resumeshape.NewService(ctx, p.DB.Queries)` alongside `aifeature.NewService`, add the handler to the router struct and mount it where `AiFeatureHandler` is mounted.
- [X] T014 Add a `ShapeProvider` port (`Shape(ctx) domain.ShapeConfig`) to `apps/api/internal/generation/application/service.go`'s `Service` struct and `NewService` signature, satisfied structurally by `*resumeshape.Service`; update the construction site in `apps/api/cmd/server/compose.go`. Resolve the config **once** at the top of each generation run and thread it as a value — this is what makes the in-flight-settings-change edge case correct by construction (research.md R3).
- [X] T015 [P] Extend the fakes in `apps/api/internal/generation/application/ports_test.go` with a configurable fake `ShapeProvider` so every existing generation test keeps compiling and future story tests can vary the config.
- [X] T016 Update `apps/api/internal/httpapi/router_test.go` for the two new routes so the route-registration assertions stay complete.

**Checkpoint**: `GET`/`PUT /v1/settings/resume-shape` work end to end against a live stack; the generation service receives a `ShapeConfig` but does not yet act on it. Quickstart Scenario 5 passes.

---

## Phase 3: User Story 1 - Control the length of generated resume content (Priority: P1) 🎯 MVP

**Goal**: Summary length, bullets per experience entry, skills volume and target page count all come from the config instead of hardcoded literals, with shortfalls and the achieved page count recorded.

**Independent Test**: Set summary length short, bullets-per-role low and target pages to 1; generate; confirm the summary, bullet counts and page count fall within the configured values. Repeat with a long/2-page config and confirm the output changes. Quickstart Scenarios 1, 2, 7, 8.

### Tests for User Story 1

- [X] T017 [P] [US1] Write tests in `apps/api/internal/generation/domain/rendercv_shape_test.go` for `ApplyHardLimits`: experience bullets clamped to `ExperienceBulletsMax` keeping the first N in order; `Max: 0` leaving content untouched; a role with fewer bullets than `ExperienceBulletsMin` producing a `Shortfall` naming the path with requested vs available counts and **no** padding (FR-017).
- [X] T018 [P] [US1] Write tests in `apps/api/internal/generation/application/rendercv_llm_test.go` asserting the built prompts carry the configured numbers rather than literals: `buildSelectPrompt` with `{SummaryLines: 2, ExperienceBulletsMin: 4, ExperienceBulletsMax: 5}` mentions 2 and 4-5 and does not contain "3-4 sentences" or "8-10"; the expand and condense prompts likewise derive from the config; and a default config reproduces the current wording (FR-003 regression guard).
- [X] T019 [P] [US1] Write tests in `apps/api/internal/generation/application/service_test.go` for the page-target loop using the existing fake renderer: target 1 with a 1-page first render returns immediately without an expand call; target 2 with a 1-page render triggers expand; target 2 with a 3-page render triggers compact-then-condense; an unreachable target exhausts the bounded attempt budget and returns the best result **without erroring** (FR-021); the achieved page count is recorded.

### Implementation for User Story 1

- [X] T020 [US1] Add `ShapeReport` and `Shortfall` types plus `ApplyHardLimits(merged RendercvMaster, cfg ShapeConfig) ShapeReport` to `apps/api/internal/generation/domain/rendercv_shape.go`, clamping experience highlights to `ExperienceBulletsMax` (`0` = unlimited) and collecting a `Shortfall` per path whose available count is below `ExperienceBulletsMin`. Never pad — truncation only.
- [X] T021 [US1] Parameterise `buildSelectPrompt` in `apps/api/internal/generation/application/rendercv_llm.go` by `ShapeConfig`: replace the "TOP 8-10 most relevant highlights" literal (line ~133) with the configured min-max range, the "3-4 sentences" literal (line ~139) with the configured summary length, and gate the "keep all relevant keywords (do not trim)" skills instruction on `SkillsMaxGroups`.
- [X] T022 [US1] Parameterise `expandContent` in `apps/api/internal/generation/application/rendercv_llm.go`: replace the "4-5 sentences" and "aim for 10-12 per job" literals (lines ~210-211) with values derived one step **above** the configured targets, and state the configured page target in the opening line instead of the hardcoded "TWO pages".
- [X] T023 [US1] Parameterise `condenseContent` in `apps/api/internal/generation/application/rendercv_llm.go`: replace "2-3 tight sentences" and "TOP 5-6 most relevant per job" (lines ~261-262) with values derived one step **below** the configured targets, and state the configured page target instead of "TWO pages". Per FR-016 the page target overrides the section length targets on this path.
- [X] T024 [US1] Thread `ShapeConfig` through the callers in `apps/api/internal/generation/application/rendercv_llm.go` — `selectAndTailor`, `retailorForStructure`, `expandContent`, `condenseContent` — and their call sites in `apps/api/internal/generation/application/service.go`.
- [X] T025 [US1] Generalise `renderResume` in `apps/api/internal/generation/application/service.go`: replace the literals `pages == 2`, `pages == 1` and `pages <= 2` (lines ~252, ~256, ~290) with comparisons against `cfg.TargetPages`, bound the adjust cycle with a `shapeAttempts = 2` constant matching the existing `groundingAttempts` idiom, and preserve the existing degrade-gracefully behaviour (`slog.Warn` + return what exists) so generation never fails on an unreachable target (FR-021).
- [X] T026 [US1] Call `ApplyHardLimits` after `MergeTailored` and before `VerifyRendercvGrounding` in `apps/api/internal/generation/application/service.go`, per the post-merge ordering in data-model.md §3.
- [X] T027 [US1] Record the shape observability on the activity row in `apps/api/internal/generation/application/service.go`: a `rec.Step(ctx, "resume shape config", meta)` carrying the resolved config at the start of the run (FR-006), a step per shortfall naming path/requested/available (FR-017), a `{"conflict": "page_target_overrides_section_lengths"}` step when the condense path runs with reduced targets (FR-016), and the final page count on completion (FR-021, SC-005).

**Checkpoint**: User Story 1 fully functional. Quickstart Scenarios 1, 2, 7 and 8 pass. This is the MVP.

---

## Phase 4: User Story 2 - Include a configurable projects section (Priority: P2)

**Goal**: Projects are selected, limited and bullet-capped per config, with identity fields preserved verbatim and grounding coverage extended.

**Independent Test**: With a master holding more projects than the configured cap, generate and confirm the section renders with exactly the configured number of entries, each capped at the configured bullet count, in the master's section position and entry order, with names/links/dates byte-identical to the master. Quickstart Scenario 3.

**Depends on**: Phase 2. Touches `rendercv.go`, `rendercv_llm.go` and `service.go`, which US1 also edits — sequence after US1 or coordinate on those files.

### Tests for User Story 2

- [X] T028 [P] [US2] Write tests in `apps/api/internal/generation/domain/rendercv_test.go` for the `MergeTailored` project path: highlights replaced by name match; `url`/`start_date`/`end_date`/`name` byte-identical to the master even when the payload carries different values (FR-018); unmatched payload project names ignored by the merge; an empty `Projects` payload leaving master projects untouched (the default path, FR-003).
- [X] T029 [P] [US2] Write tests in `apps/api/internal/generation/domain/rendercv_shape_test.go` for project limiting: entry count truncated to `ProjectsMax` preserving master order; `ProjectsMax: 0` keeping all; per-project bullets truncated to `ProjectBulletsMax`; a master with fewer projects than `ProjectsMin` recording a `Shortfall` and inventing nothing.
- [X] T030 [P] [US2] Write tests in `apps/api/internal/generation/domain/rendercv_grounding_test.go` asserting a merged project name absent from the master yields a `project "X" not in master profile` violation, and that under `GroundingStrict` a project highlight token drawn from a *different* project's bullets is flagged.
- [X] T031 [P] [US2] Write a test in `apps/api/internal/generation/application/rendercv_llm_test.go` asserting the projects prompt block is **absent** when `ProjectsLimited()` is false (default) and present when a project limit is configured (research.md R7 — keeps the default path inert and token usage unchanged).

### Implementation for User Story 2

- [X] T032 [US2] Add the `TailoredProject` type (`Name`, `Highlights`, with `jsonschema_description` tags matching the existing `TailoredExperience` style) and the `Projects []TailoredProject` field on `TailoredSections` in `apps/api/internal/generation/domain/rendercv.go`.
- [X] T033 [US2] Extend `MergeTailored` in `apps/api/internal/generation/domain/rendercv.go` with the project path, mirroring the existing experience path (lines ~277-291): index master projects by normalised `name`, replace **only** `highlights` on the matched entry map in place, and let `name`/`url`/`start_date`/`end_date` pass through from the deep-cloned master so the model structurally cannot corrupt them.
- [X] T034 [US2] Extend `ApplyHardLimits` in `apps/api/internal/generation/domain/rendercv_shape.go` to truncate the projects slice to `ProjectsMax` in master order and each project's highlights to `ProjectBulletsMax` (`0` = unlimited for both), recording a `Shortfall` when the master holds fewer projects than `ProjectsMin`.
- [X] T035 [US2] Add the projects block to `buildSelectPrompt` in `apps/api/internal/generation/application/rendercv_llm.go`, emitted **only** when `cfg.ProjectsLimited()`: list each master project's name and its own bullets, and instruct the model to return the most vacancy-relevant projects with names copied exactly and highlights drawn only from that project's own bullets.
- [X] T036 [US2] Extend `VerifyRendercvGrounding` in `apps/api/internal/generation/domain/rendercv_grounding.go` with the two project checks from data-model.md §4: merged project names ⊆ master project names at all levels (mirroring the company check at lines ~18-28), and at `GroundingStrict` each project's highlight tokens ⊆ that project's own master bullet token pool.

**Checkpoint**: User Stories 1 and 2 both work independently. Quickstart Scenario 3 passes.

---

## Phase 5: User Story 3 - Enable or disable optional sections (Priority: P2)

**Goal**: Disabling skills or projects removes the section from the generated document without tripping grounding or structure verification.

**Independent Test**: Disable skills, generate, confirm no skills section and no orphan heading with all other sections intact in master order and zero violations; re-enable and confirm skills return. Quickstart Scenario 4.

**Depends on**: Phase 2. Independent of US1 and US2 in behaviour, but edits `service.go` alongside them.

### Tests for User Story 3

- [X] T037 [P] [US3] Write tests in `apps/api/internal/generation/domain/rendercv_shape_test.go` for `ApplySectionToggles`: `SkillsEnabled: false` removes the `skills` key from `cv.sections` **and** its entry from the synthetic `_order` list; `ProjectsEnabled: false` does the same for projects; all-enabled is a no-op; other sections keep master order.
- [X] T038 [P] [US3] Write a test in `apps/api/internal/generation/domain/rendercv_grounding_test.go` confirming a master with a disabled section removed produces **zero** violations from `VerifyRendercvGrounding` — the existing section-subset check (lines ~30-39) already permits deletions, so FR-020 needs no carve-out and this test locks that in.
- [X] T039 [P] [US3] Write a test in `apps/api/internal/generation/domain/rendercv_structure_test.go` confirming `VerifyStructureIntegrity` returns no violations when `skills` or `projects` is absent (it only inspects `summary` and `experience`, which are never disable-able).

### Implementation for User Story 3

- [X] T040 [US3] Add `ApplySectionToggles(master RendercvMaster, cfg ShapeConfig)` to `apps/api/internal/generation/domain/rendercv_shape.go`, deleting each disabled section from `cv.sections` and filtering its key out of the `SectionOrderKey` (`_order`) slice so no stale entry names a removed section.
- [X] T041 [US3] Add an `_order`-aware section removal helper to `apps/api/internal/generation/domain/rendercv_config.go` next to the existing `SectionOrderKey` handling (lines ~12, ~34-41), so order maintenance lives beside the code that builds the order.
- [X] T042 [US3] Call `ApplySectionToggles` in `apps/api/internal/generation/application/service.go` immediately after `MergeTailored` and before `ApplyHardLimits`, per the post-merge ordering in data-model.md §3 (toggles → hard limits → grounding → structure), so verification always sees exactly the document that will be rendered.
- [X] T043 [US3] Skip the skills block in `buildSelectPrompt` (`apps/api/internal/generation/application/rendercv_llm.go`) when `SkillsEnabled` is false, so no tokens are spent tailoring content that is about to be removed.

**Checkpoint**: User Stories 1, 2 and 3 all work independently. Quickstart Scenario 4 passes.

---

## Phase 6: User Story 4 - Discover and reset the configuration (Priority: P3)

**Goal**: Every configurable value, its current setting and its allowed range are visible in one place on the dashboard, with a one-action reset.

**Independent Test**: Open the settings page, confirm every value is shown with its current setting and allowed range, change several, reset, confirm all return to defaults. Quickstart Scenario 6.

**Depends on**: Phase 2 (the GET/PUT endpoints and the generated DTO).

### Tests for User Story 4

- [X] T044 [P] [US4] Add reset coverage to `apps/api/internal/resumeshape/interfaces/http/resume_shape_test.go` for contract rows 3 and 8: `DELETE` after a `PUT` returns the defaults, a following `GET` agrees, and a second `DELETE` is idempotent.
- [X] T045 [P] [US4] Write `apps/dashboard/src/features/settings/ResumeShapeCard.test.tsx` (vitest, following `SettingsPage.test.tsx` conventions) asserting: every configurable value renders with its current value and its allowed range; an out-of-range entry surfaces the API's 400 message without corrupting displayed state; reset restores the defaults in the UI.

### Implementation for User Story 4

- [X] T046 [US4] Add `DELETE /settings/resume-shape` to `Mount` in `apps/api/internal/resumeshape/interfaces/http/resume_shape.go`, calling `Service.Reset` and returning the defaults payload (FR-005, contracts §DELETE).
- [X] T047 [P] [US4] Add `settings.getResumeShape`, `settings.putResumeShape` and `settings.resetResumeShape` to the existing `settings` object in `apps/dashboard/src/lib/api.ts`, typed with `ResumeShapeConfigDto` imported from `@job-finder/shared`.
- [X] T048 [P] [US4] Add `resumeShape: { all: ['resumeShape'], get: ['resumeShape', 'get'] }` to `apps/dashboard/src/lib/queryKeys.ts`, matching the `aiFeatures` shape.
- [X] T049 [US4] Add `useResumeShape`, `useUpdateResumeShape` and `useResetResumeShape` to `apps/dashboard/src/features/settings/hooks.ts`, mirroring the `useAiFeatureSettings`/`useUpdateAiFeatureSetting` pattern with mutations invalidating `queryKeys.resumeShape.all`.
- [X] T050 [US4] Create `apps/dashboard/src/features/settings/ResumeShapeCard.tsx` rendering every configurable value with its current setting, its allowed range and a short description of what it does, plus a reset action — this single card is what satisfies SC-009's "single place".
- [X] T051 [US4] Mount `ResumeShapeCard` in `apps/dashboard/src/features/settings/SettingsPage.tsx` beside `AiFeatureSettingsCard`, and extend `apps/dashboard/src/features/settings/SettingsPage.test.tsx` to assert it renders.

**Checkpoint**: All four user stories independently functional. Quickstart Scenario 6 passes.

---

## Phase 7: Polish & Cross-Cutting Concerns

- [X] T052 [P] Run `make sqlc-check` and `make tygo-check` and fix any drift so generated DB and TS code are in sync with their sources (constitution III).
- [X] T053 Add integration coverage for the settings endpoints against real Postgres via the existing `dbtest` harness, exercising the persist → restart-service → read path so FR-002 (persistence across restarts) is verified against a real database rather than a cache (constitution IV requires Docker-backed integration tests for cross-service behaviour).
- [X] T054 [P] Update the projects comment block in `resume/resume.yaml` (lines ~135-137), which currently states projects are "NOT tailored by the generation pipeline" and instructs the reader to hand-maintain 3-4 entries — both statements become false once US2 lands.
- [X] T055 [P] Document the resume shape settings in `README.md` alongside the other user-tunable settings, including the `0 = unlimited` sentinel and the defaults table.
- [ ] T056 Run the full scenario set in `specs/031-resume-generation-config/quickstart.md` (Scenarios 1-8) against a live stack (`make up`), confirming in particular Scenario 1's no-drift guarantee (SC-002) and Scenario 7's no-fabrication guarantee (SC-008). **Partially done**: the settings-API halves ran green against a live API + Postgres (Scenario 1 GET returns the exact defaults, Scenario 2's PUT round-trips, Scenario 5 returns 400 naming `targetPages must be between 1 and 3` with state unchanged, Scenario 6's DELETE resets and a following GET agrees; migration `00034` applied cleanly on boot). The generation halves (Scenarios 1-gen, 2-gen, 3, 4, 7, 8) are **not run**: they need the full compose stack so the LLM gateway host (`http://litellm:4000`) resolves — a host-run API fails these with `provider unavailable` — plus real model spend and manual PDF inspection.
- [X] T057 Run the `test-lint` target in `Makefile` (lint-go, lint-web, test-go, test-react) and confirm it passes — the constitution's required gate for a change touching more than one app.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T001 → T002 → T003 are strictly sequential (migration defines the table the queries read; sqlc generates from both).
- **Foundational (Phase 2)**: Depends on Phase 1. **Blocks every user story.**
- **User Stories (Phases 3-6)**: All depend on Phase 2 only. Story-to-story dependencies are behavioural-independent but file-contended (see below).
- **Polish (Phase 7)**: Depends on all desired stories.

### User Story Dependencies

- **US1 (P1)**: Depends on Phase 2 only. No dependency on other stories.
- **US2 (P2)**: Depends on Phase 2. Behaviourally independent of US1, but edits `rendercv.go`, `rendercv_llm.go`, `rendercv_shape.go` and `service.go` — all also touched by US1. Sequence after US1 unless the two are coordinated on those files.
- **US3 (P2)**: Depends on Phase 2. Behaviourally independent; edits `rendercv_shape.go`, `rendercv_llm.go` and `service.go` (shared with US1/US2).
- **US4 (P3)**: Depends on Phase 2. The dashboard tasks (T047-T051) touch files no other story edits and are genuinely independent; only T046 touches shared backend code.

### Within Each User Story

- Tests before implementation (constitution IV; verify they fail first).
- Domain types before the functions that use them.
- Domain before application before HTTP before wiring.
- Story complete and checkpointed before moving to the next priority.

### Parallel Opportunities

- **Phase 2**: T004 (domain type) and T009 (DTO) are independent files and can run together; T006, T008, T012, T015 are all separate test files.
- **Phase 3**: T017, T018, T019 are three different test files — fully parallel.
- **Phase 4**: T028, T029, T030, T031 — four different test files, fully parallel.
- **Phase 5**: T037, T038, T039 — three different test files, fully parallel.
- **Phase 6**: T044 (Go handler test) and T045 (vitest) are in different toolchains entirely; T047 and T048 are different dashboard files.
- **Cross-story**: US4's dashboard slice (T047-T051) can be built by a second person in parallel with US1/US2/US3 backend work as soon as Phase 2 lands, since it shares no files with them.
- **Phase 7**: T052, T054, T055 are independent.

---

## Parallel Example: User Story 1

```bash
# Launch all three US1 test files together (constitution IV — write first, confirm they fail):
Task: "Write ApplyHardLimits tests in apps/api/internal/generation/domain/rendercv_shape_test.go"
Task: "Write prompt-parameterisation tests in apps/api/internal/generation/application/rendercv_llm_test.go"
Task: "Write page-target loop tests in apps/api/internal/generation/application/service_test.go"
```

## Parallel Example: Post-Foundational Split

```bash
# Once Phase 2 lands, two tracks run without file contention:
Track A (backend): US1 → US2 → US3   # all contend on generation/ files
Track B (dashboard): T047, T048, T049, T050, T051, T045   # touches only apps/dashboard/
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (migration + queries + sqlc).
2. Complete Phase 2: Foundational (**critical — blocks everything**).
3. Complete Phase 3: User Story 1.
4. **STOP and VALIDATE**: Quickstart Scenarios 1, 2, 7, 8. Scenario 1 is the load-bearing check — it proves defaults produce byte-equivalent output to the pre-feature pipeline (SC-002).
5. Deploy/demo: summary length, bullet density and page count are now user-controlled.

### Incremental Delivery

1. Setup + Foundational → config readable/writable via API, generation unaffected.
2. + US1 → shape control live (**MVP**).
3. + US2 → projects selected and limited.
4. + US3 → sections toggleable.
5. + US4 → discoverable UI with reset.

Each increment leaves the previous ones working; defaults keep every increment inert until the user opts in.

### Risk Notes

- **FR-003 is the highest-risk requirement**: every prompt change must keep default output equivalent to today. T018 and T056/Scenario 1 are the guards — do not skip them.
- **Grounding is constitution-level (principle II)**: US2 introduces the only new LLM-generated content in this feature. T030 and T036 are non-negotiable; a merge that reads project identity fields from the model payload instead of the master clone would violate it.
- **Goose version 00034 must not be duplicated** — check for a competing migration before merging if another branch is in flight.

---

## Notes

- `[P]` = different files, no dependencies on incomplete tasks.
- `[Story]` labels map tasks to spec.md user stories for traceability.
- Go tests are colocated as `*_test.go`; there is no separate `tests/` tree in `apps/api`.
- Never hand-edit `sqlcgen/` or `packages/shared/src/generated.ts` — regenerate (constitution III).
- Commit after each task or logical group; stop at any checkpoint to validate a story independently.
