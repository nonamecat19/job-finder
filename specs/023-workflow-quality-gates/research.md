# Phase 0 Research: Enforced Workflow Quality Gates

All unknowns from Technical Context resolved. Findings verified against this repository and against current upstream documentation, not from memory.

---

## R1 — How can the trunk reject direct writes?

**Decision**: Committed git hooks in `.githooks/`, activated by `git config core.hooksPath .githooks`, plus a Claude Code `PreToolUse` hook that stops the agent before it reaches git. No server-side rule.

**Evidence**: the repository is private under a Free plan. Both gating APIs refuse:

```
$ gh api repos/nonamecat19/job-finder/rulesets
{"message":"Upgrade to GitHub Pro or make this repository public to enable this feature.","status":"403"}

$ gh api repos/nonamecat19/job-finder/branches/master/protection
{"message":"Upgrade to GitHub Pro or make this repository public to enable this feature.","status":"403"}
```

**Rationale**: `core.hooksPath` is a repository-level config value. Git worktrees share the main repository's config, so one `git config` call covers the working tree and all 12 worktrees at once — important here, where per-worktree setup would silently miss most of them. Hooks live under version control, so they are reviewable and travel with the repository (`.git/hooks/` does not).

**Consequences and limits**:
- `--no-verify` bypasses both hooks. This is accepted: it *is* the FR-005 emergency override, and it is visible in a transcript when an agent uses it.
- Nothing prevents merging a pull request whose checks are red. CI still reports the result; the human decides. SC-001/SC-002 are measured against the local gate.

**Alternatives considered**:
- *GitHub Pro (~$4/mo)* — fully satisfies FR-001..FR-005 server-side. Rejected for now by the maintainer; the exact ruleset is pre-written in `contracts/required-checks.md` so the upgrade is a one-command change.
- *Make the repository public* — rulesets become free. `.env` and `resume/` are gitignored and untracked, so the working tree is clean, but git history would need auditing and the maintainer's job-search data would become public. Rejected.
- *Server-side pre-receive hook* — GitHub Enterprise only. Not available.

---

## R2 — What exactly can a Claude Code hook do?

**Decision**: `PreToolUse` guards the branch, `PostToolUse` repairs, `Stop` verifies and blocks. Hook bodies are shell scripts under `scripts/hooks/`; `settings.json` only wires them up.

**Findings from the current hooks documentation** (these corrected two assumptions in the spec):

| Question | Answer |
|---|---|
| Can `PostToolUse` block? | **No.** "Exit 2: Non-blocking error (cannot block for PostToolUse) … the tool already ran." |
| Can `Stop` block? | **Yes.** "Exit 2: Blocking error. Stderr fed back to Claude; prevents Claude from stopping." Equivalent JSON form: `{"decision": "block", "reason": "..."}`. |
| Do matchers match file paths? | **No** — tool name only. Per-entry `if` field does path filtering, using permission-rule syntax: `"if": "Edit(*.ts)"`. |
| Is `$CLAUDE_PROJECT_DIR` available? | Yes, exported to the hook subprocess. |
| How does a hook see the edited file? | stdin JSON: `tool_name`, `tool_input.file_path`, `tool_result`. |
| Can a hook tell Claude what it did? | Yes — `hookSpecificOutput.additionalContext` on stdout. |

**Impact on design**: FR-022..FR-024 (regenerate, format) are *repair* actions, which suits a non-blocking event — the hook fixes the problem and reports it. FR-025 (session-end blocking) requires `Stop`, which does block. Had the design leaned on `PostToolUse` to reject bad edits, it would have silently done nothing.

**Rationale for scripts over inline commands**: each hook can be run by hand during development and in `quickstart.md`, keeps `settings.json` readable, and lets the repair logic be reviewed as code.

**Alternatives considered**: inline shell in `settings.json` (unreviewable, untestable); a single dispatcher hook switching on file path (one failure mode takes out every hook).

---

## R3 — Which Go linter, which rules, and how big is the backlog?

**Decision**: golangci-lint **2.12.2**, pinned in `apps/api/.golangci-version`, with a v2-format `apps/api/.golangci.yml`. The enabled rule set is **determined by measurement during implementation**, not fixed here.

**Version rationale**: 2.12.2 is the current release (2026-05-06); Go 1.26 support landed 2026-02-10, and the module targets `go 1.26`. golangci-lint only supports Go versions ≤ the version it was built with, so an older pin would not work at all. The pin file mirrors the existing `.sqlc-version` / `.tygo-version` convention, and the check script will mirror `scripts/sqlc-check.sh`'s version-mismatch guard — different linter versions report different issues, which would make the gate flap between machines and CI.

