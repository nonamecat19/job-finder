# AGENTS.md

Context for AI coding agents working in this repo.

## Branching and pull requests

**The trunk is not protected.** Nothing mechanical stops a commit or push to
`master` — not a git hook, not an agent hook, not a server-side rule. Work
lands where the maintainer says it lands; `CLAUDE.md` is the current word on
that, and it says to work directly on `master` unless asked otherwise.

Use a branch when the change wants review before it lands, or when CI on a
pull request is the point:

```
git checkout -b <nnn>-<slug>
```

This is a deliberate reversal. The repository used to enforce a branch-only
trunk in three places — `.githooks/pre-commit`, `.githooks/pre-push`, and a
Claude Code `PreToolUse` hook — and all three were removed rather than left
contradicting the working instruction in `CLAUDE.md`. Server-side branch
protection remains unavailable on the current GitHub plan (private repo, Free
tier); `specs/domains/platform-operations.md` § 2.2 keeps the ruleset recorded
against the day that changes and someone decides to restore the gate.

The consequence worth naming: a mistake on `master` is now caught by review or
not at all. CI still runs on pull requests, so a change that wants a green
check before it lands still needs a branch to get one.

## Worktrees

The checkout at the repository root is the authoritative working copy.
Isolated working copies live under `.claude/worktrees/<name>/` — created by
requesting worktree isolation on an agent task (e.g. `isolation: "worktree"`
on the Agent tool), which runs `git worktree add` under that directory on a
dedicated branch. They are not registered anywhere else.

**Retirement is manual, not automatic.** When the task using a worktree
finishes: if its branch merged, remove the worktree (`git worktree remove
.claude/worktrees/<name>`) and delete the merged branch; if the branch is
abandoned, remove the worktree the same way and leave the branch for later
reference. A directory under `.claude/worktrees/` that no longer appears in
`git worktree list` is stale — its git metadata was already pruned or it was
never a registered worktree — and can be deleted directly with `rm -rf`;
run `git worktree prune` first to clear any dangling registrations before
trusting `git worktree list` as the source of truth for what is live.
`.claude/worktrees/` itself stays untracked (`.gitignore`); nothing under it
is ever committed.

## Project layout

- `apps/api` — Go backend (HTTP API, RabbitMQ event consumers, ingestion scheduler)
- `apps/ai` — Python AI orchestration service (LangChain/LangGraph, FastAPI + FastStream)
- `apps/dashboard` — React/Vite dashboard
- `packages/shared` — shared TS types, generated from Go DTOs via tygo
- `specs/` — requirement records. `specs/domains/*.md` hold the rules that
  currently bind, consolidated per capability area — they are the **only**
  requirement record. Start at `specs/README.md`. An **in-flight** feature
  scaffolds `specs/<nnn>-<slug>/` with `spec.md`, `plan.md` and `tasks.md`; once
  it ships, its durable requirements and contracts fold into the matching domain
  doc and the whole directory is deleted, not archived. There is no top-level
  `plans/` directory and no `specs/archive/`.
- `docs/` — the Docusaurus implementation guide (architecture, data, ingestion,
  AI, async, frontend, operations). `specs/` says what must be true; `docs/`
  says how it works. Do not duplicate one in the other.

## Commands

- `make test-lint` — the merge gate: `lint-go` (golangci-lint, pinned in
  `apps/api/.golangci-version`) + `lint-web` (ESLint, `eslint.config.js`) +
  `lint-py` (ruff + mypy strict) + `test-go` (Go unit tests) + `test-react`
  (Vitest) + `test-py` (pytest, `apps/ai`). Run it, or `make lint-go` /
  `make lint-web` / `make lint-py` individually, before opening a pull
  request — it reports each violation with file, line and rule, and passes
  in seconds. `make test-integration` and `make test-e2e` are separate
  targets (they need containers/a browser) and are not part of
  `test-lint`.
- `make audit` — the supply-chain gates: `vuln-go` (govulncheck, reachability
  filtered, pinned in `apps/api/.govulncheck-version`) + `vuln-web` (`pnpm
  audit` at severity `high`) + `secrets` (gitleaks over history, redacted).
  **Deliberately not part of `test-lint`**: these depend on the network and on
  an advisory database that changes without any commit, so a green local run
  cannot promise a green CI run — the property `test-lint` exists to give.
  Run it when you touch dependencies. `make images` builds both container
  images and is separate again, because a cold build is 6–8 minutes.
  See `specs/domains/platform-operations.md` § 3.1–3.2 for the runbook,
  including how to record an expiring advisory exception.
- `make sqlc-generate` — regenerate sqlc code after editing `apps/api/internal/db/queries/*.sql`
- `make tygo-generate` — regenerate `packages/shared/src/generated.ts` from Go DTOs after editing any `apps/api/internal/dto/*.go` file
- `pnpm --filter @job-finder/shared build` — rebuild the shared package's `dist/` (the dashboard imports the built package, not source)

While you work, the committed `.claude/settings.json` hooks do some of this
automatically: editing a `*.sql` query under `apps/api/internal/db/queries/`
or a DTO under `apps/api/internal/dto/` regenerates the corresponding
generated output in the working tree (review it before committing — it is
never applied invisibly); editing Go source runs `gofmt` and `go vet` on
the affected package. Ending a session runs `lint-go`/`test-go` and/or
`lint-web`/`test-react` — whichever this session actually touched — and
blocks completion if any of them fail (see
specs/domains/platform-operations.md).

## Conventions

- Shared types are generated. Add the field to the Go DTO in `apps/api/internal/dto/`, run `make tygo-generate`, done. `packages/shared/src/index.ts` re-exports and narrows; it never restates a shape. Hand-written types with no backend counterpart live in `consumer-only.ts`.
- HTTP handlers live in `apps/api/internal/<feature>/interfaces/http/` (package `http`) — one adapter package per feature, alongside that feature's `application`/`domain`/`infrastructure` layers. Do not add handlers to `internal/httpapi`; it holds the router and its middleware only. Shared JSON helpers (`WriteJSON`, `WriteError`, `WriteAppError`, `DecodeJSON`) are in `internal/httpx`. Enforced by the `depguard` rules in `apps/api/.golangci.yml` and by `apps/api/internal/arch_test.go` (see specs/domains/codebase-structure.md).
- New HTTP handlers are still wired in `apps/api/cmd/server/` via `httpapi.NewRouter(...)`'s variadic mounts, not by editing `router.go` directly.
- sqlc queries live in `apps/api/internal/db/queries/*.sql`; regenerate after changes.

## Running the app

Infra/backend/frontend are all long-lived (`make run-backend`,
`make run-frontend`) — start them via `process-hive`, never directly in a
blocking Bash call.

## Commit guidelines

- Create commits after completing features, refactors, or significant changes.
- Only commit files you changed.
- Use conventional commit format: `feat:`, `fix:`, `chore:`, `docs:`.
