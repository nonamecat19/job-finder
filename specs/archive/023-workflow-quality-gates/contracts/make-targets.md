# Contract: Make Targets

The command surface authors, agents, hooks and CI all call. The constitution names `make` targets as the canonical entry point precisely so these four callers cannot drift apart. Nothing else may invoke a linter binary directly.

## Added targets

### `make lint-go`

```
Runs:     scripts/golangci-check.sh  (version guard) then golangci-lint run
Scope:    apps/api
Config:   apps/api/.golangci.yml
Pin:      apps/api/.golangci-version
Exit 0:   no violations
Exit !=0: violations printed as file:line: message (linter-name)
Exit !=0: golangci-lint absent or version mismatch, with the install command
```

Version-mismatch message mirrors `scripts/sqlc-check.sh` verbatim in structure — pinned version, installed version, why it matters, install line.

### `make lint-web`

```
Runs:     pnpm eslint
Scope:    apps/dashboard, packages/shared
Config:   eslint.config.js (repository root, flat config)
Ignores:  **/dist/**, packages/shared/src/generated.ts, node_modules
Exit 0:   no violations
Exit !=0: violations printed as file:line:col rule-name
```

Requires `pnpm install` to have run. A missing `node_modules` fails with that instruction, never silently passes.

### `make lint`

```
Runs:     lint-go, then lint-web
Exit:     first non-zero wins
```

### `make setup-hooks`

```
Runs:     git config core.hooksPath .githooks
Effect:   activates .githooks/pre-commit and .githooks/pre-push
Scope:    the whole repository — worktrees share this config value
Idempotent: yes, safe to re-run
```

Must be run once per clone. Referenced from `AGENTS.md` and from `quickstart.md`.

## Redefined target

### `make test-lint`

```
Before:   test-go test-react            # lints nothing, despite the name
After:    lint-go lint-web test-go test-react
Exit:     first non-zero wins
```

**Coverage invariant**: `test-lint` must cover the union of every required CI check that does not need infrastructure. If a check is added to CI without a corresponding local target, SC-005 (≥95% local/CI agreement) breaks and authors stop trusting the local run.

Integration and e2e are deliberately **not** in `test-lint` — they need containers and a browser. They remain `make test-integration` and `make test-e2e`.

## Unchanged targets

`test`, `test-go`, `test-react`, `test-integration`, `test-e2e`, `test-db-setup`, `sqlc-generate`, `sqlc-check`, `tygo-generate`, `tygo-check`, `up`, `down`, `seed`, and the rest keep their current behaviour. The per-worktree isolation block (`COMPOSE_PROJECT_NAME`, `POSTGRES_HOST_PORT`) is untouched.

## Removed

`package.json` → `"test:python": "make test-python"`. The target does not exist; the script fails on invocation. Vestigial, removed alongside the matching false claim in `AGENTS.md`.

## Caller matrix

| Caller | Calls |
|---|---|
| Author, by hand | `make test-lint` before opening a pull request |
| `Stop` hook | `lint-go`/`test-go` and/or `lint-web`/`test-react`, scoped to changed paths |
| `PostToolUse` hooks | `make sqlc-generate`, `make tygo-generate` |
| CI — `lint (go)` | `make lint-go` |
| CI — `lint (web)` | `make lint-web` |
| CI — `integration test` | the two-step integration sequence (see `required-checks.md`) |
| CI — `e2e (playwright)` | `pnpm --filter @job-finder/dashboard test:e2e` |
