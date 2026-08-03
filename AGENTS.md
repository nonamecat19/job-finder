# AGENTS.md

Context for AI coding agents working in this repo.

## Branching and pull requests

Every change — agent-authored or human-authored — goes on a feature branch
and merges via a pull request whose CI is green. **Never commit or push
directly to `master`.** Create a branch first:

```
git checkout -b <nnn>-<slug>
```

This is enforced, not just documented: `make setup-hooks` (run once per
clone; the config is shared across worktrees) installs committed git hooks
(`.githooks/pre-commit`, `.githooks/pre-push`) that reject a commit or push
targeting `master`, and a Claude Code `PreToolUse` hook
(`scripts/hooks/guard-master.sh`) stops the agent before it even reaches
git. Server-side branch protection is not available on the current GitHub
plan (private repo, Free tier) — see
`specs/023-workflow-quality-gates/contracts/required-checks.md` for the
ruleset recorded to apply the moment that changes.

**Emergency override**: if the trunk itself is broken and the normal branch
workflow can't repair it, `git commit --no-verify` / `git push --no-verify`
bypasses both git hooks. This is the documented, deliberate escape hatch —
its use is visible in shell history and in the agent's transcript, so it is
a traceable act, not a silent bypass. Reach for it only to restore a broken
trunk, never as a way to skip review. The expectation is that the trunk can
be restored from a broken state within one hour using this override — if
recovery is taking longer than that, stop and reconsider the approach
rather than accumulating more direct-to-master commits.

**New clone or new worktree**: run `make setup-hooks` once — it is not
automatic, and an unactivated hook is an absent gate. It sets
`core.hooksPath` at the repository-config level, so a single run in the
main working tree covers every worktree that shares this clone; a fresh
`git clone` elsewhere needs its own run.

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

- `apps/api` — Go backend (HTTP API, asynq workers, ingestion scheduler)
- `apps/dashboard` — React/Vite dashboard
- `packages/shared` — shared TS types, generated from Go DTOs via tygo
- `specs/` — per-feature specs (numbered); each feature's plan lives at
  `specs/<nnn>-<slug>/plan.md` alongside its `spec.md` and `tasks.md` — there
  is no separate top-level `plans/` directory

## Commands

- `make test-lint` — the merge gate: `lint-go` (golangci-lint, pinned in
  `apps/api/.golangci-version`) + `lint-web` (ESLint, `eslint.config.js`) +
  `test-go` (Go unit tests) + `test-react` (Vitest). No Python is in this
  repository, and `test-lint` never claimed to check it. Run it, or
  `make lint-go` / `make lint-web` individually, before opening a pull
  request — it reports each violation with file, line and rule, and passes
  in seconds. `make test-integration` and `make test-e2e` are separate
  targets (they need containers/a browser) and are not part of
  `test-lint`.
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
specs/023-workflow-quality-gates).

## Conventions

- Shared types are generated. Add the field to the Go DTO in `apps/api/internal/dto/`, run `make tygo-generate`, done. `packages/shared/src/index.ts` re-exports and narrows; it never restates a shape. Hand-written types with no backend counterpart live in `consumer-only.ts`.
- HTTP handlers live in `apps/api/internal/<feature>/interfaces/http/` (package `http`) — one adapter package per feature, alongside that feature's `application`/`domain`/`infrastructure` layers. Do not add handlers to `internal/httpapi`; it holds the router and its middleware only. Shared JSON helpers (`WriteJSON`, `WriteError`, `WriteAppError`, `DecodeJSON`) are in `internal/httpx`. Enforced by the `depguard` rules in `apps/api/.golangci.yml` and by `apps/api/internal/arch_test.go` (see specs/027-http-handler-decomposition).
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
