---

description: "Task list for Fully Editable Resume Profile Tab"
---

# Tasks: Fully Editable Resume Profile Tab

**Input**: Design documents from `/specs/009-editable-resume-profile/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/profile-resume-api.md, quickstart.md

**Tests**: Not explicitly requested as TDD in the spec, but included per Constitution
Principle IV ("a change is not done until its own language's test suite passes locally";
`make test-lint` required when both apps change). Test tasks below are the minimum needed
to satisfy that gate, not a full TDD sweep.

**Organization**: Tasks are grouped by user story (US1/US2/US3, matching spec.md priorities).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3)

## Path Conventions

Existing two-app layout per plan.md: `apps/api/` (Go), `apps/dashboard/` (React),
`packages/shared/` (generated TS). All paths below are repo-root-relative.

---

## Phase 1: Setup

**Purpose**: Dependency/config groundwork, no feature logic yet.

- [X] T001 Add `apps/api/internal/dto` to the package list in `apps/api/tygo.yaml` so its structs generate into `packages/shared/src/generated.ts`
- [X] T002 [P] Add `@dnd-kit/sortable` and `@dnd-kit/utilities` to `apps/dashboard/package.json` (same `@dnd-kit` family as the existing `@dnd-kit/core` dependency)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The typed DTOs, map⇄struct mapping layer, and API/frontend plumbing every user
story's editing flow depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T003 Define `Resume`, `Section`, `Entry`, `SocialNetwork`, `CustomConnection`, and the `EntryType` enum (9 canonical RenderCV types) per data-model.md in `apps/api/internal/dto/resume.go`
- [X] T004 Run `tygo generate` (or `make tygo-generate`) and rebuild `packages/shared` so `Resume`/`Section`/`Entry` types land in `packages/shared/src/generated.ts` (depends on T001, T003)
- [X] T005 Implement `MasterToResume(master generation.RendercvMaster) (dto.Resume, error)` in `apps/api/internal/generation/resume_mapping.go`, inferring each section's `EntryType` from its entries' field shapes and retaining any unmapped fields under `unrecognized` (depends on T003)
- [X] T006 Implement `ResumeToMaster(resume dto.Resume, existing generation.RendercvMaster) (generation.RendercvMaster, error)` in `apps/api/internal/generation/resume_mapping.go`, writing section/entry order back through the existing `_order`/`PrepareMasterForMarshal` convention rather than a parallel marshal path (depends on T005, same file — sequential)
- [X] T007 [P] Add Go unit test in `apps/api/internal/generation/resume_mapping_test.go` asserting `MasterToResume` → `ResumeToMaster` round-trips `testdata/sample_rendercv.yaml` losslessly, including section order (depends on T006)
- [X] T008 Add `GetResume(ctx, id)` and `UpdateResume(ctx, id, dto.Resume)` methods to `apps/api/internal/profile/service.go`, using T005/T006 and persisting through the existing `rendercvYaml`/`rendercvConfig` update path (depends on T006)
- [X] T009 Add `GET /profiles/{id}/resume` and `PUT /profiles/{id}/resume` routes and handlers in `apps/api/internal/httpapi/profiles.go`, per contracts/profile-resume-api.md (depends on T008)
- [X] T010 [P] Add server-side validation in `apps/api/internal/generation/resume_mapping.go` (or a sibling `resume_validation.go`) for the data-model.md rules — non-empty `Resume.name`/`Section.name`, `endDate` not before `startDate` unless `"present"`, no all-blank entries — returning a field-path error per contracts/profile-resume-api.md's 400 shape (depends on T006)
- [X] T011 [P] Add `useResume(profileId)` query hook and `useUpdateResume(profileId)` mutation hook in `apps/dashboard/src/features/profile/hooks.ts` (depends on T004)
- [X] T012 [P] Create shared `ConfirmDialog` component in `apps/dashboard/src/features/profile/components/ConfirmDialog.tsx` for destructive/overwrite confirmations (FR-010, FR-011)

**Checkpoint**: Foundation ready — GET/PUT resume endpoints work end-to-end (even against an empty resume), frontend can fetch/save. User story implementation can now begin.

---

## Phase 3: User Story 1 - Build a resume from scratch with no config file (Priority: P1) 🎯 MVP

**Goal**: A user with no config can build a complete multi-section resume by hand, using a
dedicated structured form for every one of the 9 RenderCV entry types.

**Independent Test**: From a profile with no config data, add at least one entry to every
supported section type, save, and confirm the data persists and reloads correctly — no
YAML file involved (quickstart.md Scenario 1).

### Tests for User Story 1

- [X] T013 [P] [US1] Go handler test in `apps/api/internal/httpapi/profiles_test.go` covering `GET /profiles/{id}/resume` on an empty profile (returns empty-but-valid `Resume`, not an error) and `PUT` populating all 9 entry types then reading them back
- [X] T014 [P] [US1] Vitest in `apps/dashboard/src/features/profile/ProfilePage.test.tsx` asserting the empty-resume state renders as editable (FR-012), not an error/blank screen

### Implementation for User Story 1

- [X] T015 [P] [US1] `IdentityForm.tsx` in `apps/dashboard/src/features/profile/components/IdentityForm.tsx` — name/headline/location/email/phone/website/social networks/custom connections
- [X] T016 [P] [US1] `EducationEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/EducationEntryForm.tsx`
- [X] T017 [P] [US1] `ExperienceEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/ExperienceEntryForm.tsx`
- [X] T018 [P] [US1] `NormalEntryForm.tsx` (projects) in `apps/dashboard/src/features/profile/components/entries/NormalEntryForm.tsx`
- [X] T019 [P] [US1] `PublicationEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/PublicationEntryForm.tsx`
- [X] T020 [P] [US1] `OneLineEntryForm.tsx` (skills) in `apps/dashboard/src/features/profile/components/entries/OneLineEntryForm.tsx`
- [X] T021 [P] [US1] `BulletEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/BulletEntryForm.tsx`
- [X] T022 [P] [US1] `NumberedEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/NumberedEntryForm.tsx`
- [X] T023 [P] [US1] `ReversedNumberedEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/ReversedNumberedEntryForm.tsx`
- [X] T024 [P] [US1] `TextEntryForm.tsx` in `apps/dashboard/src/features/profile/components/entries/TextEntryForm.tsx`
- [X] T025 [US1] `SectionEditor.tsx` in `apps/dashboard/src/features/profile/components/SectionEditor.tsx` — add-entry chrome dispatching to the entry-type forms from T016-T024 (depends on T016-T024)
- [X] T026 [US1] `SectionList.tsx` in `apps/dashboard/src/features/profile/components/SectionList.tsx` — static ordered rendering of sections, add-section flow with entry-type picker (depends on T025)
- [X] T027 [US1] Wire inline validation (required fields, date ordering per FR-007) into the entry forms from T016-T024, surfacing errors without discarding other in-progress edits
- [X] T028 [US1] Rewrite `apps/dashboard/src/features/profile/ProfilePage.tsx` to compose `IdentityForm` + `SectionList` via `useResume`/`useUpdateResume`, with empty-resume handling (FR-012) and clear save-success/failure indication (FR-008) (depends on T011, T015, T026)

**Checkpoint**: User Story 1 is fully functional and independently testable — this is the MVP.

---

## Phase 4: User Story 2 - Import a config file to pre-fill, then keep editing (Priority: P2)

**Goal**: Uploading a config remains an optional convenience; imported data maps correctly
into structured forms (with zero silent loss), and further edits don't require re-upload.

**Independent Test**: Upload `apps/api/internal/generation/testdata/sample_rendercv.yaml`,
verify all fields/sections/entries populate correctly, edit one field in the UI, confirm
persistence without a second upload (quickstart.md Scenario 2).

### Tests for User Story 2

- [X] T029 [P] [US2] Go test in `apps/api/internal/generation/resume_mapping_test.go` asserting `MasterToResume` on `testdata/sample_rendercv.yaml` produces correctly-typed entries for all 9 section types with zero data loss
- [X] T030 [P] [US2] Vitest in `apps/dashboard/src/features/profile/ProfilePage.test.tsx` asserting the reupload-overwrite confirmation dialog appears and blocks upload until confirmed (FR-010)

### Implementation for User Story 2

- [X] T031 [US2] Add a `hasExistingContent` flag to the `GET /profiles/config/status` response in `apps/api/internal/httpapi/profiles.go`, true when the profile has any populated identity field beyond bare name or any section with entries
- [X] T032 [US2] `UnrecognizedEntryFallbackForm.tsx` in `apps/dashboard/src/features/profile/components/entries/UnrecognizedEntryFallbackForm.tsx` — raw key/value editor for data that doesn't match a known entry shape (FR-009)
- [X] T033 [US2] Wire `unrecognized` section/entry data (from T005) into `SectionEditor.tsx` to render via the fallback form from T032, instead of dropping it (depends on T025, T032)
- [X] T034 [US2] Add reupload-overwrite confirmation flow to `ProfilePage.tsx`, using `ConfirmDialog` (T012) gated on the `hasExistingContent` flag (T031) before calling the existing `POST /profiles/config` upload (depends on T012, T028, T031)
- [X] T035 [US2] After a successful config upload, invalidate/refetch the `useResume` query in `apps/dashboard/src/features/profile/hooks.ts` so the structured view reflects the newly imported data immediately (depends on T011)

**Checkpoint**: User Stories 1 and 2 both work independently.

---

## Phase 5: User Story 3 - Manage resume structure: add, edit, delete, reorder (Priority: P2)

**Goal**: Users can reorder entries within a section, reorder sections, delete entries and
whole sections (with confirmation), and add custom-named sections.

**Independent Test**: On a resume with multiple sections/entries, reorder entries within a
section, reorder sections, delete an entry, delete a section — each persists and survives
reload (quickstart.md Scenario 3).

### Tests for User Story 3

- [X] T036 [P] [US3] Go test in `apps/api/internal/httpapi/profiles_test.go` asserting `PUT /profiles/{id}/resume` persists a changed section order and a changed entry order, verified via a subsequent `GET`
- [X] T037 [P] [US3] Vitest in `apps/dashboard/src/features/profile/components/SectionList.test.tsx` covering drag/up-down reorder of sections and entries, and that delete actions require confirmation before removal

### Implementation for User Story 3

- [X] T038 [US3] Add drag-and-drop (via `@dnd-kit/sortable`, T002) plus up/down move buttons for section reordering to `SectionList.tsx` (depends on T002, T026)
- [X] T039 [US3] Add drag-and-drop plus up/down move buttons for entry-within-section reordering to `SectionEditor.tsx` (depends on T002, T025)
- [X] T040 [US3] Add delete-entry action to `SectionEditor.tsx`, gated behind `ConfirmDialog` (T012) per FR-011
- [X] T041 [US3] Add delete-section action to `SectionList.tsx`, gated behind `ConfirmDialog` (T012) per FR-011
- [X] T042 [US3] Verify and, if needed, extend the add-section flow in `SectionList.tsx` (from T026) to accept arbitrary custom section names, not just a fixed preset list (FR-005)
- [X] T043 [US3] Confirm `ProfilePage.tsx` renders a fully empty resume (all sections and fields deleted) as a valid, non-error state (FR-012), adjusting the empty-state check from T028 if it only covered "never had data"

**Checkpoint**: All three user stories are independently functional.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Verification and cleanup spanning all stories.

- [X] T044 [P] Run `make tygo-check` to confirm `packages/shared/src/generated.ts` matches the Go DTOs (no stale generated types)
- [X] T045 [P] Run `make test-lint` (both `go test` and `vitest` suites) per Constitution Principle IV, since this feature touches `apps/api`, `apps/dashboard`, and `packages/shared`
- [ ] T046 Manually execute all three quickstart.md scenarios plus its edge-case checks end-to-end against the running stack
- [X] T047 [P] Sweep `apps/dashboard` for remaining hand-typed/`any`-typed consumers of `ProfileDto.rendercvFull` that could now use the generated `Resume` type instead, per Constitution Principle III

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Setup (T001 before T004). Blocks all user stories.
- **User Story 1 (Phase 3)**: Depends on Foundational completion only.
- **User Story 2 (Phase 4)**: Depends on Foundational completion; reuses `SectionEditor.tsx` (T025) and `ConfirmDialog` (T012) from Foundational/US1 but is independently testable per quickstart.md Scenario 2.
- **User Story 3 (Phase 5)**: Depends on Foundational completion; reuses `SectionList.tsx`/`SectionEditor.tsx` (T025/T026) from US1 but is independently testable per quickstart.md Scenario 3.
- **Polish (Phase 6)**: Depends on all desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: No dependency on US2/US3 — buildable and testable alone (MVP).
- **US2 (P2)**: Builds on files US1 created (`SectionEditor.tsx`, `ProfilePage.tsx`) but adds its own behavior (unrecognized-data fallback, overwrite confirmation) that is independently verifiable without US3's reorder/delete work.
- **US3 (P2)**: Builds on the same shared files but adds independently verifiable reorder/delete behavior; does not require US2 to be done first.

### Within Each User Story

- Tests before implementation where both are listed.
- Entry-type forms (parallel) before the `SectionEditor`/`SectionList` that compose them.
- Story's own checkpoint reached before starting the next priority phase (if working sequentially).

### Parallel Opportunities

- T001/T002 (Setup) in parallel.
- T007, T010, T011, T012 (Foundational, marked [P]) in parallel once their same-file dependencies (T006, T004) land.
- T013/T014 (US1 tests) in parallel.
- T016-T024 (all 9 entry-type forms) fully in parallel — 9 distinct files, no cross-dependencies.
- T029/T030 (US2 tests) in parallel; T036/T037 (US3 tests) in parallel.
- US2 and US3 implementation phases can proceed in parallel by different developers once US1's `SectionEditor.tsx`/`SectionList.tsx`/`ConfirmDialog.tsx` exist, since they touch overlapping files at different, additive call sites (coordinate merges).

---

## Parallel Example: User Story 1

```bash
# Launch both US1 tests together:
Task: "Go handler test for GET/PUT /profiles/{id}/resume in apps/api/internal/httpapi/profiles_test.go"
Task: "Vitest for ProfilePage empty-resume state in apps/dashboard/src/features/profile/ProfilePage.test.tsx"