**Generated-code exclusion**: v2 has `linters.exclusions.generated` with `strict` (default), `lax`, `disable`. `internal/db/sqlcgen` carries the standard `// Code generated … DO NOT EDIT.` header, so the default already covers it; the config will additionally list the path explicitly, because relying on a header comment for something this load-bearing is fragile.

**Rule-set strategy (FR-012)**: the backlog across 236 source files is unmeasured. Implementation measures it first:

1. Run with `linters.default: standard` (errcheck, govet, ineffassign, staticcheck, unused) and count issues per linter.
2. Apply the FR-012 thresholds mechanically: 0 violations → enable; 1–30 → fix and enable; over 30 → defer. Shared cap of **80 violations fixed across both languages**; once reached, defer everything remaining regardless of individual count.
3. Record deferred linters in the config as commented entries with their counts, so widening later is a known quantity rather than a rediscovery.

`errcheck` and `staticcheck` are the likely large contributors on a codebase that has never been linted. Starting narrow and widening is explicitly permitted by FR-012 and is what makes the gate landable at all.

**Alternatives considered**: `go vet` alone (already in CI; catches far less); `staticcheck` standalone (one tool instead of an aggregator, no unified config, no generated-file handling); `linters.default: all` (guaranteed to be unlandable).

---

## R4 — Which JS linter, and where does the config live?

**Decision**: ESLint flat config at the repository root (`eslint.config.js`), covering `apps/dashboard` and `packages/shared`, with `packages/shared/src/generated.ts` and all `dist/` ignored.

**Current state**: no ESLint anywhere — not in `apps/dashboard/package.json`, not at the root, no config file. This is a from-scratch install, not a migration, so flat config is the only format to consider.

**Packages**: `eslint`, `typescript-eslint`, `eslint-plugin-react-hooks`, `eslint-plugin-react-refresh`. React 19 + Vite 6 + TS 5.6 make this the standard set; `react-hooks` in particular catches the dependency-array and conditional-hook defects this codebase has already hit (commit `5867674` "prevent infinite re-render loop in JobDetailPage useEffect").

**Version note**: ESLint v9 reaches end of life **2026-08-06**, days from now. Implementation must install the current major (v10 if generally available) rather than pinning v9. Flat config applies to both, so the config shape is unaffected. Version is pinned exactly in `package.json` — `pnpm-lock.yaml` already guarantees reproducibility between local and CI.

**Why a root config rather than one per app**: `packages/shared` needs linting with a *different* rule (ignore the generated file) than the dashboard. Two configs that must agree about a shared package is precisely the duplication pattern feature 024 exists to remove; one root config with per-package overrides avoids introducing it here.

**Type-aware linting**: deferred. Type-aware rules require a `project` service and are markedly slower, which conflicts with the <60s budget in SC-004. `tsc --noEmit` already runs in CI and covers type errors. Recorded so the omission is deliberate.

---

## R5 — How does the integration suite get a working database in CI?

**Decision**: GitHub Actions service containers — `pgvector/pgvector:pg16` and `redis:7-alpine` — then a two-step test run: migrate first, then everything else.

**Findings**:
- The image must be pgvector, not stock `postgres`. Integration fixtures build `vector(768)` values (`internal/matching/integration_test.go` `fixedEmbedLLM`), and `docker-compose.yml` already uses `pgvector/pgvector:pg16`. Stock Postgres fails at schema creation.
- Only `internal/db`'s suite calls `Migrate(dsn)` (`internal/db/integration_test.go:31`). The other 12 integration-tagged packages assume the schema already exists. `go test ./...` runs packages in parallel, so a single combined invocation can start a dependent package before migration completes.
- `internal/dbtest/lock.go` takes a Postgres session advisory lock (`0x104BF1DE`) to serialise the suites that `TRUNCATE` shared tables. This solves data races between suites — it does **not** solve schema ordering.

**Design**: run `go test -tags integration ./internal/db/... -run TestMigrate` as its own step, then `go test -tags integration ./...`. This is exactly the sequence already present in the maintainer's permission allowlist, so it is a codification of the working local practice rather than a new invention.

**Database creation**: the suite expects `jobfinder_test`. The service container creates only its `POSTGRES_DB`, so CI sets `POSTGRES_DB: jobfinder_test` directly, avoiding the `createdb` step the Makefile needs locally.

**Worktree isolation**: untouched. `COMPOSE_PROJECT_NAME` / `POSTGRES_HOST_PORT` exist to keep 12 local worktrees from colliding; a CI runner is a fresh container with no such contention, and service containers publish a mapped port. No Makefile change.

