---

description: "Task list for Enforced Workflow Quality Gates"
---

# Tasks: Enforced Workflow Quality Gates

**Input**: Design documents from `/specs/023-workflow-quality-gates/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/{make-targets,required-checks,hooks}.md, quickstart.md

**Tests**: No unit tests requested — this feature adds no application code. Instead each phase ends with **verification tasks** that observe the gate *refusing* something. A gate never seen refusing has not been tested; these are not optional.

**Organization**: Grouped by user story. Each phase is independently landable and independently revertible.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no dependency on incomplete work)
- **[Story]**: US1–US4 from spec.md

## Path Conventions

- Repo root: `/home/nnc/Projects/job-finder`
- No application source is touched. Only `Makefile`, `.githooks/`, `scripts/`, `.claude/`, `.github/workflows/`, `apps/api/.golangci*`, `eslint.config.js`, `AGENTS.md`, `.gitignore`

---

## Phase 1: Setup

**Purpose**: work on a branch, and unblock committing agent configuration

- [X] T001 Create the feature branch: `git checkout -b 023-workflow-quality-gates` (do not work on `master` — the whole point of US1) — N/A as run: this implementation executes inside a dedicated git worktree already checked out on its own branch (`worktree-agent-a8adfa63bba994057`, off `master` @ 8c1f320), which satisfies the same isolation intent
- [X] T002 Stash or commit the 18-file in-flight change currently on `master` (feature 022 work) onto its own branch, so this feature's diff is reviewable in isolation — N/A as run: this worktree's tree started clean (no feature-022 changes present), so there was nothing to move
- [X] T003 Add the `.claude/` negation to `/home/nnc/Projects/job-finder/.gitignore` exactly as specified in plan.md Constraints: `!.claude/`, `.claude/*`, `!.claude/settings.json` — a file cannot be re-included while a parent directory is excluded, and `~/.gitignore_global:2` excludes `.claude`
- [X] T004 Verify the negation works with `git check-ignore -v .claude/settings.json` returning nothing — **not** by looking at `git status`, which is silent on globally-ignored paths

**Checkpoint**: on a branch; `.claude/settings.json` is committable.

---

## Phase 2: Foundational

**Purpose**: the directory and script conventions every later phase writes into

- [X] T005 Create `/home/nnc/Projects/job-finder/scripts/hooks/` and `/home/nnc/Projects/job-finder/.githooks/`
- [X] T006 Create `/home/nnc/Projects/job-finder/.claude/settings.json` containing only `{"hooks": {}}` and commit it, confirming with `git ls-files .claude/settings.json` that it is tracked
- [X] T007 Write `/home/nnc/Projects/job-finder/scripts/hooks/common.sh` — the shared helpers every hook script needs, mirroring the existing `.specify/scripts/bash/common.sh` convention: read the hook JSON from stdin once, expose `hook_field <jq-path>` for pulling `tool_input.file_path` / `tool_input.command` / `session_id`, `require_tool <name> <install-line>` for the FR-027 tool-presence check, and `emit_context <message>` for `hookSpecificOutput.additionalContext`. The conventions in contracts/hooks.md (`set -euo pipefail`, idempotent, never writes outside the repo) become executable here instead of being copied into five scripts by hand

**Checkpoint**: skeleton in place, nothing enforced yet.

---

## Phase 3: User Story 1 — Changes are proven before they become the baseline (P1)

**Goal**: `master` refuses commits and pushes; the agent is stopped before it reaches git.

**Independent test**: attempt a commit on `master` — rejected. Same commit on a branch — succeeds. Delivers value even with no new checks, because existing CI stops being advisory.

- [X] T008 [US1] Write `/home/nnc/Projects/job-finder/.githooks/pre-commit` per contracts/hooks.md: exit 1 when `git rev-parse --abbrev-ref HEAD` is `master`, printing the branch-and-PR rule plus the `git checkout -b <nnn>-<slug>` command; exit 0 on any other branch; `chmod +x`
- [X] T009 [P] [US1] Write `/home/nnc/Projects/job-finder/.githooks/pre-push` per contracts/hooks.md: read the stdin lines `<local ref> <local sha> <remote ref> <remote sha>`, exit 1 if any `<remote ref>` is `refs/heads/master`; `chmod +x`
- [X] T010 [US1] Add the `setup-hooks` target to `/home/nnc/Projects/job-finder/Makefile` running `git config core.hooksPath .githooks`, idempotent, per contracts/make-targets.md
- [X] T011 [P] [US1] Write `/home/nnc/Projects/job-finder/scripts/hooks/guard-master.sh`: read the hook JSON on stdin, extract `tool_input.command`, exit 2 with a stderr message when on `master` and the command is a `git commit`/`git push`, else exit 0. **Exit 2 blocks for `PreToolUse`** — unlike `PostToolUse` (research R2)
- [X] T012 [US1] Register the `PreToolUse` binding in `/home/nnc/Projects/job-finder/.claude/settings.json`: matcher `Bash`, `if` entries `Bash(git commit*)` and `Bash(git push*)`, command `$CLAUDE_PROJECT_DIR/scripts/hooks/guard-master.sh`
- [X] T013 [US1] Add the branch-and-PR rule to `/home/nnc/Projects/job-finder/AGENTS.md` (FR-006) and document the `--no-verify` emergency override as the FR-005 escape hatch, stating that using it leaves a visible trace. Keep it to the **minimal rule this feature's enforcement needs** — feature 024 owns the final wording and the per-topic ownership table, so do not also write a rules-ownership section here
- [X] T014 [US1] Verify per quickstart.md P1: commit on `master` rejected; same commit on a branch succeeds; `git push origin master --dry-run` aborted before contacting the remote; `git commit --no-verify` succeeds
- [X] T015 [US1] Verify the agent guard standalone: `echo '{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}' | ./scripts/hooks/guard-master.sh` → exit 2 on `master`, exit 0 and silent on a branch

**Checkpoint**: US1 done. `master` is write-protected locally and the agent cannot route around it invisibly.

---

## Phase 4: User Story 2 — The documented quality command actually checks quality (P1)

**Goal**: `make test-lint` runs real style and static analysis in both languages, and `AGENTS.md` stops lying about it.

**Independent test**: introduce a deliberate violation in each application; each is reported with file and line.

**Blocking note**: this phase must land before Phase 6 — the `Stop` hook calls `test-lint`, so redefining it first avoids shipping a hook that silently checks less than it claims.

### Go linting

- [X] T016 [US2] Write the pin `/home/nnc/Projects/job-finder/apps/api/.golangci-version` containing `2.12.2` (current release; Go 1.26 support landed 2026-02-10, and the module targets `go 1.26`)
- [X] T017 [US2] Install it locally: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2`
- [X] T018 [US2] **Measure the backlog before fixing the rule set** (FR-012, research R3): run with `linters.default: standard` and record the issue count per linter in a table. `errcheck` and `staticcheck` are the likely large contributors on 236 never-linted files
- [X] T019 [US2] Apply the FR-012 thresholds to that table — **no judgement in the moment**: every rule at 0 violations is enabled; a rule at 1–30 is fixed and enabled; anything above 30 is deferred. The shared cap across both languages is **80 violations fixed total**; once reached, every remaining rule is deferred regardless of its individual count. Record the resulting enable/defer decision per rule
- [X] T020 [US2] Write `/home/nnc/Projects/job-finder/apps/api/.golangci.yml` in v2 format (`version: "2"`) enabling exactly the rules T019 selected; set `linters.exclusions.generated` and additionally list `internal/db/sqlcgen` explicitly, because relying on a header comment for something this load-bearing is fragile
- [X] T021 [US2] Record every deferred linter in `.golangci.yml` as a commented entry with its measured count and the reason it was deferred (over 30, or cap reached), so widening later is a known quantity rather than a rediscovery
- [X] T022 [US2] Fix the selected violations, keeping the diff in its own commit separate from configuration changes so a reviewer can read each independently. If the fixes exceed the 80 cap, stop and defer the remainder — the cap exists so this feature cannot quietly become a cleanup project
- [X] T023 [US2] Write `/home/nnc/Projects/job-finder/scripts/golangci-check.sh` mirroring `scripts/sqlc-check.sh`'s version guard: fail with pinned-vs-installed and the install line on mismatch, fail with an install line when the binary is absent (FR-015, FR-027)

### Web linting

- [ ] T024 [P] [US2] Add ESLint devDependencies at the repository root — `eslint`, `typescript-eslint`, `eslint-plugin-react-hooks`, `eslint-plugin-react-refresh`. **Install the current major, not v9** — v9 reaches end of life 2026-08-06 (research R4)
- [ ] T025 [US2] Write `/home/nnc/Projects/job-finder/eslint.config.js` as a flat config covering `apps/dashboard` and `packages/shared`, ignoring `**/dist/**`, `packages/shared/src/generated.ts` and `node_modules`. Type-aware rules stay **off** — they conflict with the <60s budget, and `tsc --noEmit` already covers types
- [ ] T026 [US2] Measure and fix the dashboard backlog under the same FR-012 thresholds as T018–T022, drawing from the **same shared 80-violation cap** — the cap spans both languages, so whatever the Go work consumed reduces what is available here. Prioritise `react-hooks` findings — commit `5867674` ("prevent infinite re-render loop in JobDetailPage useEffect") is exactly the defect class this catches

### Wiring and honesty

- [ ] T027 [US2] Add `lint-go`, `lint-web` and `lint` targets to `/home/nnc/Projects/job-finder/Makefile` per contracts/make-targets.md
- [ ] T028 [US2] Redefine `test-lint` in the `Makefile` as `lint-go lint-web test-go test-react`, keeping `test-integration` and `test-e2e` out of it — they need containers and a browser
- [ ] T029 [P] [US2] Delete the dead `"test:python": "make test-python"` script from `/home/nnc/Projects/job-finder/package.json` — the make target does not exist, so the script fails on invocation. **This feature owns this line** (feature 024 verifies it but does not edit it)
- [ ] T030 [P] [US2] Correct the `make test-lint` line in `/home/nnc/Projects/job-finder/AGENTS.md`: drop the Python claim (no Python in the repository) and describe exactly what the recipe now runs (FR-013, SC-010). **This feature owns this description**, because it is the feature that changes what the target runs — feature 024 only checks the description is accurate when it lands
- [ ] T031 [US2] Add the `lint (go)` and `lint (web)` jobs to `/home/nnc/Projects/job-finder/.github/workflows/api-ci.yml` using the exact job names from contracts/required-checks.md, and add both to that file's declared gating set (FR-003, FR-014) — names are a contract, since a rename silently drops a check from the set the phase-2 ruleset consumes
- [ ] T032 [US2] Verify per quickstart.md P2: `time make lint` green in <60s (SC-004); a planted Go violation and a planted dashboard violation each reported with location (FR-011); `make lint 2>&1 | grep -E "sqlcgen|generated\.ts"` returns nothing (FR-010); `PATH=/usr/bin:/bin make lint-go` fails with an install line rather than passing (FR-027)
- [ ] T033 [US2] Verify local and automated verdicts agree (SC-005): with the CI lint jobs live, run `make test-lint` locally on this branch, push, and compare the local result against `gh run view` for the same commit. They must reach the same verdict on style, static analysis and unit tests. A disagreement means a pinned version or a config path differs between the two, which is the failure this feature's version pinning exists to prevent — record the comparison so the ≥95% agreement target has a baseline

**Checkpoint**: US2 done. The documented gate checks what it claims.

---

## Phase 5: User Story 3 — Cross-service behaviour is verified against real infrastructure (P2)

**Goal**: the integration suite runs against real Postgres and Redis in CI, and Playwright runs too.

**Independent test**: a migration conflicting with a query turns the integration check red.

- [ ] T034 [US3] Add the `integration test` job to `/home/nnc/Projects/job-finder/.github/workflows/api-ci.yml` with service containers **`pgvector/pgvector:pg16`** (not stock postgres — fixtures build `vector(768)` values, see `internal/matching/integration_test.go`) and `redis:7-alpine`, setting `POSTGRES_DB: jobfinder_test` directly so no `createdb` step is needed
- [ ] T035 [US3] Implement the **two-step** run inside that job, ordering matters: first `go test -tags integration ./internal/db/... -run TestMigrate`, then `go test -tags integration ./...`. Only `internal/db`'s suite calls `Migrate()` (`internal/db/integration_test.go:31`); the other 12 tagged packages assume a schema, and `go test ./...` runs packages in parallel. The advisory lock in `internal/dbtest/lock.go` serialises `TRUNCATE`, not schema creation
- [ ] T036 [US3] Set `DATABASE_URL` and `REDIS_URL` for both steps per contracts/required-checks.md, and add **no** `COMPOSE_PROJECT_NAME`/`POSTGRES_HOST_PORT` handling — those exist to stop 12 local worktrees colliding on one host; a CI runner is a fresh container
- [ ] T037 [P] [US3] Add the `e2e (playwright)` job with triggers `pull_request`, `push` to master, `schedule: '0 3 * * *'` and `workflow_dispatch` (FR-019). Needs **no** database, Redis or Go backend — `feed.spec.ts` and `sources.spec.ts` mock every call via `page.route`, `navigation.spec.ts` asserts headings only, and `playwright.config.ts` already handles `webServer` and `reuseExistingServer: !process.env.CI`
- [ ] T038 [US3] Confirm FR-020 failure surfacing: a failed scheduled run emails the workflow author by default, which satisfies "without polling" for a solo maintainer — verify it actually arrives rather than assuming
- [ ] T039 [US3] Verify per quickstart.md P3: `make test-integration` and `make test-e2e` green locally; both new jobs green on a pull request; a deliberately broken migration turns `integration test` red; `gh workflow run "API CI"` dispatches
- [ ] T040 [US3] Verify re-runnability (FR-021): take a job that failed for an infrastructure reason rather than a code reason, run `gh run rerun --failed`, and confirm it passes **without a new commit**. Every job must be re-runnable against the unmodified change, which holds only while no job depends on mutable external state — so also confirm no job reaches a live third-party job board or any resource outside the run's own service containers
- [ ] T041 [US3] If `navigation.spec.ts` proves to need real data in CI, add a `page.route` mock to that spec — do **not** boot the API in CI (plan Risks)

**Checkpoint**: US3 done. Cross-service defects are caught by automation for the first time.

---

## Phase 6: User Story 4 — Generated artefacts cannot go stale and code cannot be left unformatted (P2)

**Goal**: the environment repairs what the agent forgets, and blocks a session that ends dirty.

**Independent test**: edit a query file and watch `sqlcgen` regenerate unprompted; end a session with a failing test and be blocked.

**Depends on**: Phase 4 (the `Stop` hook calls `test-lint`).

- [ ] T042 [P] [US4] Write `/home/nnc/Projects/job-finder/scripts/hooks/regen-sqlc.sh`: run `make sqlc-generate`, always exit 0 (`PostToolUse` cannot block), report regenerated files via `hookSpecificOutput.additionalContext`, and fail loudly with an install line if `sqlc` is absent
- [ ] T043 [P] [US4] Write `/home/nnc/Projects/job-finder/scripts/hooks/regen-tygo.sh`: same shape, running `make tygo-generate`. Note in a comment that this closes only **half** the drift gap while `packages/shared/src/index.ts` remains a hand-maintained duplicate — feature 024 removes the duplicate, after which this hook suffices alone
- [ ] T044 [P] [US4] Write `/home/nnc/Projects/job-finder/scripts/hooks/go-postedit.sh`: read `tool_input.file_path` from stdin, `gofmt -w` that file, `go vet` its package only, always exit 0, report via `additionalContext`. Never repository-wide (FR-029)
- [ ] T045 [US4] Write `/home/nnc/Projects/job-finder/scripts/hooks/session-verify.sh`: scope from `git diff --name-only` (merge base plus unstaged) — Go paths touched → `make lint-go test-go`; dashboard/shared paths → `make lint-web test-react`; neither → exit 0 immediately. Exit 2 on failure, which **does** block for `Stop`
- [ ] T046 [US4] Add loop safety to `session-verify.sh`: block at most once per `session_id` (from the stdin JSON) using a marker under the system temp dir, so a second consecutive failure reports without blocking and a session can always end (research R8)
- [ ] T047 [US4] Register all four bindings in `/home/nnc/Projects/job-finder/.claude/settings.json` with the exact `if` path filters from contracts/hooks.md: `Edit(apps/api/**/*.go)`, `Edit(apps/api/internal/db/queries/*.sql)`, `Edit(apps/api/internal/dto/*.go)`, plus the matcher-less `Stop` entry; set a `timeout` on each, generous enough for the 2-minute session budget
- [ ] T048 [US4] Verify no recursion (FR-030): confirm neither `apps/api/internal/db/sqlcgen/**` nor `packages/shared/src/generated.ts` matches any `if` filter, and check it empirically by feeding `regen-sqlc.sh` a `sqlcgen/models.go` path and observing a no-op
- [ ] T049 [US4] Verify per quickstart.md P4: editing a `.sql` query regenerates `sqlcgen` in the working tree for review; editing a DTO regenerates `generated.ts`; badly formatted Go comes back formatted (`gofmt -l apps/api/internal` silent); a failing test blocks the stop; the second `Stop` call with the same `session_id` exits 0 even while still broken
- [ ] T050 [US4] Verify nothing rewrites files at commit time (FR-026): read `.githooks/pre-commit` and confirm it contains no formatting, regeneration or staging step — it may only *reject*. Regeneration must stay visible in the working tree where the author reviews it before committing; a hook that silently fixed files at commit time would put unreviewed machine edits into every commit, which is why FR-026 forbids it even though it would be convenient
- [ ] T051 [US4] Verify the budget (SC-011): touch only a dashboard file, then time `session-verify.sh` — under 2 minutes, with Go suites skipped entirely

**Checkpoint**: US4 done. Codegen drift and unformatted code are structurally prevented, not remembered.

---

## Phase 7: Polish and Cross-Cutting

- [ ] T052 [P] Document the emergency override procedure (FR-005) and the one-hour trunk-recovery expectation (SC-012) in `AGENTS.md`
- [ ] T053 [P] Add `make setup-hooks` to the setup instructions in `AGENTS.md` and `README.md` — it must run once per clone, and an unactivated hook is an absent gate
- [ ] T054 Re-read `AGENTS.md` end to end against the actual `Makefile` and workflow file, confirming no statement is false (SC-010) — the failure mode this feature exists to remove
- [ ] T055 Run the full-loop check from the end of quickstart.md: agent creates a branch, edits a query, edits a DTO, is blocked on a failing test, commits on the branch, opens a PR, and a planted lint violation shows red before merge
- [ ] T056 Confirm the declared gating set in contracts/required-checks.md matches the workflow file exactly — all ten job names (FR-003). A mismatch means the recorded ruleset would silently omit a check on the day the plan is upgraded, which is the one moment nobody would be looking
- [ ] T057 Open the pull request for this feature and confirm all ten checks appear and pass — the feature gating itself is the only end-to-end proof

---

## Dependencies

```text
Phase 1 (Setup) ──> Phase 2 (Foundational)
                        │
                        ├──> Phase 3 (US1) ──┐
                        ├──> Phase 4 (US2) ──┼──> Phase 6 (US4)   [Stop hook needs test-lint]
                        └──> Phase 5 (US3) ──┘
                                                     │
                                              Phase 7 (Polish)
```

- **T003 is a hard prerequisite for T006, T012 and T047** — without the `.gitignore` negation, every `.claude/settings.json` change appears to work locally and does nothing on a fresh clone
- **Phase 4 before Phase 6** — the `Stop` hook must call a `test-lint` that already lints
- Phases 3, 4 and 5 are otherwise independent and may land in any order or in parallel
- Phase 7 needs all four stories

## Parallel Opportunities

- T009 with T008; T011 with T010
- T024 and T025 (web lint) alongside T016–T023 (Go lint) — different toolchains, different files
- T029 and T030 alongside any Makefile work — different files
- T037 (e2e job) alongside T034–T036 (integration job) — they share no infrastructure
- T042, T043, T044 — three independent hook scripts
- T052 and T053 — different sections of `AGENTS.md`, but serialise if editing the same region

## Implementation Strategy

**MVP: Phase 3 (US1) alone.** It is the structural change that makes every existing check meaningful — CI stops being a post-mortem. Four files, no toolchain additions, immediately reversible.

**Then Phase 4 (US2)**, because the constitution already names `test-lint` as the merge gate and it currently checks less than it claims — false confidence is worse than no gate.

**Then Phase 5 and Phase 6** in either order. Phase 5 catches the defect class agents produce most; Phase 6 makes the fastest feedback loop.

Land each phase as its own pull request. That both keeps review small and exercises the gate this feature installs.
