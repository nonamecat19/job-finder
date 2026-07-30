# Data Model: Enforced Workflow Quality Gates

This feature stores no data and adds no database entities. What it *does* define is a set of **configuration surfaces** — files whose contents are the machine-readable form of the project's definition of "done". They are modelled here because they have fields, validation rules, and relationships, and because keeping them consistent with each other is the whole point of the feature.

## Entity map

```text
Quality command (Makefile)
   ├── owns ──> Lint rule set (Go)      apps/api/.golangci.yml + .golangci-version
   ├── owns ──> Lint rule set (JS/TS)   eslint.config.js + package.json devDeps
   └── invoked by ──┬── Check definition (CI)   .github/workflows/api-ci.yml
                    └── Hook binding (agent)    .claude/settings.json → scripts/hooks/*.sh

Branch guard
   ├── Git hook        .githooks/pre-commit, .githooks/pre-push  (via core.hooksPath)
   └── Agent hook      .claude/settings.json PreToolUse → scripts/hooks/guard-master.sh
```

---

## Quality command

The single documented answer to "is my change acceptable". Lives in `Makefile`.

| Field | Value | Rule |
|---|---|---|
| `lint-go` | runs pinned golangci-lint over `apps/api` | must fail non-zero on any violation |
| `lint-web` | runs pinned ESLint over dashboard + shared | must fail non-zero on any violation |
| `lint` | `lint-go` + `lint-web` | convenience aggregate |
| `test-go` | existing | unchanged |
| `test-react` | existing | unchanged |
| `test-lint` | `lint-go lint-web test-go test-react` | **redefined**; fails if any of the four fails |
| `setup-hooks` | `git config core.hooksPath .githooks` | idempotent; safe to re-run |

**Validation rules**
- Every capability CI uses must exist as a make target; CI must not invoke a linter binary directly (FR-009, SC-005).
- `test-lint`'s coverage must equal the union of the required CI checks, or local and automated verdicts diverge.
- `AGENTS.md`'s description of `test-lint` must match its recipe exactly (FR-013, SC-010).

**Known-false state being corrected**
- `AGENTS.md` claims Python coverage; there is none.
- `package.json` declares `"test:python": "make test-python"`; no such target exists.

---

## Lint rule set (Go)

| Field | Value | Rule |
|---|---|---|
| config path | `apps/api/.golangci.yml` | v2 format, `version: "2"` |
| version pin | `apps/api/.golangci-version` → `2.12.2` | mirrors `.sqlc-version` / `.tygo-version` |
| enabled linters | determined by measurement, then the FR-012 thresholds | 0 violations → enable; 1–30 → fix and enable; >30 → defer; shared cap 80 fixed across both languages |
| generated exclusion | `linters.exclusions.generated` + explicit `internal/db/sqlcgen` path | generated code is never held to hand-written rules (FR-010) |
| deferred linters | recorded as commented entries with violation counts | widening later is a known quantity |

**Validation rules**
- Installed version must equal the pin, or the check fails with an install line — same guard as `scripts/sqlc-check.sh` (FR-015).
- A missing binary must fail, never pass silently (FR-027).

---

## Lint rule set (JS/TS)

| Field | Value | Rule |
|---|---|---|
| config path | `eslint.config.js` at repository root | flat config; covers `apps/dashboard` and `packages/shared` |
| packages | `eslint`, `typescript-eslint`, `eslint-plugin-react-hooks`, `eslint-plugin-react-refresh` | current major (v9 EOL 2026-08-06 — install v10 if GA) |
| ignores | `**/dist/**`, `packages/shared/src/generated.ts`, `node_modules` | generated and built output excluded (FR-010) |
| type-aware rules | disabled | deliberate: conflicts with the <60s budget; `tsc --noEmit` already covers types |
| version pinning | exact versions in `package.json`, reproduced by `pnpm-lock.yaml` | local and CI resolve identically (FR-015) |

---

## Check definition (CI)

A named automated verification in `.github/workflows/api-ci.yml`. Existing jobs are unchanged; four are added.

