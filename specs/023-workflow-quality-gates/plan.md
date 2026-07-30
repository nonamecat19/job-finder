# Implementation Plan: Enforced Workflow Quality Gates

**Branch**: `023-workflow-quality-gates` | **Date**: 2026-07-29 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/023-workflow-quality-gates/spec.md`

## Summary

Convert the project's stated quality rules into mechanisms. Four workstreams, no runtime code changes:

1. **Isolation**: committed git hooks (`core.hooksPath`) reject commits and pushes to `master`; a Claude Code `PreToolUse` hook stops the agent before it tries. Server-side branch protection is **unavailable** (private repo on GitHub Free — see Constraints), so enforcement is client-side with a documented `--no-verify` override.
2. **Real linting**: pin golangci-lint and ESLint, add `lint-go` / `lint-web` targets, redefine `test-lint` to include them, correct `AGENTS.md`.
3. **Real-infrastructure CI**: an integration job with pgvector + Redis service containers, and an e2e job (cheaper than the spec assumed — the Playwright suite mocks the API and needs only the dev server).
4. **Edit-time repair**: `PostToolUse` hooks regenerate sqlc/tygo output and format Go on matching edits; a `Stop` hook runs scoped verification and blocks completion on failure.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`); TypeScript 5.6 / React 19 / Vite 6 (`apps/dashboard`); Bash for hook and check scripts; YAML for GitHub Actions; JSON for Claude Code settings

**Primary Dependencies**: *new* — golangci-lint 2.12.2 (Go 1.26 supported since 2026-02-10), ESLint + typescript-eslint + eslint-plugin-react-hooks + eslint-plugin-react-refresh (flat config); *existing* — sqlc 1.31.1, tygo 0.2.21, goose, Playwright 1.61, Vitest 4, pnpm 11

**Storage**: none added. CI provisions `pgvector/pgvector:pg16` and `redis:7-alpine` as service containers, matching `docker-compose.yml` (pgvector is required — integration fixtures use `vector(768)` columns)

**Testing**: `go test ./...`, `go test -tags integration ./...`, `vitest run`, `playwright test`; all four already exist and are unchanged by this feature

**Target Platform**: GitHub Actions `ubuntu-latest` + local Linux development

**Project Type**: monorepo tooling/CI change — no application source is modified

**Performance Goals**: `lint-go` + `lint-web` complete in <60s locally (SC-004); `Stop` hook adds ≤2 min to a session (SC-011); PR check set completes in ≤10 min wall clock

**Constraints**:

- Server-side gating is impossible today: `gh api repos/nonamecat19/job-finder/rulesets` → `403 Upgrade to GitHub Pro or make this repository public`. Same for `/branches/master/protection`. FR-001..FR-005 are satisfied client-side; the server-side ruleset is written up as a ready-to-apply phase 2.
- `PostToolUse` hooks **cannot block** — exit 2 is explicitly non-blocking for that event. Repair hooks must therefore *fix* rather than *reject*, and report via `additionalContext`. Only `Stop` blocks (exit 2 / `decision: "block"`).
- Hook matchers filter on tool name only; file-path filtering uses the entry's `if` field (`Edit(apps/api/**/*.go)`), with a defensive path re-check inside each script.
- Per-worktree isolation (`COMPOSE_PROJECT_NAME`, `POSTGRES_HOST_PORT` in the Makefile) must survive unchanged; 12 worktrees share one host.
- **`.claude/` cannot currently be committed.** `git check-ignore -v .claude/skills` → `/home/nnc/.gitignore_global:2:.claude`, and `git ls-files .claude` returns nothing. FR-028 requires a committed `settings.json`, so this feature must first add a negation to the repository's own `.gitignore` (a file cannot be re-included while a parent directory is excluded):

  ```gitignore
  !.claude/
  .claude/*
  !.claude/settings.json
  ```

  `settings.local.json` stays excluded — it holds a 140-entry allowlist containing a database password. Feature 024 needs the same negation for `.claude/skills/`; whichever lands first carries it.
