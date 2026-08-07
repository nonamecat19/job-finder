---

description: "Task list for resume generation strictness & model improvement"
---

# Tasks: Resume Generation Strictness & Model Improvement

**Input**: Design documents from `/specs/033-resume-gen-strictness/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/contracts.md, quickstart.md

**Tests**: Included — the constitution (Principle IV) mandates `make test-lint` before merge, and the quickstart validation scenarios are unit-testable. Tests are written alongside implementation in the Go native toolchain.

**Organization**: Tasks are grouped by user story (US1 P1, US2 P2, US3 P3) to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story this task belongs to (e.g. US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- Go backend: `apps/api/internal/...`
- Gateway config: `gateway/config.yaml`
- Tests: alongside source in `apps/api/internal/.../*_test.go`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: No project initialization needed — this is an existing Go backend feature. The only setup is creating the feature branch and confirming the test baseline is green.

- [ ] T001 Create feature branch `033-resume-gen-strictness` from `master` (per AGENTS.md branching rules — never commit to master)
- [ ] T002 Run `make test-go` on `master` and record the baseline grounding test pass count, so post-feature regression is measurable

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The `ResponseMode` enum and the gateway `chatRequest.ResponseFormat` struct upgrade are shared infrastructure used by US2 and referenced by US1's grounding loop. They MUST land before any user story work.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [ ] T003 Add `ResponseMode` enum and `ResponseMode` field to `CompleteOptions` in `apps/api/internal/platform/llm/domain/port.go` (zero value = `ResponseModeJSON`, preserving current behaviour). Contract C5.
- [ ] T004 Upgrade `chatRequest.ResponseFormat` from `map[string]string` to a pointer-to-struct (`*responseFormat` with `Type` + optional `JSONSchema`) in `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go`. Update `CompleteJSON` to build the format from `opts.ResponseMode`: `ResponseModeJSON` → `{"type":"json_object"}` (byte-identical to today), `ResponseModeStrict` → `{"type":"json_schema","json_schema":{...,"strict":true}}`. Contracts C5, C6.
- [ ] T005 [P] Add unit test asserting `CompleteJSON` with `ResponseModeJSON` produces a byte-identical `response_format` to the current request shape, in `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go`
- [ ] T006 [P] Add unit test asserting `CompleteJSON` with `ResponseModeStrict` produces `response_format.type == "json_schema"` and `json_schema.strict == true`, in `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go`
- [ ] T007 Thread the JSON Schema from `CompleteStructured` (`schemaFor` in `port.go:84`) to the gateway adapter so `ResponseModeStrict` can attach it. Path: `CompleteStructured` sets `opts.ResponseMode = ResponseModeStrict` and attaches the schema string; the gateway parses it into `jsonSchema.Schema`. Contract C6.
- [ ] T008 Run `make test-go` — foundational tests pass, no caller behaviour changed (every existing caller leaves `ResponseMode` at zero value = `ResponseModeJSON`)

**Checkpoint**: Foundation ready — `ResponseMode` is opt-in, existing callers unchanged, strict schema path is wired through the gateway.

---

## Phase 3: User Story 1 — Grounded tailoring at the default level (Priority: P1) 🎯 MVP

**Goal**: Close the moderate-grounding skill-token gap and the unused highlight-drift check, so a tailoring run at the default level produces a resume whose skills and bullets all trace to the master profile.

**Independent Test**: Run a tailoring pass at moderate grounding against a master with {Go, Postgres, Docker} and a vacancy asking for {Terraform, Kubernetes}. Assert the merged resume contains only master skills, the activity trail logs the drops, and no highlight is below the word-overlap threshold. (quickstart.md Scenario 1 & 2)

### Tests for User Story 1

> Write these FIRST, ensure they FAIL before implementation.

- [ ] T009 [P] [US1] Add unit test in `apps/api/internal/generation/domain/rendercv_grounding_test.go`: a merged resume with a non-master skill token at `GroundingModerate` produces a `skill "..." not in master profile` violation. Today this passes (no violation) — the test must assert it fails first.
- [ ] T010 [P] [US1] Add unit test in `apps/api/internal/generation/domain/rendercv_grounding_test.go`: a merged experience highlight with <50% word overlap against every master bullet for that company produces a `experience "..." highlight not grounded in master` violation at all three grounding levels.
- [ ] T011 [P] [US1] Add unit test in `apps/api/internal/generation/domain/rendercv_structure_test.go`: `StripUngroundedHighlights` replaces a drifted highlight with the highest-overlap master bullet and leaves grounded highlights untouched.

### Implementation for User Story 1

- [ ] T012 [US1] Extend `VerifyRendercvGrounding` in `apps/api/internal/generation/domain/rendercv_grounding.go` to check skill tokens at `GroundingModerate` and `GroundingAggressive`, not only `GroundingStrict`. Add the `analysis VacancyAnalysis` parameter (contract C1). Implement `AdjacentSkillAllowed(master, token, analysis)` for the moderate adjacency allowance (research.md R1, data-model.md §AdjacentSkillAllowed). Strict path unchanged (master tokens only, `analysis` ignored).
- [ ] T013 [US1] Add highlight-drift detection to `VerifyRendercvGrounding` (same file): for every experience highlight, run `lcsCovered` against the master's highlights for that company at all grounding levels. Emit `experience "<company>" highlight not grounded in master: "<truncated>"`. Contract C1.
- [ ] T014 [US1] Add `StripUngroundedHighlights(master, merged)` in `apps/api/internal/generation/domain/rendercv_structure.go`: for each highlight that fails `lcsCovered`, replace it with the highest-overlap master bullet for that company. Returns a new `RendercvMaster`. Contract C3. Add `StructureHighlightDrift` to the `StructureKind` constants.
- [ ] T015 [US1] Wire `DropUngroundedSkillTokens(master, merged)` into the primary tailoring pass in `apps/api/internal/generation/application/service.go` — call it after `MergeTailored` + `ApplyHardLimits` at line ~232, before `VerifyRendercvGrounding`. Contract C2.
- [ ] T016 [US1] Update the `VerifyRendercvGrounding` call site in `service.go` (~line 238) to pass `analysis` (the new parameter from T012).
- [ ] T017 [US1] Extend `fixStructureIntegrity` in `service.go` (~line 419) to also call the highlight-drift check after the years-assertion check: verify → one re-prompt via `retailorForStructure` → verify → `StripUngroundedHighlights` if still drifted. Log each intervention on the activity row via `rec.Step`. Contract C3, FR-003, FR-010.
- [ ] T018 [US1] Run `DropUngroundedSkillTokens` + the extended `VerifyRendercvGrounding` after the expand and condense merges in `service.go` (~lines 359, 398) so the page-fitting passes have the same strictness as the primary pass. FR-009.
- [ ] T019 [US1] Run `make test-go` — all US1 tests pass, no regression in existing grounding/structure tests

**Checkpoint**: User Story 1 is fully functional. A tailoring run at the default level now enforces skill tokens and highlight grounding end-to-end.

---

## Phase 4: User Story 2 — A prompt that enforces, not just suggests (Priority: P2)

**Goal**: The prompt matches the data contract (no removed fields) and the structured-output call uses `json_schema` with `strict: true`, constraining the model at the API level.

**Independent Test**: Capture the prompt and the request body for a generation call. Assert (a) the prompt has no `sectionsToDrop`/`ExperienceOrder`/`Drop` references, (b) `response_format.type == "json_schema"` and `strict == true`, (c) `additionalProperties == false` in the schema. (quickstart.md Scenario 3 & 4)

### Tests for User Story 2

- [ ] T020 [P] [US2] Add unit test in `apps/api/internal/generation/application/rendercv_llm_test.go`: `buildSelectPrompt` output contains no occurrence of `sectionsToDrop`, `ExperienceOrder`, or `Drop` as a field reference.
- [ ] T021 [P] [US2] Add unit test in `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go`: `CompleteJSON` with `ResponseModeStrict` sends a request whose `response_format.json_schema.schema` has `additionalProperties: false`.

### Implementation for User Story 2

- [ ] T022 [US2] Clean `buildSelectPrompt` in `apps/api/internal/generation/application/rendercv_llm.go:109-218`: remove the `Do not populate sectionsToDrop.` line (line 173). Audit all other prompt builders (`buildExpandPrompt`, `buildCondensePrompt`) for references to removed `TailoredSections` fields and remove any found. Contract C4.
- [ ] T023 [US2] Set `ResponseMode: ResponseModeStrict` on every generation structured call in `apps/api/internal/generation/application/rendercv_llm.go`: `selectAndTailor` (line 222), `retailorForStructure` (line 236), `expandContent` (line 244), `condenseContent` (line 321). Leave `analyzeVacancy` (line 55) on the default `ResponseModeJSON` for now — it is not the quality-critical path and its chain is not yet verified for `json_schema` (out of scope per research.md R4).
- [ ] T024 [US2] Set `MaxTokens` to an explicit cap (~4096, tuned to the largest `TailoredSections` payload) on every generation structured call in `rendercv_llm.go` (same four call sites as T023). Also set a smaller cap on `analyzeVacancy` and `writeCoverLetter` in `service.go`. Contract C7, FR-012.
- [ ] T025 [US2] Verify the `jsonschema.Reflector` config in `port.go:84-97` produces `additionalProperties: false` in the generated schema. If the `invopop/jsonschema` default does not, set the reflector option or post-process the schema map before sending. Contract C6.
- [ ] T026 [US2] Run `make test-go` — all US2 tests pass; verify existing `CompleteStructured` callers (match, rephrase, ghost, default) are unchanged (they leave `ResponseMode` at zero value)

**Checkpoint**: User Story 2 is functional. The generation task sends a strict schema; the prompt matches the data contract. Non-generation tasks are byte-identical to before.

---

## Phase 5: User Story 3 — A generation model chosen for strictness (Priority: P3)

**Goal**: The primary `generation` model is selected from an empirical strictness benchmark, not a general-quality hunch.

**Independent Test**: Run the benchmark fixture across all generation-chain models against a fixed profile×vacancy matrix. Assert the selected primary has the lowest combined violation rate and meets the 60s average bar. Update `gateway/config.yaml`, restart litellm, confirm the served-model log shows the new model. (quickstart.md Scenario 6)

### Tests for User Story 3

- [ ] T027 [P] [US3] Add a benchmark fixture in `apps/api/internal/generation/application/benchmark_test.go`: a Go test that runs `tailorRendercvResume` (or the grounding loop directly) for each model in the generation chain against a fixed set of master profiles × vacancies, recording grounding violations, structural violations, JSON-parse failures, and wall-clock time per model. This is a manual/long-running test (guard with `testing.Short()` skip or a build tag) — it calls live providers.

### Implementation for User Story 3

- [ ] T028 [US3] Run the benchmark (T027) against the live generation chain (`gateway/config.yaml` tiers 1-4 + local Ollama). Record the results in a table: model | grounding violations | structural violations | JSON-parse failures | median time. Research.md R6.
- [ ] T029 [US3] Select the primary generation model: the lowest combined violation rate that meets the 60s average bar (020-SC-007). Document the selection rationale (which model, why, the numbers) in a comment block at the top of the `generation` section in `gateway/config.yaml`. Contract C8, FR-007, SC-004.
- [ ] T030 [US3] Update the `model:` line for the `generation` primary in `gateway/config.yaml` to the selected model. Verify the selected model supports `json_schema` strict mode (extends 030-C5 — config-time verification, not runtime). Contract C8.
- [ ] T031 [US3] Restart litellm (`docker compose restart litellm`) and run a tailoring pass; confirm the `served_model` log line shows the new primary model. Verify the chain still terminates at `local` (Constitution V, 030-FR-008). SC-005.
- [ ] T032 [US3] Run the quickstart Scenario 6 validation end-to-end

**Checkpoint**: All user stories complete. The generation model is evidence-backed.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cross-story validation, docs, and the merge gate.

- [ ] T033 [P] Run `make test-go` — full Go suite green
- [ ] T034 [P] Run `make lint-go` — golangci-lint clean on touched packages (`generation`, `platform/llm`)
- [ ] T035 Run `make test-lint` — the merge gate (lint-go + lint-web + test-go + test-react). Must pass before PR.
- [ ] T036 [P] Run the full `quickstart.md` validation (all 7 scenarios) end-to-end against the live stack
- [ ] T037 Verify no `make tygo-generate` is needed — confirm `packages/shared/src/generated.ts` is unchanged (contract C10, no DTO changes)
- [ ] T038 Commit on the feature branch (conventional commit format, per AGENTS.md). Do NOT push to master — open a PR.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — can start immediately.
- **Foundational (Phase 2)**: Depends on Setup. BLOCKS all user stories (the `ResponseMode` plumbing is shared).
- **US1 (Phase 3)**: Depends on Foundational. No dependency on US2 or US3 — the grounding changes work regardless of prompt/schema mode.
- **US2 (Phase 4)**: Depends on Foundational. No hard dependency on US1 — the prompt cleanup and strict schema are independent of the grounding verifier. May be done in parallel with US1.
- **US3 (Phase 5)**: Depends on US1 + US2 — the benchmark measures the violation rates produced by the new grounding checks (US1) and the strict schema (US2). Running it before those land measures the old behaviour, not the new.
- **Polish (Phase 6)**: Depends on all user stories being complete.

### User Story Dependencies

- **US1 (P1)**: Can start after Foundational. No dependencies on other stories.
- **US2 (P2)**: Can start after Foundational. Independent of US1 (different files: `rendercv_llm.go` prompt + `gateway.go` schema vs `rendercv_grounding.go` + `service.go` loop).
- **US3 (P3)**: Depends on US1 + US2 being complete — the benchmark is only meaningful against the new strictness stack.

### Within Each User Story

- Tests written FIRST and must FAIL before implementation.
- Domain checks (`rendercv_grounding.go`, `rendercv_structure.go`) before the service-loop wiring (`service.go`).
- Prompt/schema changes (`rendercv_llm.go`, `gateway.go`) before the benchmark (US3).

### Parallel Opportunities

- T005, T006 (foundational gateway tests) — different test cases, same file, can be written together but are independent assertions.
- T009, T010, T011 (US1 tests) — different files (`rendercv_grounding_test.go` vs `rendercv_structure_test.go`), fully parallel.
- T020, T021 (US2 tests) — different files (`rendercv_llm_test.go` vs `gateway_test.go`), fully parallel.
- US1 (Phase 3) and US2 (Phase 4) can be worked on in parallel by different developers after Foundational completes — they touch disjoint files except `service.go` (US1 touches the loop; US2 touches the call-site options, which are in `rendercv_llm.go`). Coordinate the one shared file if both touch `service.go`.
- T027 (benchmark fixture) can be written in parallel with US1/US2 implementation, but can only be meaningfully run after both ship.

---

## Parallel Example: User Story 1

```bash
# Launch all US1 test tasks together (different files):
Task: "T009 grounding test for moderate skill tokens in rendercv_grounding_test.go"
Task: "T010 grounding test for highlight drift in rendercv_grounding_test.go"
Task: "T011 strip-and-replace test in rendercv_structure_test.go"

# Then implement (sequential — same files as the tests, ordered by dependency):
Task: "T012 extend VerifyRendercvGrounding for skill tokens at moderate"
Task: "T013 add highlight-drift detection to VerifyRendercvGrounding"
# T012 and T013 are the same file — do not parallelize.
```

## Parallel Example: US1 + US2 concurrent

```bash
# After Foundational (Phase 2):
Developer A (US1): rendercv_grounding.go → rendercv_structure.go → service.go loop
Developer B (US2): rendercv_llm.go prompt cleanup + ResponseMode → gateway.go schema
# Shared file: service.go — US1 edits the loop, US2 edits the call-site options in rendercv_llm.go (no service.go edit). No conflict.
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (branch + baseline).
2. Complete Phase 2: Foundational (`ResponseMode` plumbing — shared, but only the `ResponseModeJSON` path is exercised by US1; the strict path is dormant until US2).
3. Complete Phase 3: User Story 1 — the grounding gap is the root cause the user reported.
4. **STOP and VALIDATE**: run quickstart Scenario 1 & 2. The core complaint is fixed.
5. Open a PR if ready — US1 alone is a shippable improvement.

### Incremental Delivery

1. Setup + Foundational → foundation ready, zero behaviour change.
2. Add US1 → test independently → the default-level grounding gap is closed (the reported bug).
3. Add US2 → test independently → the prompt matches the contract, strict schema reduces violations at the source.
4. Add US3 → test independently → the model is evidence-backed.
5. Each story adds value without breaking previous stories; US1 alone is the MVP.

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks.
- [Story] label maps task to a specific user story for traceability.
- Each user story is independently completable and testable.
- Verify tests fail before implementing (T009-T011, T020-T021 must fail first).
- Commit after each task or logical group (conventional commit format, per AGENTS.md).
- Stop at any checkpoint to validate a story independently.
- Never commit or push to `master` — branch + PR only (AGENTS.md).
- `make setup-hooks` must be run once per clone (AGENTS.md) — the pre-commit/pre-push hooks enforce the branch rule.

---

## Phase 7: Convergence

**Purpose**: Remaining work found by assessing the codebase against spec.md, plan.md, and the existing tasks. These close gaps the original task generation missed.

- [ ] T039 [US1] Rewrite the assertion in `apps/api/internal/generation/domain/rendercv_test.go:394-413` (`TestVerifyRendercvGrounding_StrictRejectsUnlistedSkill`) so the moderate-grounding branch asserts moderate now rejects the unlisted skill token (the old `if len(violationsModerate) != 0 { t.Fatalf("moderate grounding should not check skill tokens"...) }` assertion is inverted by T012). Also add a `VacancyAnalysis{}` argument to every `VerifyRendercvGrounding` call site in that test function. Per FR-001, T012 (missing)
- [ ] T040 [US1] Update all remaining existing test call sites of `VerifyRendercvGrounding` to the new 4-argument signature `(master, merged, level, analysis)`: `apps/api/internal/generation/domain/rendercv_test.go:388,404,409,424` and `apps/api/internal/generation/domain/rendercv_grounding_test.go:41,54,68,80`. Pass `VacancyAnalysis{}` at non-moderate call sites; at moderate call sites, pass a `VacancyAnalysis` appropriate to the adjacency under test. Per T012 (missing)
- [ ] T041 [US1] Log skill-token drops on the primary pass in `apps/api/internal/generation/application/service.go`: after `domain.DropUngroundedSkillTokens(master, merged)` (T015), emit a `rec.Step(ctx, "grounding: ungrounded skill tokens dropped", map[string]any{"tokens": [...]})` recording which tokens were removed. `DropUngroundedSkillTokens` mutates in place and is silent — capture the removed tokens before the call and log the diff. Per FR-010 (partial)
- [ ] T042 [US2] Add runtime fallback to `ResponseModeJSON` in `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go`: when a `ResponseModeStrict` call receives a 400/422 indicating the provider does not support `json_schema` (or that `response_format` was dropped), retry the same call once with `response_format: {"type":"json_object"}` and rely on the existing JSON-parse retry loop in `CompleteStructured`. This prevents the 030-C5 capability trap at runtime and satisfies quickstart Scenario 5. Per FR-006, US2/AC3 (missing)
- [ ] T043 [US3] Run the benchmark fixture (T027) against the pre-feature `master` branch to record the "before" grounding/structural/JSON-parse violation rate per model, so the post-feature run can compute the ≥50% drop required by SC-001. Record the before-numbers alongside the T028 after-numbers in the `gateway/config.yaml` selection-rationale comment block. Per SC-001 (missing)