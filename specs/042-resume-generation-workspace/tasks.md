---

description: "Task list for 042 — Resume Generation Workspace"
---

# Tasks: Resume Generation Workspace

**Input**: Design documents from `/specs/042-resume-generation-workspace/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/rest-api.md`, `contracts/llm-contracts.md`, `quickstart.md`

**Tests**: Test tasks are included. Not optional here — Constitution IV requires each language's
suite to pass, and the 038 standing rule requires a corpus case to arrive with the change it
guards. Every task below that adds a rule adds its detector in the same phase.

**Organization**: Tasks are grouped by user story. Each story phase is an independently testable
increment, in the priority order `spec.md` sets.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task)
- **[Story]**: `[US1]`–`[US5]`, mapping to the five user stories in `spec.md`
- Exact file paths are given in every task

## Path Conventions

Web app, per `plan.md` § Project Structure: Go API at `apps/api/`, React dashboard at
`apps/dashboard/`, generated shared types at `packages/shared/`. 042 extends the existing
`apps/api/internal/generation/` feature module rather than creating a new one.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Schema, generated DB access, and generated wire types — everything downstream is
typed against these.

- [X] T001 Create migration `apps/api/internal/db/migrations/00042_generation_runs.sql` with the three tables from `data-model.md` §1–§3 (`generation_runs`, `generation_sections`, `generation_items`), including every CHECK, UNIQUE and index listed there — in particular `CHECK (origin <> 'profile' OR edited_text IS NULL)` (FR-009) and `UNIQUE (section_id, origin, source_index)` (FR-010). Goose version must be unique and sequential; do not reuse 00041.
- [X] T002 Write sqlc query set in `apps/api/internal/db/queries/generationrun.sql`: `CreateRun`, `GetRun`, `GetRunForUpdate`, `ListRunsByProfile`, `SetRunState`, `SetRunAnalysis`, `SetRunExport`, `DeleteRun`, `CreateSections`, `ListSectionsByRun`, `SetSectionState`, `DeleteSectionItems`, `CreateItems`, `ListItemsByRun`, `GetItemForUpdate`, `UpdateItemSelection`, `UpdateItemText`, `ReorderSectionItems`, `MarkItemsUnavailable`
- [X] T003 Run `make sqlc-generate` and confirm `apps/api/internal/db/sqlcgen/generationrun.sql.go` is produced (depends on T001, T002 — sqlc reads the schema from the migrations)
- [X] T004 [P] Create wire DTOs in `apps/api/internal/dto/generation_workspace.go` for every shape in `contracts/rest-api.md`: `GenerationRunDto`, `GenerationSectionDto`, `GenerationItemDto`, `GenerationExportDto`, `OverflowReportDto`, `OverflowCandidateDto`, `StartGenerationRequestDto`, `PatchGenerationItemRequestDto`, `RerunGenerationRequestDto`
- [X] T005 Run `make tygo-generate` then `pnpm --filter @job-finder/shared build`; confirm the new types appear in `packages/shared/src/generated.ts` and that `make tygo-check` passes (depends on T004)
- [X] T006 [P] Add `Nullable<>` narrowings to `packages/shared/src/index.ts` for any new pointer field lacking `omitempty` — and only for those. Adding a field must otherwise require zero edits here (024-FR-003, rule 3)

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: A run that persists, is enqueued, is served over HTTP, and seeds its items **in
master order**. Master order is not throwaway scaffolding — FR-010 requires it as the fallback
when a ranking is rejected, so building it first builds a required path.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T007 Add `GenerationRunID *string \`json:"generationRunId,omitempty"\`` to `GeneratePayload` in `apps/api/internal/queue/queue.go` — wire-nullable, so existing callers are unaffected
- [X] T008 [P] Create run/section/item domain value types and the `EffectiveText()` helper (`edited_text ?? source_text`, computed never stored, per `data-model.md` §3) in `apps/api/internal/generation/domain/workspace.go`
- [X] T009 [P] Implement `SeedFromMaster(master RendercvMaster, cfg ShapeConfig) []Section` in `apps/api/internal/generation/domain/workspace_seed.go`: one `summary` section, one `experience` section per master entry keyed by exact company name in master order, one `skills` section; items in master order with `rank = position = source index`, top `min(N, A)` selected, `origin='profile'`
- [X] T010 [P] Write table tests for `SeedFromMaster` in `apps/api/internal/generation/domain/workspace_seed_test.go`: every master bullet appears exactly once; entry order equals master order (028-FR-003); an entry with zero bullets seeds an empty section rather than being skipped
- [X] T011 Implement `StartRun` in `apps/api/internal/generation/application/workspace.go`: resolve `ShapeConfig`, summary option and grounding level **once at the top** (`research.md` R3), snapshot the master and its content hash, run `analyzeVacancy`, persist run + sections + seeded items, run the existing summary stage into the `summary` section, set run state (depends on T003, T008, T009)
- [X] T012 Dispatch on `payload.GenerationRunID` in `apps/api/internal/generation/interfaces/worker/handler.go`, routing to `application.Service.StartRun` and leaving the existing merged-resume path unchanged (depends on T007, T011)
- [X] T013 Implement `POST /v1/generations`, `GET /v1/generations/{runId}`, `GET /v1/generations`, `DELETE /v1/generations/{runId}` in `apps/api/internal/generation/interfaces/http/generations.go`, using `httpx.WriteJSON` / `WriteAppError` / `DecodeJSON` (027-FR-008) and the status codes in `contracts/rest-api.md`
- [X] T014 Register the handler through the variadic `httpapi.NewRouter(...)` call in `apps/api/cmd/server/compose.go` — one registration producing both versioned and unversioned mounts (027-FR-005/FR-007). Do not edit `apps/api/internal/httpapi/router.go`
- [X] T015 Write a `dbtest`-backed integration test in `apps/api/internal/generation/interfaces/http/generations_test.go` covering start → poll → fetch, plus the `400` for a profile with no master content and the `404` for an unknown `jobId`
- [X] T016 [P] Add a `generations` group to `apps/dashboard/src/lib/api.ts` mirroring the existing `settings` group's shape (`start`, `get`, `list`, `patchItem`, `reorder`, `rerun`, `export`, `exportStatus`, `remove`)
- [X] T017 [P] Add query keys `generations.all` / `generations.get(id)` and TanStack Query hooks in `apps/dashboard/src/features/generate/hooks.ts`, polling while `state === 'running'` at the existing activity-poll interval
- [X] T018 Add the `/generate` route in `apps/dashboard/src/app/routes.tsx` wrapped in `RequireProfileConfig`, and `routeLayoutModes['/generate'] = 'fit'` so the two panes scroll independently
- [X] T019 [P] Add the `Generate` nav entry to the nav array at `apps/dashboard/src/app/shell.tsx:12`, beside the existing `/tailor` entry (both coexist through the transition, per `spec.md` assumption 1)
- [X] T020 Create the two-pane shell `apps/dashboard/src/features/generate/GenerateWorkspacePage.tsx`: generated items left, vacancy and controls right, with loading / error / running states built from the existing `LoadingRegion`, `ErrorState` and `Spinner` primitives (depends on T017, T018)
- [X] T021 [P] Create `apps/dashboard/src/features/generate/components/VacancyPane.tsx`: company, title, vacancy textarea, grounding-level select **labelled as governing the summary only** (`research.md` R1), 034 summary-writer select, and the Generate button
- [X] T022 Write a vitest for the shell in `apps/dashboard/src/features/generate/GenerateWorkspacePage.test.tsx` asserting the two-pane layout renders and a `running` run shows progress rather than an empty workspace