- Size of the pre-existing lint-violation backlog is unknown and must be measured before the rule set is fixed (FR-012).

**Scale/Scope**: 236 Go source files + 136 Go test files (13 integration-tagged); 77 dashboard TS/TSX + 25 test files; 3 Playwright specs; 47 files importing `@job-finder/shared`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment |
|---|---|
| I. No Auto-Apply, Ever | **N/A** — no code path touching applications or employers is added. |
| II. Grounded Generation | **N/A** — no LLM-generated content involved. |
| III. Typed Contracts Across Service Boundaries | **Strengthened.** Edit-time sqlc/tygo regeneration makes "regenerate instead of hand-editing" automatic rather than remembered. No new hand-maintained types. |
| IV. Test Discipline Per Language, Enforced at the Boundary | **Directly implements it.** First time `test-integration` runs in automation, and first time `test-lint` actually lints. Principle IV's requirement that integration paths exercise real Postgres/Redis is honoured by service containers, not mocks. |
| V. Local-First, Self-Hosted by Default | **Pass.** No third-party inference added. CI already runs on GitHub Actions; the integration job runs Postgres/Redis containers, not hosted services. No Ollama dependency enters CI — integration suites stub the LLM with `noopLLM`/`fixedEmbedLLM`. |
| Tech constraints — `make` targets canonical | **Pass.** Every new capability is a make target first; CI and hooks call the make targets, so local and automated runs cannot diverge. |
| Tech constraints — goose versions unique/sequential | **N/A** — no migrations. |

**Post-Phase-1 re-check**: still passing. The design adds no new hand-maintained cross-language types, no new inference dependency, and no code path that acts on a job listing.

**Violations requiring justification**: none. Complexity Tracking section omitted.

### Deviation from spec, recorded

An earlier draft of the spec assumed a hosting platform that can reject writes and block merges. It cannot, on the current plan. The user chose client-side enforcement over paying for GitHub Pro or making the repository public.

**The spec has since been reworded to match** — FR-002 now requires that checks run and report before integration, and states that mechanically preventing a red merge is out of scope until the plan is upgraded; FR-004, FR-014 and FR-018 refer to a "declared gating set" rather than host-enforced required checks; SC-001–SC-003 are measured against the local gate and check visibility. So this is no longer a deviation, it is the specified behaviour. `contracts/required-checks.md` records the exact ruleset to apply on upgrade, and spec scenario 6 covers that transition.

What remains true and worth restating: nothing stops a maintainer merging a red pull request today. The local hooks carry FR-001; the maintainer's judgement carries the rest.

## Project Structure

### Documentation (this feature)

```text
specs/023-workflow-quality-gates/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output — configuration surfaces, not DB entities
├── quickstart.md        # Phase 1 output — how to verify the gates actually gate
├── contracts/
│   ├── make-targets.md      # The command surface authors and CI both call
│   ├── required-checks.md   # CI job names + the phase-2 ruleset to apply
│   └── hooks.md             # Claude Code + git hook contracts (I/O, exit codes)
├── checklists/
│   └── requirements.md  # Spec quality checklist (already complete)
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
.githooks/                      # NEW — committed git hooks, activated via core.hooksPath
├── pre-commit                  #   rejects commits on master
└── pre-push                    #   rejects pushes to master

.claude/
└── settings.json               # NEW (committed) — PreToolUse/PostToolUse/Stop hook registry
                                #   settings.local.json stays local and untouched

scripts/
├── sqlc-check.sh               # existing — the pattern the new scripts mirror
├── tygo-check.sh               # existing
└── hooks/                      # NEW — hook implementations, callable standalone
    ├── guard-master.sh         #   PreToolUse: block git commit/push on master
    ├── go-postedit.sh          #   PostToolUse: gofmt + go vet the edited package
    ├── regen-sqlc.sh           #   PostToolUse: make sqlc-generate
    ├── regen-tygo.sh           #   PostToolUse: make tygo-generate
    └── session-verify.sh       #   Stop: scoped test-lint, blocks on failure

apps/api/
├── .golangci.yml               # NEW — v2 config, generated output excluded
└── .golangci-version           # NEW — pin, mirroring .sqlc-version / .tygo-version

eslint.config.js                # NEW — flat config at root, covers dashboard + shared
package.json                    # devDeps for ESLint; remove the dead test:python script

Makefile                        # lint-go, lint-web, lint, setup-hooks; test-lint redefined

.github/workflows/
└── api-ci.yml                  # + integration job, + e2e job

AGENTS.md                       # corrected and extended
```

