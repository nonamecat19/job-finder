# Contract: CI check names

**Feature**: `039-supply-chain-ci`

The `name:` value of a workflow job is the identifier a GitHub ruleset lists as a required
status check. Renaming a job silently drops it from that list — the check keeps running and
stops gating (`specs/domains/platform-operations.md` § 2.1). These names are therefore a
contract, not a label.

## The gating set after this feature

Nine existing jobs, unchanged:

| Job name | Filter | Budget |
|---|---|---|
| `sqlc generate is up to date` | `sqlc` | ~1 min |
| `tygo generate is up to date` | `tygo` | ~1 min |
| `shared types — no duplicates / no weakened fields / complete nullability` | `tygo` | ~1 min |
| `go vet` | `go` | ~2 min |
| `go test` | `go` | ~3 min |
| `lint (go)` | `go` | ~2 min |
| `integration test` | `go` | ~5 min |
| `frontend test (vitest)` | `web` | ~2 min |
| `frontend typecheck` | `web` | ~2 min |
| `lint (web)` | `web` | ~1 min |

> Note: § 2.1 of the domain doc currently lists nine rows and omits `shared types — …`,
> which the workflow does define and report. Ten jobs gate today, not nine. This feature
> corrects that table in the same change that adds to it.

Five new jobs:

| Job name | Filter | Budget | Fails when |
|---|---|---|---|
| `secret scan` | `any` | ~20 s | a recognised secret pattern appears in the pull request's commit range |
| `vulnerability scan (go)` | `go` | ~1 min | a reachable, non-excepted advisory affects a Go dependency |
| `vulnerability scan (web)` | `web` | ~1 min | a workspace dependency carries a non-ignored advisory at `high` or above |
| `build image (api)` | `go` | ~2 min warm / ~8 min cold | `apps/api/Dockerfile` fails to build |
| `build image (dashboard)` | `web` | ~2 min warm / ~4 min cold | `apps/dashboard/Dockerfile` fails to build |

`detect changed areas` remains scaffolding and must never appear in a ruleset.

## Naming rules these names follow

1. **Lowercase, space-separated, parenthesised qualifier.** Matches `lint (go)` /
   `lint (web)` and `frontend test (vitest)`.
2. **The subject first, the scope in parentheses.** `vulnerability scan (go)`, not
   `go vulnerability scan` — so related checks sort together in the pull-request list.
3. **No tool name in the check name.** `vulnerability scan (go)` rather than
   `govulncheck`, and `secret scan` rather than `gitleaks`. Replacing the tool must not
   force a ruleset edit; the existing `lint (go)` sets this precedent while running
   `golangci-lint`.
4. **No version, date, or feature number.**

## Ruleset consequence

`specs/domains/platform-operations.md` § 2.2 must list all fifteen names above in
`required_status_checks` before the ruleset is applied. Adding a job to the workflow
without adding its name there produces a check that runs and never gates — the exact
failure mode § 2.1 was written to prevent.

## Verification

- Every name in the table appears verbatim as a `name:` in `.github/workflows/api-ci.yml`.
- Every name in the table appears verbatim in the § 2.1 table and the § 2.2 ruleset list.
- No two jobs share a name.
