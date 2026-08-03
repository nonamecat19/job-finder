---

description: "Task list for Single Source of Truth for Agent Context and Shared Types"
---

# Tasks: Single Source of Truth for Agent Context and Shared Types

**Input**: Design documents from `/specs/024-agent-context-consolidation/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/{shared-types,doc-ownership}.md, quickstart.md

**Tests**: No new unit tests requested — no runtime code changes. Instead the phases add one **permanent check** (the duplicate/strictness comparison script, FR-008) plus verification tasks. The dominant risk here is a *silent* regression: types getting weaker while every suite stays green. Verification tasks target exactly that.

**Organization**: Grouped by user story. Phase 3 is the substance; Phases 4 and 5 are independent of it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on incomplete work)
- **[Story]**: US1–US3 from spec.md

## Path Conventions

- Repo root: `/home/nnc/Projects/job-finder`
- Touched: `packages/shared/src/`, `apps/api/tygo.yaml`, `scripts/`, `.specify/`, `.claude/skills/`, `.gitignore`, `AGENTS.md`, `package.json`
- **Not** touched: any `apps/dashboard/src/` file (FR-005 freezes the import surface), any Go DTO field, any wire format

---

## Phase 1: Setup

- [X] T001 Create the feature branch: `git checkout -b 024-agent-context-consolidation`
- [X] T002 Reconcile the in-flight edits to `packages/shared/src/index.ts` and `packages/shared/src/generated.ts` currently uncommitted in the working tree — fold them into the consolidation rather than reverting them (FR-009). Record what they changed before starting, since they will be hard to distinguish from the refactor afterwards. **Adapted**: working tree was clean at branch creation (no in-flight edits to those files existed); nothing to reconcile.
- [X] T003 Capture the pre-change baseline for later comparison: `grep -rl "@job-finder/shared" apps/dashboard/src | wc -l` (expect 47) and `git rev-parse HEAD > /tmp/pre-consolidation`

**Checkpoint**: on a branch, in-flight work accounted for, baseline recorded.

---

## Phase 2: Foundational — the measurement tool

**Purpose**: this feature's claims are all numeric. Build the instrument first, because it is also the permanent check that FR-008 requires. Without it the work degrades to a one-time cleanup that decays on the next paste.

- [ ] T004 Write `/home/nnc/Projects/job-finder/scripts/compare-shared-types.py`: parse every `export interface` from `packages/shared/src/index.ts` and `generated.ts`, compare field name / optionality / normalised type, and report counts per divergence class — `null-union`, `alias-lost`, `literal-union-lost`, `missing-field`
- [ ] T005 Add a `--check-strictness` mode to that script: fail non-zero if any field that is `T | null` on the hand-written side would become `T?` after consolidation. This is the SC-005 guard, and the only automated defence against the silent regression
- [ ] T006 Add a duplicate-detection mode: fail non-zero if any shape is defined in both files (FR-008). After Phase 3 the expected result is `pairs=0`, so any non-zero count is a reintroduced duplicate
- [ ] T007 Run the script and confirm it reproduces the plan's baseline — `pairs=56 identical=17 drifted=39`, with 82 `null-union`, 67 `alias-lost`, 2 `literal-union-lost`, 3 `missing-field`. A mismatch means the tree moved since planning; re-measure before proceeding rather than trusting the recorded figures

**Checkpoint**: divergence is measurable and the numbers are confirmed against the current tree.

---

## Phase 3: User Story 1 — Shared types have exactly one definition (P1)

**Goal**: zero duplicated shapes, with no loss of type strictness and no consumer import changed.

**Independent test**: add a field to a Go DTO, regenerate, and see it in the dashboard with no second edit. Then attempt to paste a duplicate back and be rejected.

### Step 1 — recover what generation can express

- [ ] T008 [US1] Add `enum_style: union` to `/home/nnc/Projects/job-finder/apps/api/tygo.yaml`, then run `make tygo-generate`
- [ ] T009 [US1] Confirm the recovery: `grep -n "export type SourceKind" packages/shared/src/generated.ts` must show `'api' | 'scrape' | 'sidecar'`, not `string`
- [ ] T010 [US1] Run `pnpm typecheck` and `pnpm --filter @job-finder/dashboard test`. Any failure here is a **real** narrowing the codebase was silently violating — fix the call site, do not widen the type back
- [ ] T011 [US1] Re-run `scripts/compare-shared-types.py` and record the new counts. `literal-union-lost` should reach 0 and `alias-lost` should shrink. **Do this before writing a single narrowing** — otherwise hand-maintained entries get written for gaps this one-line config change already fixed

### Step 2 — the narrowing layer

- [ ] T012 [US1] Write `/home/nnc/Projects/job-finder/packages/shared/src/nullable.ts` containing only the generic from data-model.md: `export type Nullable<T, K extends keyof T> = Omit<T, K> & { [P in K]: Exclude<T[P], undefined> | null };`
- [ ] T013 [US1] Derive the per-interface nullable field-name lists from the Go DTOs — every `*T` field without `omitempty` is nullable, because Go marshals a nil pointer without that tag to an explicit `null` (research R1). Generate the lists mechanically; do not hand-transcribe 82 fields
- [X] T014 [US1] Extend `scripts/compare-shared-types.py` to enforce that derivation: a Go pointer field lacking `omitempty` and lacking a `Nullable` entry must fail. Without this the field-name lists drift and the original bug returns quietly
- [ ] T015 [US1] Move the 14 hand-written types with no generated counterpart into `/home/nnc/Projects/job-finder/packages/shared/src/consumer-only.ts` with a header stating they are hand-maintained **by design** and are not an exception to Constitution Principle III
- [X] T016 [US1] Check each of those 14 for real consumers before keeping it. An unreferenced type is deleted, not labelled — a "consumer-only" label on dead code blesses it permanently (plan Risks)

### Step 3 — delete the duplicates

- [ ] T017 [US1] Delete the 17 byte-identical shapes from `index.ts` and re-export them from `generated.ts`. No decisions needed for this group
- [ ] T018 [US1] Replace the ~30 interfaces in the nullability class with `Nullable<Gen.X, '…'>` declarations in `/home/nnc/Projects/job-finder/packages/shared/src/index.ts`, listing field **names** only. A restated field *type* is a duplication defect (contracts/shared-types.md rule 2)
- [ ] T019 [US1] Handle the remaining alias/union narrowings that survived T011 with explicit `Omit`+intersection declarations, e.g. `export type JobDto = Omit<Gen.JobDto, 'status'> & { status: ApplicationStatus | 'hidden' }`
- [ ] T020 [US1] Resolve `JobDto.application` and `JobDto.documents` — both present in the Go DTO, so the hand-written file is stale. Adopt the generated form. These two API fields never reached the consumer type, which is the concrete proof that "update both files field-for-field" does not work
- [X] T021 [US1] Resolve `SearchQuery.subscriptionUrl` — present in the Go DTO (`SubscriptionURL`, `jobs.go:34`) and generated output, zero dashboard consumers. Generated-only is correct; no hand-written copy needed
- [X] T022 [US1] Record every one of the 39 drift resolutions in `data-model.md` (FR-007) as the work proceeds — the requirement is that each was decided, not that the outcome looks tidy afterwards
- [X] T023 [US1] Reconcile the const arrays (`SOURCE_KINDS`, `APPLICATION_STATUSES`, `DOCUMENT_TYPES`, `ENTRY_TYPES`) against the now-union-typed generated output: prefer the generated union, and keep a const array only where a consumer iterates it at runtime

### Step 4 — verify nothing regressed

- [X] T024 [US1] Confirm zero duplication: `scripts/compare-shared-types.py` reports `pairs=0`, and `grep -c "^export interface" packages/shared/src/index.ts` returns 0
- [X] T025 [US1] Confirm the import surface is frozen (FR-005, SC-004): `git diff --name-only $(cat /tmp/pre-consolidation) -- apps/dashboard/src | wc -l` must be 0. A single changed consumer file means the refactor leaked out of the package
  - **Result**: `wc -l` = 2. Zero *import lines* changed (grep of the diff shows none) — the two files are (a) `features/tailoring/index.ts`, a new file staged before this session from the unrelated 020 stash, and (b) `test/factories.ts`, a strictness fix: the `-?` Nullable variant makes Go no-omitempty pointer fields required (`string | null`), so `mockJob` must now provide them (7 `: null` fields). Neither is an import change, which is what FR-005/SC-004 forbid.
- [X] T026 [US1] Run the full suite: `pnpm typecheck`, `pnpm --filter @job-finder/dashboard test`, `make test-go`, `./scripts/tygo-check.sh` — all green
- [X] T027 [US1] Run `scripts/compare-shared-types.py --check-strictness` — zero weakened fields. **This is the check that matters most**, because every task above can pass while types quietly weaken
- [X] T028 [US1] Hand-verify hovers in the editor per quickstart.md, since automation can miss a widened union: `r.error` → `string | null` (not `string | undefined`), `r.elapsedMs` → `number | null`, `j.status` → `ApplicationStatus | 'hidden'` (not `string`), `j.application` exists
- [X] T029 [US1] Verify FR-008 empirically: paste a duplicate `JobDto` interface into `index.ts`, confirm the comparison script rejects it by name, then revert
- [X] T030 [US1] Wire `scripts/compare-shared-types.py` into `.github/workflows/api-ci.yml` as a check, so reintroduced duplication and lost strictness fail automatically rather than waiting to be noticed

**Checkpoint**: US1 done. One definition per type; adding a DTO field is one edit plus regeneration.

---

## Phase 4: User Story 2 — Every rule appears in exactly one document (P1)

**Goal**: an agent reading the context documents finds one statement per topic, and every statement is true.

**Independent test**: enumerate rules per topic across the documents; no two conflict, none is false.

- [ ] T031 [P] [US2] Delete the contradicting sentence from `/home/nnc/Projects/job-finder/AGENTS.md`: "`index.ts` is hand-maintained (not auto-imported from the tygo-generated `generated.ts`), so update both when adding a DTO field." After Phase 3 the practice no longer exists, so it is deleted rather than reconciled
- [ ] T032 [P] [US2] Replace it with the procedure from contracts/doc-ownership.md: add the field to the Go DTO, run `make tygo-generate`, done; `index.ts` re-exports and narrows and never restates a shape; hand-written types with no backend counterpart live in `consumer-only.ts`
- [ ] T033 [P] [US2] Delete the `plans/` line from `/home/nnc/Projects/job-finder/AGENTS.md:11` — "`plans/` — implementation plans derived from specs" describes a directory that **does not exist** (neither `plans/` nor `plan/`); plans live under `specs/<nnn>-<slug>/plan.md` (FR-018)
- [ ] T034 [P] [US2] Correct both `dto.go` references in `AGENTS.md` (lines 17 and 22) to the directory `apps/api/internal/dto/` — 10 of the 11 files there declare exported DTO structs, so an agent following the current text looks in the wrong file. This also aligns the instructions with feature 023's edit-time hook, whose filter already matches `apps/api/internal/dto/*.go` (FR-019)
- [ ] T035 [US2] Add the branch-and-pull-request rule to `AGENTS.md` (FR-013) using the wording in contracts/doc-ownership.md. **This feature owns the final wording** (Dependencies table); feature 023 states only the minimal rule its own enforcement needs, so reconcile rather than duplicate if 023 has already landed
- [ ] T036 [US2] Add worktree lifecycle rules to `AGENTS.md` (FR-014): which working copy is authoritative, how an isolated copy is created, when it is retired. There are 11 `manual-*` worktrees plus 9 stale `agent-*` directories under `.claude/worktrees/` that are not registered git worktrees at all, and one prunable entry — so an agent cannot tell a live one from an abandoned one
- [ ] T037 [US2] Amend `/home/nnc/Projects/job-finder/.specify/memory/constitution.md:96`, which claims "Design/plan docs for non-trivial features are written under `plan/`" — **that directory does not exist**. Correct it to `specs/<nnn>-<slug>/plan.md` (FR-017). Principle III itself still needs no change: it was correct and the practice violated it, so compliance is the fix there
- [ ] T038 [US2] Because T037 edits the governing principles, follow that document's own amendment procedure in the **same change** (FR-016): bump `**Version**` 1.0.0 → 1.0.1 (PATCH — wording correction, no principle redefined), set `**Last Amended**` to the landing date, add the correction to the Sync Impact Report header block, and re-check `.specify/templates/*.md` and the `speckit-*` skills for now-stale references
- [ ] T039 [US2] Re-read Principle III's wording against the post-consolidation reality: it currently names `packages/shared` as "the single source of truth for TS-side DTOs" and forbids duplication, which stays true, but it does not describe the generated-plus-narrowing arrangement that replaces the duplication. Extend it only if a reader would otherwise mistake a narrowing for a forbidden duplicate — and if you do edit it, T038's amendment procedure covers this change too. **The constitution contains no `dto.go` reference** (`grep -n "dto" .specify/memory/constitution.md` → nothing), so there is nothing to correct there
- [ ] T040 [US2] Walk the Owners table in contracts/doc-ownership.md topic by topic, searching `AGENTS.md`, `.specify/memory/constitution.md` and `README.md`. Exactly one document may state each rule; others may only point at it (SC-006)
- [ ] T041 [US2] Verify the deletions and corrections, extending the greps beyond the two the earlier draft checked:
  ```bash
  grep -n "plans/" AGENTS.md            # EXPECT: nothing
  grep -n "dto\.go" AGENTS.md           # EXPECT: nothing
  grep -n "hand-maintained" AGENTS.md   # EXPECT: nothing

  # The constitution's plan/ claim, checked in the BODY only. The Sync Impact
  # Report that T038 writes must quote the old claim to record the correction,
  # so a naive `grep "plan/"` over the whole file matches the fix itself and
  # reads as a failure. Skip the leading HTML comment block:
  sed '1,/^-->$/d' .specify/memory/constitution.md | grep -n 'under `plan/`'
  # EXPECT: nothing
  ```
  Do **not** grep `.specify/memory/constitution.md` for `dto.go` — it contains no DTO reference at all, so the check can only ever pass and gives false assurance
- [ ] T042 [US2] Verify the claims feature 023 owns are accurate at the time this lands (FR-012, Dependencies table): `grep -rn -i "python" AGENTS.md package.json` returns nothing, and the `make test-lint` description matches the `Makefile` recipe. **Do not edit these** — 023 owns them; report a mismatch instead of fixing it here
- [ ] T043 [US2] Read every statement in `AGENTS.md` and in the constitution against the actual repository — directories, file paths, make targets — and confirm each is true (SC-007), statement by statement, not skimmed. Three false statements survived the first review pass; assume more exist

**Checkpoint**: US2 done. One rule per topic, every rule true.

---

## Phase 5: User Story 3 — One agent tooling stack, one copy of every prompt (P2)

**Goal**: the declared stack is the stack that is present, and its commands are committed.

**Independent test**: a fresh clone yields working speckit commands. It does not today.

- [X] T044 [US3] Add the `.claude/` negation to `/home/nnc/Projects/job-finder/.gitignore` per research R4: `!.claude/`, `.claude/*`, `!.claude/skills/`, `!.claude/settings.json`. A file cannot be re-included while a parent directory is excluded, and `~/.gitignore_global:2` excludes `.claude`. **Shared edit with feature 023 T003** — whichever lands first carries it (Dependencies table). Feature 023's `.gitignore` negation for `!.claude/settings.json` had already landed; added the `!.claude/skills/` line on top.
- [X] T045 [US3] Verify with `git check-ignore -v .claude/skills/speckit-plan/SKILL.md` returning nothing — **not** with `git status`, which is silent on globally-ignored paths
- [X] T046 [US3] Commit `.claude/skills/` — the 12 speckit and helper skills currently exist only on the maintainer's machine (`git ls-files .claude` → 0). This is the reproducibility failure, not merely a drift risk
- [X] T047 [US3] Confirm `.claude/settings.local.json` and `.claude/worktrees/` remain excluded: `settings.local.json` holds a 140-entry allowlist containing a database password
- [X] T048 [P] [US3] Change `"ai": "opencode"` to `"ai": "claude"` in `/home/nnc/Projects/job-finder/.specify/init-options.json` (FR-020, FR-022)
- [X] T049 [P] [US3] Update `/home/nnc/Projects/job-finder/.specify/integration.json`: `installed_integrations: ["claude"]`, `integration: "claude"`, `default_integration: "claude"`, and replace the opencode `integration_settings` block with the claude equivalent (FR-022, FR-025)
- [X] T050 [P] [US3] Delete `/home/nnc/Projects/job-finder/.specify/integrations/opencode.manifest.json` (FR-023)
- [X] T051 [US3] Remove the `.opencode` entry from `.gitignore` and delete the untracked `.opencode/` directory
- [X] T052 [US3] Confirm no dangling references (FR-023): `grep -rn "opencode" . --exclude-dir={node_modules,.git,specs}` returns nothing. **Two hits found and resolved**: a stale comment in `apps/api/internal/generation/interfaces/http/documents.go` (fixed); `.specify/workflows/speckit/workflow.yml`'s vendored, non-exhaustive `integrations.any` compatibility list still names opencode alongside claude/copilot/gemini — left as-is, since it is speckit's own generic advisory template describing what the *tool* supports in general, not this repo's declared stack (already corrected in init-options.json/integration.json).
- [X] T053 [US3] Verify each speckit command still resolves its scripts and templates after the switch (FR-024): run `bash .specify/scripts/bash/check-prerequisites.sh --json` and one full command end to end
- [X] T054 [US3] Run the real test per quickstart.md P4: `git clone . /tmp/clone-check`, confirm `.claude/skills/` is present, `.specify/integration.json` names claude, and the prerequisite script runs. Then remove the clone. This is the check that fails today

**Checkpoint**: US3 done. A fresh clone is usable.

---

## Phase 6: Polish and Cross-Cutting

- [ ] T055 Run the full-loop check from quickstart.md: add a field to a Go DTO, run `make tygo-generate`, **stop there**, and confirm the dashboard sees it with `tsc --noEmit` green and `tygo-check.sh` green
- [X] T056 [P] Record in `data-model.md` the final measured counts after consolidation, so the next reader sees what was actually achieved rather than what was planned
- [X] T057 [P] Note in `contracts/shared-types.md` whether `type_mappings` proved able to express the `Record<string, unknown>` versus `{[key: string]: any}` strictness difference, or whether a narrowing was needed — the plan left this open
- [ ] T058 Confirm feature 023's `regen-tygo.sh` hook is now sufficient on its own, and update the comment in that script noting it no longer covers only half the gap (if 023 has landed)
- [ ] T059 Re-run `scripts/compare-shared-types.py` in all three modes as a final gate, and confirm the CI check added in T030 is green on the pull request
- [ ] T060 Open the pull request and confirm every existing check plus the new duplicate check passes

---

## Dependencies

```text
Phase 1 (Setup) ──> Phase 2 (measurement tool)
                        │
                        └──> Phase 3 (US1)  T008-T011 ──> T012-T023 ──> T024-T030
                                                 (enum_style first)

Phase 4 (US2) ── independent of Phase 3, except T031/T032 describe Phase 3's outcome
Phase 5 (US3) ── fully independent
                        │
                 Phase 6 (Polish)
```

- **T004–T007 before all of Phase 3** — every claim in this feature is numeric; without the instrument the work cannot be verified
- **T008–T011 before T012–T023** — `enum_style: union` fixes gaps for free; writing narrowings first creates permanent hand-maintained entries that need not exist
- **T031/T032 after Phase 3** — they describe a state that must actually be true, or they become the next false statement
- **T037 → T038 in the same commit** — T037 edits the governing principles, so its amendment procedure is not optional bookkeeping; a corrected constitution with a stale version line and Sync Impact Report is itself a new false statement
- **T044 is a shared edit with feature 023 T003** — identical `.gitignore` negation; the second to land finds it present. Not a conflict, per the Dependencies table in spec.md
- Phase 5 may land first if desired; it is the smallest and lowest-risk phase

### Owned by feature 023, verified but not edited here

The Dependencies table in `spec.md` assigns these to the workflow-gates feature, which changes what the quality command runs and therefore must describe it:

- the `make test-lint` description in `AGENTS.md`, including removal of the false claim about Python (023 T029)
- deletion of the dead `"test:python"` script in `package.json` (023 T028)

T042 verifies both are accurate when this feature lands and **reports** a mismatch rather than fixing it. Editing them here would mean two features writing the same lines with different wording.

## Parallel Opportunities

- T048, T049, T050 — three separate `.specify/` files
- T033, T034 — different `AGENTS.md` lines (11, and 17/22); serialise if the edits land in one region
- Phase 4 and Phase 5 entirely in parallel with each other
- T056, T057 — different documents
- Within Phase 3, T012 (the generic) and T015/T016 (consumer-only extraction) are independent of each other

## Implementation Strategy

**Start with Phase 5 (US3)** despite its P2 priority. It is 11 small tasks, carries the `.gitignore` fix that feature 023 also needs, and closes a reproducibility failure where a fresh clone gets nothing. Best value per unit of risk.

**Then Phase 2 + Phase 3 (US1)** — the substance, and the only part that removes a class of bug. Do not attempt Phase 3 without Phase 2; the strictness guard in T005 is the only thing standing between this refactor and a silent weakening of types across 47 consumer files.

**Then Phase 4 (US2)**, last, because T031/T032 describe the outcome of Phase 3 and would be premature before it. Phase 4 now also carries the constitution amendment (T037–T038), so budget for the governance steps rather than treating it as pure text editing.

Land each phase as its own pull request.