**Structure Decision**: No application source changes. Everything lands in build/CI/agent configuration. The ESLint config sits at the repository root rather than inside `apps/dashboard` because it must also cover `packages/shared` while ignoring the generated file there — one config with per-package overrides beats two configs that can disagree. Hook logic lives in `scripts/hooks/*.sh` rather than inline in `settings.json` so each hook is independently runnable, testable and reviewable, and so the JSON stays readable.

## Phased Delivery

Ordered so each phase is independently valuable and independently revertible.

| Phase | Delivers | Spec coverage | Gate to proceed |
|---|---|---|---|
| **P1** | Committed git hooks + `make setup-hooks` + agent guard hook + AGENTS.md branch/PR rule | US1, FR-001..FR-006 | Commit to master rejected; commit on branch succeeds |
| **P2** | Lint configs pinned, `lint-go`/`lint-web`/`lint` targets, `test-lint` redefined, backlog measured and rule set fixed, CI lint jobs | US2, FR-007..FR-015 | `make test-lint` green on a clean tree, lint portion <60s |
| **P3** | Integration CI job (pgvector + Redis), e2e CI job | US3, FR-016..FR-021 | Both jobs green on a PR; a deliberately broken migration turns integration red |
| **P4** | PostToolUse regeneration/formatting hooks, Stop verification hook | US4, FR-022..FR-030 | Editing a query file regenerates sqlcgen unprompted; Stop blocks on a failing suite |

P2 must land before P4 — the Stop hook calls `test-lint`, so redefining it first avoids a hook that silently checks less than it claims, which is the exact bug this feature exists to remove.

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Lint backlog large enough to block adoption | P2 stalls | Measure first (research R3), then apply FR-012's numeric thresholds rather than judging in the moment: 0 violations → enable, 1–30 → fix and enable, over 30 → defer with the count recorded; shared cap of 80 violations fixed across both languages. The cap is what stops this feature becoming a cleanup project. |
| `--no-verify` makes the local gate advisory | FR-001 is satisfied only client-side | Accepted and specified — it *is* the FR-005 override, and FR-001 now says so. The agent guard hook closes the common path, since the agent cannot pass `--no-verify` without it being visible in the transcript. |
| Integration suites race on schema in CI | Flaky red | Run the migration suite first as its own step, then the rest — mirrors what the maintainer already does locally (visible in the permission allowlist). The advisory lock in `internal/dbtest` already serialises truncation. |
| Stop hook too slow, gets disabled | SC-011 missed, feature abandoned | Scope to touched apps via `git diff --name-only`; skip Go tests entirely when no Go file changed. |
| Hook schema drift between Claude Code versions | Hooks silently stop firing | Every hook script is runnable standalone and covered by `quickstart.md` checks, so breakage is detectable without reading Claude Code internals. |
| `navigation.spec.ts` does not mock the API and may need a backend | e2e job red in CI | The other two specs mock via `page.route`; navigation asserts only headings. If it proves to need data, add a route mock rather than booting the API in CI. |
| Global gitignore silently drops the committed hook settings | P4 appears to work locally, does nothing on a fresh clone | `.gitignore` negation lands in P1, verified with `git ls-files .claude/settings.json` returning the file — not with `git status` looking clean. |