| Job name | Trigger | Services | Added |
|---|---|---|---|
| `sqlc generate is up to date` | PR, push to master | — | existing |
| `tygo generate is up to date` | PR, push to master | — | existing |
| `go vet` | PR, push to master | — | existing |
| `go test` | PR, push to master | — | existing |
| `frontend test (vitest)` | PR, push to master | — | existing |
| `frontend typecheck` | PR, push to master | — | existing |
| `lint (go)` | PR, push to master | — | **new** |
| `lint (web)` | PR, push to master | — | **new** |
| `integration test` | PR, push to master | pgvector/pgvector:pg16, redis:7-alpine | **new** |
| `e2e (playwright)` | PR, push to master, nightly schedule, manual dispatch | — | **new** |

**Validation rules**
- Job names are a contract: they are the **declared gating set** (FR-003), recorded in `contracts/required-checks.md`, and the exact input a future GitHub ruleset would consume. Renaming a job silently drops it from that set — the check keeps running and stops gating.
- Every job must be re-runnable without amending the change under test (FR-021) — satisfied by GitHub's native re-run, provided no job depends on a mutable external resource.
- The integration job must migrate before running dependent suites (research R5).

---

## Branch guard

Two independent layers; either alone is a partial gate.

| Field | Git layer | Agent layer |
|---|---|---|
| location | `.githooks/pre-commit`, `.githooks/pre-push` | `.claude/settings.json` `PreToolUse` → `scripts/hooks/guard-master.sh` |
| activation | `git config core.hooksPath .githooks` (once per clone; shared by all worktrees) | committed settings, active on clone |
| trigger | committing or pushing while on `master` | agent about to run `git commit` / `git push` |
| effect | abort with a message naming the branch-and-PR rule | block the tool call, feed the reason back to the agent |
| override | `git commit --no-verify` | not available to the agent without it appearing in the transcript |

**Validation rules**
- Activation must be idempotent and discoverable — `make setup-hooks`, referenced from `AGENTS.md` (FR-006).
- The override must leave a visible trace (FR-005): `--no-verify` appears in shell history and in an agent transcript.
- Neither layer may fire on a non-`master` branch.

---

## Hook binding

An entry in the committed `.claude/settings.json` mapping an event to a script.

| Field | Constraint |
|---|---|
| `matcher` | tool name only — file paths are **not** matchable here (research R2) |
| `if` | permission-rule syntax for path filtering, e.g. `Edit(apps/api/internal/db/queries/*.sql)` |
| `type` / `command` | `"command"` pointing at `$CLAUDE_PROJECT_DIR/scripts/hooks/*.sh` |
| `timeout` | set per hook; regeneration is seconds, session verification is up to the 2-minute budget |
| blocking | `PostToolUse` **cannot** block; only `Stop` can (exit 2 or `decision: "block"`) |

**Bindings**

| Event | Path filter | Script | Effect |
|---|---|---|---|
| `PreToolUse` | `Bash(git commit*)`, `Bash(git push*)` | `guard-master.sh` | blocks on `master` |
| `PostToolUse` | `apps/api/**/*.go` | `go-postedit.sh` | `gofmt -w` + `go vet` the affected package |
| `PostToolUse` | `apps/api/internal/db/queries/*.sql` | `regen-sqlc.sh` | `make sqlc-generate` |
| `PostToolUse` | `apps/api/internal/dto/*.go` | `regen-tygo.sh` | `make tygo-generate` |
| `Stop` | — (no matcher support) | `session-verify.sh` | scoped `test-lint`; blocks on failure |

**Validation rules**
- Committed in `.claude/settings.json`, not `settings.local.json`, so every clone and worktree inherits them (FR-028).
- Scoped to the edited file; no repository-wide work per edit (FR-029).
- Must not re-trigger on their own generated output (FR-030) — regeneration writes to `sqlcgen/` and `generated.ts`, neither of which matches any binding's `if` filter.
- Results land in the working tree for review; nothing rewrites files at commit time (FR-026).
- `session-verify.sh` blocks at most once per `session_id`, so a session can always terminate (research R8).