**Checkpoint**: A run persists, is served, and returns every master bullet in master order. User story work can begin.

---

## Phase 3: User Story 1 — Review the generated resume as an inspectable list (Priority: P1) 🎯 MVP

**Goal**: The left pane renders the run as Summary / Work Experience / Skills, every item
individually addressable, origin visible, toggles surviving a navigation away and back.

**Independent Test**: Navigate to `/generate` with a vacancy, run generation, assert the left
pane renders a summary block, one block per master work entry with selected achievements, and a
skills block — each item individually identifiable and toggleable; toggle an item, navigate away
and back, assert the selection persisted.

### Tests for User Story 1

- [X] T023 [P] [US1] Contract test in `apps/api/internal/generation/interfaces/http/generations_items_test.go`: `PATCH …/items/{itemId}` returns `403` when `text` is sent for an `origin="profile"` item (FR-009 at the API boundary), `409` when the run is `running`, and is idempotent on a repeated identical body
- [X] T024 [P] [US1] Contract test in `apps/api/internal/generation/interfaces/http/generations_get_test.go`: `GET …/{runId}` returns items in `position` order, and every item with `origin: "profile"` has `text` byte-identical to the master bullet at its `sourceIndex` (SC-001's measurement)

### Implementation for User Story 1

- [X] T025 [US1] Implement `PATCH /v1/generations/{runId}/items/{itemId}` in `apps/api/internal/generation/interfaces/http/generations.go`, taking a row-level `SELECT … FOR UPDATE` on the run first and rejecting `text` on a profile-origin item with `403`
- [X] T026 [US1] Implement `PATCH /v1/generations/{runId}/sections/{sectionId}/order` in the same file, applying a whole-section reorder in one transaction
- [X] T027 [P] [US1] Create `apps/dashboard/src/features/generate/components/OriginBadge.tsx` rendering three distinct states: "from your profile", "AI · unverified", and the summary's own grounded-prose state — the three are semantically different and must not share a badge
- [X] T028 [P] [US1] Create `apps/dashboard/src/features/generate/components/ItemRow.tsx`: checkbox, effective text, origin badge, dnd-kit drag handle, and an `unavailable` presentation
- [X] T029 [US1] Create `apps/dashboard/src/features/generate/components/WorkEntryBlock.tsx`: entry label, ranked profile items in `position` order, and an "no bullets in your profile for this role" empty state — never a fabricated bullet standing in for one (depends on T028)
- [X] T030 [P] [US1] Create `apps/dashboard/src/features/generate/components/SummaryBlock.tsx` rendering the single summary item as accept / edit / drop
- [X] T031 [P] [US1] Create `apps/dashboard/src/features/generate/components/SkillsBlock.tsx` rendering skill-group items with the same toggle affordance as achievements
- [X] T032 [US1] Assemble the three blocks into the left pane of `apps/dashboard/src/features/generate/GenerateWorkspacePage.tsx`, grouping items by `origin` client-side (the server returns them interleaved by `position`, per `contracts/rest-api.md`) (depends on T029, T030, T031)
- [X] T033 [US1] Wire optimistic toggle and reorder mutations in `apps/dashboard/src/features/generate/hooks.ts` — local state updates immediately and the PATCH persists asynchronously, which is what makes SC-006 (<1 s, zero model calls) true by construction rather than by measurement
- [X] T034 [US1] Add a "Tailor for this job" action to `apps/dashboard/src/features/job-detail/JobDetailPage.tsx` that starts a run with the `jobId` and navigates to `/generate?runId=…` (FR-001)
- [X] T035 [P] [US1] Vitest in `apps/dashboard/src/features/generate/components/WorkEntryBlock.test.tsx`: an entry with zero master bullets renders the explicit empty state, and every rendered profile item carries the profile badge
- [X] T036 [P] [US1] Vitest in `apps/dashboard/src/features/generate/GenerateWorkspacePage.test.tsx`: toggling an item changes the previewed selection without issuing a generation request (SC-006)

**Checkpoint**: The workspace is a reviewable, structured, persistent surface — with items in master order. Independently demoable.

---

## Phase 4: User Story 2 — AI ranks real achievements and never invents them (Priority: P1)

**Goal**: The ranking stage returns indices only; `min(2N, A)` candidates are ranked, the top
`min(N, A)` selected, an invalid ranking retries once then falls back to master order.

**Independent Test**: Run generation against a master profile with known bullets; assert every
item in the profile-sourced group is byte-identical to a master bullet, no master bullet appears
twice, and the ordering differs from master order when the vacancy justifies it.

### Tests for User Story 2

- [X] T037 [P] [US2] Table tests for `VerifyRanking` in `apps/api/internal/generation/domain/ranking_verify_test.go`: `out_of_range`, `duplicate` and `short` each detected; `len(ranking) > K` accepted as valid; K computed as `min(2*target, available)`
- [X] T038 [P] [US2] Test in `apps/api/internal/generation/application/workspace_rank_test.go` that a twice-rejected ranking falls back to master order and sets `fallback_used = true` rather than failing the run (FR-010)

### Implementation for User Story 2

- [X] T039 [P] [US2] Define `RankedExperience`, `RankedProject`, `RankedSkills` and `RankedSelection` in `apps/api/internal/generation/domain/ranking.go` exactly as `contracts/llm-contracts.md` §1 specifies — `[]int` only, with **no** `rephrased`, `summary`, `suggestions`, `drop` or skill-text field
- [X] T040 [P] [US2] Implement `VerifyRanking(available, target int, ranking []int) []RankingViolation` in `apps/api/internal/generation/domain/ranking_verify.go`
- [X] T041 [US2] Implement `buildRankPrompt` in `apps/api/internal/generation/application/rankcv_llm.go`, reusing `buildSelectPrompt`'s numbered-bullet rendering verbatim so there is exactly one index space, and printing `K = min(2N, A)` after each entry's bullet list
- [X] T042 [US2] Implement `rankContent` in the same file: `llm.CompleteStructured[domain.RankedSelection]` against the `generation-select` router with `selectStageTimeout` and `selectMaxTokens`
- [X] T043 [US2] Replace master-order seeding with ranked seeding in `apps/api/internal/generation/application/workspace.go`: rank → verify → retry once → fall back to master order; top `min(N, A)` selected; ranked remainder unselected; the unranked tail appended in master order, unselected and visible (`research.md` R2) (depends on T039–T042)
- [X] T044 [US2] Feed the summary stage's `SummaryBrief.Highlights` from the run's **selected profile items** instead of `SelectedHighlights(TailoredSelection)` in `apps/api/internal/generation/application/workspace.go`, keeping the summary otherwise unchanged (`contracts/llm-contracts.md` §3)
- [X] T045 [US2] Add a `ranking_violations` scorer delegating to `domain.VerifyRanking` in `apps/api/internal/generation/application/eval_scorer_test.go`, and extend `TestScorerDelegationIsExact` and `TestScorersDetectInjectedDefects` to cover it
- [X] T046 [US2] Add corpus case `apps/api/internal/generation/application/evaldata/cases/ranked-oversized-entry/` (`case.yaml` with a `why` and `page_counts`, `master.yaml`, `vacancy.txt`) — one entry with far more than 2N bullets, synthetic fixtures, closed date ranges only
- [X] T047 [US2] Bump `ScorerSetVersion` and re-record every baseline with a stated reason, per case, never for the whole corpus in one command: `go test ./internal/generation/application/ -run TestEvalCorpus -eval.update-baseline -eval.case <name> -eval.reason "…"` (depends on T045, T046)
- [X] T048 [US2] Add the ranked/unranked visual split to `apps/dashboard/src/features/generate/components/WorkEntryBlock.tsx`: selected top-N first, ranked-but-unselected below, then the master-order tail — all in one list the user can promote from

**Checkpoint**: Ranking is grounded by construction. `rg 'rephrased|Rephrased' apps/api/internal/generation/` returns nothing on the workspace path.

---

## Phase 5: User Story 3 — AI suggestions offered, marked, off by default (Priority: P2)

**Goal**: A separate suggestion channel per work entry and for skills — unselected, badged
unverified, editable once included, absent from an untouched export.

**Independent Test**: Run generation on a vacancy demanding skills and experience the profile
lacks; assert suggestions appear in their own marked group, an export with no user action
contains none of them, and including one adds it to the export.

### Tests for User Story 3

- [X] T049 [P] [US3] Test in `apps/api/internal/generation/domain/suggestions_test.go` that `SuppressDuplicateSuggestions` removes a bullet matching a master bullet for that entry on normalised form or ≥0.9 word-set containment, and leaves the profile item untouched (FR-017)
- [X] T050 [P] [US3] Test in `apps/api/internal/generation/application/workspace_suggest_test.go` that every `origin='ai'` item is created with `selected = false` (FR-013 / SC-004)

### Implementation for User Story 3

- [X] T051 [P] [US3] Define `ExperienceSuggestions` and `SuggestionSet` in `apps/api/internal/generation/domain/ranking.go` per `contracts/llm-contracts.md` §2 — text only, **no index field**
- [X] T052 [P] [US3] Implement `SuppressDuplicateSuggestions` in `apps/api/internal/generation/domain/suggestions.go`, reusing the existing `norm()` and `tokens()` helpers from `rendercv.go` so the comparison basis is the pipeline's existing one
- [X] T053 [US3] Implement `buildSuggestPrompt` and `suggestContent` in `apps/api/internal/generation/application/rankcv_llm.go`, routed through the existing `generation-select` task key (no `gateway/config.yaml` change — `research.md` R4), taking the analysis plus company names and skill-group labels but **not** the master's bullet text
- [X] T054 [US3] Run the suggestion stage concurrently with the summary stage in `apps/api/internal/generation/application/workspace.go`, dropping entries whose company does not match a master company, applying T052, and persisting survivors as `origin='ai'`, `selected=false`, ranked after every profile item (depends on T051–T053)
- [X] T055 [US3] Add the suggestion group to `apps/dashboard/src/features/generate/components/WorkEntryBlock.tsx`: visually distinct, each item badged "AI · unverified", with an empty state when a run produced none (never a missing or broken section)
- [X] T056 [US3] Allow inline editing of an included AI item in `apps/dashboard/src/features/generate/components/ItemRow.tsx` (FR-015), keeping the unverified badge after inclusion and after editing (FR-014)
- [X] T057 [P] [US3] Vitest in `apps/dashboard/src/features/generate/GenerateWorkspacePage.test.tsx` asserting that a run with suggestions produces zero selected AI items until the user acts, and that including one keeps its badge

**Checkpoint**: The escape hatch exists without weakening grounding. Stories 1–3 all work.

---

## Phase 6: User Story 4 — Skills are ranked, not rewritten (Priority: P2)

**Goal**: Skill groups ordered by relevance, nothing invented inside the user's own groups, and
every omission visible as an unselected item rather than hidden.

**Independent Test**: Run generation with a known skill set; assert the rendered skills are
exactly a permutation/subset of the master's, with every omission visible and unselected.

### Tests for User Story 4

- [X] T058 [P] [US4] Test in `apps/api/internal/generation/domain/workspace_skills_test.go` that the seeded skills section is a permutation of the master's groups — no group added, reworded or absent from the list — and that a `skillsMaxGroups` cap leaves the excess groups present-but-unselected rather than removed

### Implementation for User Story 4

- [X] T059 [P] [US4] Extend `RankedSkills.GroupOrder` handling in `apps/api/internal/generation/domain/ranking_verify.go` with the same range/duplicate/short checks the achievement ranking uses
- [X] T060 [US4] Order skill-group items from `RankedSkills.GroupOrder` in `apps/api/internal/generation/application/workspace.go`, keeping within-group entry order from the existing deterministic `domain.RankSkills` and leaving pinned groups (`Spoken Languages`) exactly as authored
- [X] T061 [US4] Apply `skillsMaxGroups` as a **selection** boundary rather than a removal in the same file: the lowest-ranked groups become unselected items, still rendered (FR-011, US4 AS-2)
- [X] T062 [US4] Add AI-suggested skills to the skills section as `origin='ai'`, `selected=false` in `apps/api/internal/generation/application/workspace.go`, separate from the user's real groups (US3 AS-4, delivered here because it is the skills surface)
- [X] T063 [US4] Render the profile / suggested split in `apps/dashboard/src/features/generate/components/SkillsBlock.tsx`, matching `WorkEntryBlock`'s grouping so the two sections read identically
- [X] T064 [P] [US4] Add corpus case `apps/api/internal/generation/application/evaldata/cases/suggestion-duplicates-profile/` asserting the R6 suppression fires, and re-record its baseline with a stated reason

**Checkpoint**: Both grounded surfaces obey the same contract.

---

## Phase 7: User Story 5 — Export what I approved (Priority: P2)

**Goal**: The exported document is exactly the selected items, in the displayed order, with the
displayed wording — no expand call, no `TrimHighlights`, no silent trim.

**Independent Test**: Make a known set of toggles, export, and assert the exported document's
content is exactly the selected items in the displayed order.

### Tests for User Story 5

- [X] T065 [P] [US5] Test in `apps/api/internal/generation/domain/assemble_test.go` that `Assemble` emits exactly the selected items in `position` order, includes no unselected or `unavailable` item, and preserves master experience order and every out-of-scope field (company, dates, education) verbatim
- [X] T066 [P] [US5] Test in `apps/api/internal/generation/application/workspace_export_test.go` (`TestWorkspaceExport`) that the export path issues **zero** LLM calls and never calls `TrimHighlights`, using the injected `renderDeps` seam

### Implementation for User Story 5

- [X] T067 [P] [US5] Implement `Assemble(master RendercvMaster, sections []Section) (RendercvMaster, error)` in `apps/api/internal/generation/domain/assemble.go`, deep-cloning the run's `master_snapshot` and writing only section contents — never a section key, never `_order`, matching `MergeTailored`'s discipline
- [X] T068 [P] [US5] Implement `OverflowCandidates(sections []Section, over int) []OverflowCandidate` in `apps/api/internal/generation/domain/overflow.go`, returning the lowest-`rank` **selected** items worst-first
- [X] T069 [US5] Implement the render-once export in `apps/api/internal/generation/application/workspace_export.go`: `Assemble` → `ApplyFontSize` → `RenderCvRenderer.Render` → `CountPages`; over target → `CompactDesign` and re-render once; still over → `blocked` with the overflow report. `expandContent`, `TrimHighlights`, `padHighlights` and the `ApplyHardLimits` truncation must not appear on this path (`research.md` R5) (depends on T067, T068)
- [X] T070 [US5] Insert the resulting `GeneratedDocument` row and set `generation_runs.export_document_id` / `export_status` in the same file, so `GET /api/documents` and the existing PDF download work unchanged
- [X] T071 [US5] Implement `POST /v1/generations/{runId}/export` and `GET /v1/generations/{runId}/export` in `apps/api/internal/generation/interfaces/http/generations.go` with the `202` / `200 blocked` / `409` semantics from `contracts/rest-api.md`
- [X] T072 [US5] Add the export control and the overflow report to `apps/dashboard/src/features/generate/components/VacancyPane.tsx`: pages rendered vs target, and the named drop candidates as a list the user acts on — the UI must not offer to apply them automatically (FR-019)
- [X] T073 [US5] Warn before export when a section has zero selected items in `apps/dashboard/src/features/generate/GenerateWorkspacePage.tsx` ("this resume has no summary / no skills"), and warn at include-time that AI-written content is unverified
- [X] T074 [P] [US5] Integration test in `apps/api/internal/generation/interfaces/http/generations_export_test.go` asserting a `blocked` export mutates no item and returns candidates ordered worst-rank-first
- [X] T075 [P] [US5] Vitest in `apps/dashboard/src/features/generate/GenerateWorkspacePage.test.tsx` asserting an export taken with no user action on any suggestion ships zero AI-origin items (SC-004)

**Checkpoint**: All five user stories are independently functional.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: The run-lifecycle rules that span every story, plus the documentation the
constitution requires to be true when the change lands.

- [ ] T076 Implement `POST /v1/generations/{runId}/rerun` in `apps/api/internal/generation/interfaces/http/generations.go` and its service half in `apps/api/internal/generation/application/workspace.go`: whole-run or named sections, in place on the same run id, `409` while running (FR-021)
- [ ] T077 Preserve selections across a rerun in `apps/api/internal/generation/application/workspace.go` — profile items matched by `source_index`, AI items by normalised `source_text`, matched items keeping `selected`, `position` and `edited_text` (`data-model.md` §4) (depends on T076)
- [ ] T078 [P] Add the rerun control and its FR-021 warning ("re-running replaces the AI's ordering for this section") to `apps/dashboard/src/features/generate/components/VacancyPane.tsx`, with a per-section retry on any `failed` section
- [ ] T079 Implement staleness detection in `apps/api/internal/generation/application/workspace.go`: compare `master_content_hash` against the profile's current hash, expose `masterChanged`, and set `unavailable = true` on items whose `source_index` no longer resolves (FR-022)
- [ ] T080 [P] Surface `masterChanged` as a re-run prompt and render `unavailable` items as unavailable rather than dropping them, in `apps/dashboard/src/features/generate/GenerateWorkspacePage.tsx` and `components/ItemRow.tsx` (depends on T079)
- [ ] T081 [P] Add per-section `failed` handling to `apps/api/internal/generation/application/workspace.go` so a partly-failed run reaches `partial` with completed sections intact, never discarding the whole run
- [ ] T082 [P] Test in `apps/api/internal/generation/application/workspace_rerun_test.go` that a rerun preserves a matched item's `selected`/`position`/`edited_text` and discards only unmatched items
- [ ] T083 **Requires explicit user approval before running** — create `apps/api/internal/db/migrations/00043_drop_tailored_drafts.sql` dropping `edit_proposals` and `tailored_drafts`, delete `apps/api/internal/tailoring/`, `apps/api/internal/dto/tailoring.go`, `apps/dashboard/src/features/tailoring/`, and the now-unread `TailoringDraftID` field in `apps/api/internal/queue/queue.go`. Evidence that this is safe is in `research.md` R8: no code writes these tables. Skip this task if the user declines; nothing else in this list depends on it
- [ ] T084 [P] Update `docs/docs/ai/generation.md` to describe the workspace pipeline: ranking-by-index, the separate suggestion channel, and the render-once export
- [ ] T085 Correct `specs/domains/resume-generation.md` §4.1 and §7.1, which document a `/api/tailoring` REST surface and a `singlepage` page fitter that do not exist in the tree (`research.md` R8) — 024-FR-015 requires every stated rule to be true of the repository
- [ ] T086 [P] Verify `make lint-go` still passes the `depguard` rules and that `apps/api/internal/arch_test.go` accepts the new handler's placement under `interfaces/http`
- [ ] T087 Run `make test-lint` — required, since this change spans `apps/api`, `apps/dashboard` and `packages/shared` (Constitution IV)
- [ ] T088 Walk `quickstart.md` § "Manual walkthrough" end to end against `make dev`, and confirm the three verification commands in § "Verifying the three claims that matter" pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: no dependencies — start immediately. T001 → T002 → T003; T004 → T005.
- **Foundational (Phase 2)**: depends on Setup. **Blocks every user story.**
- **US1 (Phase 3)**: depends on Foundational only.
- **US2 (Phase 4)**: depends on Foundational only. Renders through US1's components when both are done, but is testable at the API level without them.
- **US3 (Phase 5)**: depends on Foundational; needs US2's `ranking.go` file to exist (T039) before T051 adds to it — a file dependency, not a behavioural one.
- **US4 (Phase 6)**: depends on Foundational; T059 extends US2's verifier and T062 uses US3's suggestion stage.
- **US5 (Phase 7)**: depends on Foundational. Assembles whatever is selected, so it works with master-order items alone — its value grows with the other stories but its correctness does not depend on them.
- **Polish (Phase 8)**: depends on the desired user stories being complete.

### User Story Dependencies

- **US1 (P1)**: independent. **This is the MVP.**
- **US2 (P1)**: independent of US1 at the API level; T048 touches a US1 component.
- **US3 (P2)**: soft dependency on US2 (shares `ranking.go`) and on US1's `WorkEntryBlock`.
- **US4 (P2)**: soft dependency on US2 (verifier) and US3 (suggested skills).
- **US5 (P2)**: independent of US2–US4.

### Within Each Story

- Tests before the implementation they guard
- Domain types before verifiers before prompts before service wiring
- Service before endpoint before UI
- A corpus case lands with the rule it guards, never after

### Parallel Opportunities

- Setup: T004 and T006 are `[P]`; T001–T003 are a chain
- Foundational: T008/T009/T010 in parallel; T016/T017/T019/T021 in parallel across the dashboard
- US1: T023/T024 in parallel; T027/T028/T030/T031 in parallel; T035/T036 in parallel
- US2: T037/T038 in parallel; T039/T040 in parallel
- US3: T049/T050 in parallel; T051/T052 in parallel
- US5: T065/T066 in parallel; T067/T068 in parallel; T074/T075 in parallel
- Across stories: once Phase 2 closes, **US1 and US5 can be built by different people with no shared file**; US2 → US3 → US4 share `ranking.go` and `WorkEntryBlock.tsx` and are better done in sequence

---

## Parallel Example: User Story 1

```bash
# Contract tests together:
Task: "PATCH items contract test in apps/api/internal/generation/interfaces/http/generations_items_test.go"
Task: "GET run contract test in apps/api/internal/generation/interfaces/http/generations_get_test.go"

# Leaf components together:
Task: "OriginBadge in apps/dashboard/src/features/generate/components/OriginBadge.tsx"
Task: "ItemRow in apps/dashboard/src/features/generate/components/ItemRow.tsx"
Task: "SummaryBlock in apps/dashboard/src/features/generate/components/SummaryBlock.tsx"
Task: "SkillsBlock in apps/dashboard/src/features/generate/components/SkillsBlock.tsx"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1 Setup — schema, sqlc, DTOs, tygo
2. Phase 2 Foundational — run lifecycle, HTTP surface, workspace shell **(blocks everything)**
3. Phase 3 US1 — the inspectable, toggleable, persistent workspace
4. **STOP and VALIDATE**: run the Independent Test for US1
5. Demoable: a structured, reviewable resume with items in master order and every origin visible

The MVP is honest at this point — nothing is fabricated, because nothing has been generated
beyond the summary. It is simply not yet *ranked*.

### Incremental Delivery

1. Setup + Foundational → foundation ready
2. + US1 → the workspace (MVP)
3. + US2 → the ranking becomes real, and SC-001/SC-003 become measurable
4. + US3 → suggestions, the escape hatch
5. + US4 → skills brought under the same contract
6. + US5 → export, closing the loop
7. Phase 8 → rerun, staleness, docs corrections, the conditional schema drop

US5 can be pulled forward to sit immediately after US1 if an end-to-end demo matters more than
ranking quality — it depends on nothing in US2–US4.

### Parallel Team Strategy

1. Everyone on Setup + Foundational
2. Then: Developer A on US1 (dashboard-heavy), Developer B on US5 (Go-heavy, no shared file),
   Developer C on the US2 → US3 → US4 chain (shares `ranking.go`, best kept with one owner)
3. Phase 8 after the chosen stories land

---

## Notes

- `[P]` = different files, no dependency on an incomplete task
- Commit after each task or logical group; work directly on `master` per the project's `CLAUDE.md`
- **T083 is gated on explicit user approval** — it drops two database tables. Everything else is unconditional
- A DTO edit that skips `make tygo-generate` fails CI on `make tygo-check`, not locally
- Bumping `ScorerSetVersion` (T047) re-records every baseline; that refusal to compare across scorer sets is designed behaviour, not an obstacle
- A test requiring edits after a mechanical move is a signal the move changed behaviour — investigate rather than adjust the test
