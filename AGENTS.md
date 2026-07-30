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
trunk, never as a way to skip review.

## Project layout

- `apps/api` — Go backend (HTTP API, asynq workers, ingestion scheduler)
- `apps/dashboard` — React/Vite dashboard
- `packages/shared` — shared TS types, generated from Go DTOs via tygo
- `specs/` — per-feature specs (numbered)
- `plans/` — implementation plans derived from specs

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
- `tygo generate` (run from `apps/api`) — regenerate `packages/shared/src/generated.ts` from Go DTOs after editing `apps/api/internal/dto/dto.go`
- `pnpm --filter @job-finder/shared build` — rebuild the shared package's `dist/` (the dashboard imports the built package, not source)

While you work, the committed `.claude/settings.json` hooks do some of this
automatically: editing a `*.sql` query under `apps/api/internal/db/queries/`
or a DTO under `apps/api/internal/dto/` regenerates the corresponding
generated output in the working tree (review it before committing — it is
never applied invisibly); editing Go source runs `gofmt` and `go vet` on
the affected package. Ending a session runs a scoped `test-lint` and blocks
completion if it fails (see specs/023-workflow-quality-gates).

## Conventions

- Go DTO field names/JSON tags in `apps/api/internal/dto/dto.go` must match `packages/shared/src/index.ts` field-for-field — `index.ts` is hand-maintained (not auto-imported from the tygo-generated `generated.ts`), so update both when adding a DTO field.
- New HTTP handlers are wired in `apps/api/cmd/server/main.go` via `httpapi.NewRouter(...)`'s variadic mounts, not by editing `router.go` directly.
- sqlc queries live in `apps/api/internal/db/queries/*.sql`; regenerate after changes.

## Running the app

Infra/backend/frontend are all long-lived (`make run-backend`,
`make run-frontend`) — start them via `process-hive`, never directly in a
blocking Bash call.

## Commit guidelines

- Create commits after completing features, refactors, or significant changes.
- Only commit files you changed.
- Use conventional commit format: `feat:`, `fix:`, `chore:`, `docs:`.
