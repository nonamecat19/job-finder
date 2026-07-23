# AGENTS.md

Context for AI coding agents working in this repo.

## Project layout

- `apps/api` — Go backend (HTTP API, asynq workers, ingestion scheduler)
- `apps/dashboard` — React/Vite dashboard
- `apps/extension` — browser extension (autofill)
- `apps/jobspy-sidecar` — Python scraping sidecar
- `packages/shared` — shared TS types, generated from Go DTOs via tygo
- `specs/` — per-feature specs (numbered)
- `plans/` — implementation plans derived from specs

## Commands

- `make test-lint` — full test suite (Go + React + Python) + lint
- `make sqlc-generate` — regenerate sqlc code after editing `apps/api/internal/db/queries/*.sql`
- `tygo generate` (run from `apps/api`) — regenerate `packages/shared/src/generated.ts` from Go DTOs after editing `apps/api/internal/dto/dto.go`
- `pnpm --filter @job-finder/shared build` — rebuild the shared package's `dist/` (the dashboard imports the built package, not source)

## Conventions

- Go DTO field names/JSON tags in `apps/api/internal/dto/dto.go` must match `packages/shared/src/index.ts` field-for-field — `index.ts` is hand-maintained (not auto-imported from the tygo-generated `generated.ts`), so update both when adding a DTO field.
- New HTTP handlers are wired in `apps/api/cmd/server/main.go` via `httpapi.NewRouter(...)`'s variadic mounts, not by editing `router.go` directly.
- sqlc queries live in `apps/api/internal/db/queries/*.sql`; regenerate after changes.

## Running the app

Infra/backend/frontend are all long-lived (`make up`, `make run-backend`,
`make run-frontend`) — start them via `process-hive`, never directly in a
blocking Bash call.

## Commit guidelines

- Create commits after completing features, refactors, or significant changes.
- Only commit files you changed.
- Use conventional commit format: `feat:`, `fix:`, `chore:`, `docs:`.
