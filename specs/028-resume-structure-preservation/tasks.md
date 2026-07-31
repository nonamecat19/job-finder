# Tasks: Strict Resume Structure Preservation During AI Tailoring

**Input**: Design documents from `/specs/028-resume-structure-preservation/`

**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/, quickstart.md

**Tests**: The quickstart.md defines 9 test scenarios. Only unit tests (scenarios 1-8) are in scope for task generation. The production `TailoredSections` change also acts as a compile gate (scenario 7).

**Organization**: Tasks are grouped by invariant enforcement layer (Foundational), then by user story for independent test verification, with a final Polish phase for cross-boundary type regeneration.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Codebase Orientation)

**Purpose**: Understand the current state of files that will be changed

- [X] T001 Review current `TailoredSections` and `TailoredExperience` struct definitions in `apps/api/internal/generation/domain/rendercv.go`
- [X] T002 Review current `MergeTailored` implementation (reorder block, SectionsToDrop loop, Drop filter) in `apps/api/internal/generation/domain/rendercv.go`
- [X] T003 Review current `buildSelectPrompt` instructions (reorder/drop/section-drop) in `apps/api/internal/generation/application/rendercv_llm.go`
- [X] T004 Review existing `VerifyRendercvGrounding` pattern in `apps/api/internal/generation/domain/rendercv_grounding.go` as reference for new verifier

---

## Phase 2: Foundational (Structural-Invariant Enforcement Layer)

**Purpose**: Wire the three non-negotiable invariants into the merge layer and prompt — BLOCKS all user story test verification

**⚠️ CRITICAL**: All structural enforcement must be in place before story-specific tests can pass

### Struct & Merge Layer Changes (Invariants 1 & 2 — block sequence, experience order)

- [X] T005 [P] Remove `SectionsToDrop` and `ExperienceOrder` fields from `TailoredSections` struct in `apps/api/internal/generation/domain/rendercv.go`
- [X] T006 [P] Remove `Drop` field from `TailoredExperience` struct in `apps/api/internal/generation/domain/rendercv.go`
- [X] T007 Remove `SectionsToDrop` delete-loop handling from `MergeTailored` (rendercv.go delete(sections, key) block) in `apps/api/internal/generation/domain/rendercv.go`
- [X] T008 Remove `ExperienceOrder` reorder block and `Drop` kept-filter from `MergeTailored` (rendercv.go reorder loop + `_drop` marker logic) in `apps/api/internal/generation/domain/rendercv.go`

### Prompt Changes (all three invariants — stop asking the LLM to violate)

- [X] T009 Edit `buildSelectPrompt` HARD RULES in `apps/api/internal/generation/application/rendercv_llm.go`: replace "Set drop: true" with "Keep every experience entry; never set drop"; replace "Reorder experience: most relevant company first" with "Keep experience entries in the EXACT order shown in the master; do not reorder"; replace "Decide which sections to drop" with "Do not drop, add, rename, or reorder any resume section. Do not populate sectionsToDrop"
- [X] T010 Add "no numeric years claim" instruction to summary guidance in `buildSelectPrompt` in `apps/api/internal/generation/application/rendercv_llm.go`: "Do not state a total number of years of experience (e.g. 'over 8 years'); describe seniority descriptively without a numeric claim"

### Verifier (Invariant 3 — total experience years text-assertion check)

- [X] T011 Create `apps/api/internal/generation/domain/rendercv_structure.go` with `StructureViolation` type (Kind, Path, Message) and `StructureKind` constants (`StructureTotalExperienceYears`)
- [X] T012 Implement total-experience-years derivation helper in `apps/api/internal/generation/domain/rendercv_structure.go`: parse years from `cv.sections.experience[].start_date/end_date`, treat "Present"/empty end as current year, sum per-entry `(endYear - startYear)` clamped to ≥0
- [X] T013 Implement `VerifyStructureIntegrity(master, merged RendercvMaster) []StructureViolation` in `apps/api/internal/generation/domain/rendercv_structure.go`: regex-scan `merged.cv.sections.summary[0]` and `merged.cv.sections.experience[].highlights[]` for numeric years-of-experience assertions (e.g., "over N years", "N+ years", "N years of experience"), compare against master's derived total, return violations on mismatch

### Service Wiring (Invariant 3 — re-prompt + strip fallback)

- [X] T014 Wire `VerifyStructureIntegrity` call after merge in `tailorRendercvResume` in `apps/api/internal/generation/application/service.go`: on violation detection, execute one targeted re-prompt (feed violation message back), then on recurrence strip the offending clause from the text and log the intervention on the activity row

