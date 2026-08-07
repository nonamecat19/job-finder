---

description: "Task list for split-model resume generation"
---

# Tasks: Split-Model Resume Generation

**Input**: Design documents from `/specs/035-split-model-generation/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/contracts.md, quickstart.md

**Preconditions (already landed, not tasks here)**: the two fixes without which no generation run
succeeds at all — `generationMaxTokens` 4096 → 16384 and `analysisMaxTokens` 2048 → 8192 in
`apps/api/internal/generation/application/rendercv_llm.go`, and `strictifySchema` in
`apps/api/internal/platform/llm/domain/port.go`. Committed separately as a bugfix; T002/T003 below
add their regression cover.

**Tests**: Included. Constitution IV requires each language's suite to pass, and this feature's
core risk (silently truncated selection output) is only observable through tests against
deliberately degraded fixtures.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to
- Exact file paths included in every task

## Path Conventions

Go API at `apps/api/`, React dashboard at `apps/dashboard/`, shared TS types at `packages/shared/`,
routing config at `gateway/config.yaml`. Paths below are repository-relative.

---

## Phase 1: Setup

**Purpose**: Routing configuration the stages address, and regression cover for the two
preconditions listed above.

- [X] T001 Add `generation-analyze`, `generation-select`, `generation-select-premium` and `generation-summary` deployments with their fallback chains — including a chain for the escalation key — and reasoning switches to `gateway/config.yaml`, per contracts/contracts.md §1
- [X] T002 [P] Add a table test for `strictifySchema` covering all-properties-required, `additionalProperties:false`, optional-field nullability and `$schema`/`$id` removal in `apps/api/internal/platform/llm/domain/port_test.go`
- [X] T003 [P] Add a regression test asserting per-stage output caps are sent as `max_completion_tokens` in `apps/api/internal/platform/llm/infrastructure/gateway/gateway_test.go`

**Checkpoint**: Gateway resolves each new task key; `docker compose restart litellm` then `curl` each key directly to confirm before proceeding.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The type split, the router set and the persistence columns every story below depends on.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T004 Split `TailoredSections` into `TailoredSelection` (no summary field) and `TailoredSummary` in `apps/api/internal/generation/domain/rendercv.go`, keeping `TailoredSections` as the merged shape
- [X] T005 Update `MergeTailored` to assemble a merged document from a `TailoredSelection` plus an optional `TailoredSummary` in `apps/api/internal/generation/domain/rendercv.go`
- [X] T006 [P] Add the `SummaryBrief` type (analysis, derived years, selected highlights, leading skill groups) in `apps/api/internal/generation/domain/rendercv.go`
- [X] T007 [P] Add `StageOutcome` (stage, requested key, served model, substituted, escalated, duration, cost, tokens) in `apps/api/internal/generation/application/stage_outcome.go`
- [X] T008 Capture `usage.cost`, prompt and completion tokens from the gateway response alongside the existing served-model capture in `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go`
- [X] T009 Add `GenerationRouters` and construct the four routers (`generation-analyze`, `generation-select`, `generation-select-premium`, `generation-summary`) plus the retained `generation` cover router in `apps/api/cmd/server/compose.go`
- [X] T010 Change `NewService` to take `GenerationRouters` and store one provider per stage in `apps/api/internal/generation/application/service.go`
- [X] T011 Write migration `apps/api/internal/db/migrations/00038_document_stage_provenance.sql` adding `summaryModel`, `summarySubstituted`, `selectionModel`, `selectionEscalated`, `stageCostUsd` to `GeneratedDocument`, all nullable or defaulted
- [X] T012 Regenerate sqlc models and update `InsertGeneratedDocument` for the new columns in `apps/api/internal/db/`
- [X] T013 Add per-stage context deadlines (analyze 90s, select 240s, summary 120s) and per-stage output caps in `apps/api/internal/generation/application/rendercv_llm.go`

**Checkpoint**: `go build ./...` passes, migration applies, every stage has a router and a deadline.

---

## Phase 3: User Story 1 - A summary worth the money, on a resume that costs cents (Priority: P1) 🎯 MVP

**Goal**: Summary produced by the premium stage, everything else by the economy stage, with per-part provenance recorded and shown.

**Independent Test**: Run one tailoring pass; assert the API log shows one request per task key, that the summary's served model differs from the selection's, and that measured cost and wall clock beat the pre-split baseline.

### Tests for User Story 1

- [X] T014 [P] [US1] Integration test asserting each stage requests its own task key and no provider or model name leaves the application, using a fake provider per stage in `apps/api/internal/generation/application/stage_routing_test.go`
- [X] T015 [P] [US1] Test that `SummaryBrief` excludes the master profile and includes the derived years figure and selected highlights in `apps/api/internal/generation/application/rendercv_llm_test.go`
- [X] T016 [P] [US1] Test that page fitting cannot alter the summary (fake page-fit provider returning a different summary; rendered summary unchanged) in `apps/api/internal/generation/application/page_fit_test.go`
- [X] T017 [P] [US1] Golden-file test that the merged document's section set and ordering are byte-identical before and after the split, for a fixed fixture and fake stage responses, in `apps/api/internal/generation/application/structure_parity_test.go` (FR-018, SC-006)
- [X] T018 [P] [US1] Service test that a fallback-served summary sets `summarySubstituted` and records the served model, using a fake provider reporting a fallback model, in `apps/api/internal/generation/application/substitution_test.go` (FR-012, SC-003)
- [X] T019 [P] [US1] Integration test that with no gateway configured every stage routes to the local provider and a resume is still produced, in `apps/api/internal/generation/application/local_only_test.go` (FR-011, SC-008)
- [X] T020 [P] [US1] Dashboard test that the substitution marker renders when `summarySubstituted` is true and is absent otherwise in `apps/dashboard/src/features/tailor/TailorPage.test.tsx`

### Implementation for User Story 1

- [X] T021 [US1] Split `selectAndTailor` into `selectContent` returning `TailoredSelection`, dropping all summary instructions from its prompt, in `apps/api/internal/generation/application/rendercv_llm.go`
- [X] T022 [US1] Add `writeSummary` taking a `SummaryBrief` and returning `TailoredSummary`, with its own trimmed prompt, in `apps/api/internal/generation/application/rendercv_llm.go`
- [X] T023 [US1] Change `expandContent` and `condenseContent` to return `TailoredSelection` so the summary is structurally untouchable in `apps/api/internal/generation/application/rendercv_llm.go`
- [X] T024 [US1] Rewrite `tailorRendercvResume` to orchestrate analyze → select → summary → merge → page fit, routing each stage to its own provider, in `apps/api/internal/generation/application/service.go`
- [X] T025 [US1] Record a `StageOutcome` per stage as activity steps and persist summary/selection provenance and total cost onto the document in `apps/api/internal/generation/application/service.go`
- [X] T026 [P] [US1] Add `summaryModel`, `summarySubstituted`, `selectionEscalated`, `stageCostUsd` to `GeneratedDocumentDto` in `apps/api/internal/dto/documents.go` and regenerate `packages/shared`
- [X] T027 [US1] Render the substitution marker on the resume result surface in `apps/dashboard/src/features/tailor/TailorPage.tsx`

**Checkpoint**: A tailoring run produces a resume whose summary and selection came from different models, with provenance visible in the response and on the page, and whose structure is provably unchanged.

---

## Phase 4: User Story 2 - Cheap work that is caught when it cuts corners (Priority: P1)

**Goal**: Silently truncated selection output is detected before rendering and escalated rather than shipped.

**Independent Test**: Feed a deliberately truncated selection response; assert the shortfall is detected, the stage retries, a second shortfall escalates to the premium router, and the truncated document never renders.

### Tests for User Story 2

- [X] T028 [P] [US2] Unit tests for `VerifyCompleteness`: a missing vacancy-required token is a shortfall; 79% nice-to-have retention is a shortfall and 80% is not; a company below `ExperienceBulletsMin` is reported; an analysis with no required skills sets `StructuralFallback` — in `apps/api/internal/generation/domain/rendercv_completeness_test.go`
- [X] T029 [P] [US2] Integration test that two consecutive shortfalls escalate the selection stage to the premium router, the run completes, and `selectionEscalated` is true in `apps/api/internal/generation/application/escalation_test.go`
- [X] T030 [P] [US2] Test that healthy selection output triggers no retry and no escalation in `apps/api/internal/generation/application/escalation_test.go`

### Implementation for User Story 2

- [X] T031 [US2] Implement `VerifyCompleteness` returning `CompletenessReport`, partitioning master skill tokens by `analysis.RequiredSkills` / `NiceToHaveSkills` and reusing the existing tokenizer, in `apps/api/internal/generation/domain/rendercv_completeness.go`
- [X] T032 [US2] Implement the structural fallback path (skill group count equals master's) when the analysis lists no required skills, setting `StructuralFallback`, in `apps/api/internal/generation/domain/rendercv_completeness.go`
- [X] T033 [US2] Gate rendering on the completeness report and wire the retry-then-escalate ladder into the selection loop in `apps/api/internal/generation/application/service.go`
- [X] T034 [US2] Record every shortfall, the structural fallback, and the escalation with reasons as activity steps in `apps/api/internal/generation/application/service.go`

**Checkpoint**: US1 + US2 together are shippable — the cost saving no longer risks a hollowed-out resume.

---

## Phase 5: User Story 3 - The summary is verified as its own artifact (Priority: P2)

**Goal**: Summary-specific grounding: no absent skill, no unsupported metric, no contradicting years figure; one re-prompt, then strip and log.

**Independent Test**: Run the summary stage against a vacancy demanding skills the candidate lacks; assert none appear and any violation was logged.

### Tests for User Story 3

- [X] T035 [P] [US3] Unit tests for summary grounding: absent skill token, unsupported numeric metric, contradicting years figure — each detected — in `apps/api/internal/generation/domain/rendercv_grounding_test.go`
- [X] T036 [P] [US3] Integration test that a violating summary is re-prompted once and, if it still violates, the claim is stripped, logged, and the resume still delivered in `apps/api/internal/generation/application/summary_grounding_test.go`

### Implementation for User Story 3

- [X] T037 [US3] Add `VerifySummaryGrounding` (skill tokens against master, numeric metrics against master-supported figures, years figure against the derived value) in `apps/api/internal/generation/domain/rendercv_grounding.go`
- [X] T038 [US3] Wire the summary verification loop — verify, one re-prompt, then strip-and-log — into the summary stage in `apps/api/internal/generation/application/service.go`
- [X] T039 [US3] Record each summary intervention with its reason on the activity record in `apps/api/internal/generation/application/service.go`

**Checkpoint**: The failure that motivated this feature (a fabricated summary) is now caught by an automated check, not by reading the PDF.

---

## Phase 6: User Story 4 - Cover letters only when asked for (Priority: P2)

**Goal**: No automatic cover letter from either entry point; one produced on demand against an existing resume.

**Independent Test**: Run tailoring and assert no cover letter is produced; request one for that resume and assert it is produced and retrievable.

### Tests for User Story 4

- [X] T040 [P] [US4] Handler test that `POST /api/documents/tailor` returns a null cover letter and that job-triggered generation produces a resume only, in `apps/api/internal/generation/interfaces/http/documents_test.go`
- [X] T041 [P] [US4] Handler test for `POST /api/documents/{id}/cover-letter`: 200 with a cover letter, 404 for an unknown id, 409 when the target is not a resume, and version increment on repeat, in `apps/api/internal/generation/interfaces/http/documents_test.go`

### Implementation for User Story 4

- [X] T042 [US4] Remove the cover-letter call from `GenerateAdHoc` and from the job-triggered generation path in `apps/api/internal/generation/application/service.go`
- [X] T043 [US4] Add `GenerateCoverLetterFor(ctx, resumeID)` served by the `generation` router, reusing the existing versioning rules, in `apps/api/internal/generation/application/service.go`
- [X] T044 [US4] Mount `POST /documents/{id}/cover-letter` in `apps/api/internal/generation/interfaces/http/documents.go`
- [X] T045 [US4] Replace the automatic cover-letter result with an on-demand action on the tailor surface in `apps/dashboard/src/features/tailor/TailorPage.tsx` and its hook in `apps/dashboard/src/features/tailor/hooks.ts`

**Checkpoint**: Every run is one LLM call cheaper and shorter; cover letters remain available.

---

## Phase 7: User Story 5 - The operator retunes stages without touching the application (Priority: P3)

**Goal**: Stage-to-model assignment, including reasoning bounds, is a configuration edit plus one service restart.

**Independent Test**: Repoint one stage at a different model, restart only the routing service, confirm the next run uses it with no rebuild or migration.

### Tests for User Story 5

- [X] T046 [P] [US5] Config test asserting **every** `generation-*` model group appears in `litellm_settings.fallbacks`, every chain's last entry is `local`, every deployment declares a reasoning bound, and no literal API key appears, in `apps/api/internal/platform/llm/gateway_config_test.go`

### Implementation for User Story 5

- [X] T047 [US5] Document the stage keys, their chains (including the escalation key), the reasoning-switch requirement per provider family, and the retune procedure in `specs/domains/llm-routing.md`
- [X] T048 [US5] Add the local-model env fallbacks for the new stage keys (`LLM_MODEL_GENERATION_*`) to `apps/api/internal/config/config.go` and `.env.example`

**Checkpoint**: A model swap is a YAML edit and a restart, and the config test fails loudly if any chain — escalation included — stops short of the local model.

---

## Phase 8: Polish & Cross-Cutting Concerns

- [ ] T049 [P] Extend the generation benchmark to record per-stage latency, tokens and cost and assert the SC-001 (≤⅕ cost) and SC-002 (≤½ time) targets against the recorded baseline, in `apps/api/internal/generation/application/benchmark_test.go`
- [X] T050 [P] Update `specs/domains/resume-generation.md` with the staged pipeline, the completeness gate and summary immutability
- [X] T051 [P] Remove the now-unused combined tailoring prompt path and its dead helpers in `apps/api/internal/generation/application/rendercv_llm.go`
- [ ] T052 Run the quickstart scenarios end to end per `specs/035-split-model-generation/quickstart.md`, including the forced premium outage and the gateway-unconfigured run
- [X] T053 Run `make test-lint` and `make test-integration`; both must pass before this feature is done

---

## Dependencies

**Phase order**: Setup (T001-T003) → Foundational (T004-T013) → US1 → US2 → US3 → US4 → US5 → Polish.

**Story dependencies**:

- **US1** depends only on Foundational. It is the MVP.
- **US2** depends on US1 (it verifies the selection stage US1 creates). US1 and US2 are both P1 and ship together — US1 alone delivers the cost saving with the truncation risk unguarded.
- **US3** depends on US1 (it verifies the summary stage US1 creates). Independent of US2.
- **US4** is independent of US1-US3 after Foundational; it can be built in parallel by a second worker.
- **US5** depends on T001 only; its documentation task can start any time.

**Within Foundational**: T004 blocks T005; T007 and T008 are independent of the type split; T009 blocks T010; T011 blocks T012.

## Parallel Execution Examples

**Foundational**: T006, T007, T008 in parallel (three different files, no shared symbols). T011 in parallel with all of them.

**US1 tests**: T014-T020 all in parallel — seven separate test files across Go and TypeScript.

**US2**: T028 (verifier unit tests) parallel with T029/T030 (integration), since the verifier and the ladder live in different packages.

**Across stories**: once Foundational is done, one worker can take US4 (cover letter, HTTP + dashboard) while another takes US1 → US2 (generation pipeline). They share only `service.go`, so sequence T042/T043 after T024/T025 to avoid a conflict.

**Polish**: T049, T050, T051 in parallel.

## Implementation Strategy

**MVP** = Phase 1 + Phase 2 + US1. That delivers the measured ~10x cost reduction and ~3x speedup with the premium summary intact.

**Ship boundary** = MVP + US2. The completeness gate is what makes the cheap tier safe; shipping US1 without it trades cost for silent quality loss, which is the outcome this feature exists to avoid.

**Increment 2** = US3 (automated summary grounding) + US4 (on-demand cover letters), independent of each other.

**Increment 3** = US5 (operator documentation and config guardrails) + Polish.