# Launch all 9 entry-type form components together:
Task: "EducationEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/EducationEntryForm.tsx"
Task: "ExperienceEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/ExperienceEntryForm.tsx"
Task: "NormalEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/NormalEntryForm.tsx"
Task: "PublicationEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/PublicationEntryForm.tsx"
Task: "OneLineEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/OneLineEntryForm.tsx"
Task: "BulletEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/BulletEntryForm.tsx"
Task: "NumberedEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/NumberedEntryForm.tsx"
Task: "ReversedNumberedEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/ReversedNumberedEntryForm.tsx"
Task: "TextEntryForm.tsx in apps/dashboard/src/features/profile/components/entries/TextEntryForm.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL — blocks all stories)
3. Complete Phase 3: User Story 1
4. **STOP and VALIDATE**: run quickstart.md Scenario 1 end-to-end
5. Demo: a resume built entirely by hand, no config file

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. Add US1 → validate via quickstart.md Scenario 1 → MVP demo
3. Add US2 → validate via quickstart.md Scenario 2 → demo
4. Add US3 → validate via quickstart.md Scenario 3 → demo
5. Polish (Phase 6) → `make test-lint`, `make tygo-check`, full quickstart pass

### Parallel Team Strategy

1. One developer/pair completes Setup + Foundational.
2. Once Foundational lands: one dev takes US1 (blocking, since US2/US3 build on its files),
   or — if timeline allows sequencing — US2 and US3 can be split across two developers
   immediately after US1's `SectionEditor.tsx`/`SectionList.tsx`/`ConfirmDialog.tsx` exist,
   coordinating merges on those shared files.

---

## Notes

- [P] tasks touch different files with no unmet dependencies.
- All 9 entry-type forms are structurally identical in role (typed form over a bounded
  field set) — safe to parallelize freely.
- `SectionEditor.tsx` and `SectionList.tsx` are the two files every story after US1 extends;
  keep their diffs additive (new props/handlers) to avoid US2/US3 merge conflicts.
- Commit after each task or logical group; stop at each phase checkpoint to validate that
  story independently before moving on, per repo convention favoring small, per-feature commits.