**Checkpoint**: All three invariants are structurally enforced — user story tests can now be verified

---

## Phase 3: User Story 1 - Fixed Block Sequence Across AI Tailoring (Priority: P1)

**Goal**: Prove that the tailored resume's block sequence equals the master resume's block sequence — no blocks added, removed, renamed, or reordered

**Independent Test**: `TestMergeTailoredPreservesBlockOrder` — construct a master with non-canonical order, merge, assert `_order` key unchanged and all sections present

### Tests for User Story 1

- [X] T015 [P] [US1] Add `TestMergeTailoredPreservesBlockOrder` test in `apps/api/internal/generation/domain/rendercv_test.go`: master with order `[experience, education, skills, summary, projects]`, TailoredSections with new summary/highlights, assert merged `_order` equals master order, projects section preserved, no sections added/removed/renamed/reordered
- [X] T016 [P] [US1] Add `TestBuildSelectPromptNoReorderOrDrop` test in `apps/api/internal/generation/application/rendercv_llm_test.go`: call `buildSelectPrompt`, assert prompt does NOT contain "Reorder experience", "drop: true", "Decide which sections to drop"; assert prompt DOES contain "Keep experience entries in the EXACT order", "never set drop", "Do not drop, add, rename, or reorder any resume section"

**Checkpoint**: Block sequence invariant is verified — merge and prompt enforce US1. Run `go test ./internal/generation/domain/ -run TestMergeTailoredPreservesBlockOrder` and `go test ./internal/generation/application/ -run TestBuildSelectPromptNoReorderOrDrop`

---

## Phase 4: User Story 2 - Job Order Preservation Within the Experience Block (Priority: P1)

**Goal**: Prove that the tailored resume's experience entries appear in the same order as the master, with no entries added, removed, or reordered

**Independent Test**: `TestMergeTailoredPreservesExperienceOrder` — LLM tries to reorder and omit entries; merge ignores both

### Tests for User Story 2

- [X] T017 [US2] Add `TestMergeTailoredPreservesExperienceOrder` test in `apps/api/internal/generation/domain/rendercv_test.go`: master with experience `[Acme, Globex, Initech]`, payload with different order `[Initech, Acme, Globex]` and only two Highlights, assert merged experience lists companies in master order `[Acme, Globex, Initech]`, no entry dropped, omitted entry retains master highlights verbatim

**Checkpoint**: Experience order invariant is verified — merge enforces US2. Run `go test ./internal/generation/domain/ -run TestMergeTailoredPreservesExperienceOrder`

---

## Phase 5: User Story 3 - Total Experience Years Integrity (Priority: P1)

**Goal**: Prove that the AI cannot alter experience dates and that text-asserted years contradictions are flagged

**Independent Test**: `TestMergeTailoredPreservesDates` + `TestVerifyStructureIntegrityFlagsYearsAssertion` + `TestVerifyStructureIntegrityNoYearsAssertion`

### Tests for User Story 3

- [X] T018 [P] [US3] Add `TestMergeTailoredPreservesDates` test in `apps/api/internal/generation/domain/rendercv_test.go`: master with experience entry having `start_date: "2019-01"`, `end_date: "2023-06"`, merge with new highlights, assert merged dates are byte-for-byte identical
- [X] T019 [P] [US3] Add `TestVerifyStructureIntegrityFlagsYearsAssertion` test in `apps/api/internal/generation/domain/rendercv_test.go`: master with 5-year derivable total, merged summary asserting "over 12 years of experience", assert exactly one `StructureTotalExperienceYears` violation citing both 12 and 5
- [X] T020 [P] [US3] Add `TestVerifyStructureIntegrityNoYearsAssertion` test in `apps/api/internal/generation/domain/rendercv_test.go`: master with 5-year derivable total, merged summary with no numeric years claim, assert zero violations

### Implementation for User Story 3

- [X] T021 [US3] Implement targeted re-prompt logic for text-asserted-years violation in `apps/api/internal/generation/application/service.go`: on first detection, feed violation message back to LLM in a single additional call
- [X] T022 [US3] Implement strip-and-log fallback for recurrent text-asserted-years violation in `apps/api/internal/generation/application/service.go`: on second detection, strip offending clause from merged text, log intervention on activity row

**Checkpoint**: Total experience years invariant is verified — dates are immutable and text-asserted years contradictions are caught. Run `go test ./internal/generation/domain/ -run "TestMergeTailoredPreservesDates|TestVerifyStructureIntegrity"`

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cross-boundary type regeneration, linting, and final validation