**Alternatives considered**: `docker compose up` inside the runner (slower, duplicates what service containers do natively, and drags in ollama/minio/flaresolverr, none of which the integration suites need); testcontainers (a new Go dependency and a rewrite of every `TestMain`).

---

## R6 — What does the e2e suite actually need?

**Decision**: run Playwright on **every pull request**, not nightly. It needs only the Vite dev server.

**Evidence**: `apps/dashboard/tests/e2e/` holds 3 specs. `feed.spec.ts` and `sources.spec.ts` mock every backend call with `page.route('**/api/…')`. `navigation.spec.ts` asserts headings and URLs only. `playwright.config.ts` already declares `webServer: { command: 'pnpm dev', url: 'http://localhost:5173' }`, and the Vite proxy points `/api` at `localhost:3000` — used only by unmocked calls, which resolve to failed fetches the assertions do not depend on.

**This contradicts a spec assumption.** The spec assumed e2e was "too slow and too infrastructure-heavy for every change request" and scheduled it nightly. Measured, it needs no database, no Redis, and no Go backend — a Node install plus a browser download. FR-019's nightly schedule is still honoured (`schedule:` plus `workflow_dispatch:`), but the suite also runs per pull request, which is strictly better than the spec required.

**Caveat**: `retries: 1`, `workers: 2` are already configured, so a flake retries once. `webServer.reuseExistingServer: !process.env.CI` already does the right thing under CI. No config change needed.

**Residual risk**: if `navigation.spec.ts` turns out to need real data on some route, the fix is a `page.route` mock in that spec — not booting the API in CI.

---

## R7 — How does the quality command stay honest?

**Decision**: `test-lint` becomes `lint-go lint-web test-go test-react`. CI calls the same make targets. `AGENTS.md` is corrected in the same change.

**Two false claims found, both to be fixed**:
1. `AGENTS.md` — "`make test-lint` — full test suite (Go + React + Python) + lint". There is no Python in the repository, and no lint runs.
2. `package.json` — `"test:python": "make test-python"`. That make target **does not exist**; the script fails on invocation. Same phantom-Python lineage. Removed.

**Rationale for make targets as the single surface**: the constitution already names `make` targets as the canonical entry point so CI and local runs stay aligned. If CI invoked `golangci-lint` directly while an author invoked `make lint-go`, the two would drift the moment a flag changed — reproducing the class of bug this whole feature targets. SC-005's ≥95% local/CI agreement depends on this.

---

## R8 — How does session-end verification stay under 2 minutes?

**Decision**: `scripts/hooks/session-verify.sh` scopes by changed path, using `git diff --name-only` against the merge base plus unstaged changes.

- Go files touched → `lint-go` + `test-go`
- Dashboard/shared files touched → `lint-web` + `test-react`
- Neither → exit 0 immediately
- Nothing changed at all → exit 0 immediately

**Rationale**: the unscoped suite is 236 Go files plus 25 dashboard test files; running everything after a one-line documentation edit is the fastest route to the hook being deleted by an irritated user. SC-011 sets a 2-minute budget, and FR-029 already requires scoping to files actually edited.

**Loop safety**: a `Stop` hook that blocks re-enters the agent loop, which can end in another `Stop`. The script blocks at most once per session by recording a marker keyed on `session_id` (available in the hook's stdin JSON) under the system temporary directory, so a second consecutive failure reports rather than blocks and the session can always end.

**Tool-absence behaviour (FR-027)**: every script checks for its tool first and exits non-zero with an install line, mirroring `scripts/sqlc-check.sh`'s existing message style. A missing linter must never read as a pass.

---

## Sources

- [Claude Code hooks reference](https://code.claude.com/docs/en/hooks) — event schemas, matcher and `if` semantics, exit-code behaviour per event
- [golangci-lint configuration file](https://golangci-lint.run/docs/configuration/file/) — v2 format, `linters.exclusions.generated`
- [golangci-lint changelog](https://golangci-lint.run/docs/product/changelog/) — 2.12.2, Go 1.26 support date
- [golangci-lint Go 1.26 support issue](https://github.com/golangci/golangci-lint/issues/6272)
- [ESLint flat config `extends` / `defineConfig`](https://eslint.org/blog/2025/03/flat-config-extends-define-config-global-ignores/)
- [eslint-plugin-react-hooks](https://www.npmjs.com/package/eslint-plugin-react-hooks) — flat recommended config
- Repository evidence: `gh api` 403 responses, `apps/api/internal/dbtest/lock.go`, `apps/api/internal/db/integration_test.go:31`, `apps/dashboard/playwright.config.ts`, `apps/dashboard/tests/e2e/*.spec.ts`, `docker-compose.yml`, `Makefile`, `AGENTS.md`, `package.json`
