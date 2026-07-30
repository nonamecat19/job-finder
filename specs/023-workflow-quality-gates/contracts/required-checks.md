# Contract: CI Checks and the Phase-2 Ruleset

## Job names are a contract

This table **is** the declared gating set that FR-003 requires: the single place recording which checks are meant to gate integration. Today nothing on the host consumes it — the plan cannot block a merge — so it serves two purposes: it tells the maintainer what must be green before integrating, and it is the exact input to the ruleset in the Phase 2 section below.

Job `name:` values are the identifiers a GitHub ruleset would list as required status checks. Renaming a job silently drops it from that list — the check keeps running and stops gating. Any rename must update this file in the same change (FR-003).

| Job name | Trigger | Runtime budget | Status |
|---|---|---|---|
| `sqlc generate is up to date` | PR, push master | ~1 min | existing |
| `tygo generate is up to date` | PR, push master | ~1 min | existing |
| `go vet` | PR, push master | ~2 min | existing |
| `go test` | PR, push master | ~3 min | existing |
| `frontend test (vitest)` | PR, push master | ~2 min | existing |
| `frontend typecheck` | PR, push master | ~2 min | existing |
| `lint (go)` | PR, push master | ~2 min | **new** |
| `lint (web)` | PR, push master | ~1 min | **new** |
| `integration test` | PR, push master | ~5 min | **new** |
| `e2e (playwright)` | PR, push master, nightly, manual | ~4 min | **new** |

Whole set runs in parallel; wall clock target ≤10 min (plan Performance Goals).

## `lint (go)`

```yaml
- uses: actions/setup-go@v5
  with:
    go-version-file: apps/api/go.mod
- name: Read pinned golangci-lint version
  id: gl
  run: echo "version=$(tr -d '[:space:]' < apps/api/.golangci-version)" >> "$GITHUB_OUTPUT"
- uses: golangci/golangci-lint-action@v8
  with:
    version: v${{ steps.gl.outputs.version }}
    working-directory: apps/api
```

Version is read from the pin file, exactly as the existing `sqlc-drift` and `tygo-drift` jobs do. The action is used rather than `make lint-go` here only because it provides result caching; the config and pin are identical, so verdicts match `make lint-go`.

## `lint (web)`

Standard pnpm setup (mirroring `frontend test`), then `pnpm --filter … build` for shared, then `make lint-web`.

## `integration test`

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16          # NOT stock postgres — vector(768) columns
    env:
      POSTGRES_DB: jobfinder_test          # avoids a separate createdb step
      POSTGRES_USER: jobfinder
      POSTGRES_PASSWORD: postgres
    options: >-
      --health-cmd "pg_isready -U jobfinder"
      --health-interval 5s --health-retries 10
    ports: ['5432:5432']
  redis:
    image: redis:7-alpine
    ports: ['6379:6379']
```

Two-step run — **the ordering matters**:

```bash
# Step 1: create the schema. Only internal/db's suite calls Migrate().
go test -tags integration ./internal/db/... -run TestMigrate

# Step 2: everything else, now that the schema exists.
go test -tags integration ./...
```

`go test ./...` runs packages in parallel, and the other 12 integration-tagged packages assume an existing schema. A single combined invocation can start a dependent package before migration finishes. The advisory lock in `internal/dbtest/lock.go` serialises `TRUNCATE` between suites but does nothing for schema ordering.

Environment for both steps:

```
DATABASE_URL=postgresql://jobfinder:postgres@localhost:5432/jobfinder_test
REDIS_URL=redis://localhost:6379/1
```

No `COMPOSE_PROJECT_NAME` / `POSTGRES_HOST_PORT` handling — those exist to keep 12 local worktrees from colliding on one host. A CI runner is a fresh container.

## `e2e (playwright)`

```yaml
on:
  pull_request:
  push: { branches: [master] }
  schedule: [{ cron: '0 3 * * *' }]     # FR-019: at least daily
  workflow_dispatch:                     # FR-019: on demand
```

Needs **no** database, Redis or Go backend. `feed.spec.ts` and `sources.spec.ts` mock every API call with `page.route('**/api/…')`; `navigation.spec.ts` asserts headings and URLs only. `playwright.config.ts` already starts the dev server via `webServer` and already sets `reuseExistingServer: !process.env.CI`.

Steps: pnpm install → build shared → `pnpm exec playwright install --with-deps chromium` → `pnpm --filter @job-finder/dashboard test:e2e`.

Failure surfacing (FR-020): GitHub's default is an email on a failed scheduled run to the workflow author — sufficient for a solo maintainer, and it satisfies "without polling".

## Re-runnability (FR-021)

Every job is re-runnable through GitHub's native re-run without amending the change. No job depends on mutable external state: service containers are fresh per run, and no job calls a live third-party job board.

---

## Phase 2: the ruleset to apply on upgrade

Not applicable today — `gh api repos/nonamecat19/job-finder/rulesets` returns `403 Upgrade to GitHub Pro or make this repository public`. Recorded so the upgrade is one command, not a rediscovery.

```bash
gh api -X POST repos/nonamecat19/job-finder/rulesets \
  -f name='master protection' -f target=branch -f enforcement=active \
  -f 'conditions[ref_name][include][]=refs/heads/master' \
  -f 'rules[][type]=deletion' \
  -f 'rules[][type]=non_fast_forward' \
  -f 'rules[][type]=pull_request' \
  -f 'rules[][type]=required_status_checks'
```

With `required_status_checks.required_status_checks` listing exactly the ten job names in the table above, and `pull_request.required_approving_review_count: 0` — the project has one maintainer, so approval must not be required (FR-004). Self-merge after green checks is the intended flow.

Until then, FR-002 is met as written: checks run and report before integration. What is deferred — explicitly, in FR-002 itself — is the host refusing the merge button. The local git hooks (see `hooks.md`) carry FR-001, and spec scenario 6 is the acceptance test for the day this ruleset is applied.
