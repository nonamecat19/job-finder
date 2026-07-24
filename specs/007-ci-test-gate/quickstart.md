# Quickstart: Validating the CI Test Gate

## Prerequisites

- Local checkout of `007-ci-test-gate` branch with the updated
  `.github/workflows/api-ci.yml`.
- `act` (https://github.com/nektos/act) if you want to dry-run the workflow locally,
  **or** push to a branch/PR and watch GitHub Actions directly (simpler, recommended —
  service containers are easiest to trust when run for real).
- Local Docker running (for the existing `make test-db-setup` / `make test-go` parity
  check below).

## 1. Confirm the local targets this feature wires into CI still work

```sh
make up                # postgres, redis, ollama
make test-db-setup     # creates jobfinder_test
make test-go           # should pass on master before you touch CI
make test-react
make typecheck
go vet ./... # from apps/api
```

If any of these fail on `master` before your change, fix that first — CI should not be
the first place a real regression is discovered.

## 2. Validate CI catches a backend regression (User Story 1)

1. On the feature branch, introduce a deliberate failure, e.g. in
   `apps/api/internal/keyword/questions_test.go` change an assertion to a wrong value.
2. Push and open a PR (or push to the branch if PR checks run on branch push).
3. Expect: new `go-test` (or equivalent name) job fails; `sqlc-drift`/`tygo-drift` still
   pass (SC-003).
4. Revert the deliberate failure, push again.
5. Expect: `go-test` job passes.

Repeat with a `go vet`-detectable issue (e.g. an unreachable `return` after a `panic`) to
confirm the `go-vet` job fails independently and reports which job failed (SC-004).

## 3. Validate CI catches a frontend regression (User Story 2)

1. Introduce a deliberate failure in an `apps/dashboard` test file (flip an expected
   value) or a TypeScript type error (e.g. assign a `string` to a `number`-typed prop).
2. Push and open a PR.
3. Expect: `frontend-test` job fails for the broken test; `frontend-typecheck` job fails
   independently for the type error (SC-002, SC-004).
4. Revert, push again, expect both jobs pass.

## 4. Validate existing drift checks are unaffected (User Story 3)

1. On the feature branch, edit a `.sql` query file under
   `apps/api/internal/db/queries/` without regenerating `sqlcgen`, or edit a Go DTO
   without regenerating the tygo-generated TS types.
2. Push and open a PR.
3. Expect: `sqlc-drift` or `tygo-drift` job fails exactly as it did before this feature
   (SC-003) — this feature must not have altered their behavior.
4. Revert the un-regenerated change.

## Expected end state

All of: `sqlc-drift`, `tygo-drift`, `go-test`, `go-vet`, `frontend-test`,
`frontend-typecheck` show as distinct, named jobs on a PR's checks list, each
independently green/red (SC-004). No `golangci-lint` or ESLint job exists yet (FR-008 —
intentionally deferred, not a bug).