- [X] T023 Run `make tygo-generate` to regenerate `packages/shared/src/generated.ts` (removes `sectionsToDrop`, `experienceOrder`, `drop` from TS types)
- [X] T024 Hand-mirror `TailoredSections`/`TailoredExperience` field removals in `packages/shared/src/index.ts` (per AGENTS.md convention)
- [X] T025 Run `make lint-go` and fix any violations
- [X] T026 Run `make lint-web` (ESLint) and fix any violations
- [X] T027 Run `make test-go` — all Go unit tests pass (including the 5 new domain tests + 1 new application prompt test)
- [X] T028 Run `make test-react` — Vitest tests pass
- [X] T029 Run `make test-lint` — full merge gate: `lint-go` + `lint-web` + `test-go` + `test-react` all pass
- [X] T030 Validate quickstart.md scenarios 1-8 all pass (`make test-lint` covers them all — compile gate is scenario 7)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately to understand current code
- **Foundational (Phase 2)**: Depends on Phase 1 review — BLOCKS all user story test phases
- **US1 Tests (Phase 3)**: Depends on Phase 2 completion
- **US2 Tests (Phase 4)**: Depends on Phase 2 completion (can run parallel with Phase 3)
- **US3 Tests + Implementation (Phase 5)**: Depends on Phase 2 completion + T011-T013 (verifier) + T014 (wiring)
- **Polish (Phase 6)**: Depends on all preceding phases complete

### User Story Dependencies

- **US1 (Block Sequence)**: Independently testable after Phase 2 — no dependency on US2 or US3
- **US2 (Experience Order)**: Independently testable after Phase 2 — no dependency on US1 or US3
- **US3 (Experience Years)**: Depends on Phase 2 + verifier (T011-T013) + wiring (T014); no dependency on US1 or US2 tests

### Within Each User Story

- Domain tests before application tests
- Core merge tests before verifier tests
- Tests run independently per story

### Parallel Opportunities

- T005 and T006 can run in parallel (different struct fields, same file — but sequential may be simpler)
- Phase 3 (US1) and Phase 4 (US2) can run in parallel after Phase 2
- T015 and T016 within US1 can run in parallel
- T018, T019, T020 within US3 can run in parallel
- T023 and T024 can run in parallel

---

## Parallel Example: User Story 1 & 2

```bash
# After Foundational (Phase 2), launch US1 and US2 test tasks in parallel:

# US1 tests:
Task: "T015 - TestMergeTailoredPreservesBlockOrder in rendercv_test.go"
Task: "T016 - TestBuildSelectPromptNoReorderOrDrop in rendercv_llm_test.go"

# US2 tests:
Task: "T017 - TestMergeTailoredPreservesExperienceOrder in rendercv_test.go"
```

---

## Parallel Example: User Story 3

```bash
# All three US3 tests can run in parallel (different test functions, same file):

Task: "T018 - TestMergeTailoredPreservesDates in rendercv_test.go"
Task: "T019 - TestVerifyStructureIntegrityFlagsYearsAssertion in rendercv_test.go"
Task: "T020 - TestVerifyStructureIntegrityNoYearsAssertion in rendercv_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only — just block sequence)

1. Complete Phase 1: Setup (T001-T004)
2. Complete Phase 2: Foundational (T005-T014) — all three invariants structurally enforced
3. Complete Phase 3: US1 tests (T015-T016)
4. **STOP and VALIDATE**: `go test ./internal/generation/domain/ -run TestMergeTailoredPreservesBlockOrder && go test ./internal/generation/application/ -run TestBuildSelectPromptNoReorderOrDrop`

### Incremental Delivery

1. Phase 1 + 2 → Structural invariants enforced
2. Phase 3 → US1 verified (block sequence)
3. Phase 4 → US2 verified (experience order)
4. Phase 5 → US3 verified (experience years)
5. Phase 6 → Cross-boundary types + full merge gate
6. Each phase adds independently verifiable test coverage

### Single Developer Strategy

Execute phases sequentially:
1. Understand code (Phase 1)
2. Implement all enforcement (Phase 2)
3. Verify US1 tests (Phase 3)
4. Verify US2 tests (Phase 4)
5. Verify US3 tests + implementation (Phase 5)
6. Polish (Phase 6)

The foundational phase (Phase 2) is the critical path — once complete, all three stories are structurally enforced; the remaining phases add test verification and cross-boundary type sync.

---

## Notes

- [P] tasks = different files or independent test functions, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story has independently runnable tests
- No schema migration; no `edit_proposals`/`tailored_drafts` tables touched
- Structural enforcements NOT surfaced as user-accept/reject proposals (research R6)
- `protectedSections` map in rendercv.go is kept but no longer on the enforcement path (data-model.md)
- Commit after each phase or logical group
