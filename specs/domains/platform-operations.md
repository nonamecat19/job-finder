# Domain: Platform Operations

Consolidates **023** enforced workflow quality gates, **007** CI test gate (superseded by
023), **008** health/readiness checks, **018** asynqmon queue monitoring, **026** DB
connection capacity.

Implementation: `.github/workflows/`, `.githooks/`, `Makefile`, `.claude/settings.json`,
`apps/api/internal/health/`, `internal/dbutil/`, `docker-compose*.yml`. How it works:
[`docs/operations/ci-cd.md`](../../docs/docs/operations/ci-cd.md),
[`docs/operations/testing.md`](../../docs/docs/operations/testing.md),
[`docs/async/monitoring.md`](../../docs/docs/async/monitoring.md).

The workflow rules an agent must **follow** are in `AGENTS.md`. This document records why
they exist and what they must guarantee.

---

## 1. Trunk protection (023-FR-001..006)

- 023-FR-001: the trunk rejects direct writes; every change arrives via a branch and a pull
  request. Enforcement is **local** — server-side branch protection is unavailable on the
  repository's current GitHub plan. `.githooks/pre-commit` and `.githooks/pre-push` reject a
  commit or push targeting `master`; a Claude Code `PreToolUse` hook
  (`scripts/hooks/guard-master.sh`) stops the agent earlier still. § 2.2 records the ruleset
  to apply the moment a paid plan makes it mechanical.
- 023-FR-004: when mechanical enforcement is adopted it must **not** require a second
  person's approval — the project has one maintainer and a review requirement would deadlock
  it.
- 023-FR-005: a documented override exists for emergency trunk repair, and using it leaves a
  visible trace. `--no-verify` bypasses both hooks; its use is visible in shell history and
  in the agent transcript. 023-SC-012 bounds it: the trunk can be restored from a broken
  state within one hour, so the gate never makes the repository unrecoverable.
- 023-FR-006: the repository's agent instructions state the branch-and-PR rule.

`make setup-hooks` sets `core.hooksPath` at repository-config level — one run covers every
worktree sharing the clone, but it is **not** automatic, and an unactivated hook is an absent
gate.

## 2. The declared check set (023-FR-002, 003, 014, 018, 021)

Every automated check runs on a pull request and reports before integration, so a failing
change is visibly red. The gating set is declared in **one** place, and adding or renaming
one is deliberate.

Current jobs — `.github/workflows/api-ci.yml`:

| Job | Checks |
|---|---|
| `sqlc-drift` | `sqlc generate` is up to date |
| `tygo-drift` | `tygo generate` is up to date |
| `shared-types-no-duplicates` | no duplicated / weakened / incompletely-nullable shared types (024) |
| `lint-go` | golangci-lint, version pinned in `apps/api/.golangci-version` |
| `lint-web` | ESLint, `eslint.config.js` |
| `go-vet` | `go vet ./...` |
| `go-test` | `go test ./...` — unit only; DB/Redis tests carry `//go:build integration` |
| `integration-test` | backend integration suite against real Postgres and Redis (023-FR-016) |
| `frontend-test` | Vitest |
| `frontend-typecheck` | `pnpm typecheck` |

> ### ⚠ 023-FR-019/020 are revoked — the e2e suite no longer runs in CI
>
> `.github/workflows/e2e.yml` ran the Playwright suite on a nightly schedule, on pull
> requests, on pushes to master and on demand. **It was deleted.** The suite itself is
> intact and runs locally through `make test-e2e`.
>
> Revoked with it: **023-FR-019** (the end-to-end suite runs at least daily and on demand),
> **023-FR-020** (failures surfaced without polling) and **023-SC-009**. The `e2e
> (playwright)` job leaves the gating set in § 2.1 — it must not be listed in the Phase 2
> ruleset (§ 2.2), because a required check that no workflow reports would sit at "Expected"
> forever and block every merge.
>
> **Nothing now exercises the dashboard end to end automatically.** `frontend-test` (Vitest)
> and `frontend-typecheck` remain, so component and type regressions are still caught; a
> break in routing, navigation or page rendering is not. That gap is accepted, not covered
> elsewhere.

**Path filtering.** Every job carries `needs: changes` and an `if:` on a
`dorny/paths-filter` output, so a change touching only documentation or repo workflow —
`specs/**`, `docs/**`, `.specify/**`, `.githooks/**`, `scripts/hooks/**`, `AGENTS.md`,
`README.md`, `Makefile`, `.github/workflows/**` — matches no filter and skips the entire
set. Two rules keep this from weakening the gate:

- **Never a top-level `on: paths:`.** A workflow skipped that way reports no checks at all,
  so every required check would sit at "Expected" forever and block the merge. A job
  skipped by `if:` reports `skipped`, which GitHub counts as passing. That distinction is
  the only reason the `changes` job exists.
- **Filters are pull-request-only.** Each output is
  `github.event_name != 'pull_request' || ...`, so pushes to master and
  `workflow_dispatch` always run everything unfiltered.

`Makefile` and `.github/workflows/**` were originally in every filter, on the correct
observation that either can change any job's verdict. They were removed because the cost
was disproportionate — a docs-only change adjusting a Makefile comment rebuilt Go and Node
for ~20 runner-minutes. The second rule is what makes that safe: such a change is still
fully exercised, on the merge commit rather than on the pull request.

- 023-FR-017: the integration check creates any database the suite needs and waits for
  readiness, so it never fails on a cold start.
- 023-FR-021: any check is re-runnable without modifying the change under test.
- 023-SC-003: a change with a style violation, a static-analysis violation, a failing unit
  test, a failing integration test, or stale generated output is red.
- 023-SC-008: cross-service behaviour is exercised against real infrastructure on every PR.

**007 is superseded by 023.** 007 added `go-vet`, `go-test`, `frontend-test` and
`frontend-typecheck` to a CI that had only the two drift checks, and explicitly deferred lint
gating (007-FR-008: "MUST NOT introduce golangci-lint or ESLint gating, since no
configuration exists"). 023-FR-007/008 authored those configurations and 023-FR-014 made them
gating. **007-FR-008 is void** — do not cite it as a reason not to add lint checks.
007-FR-005 (CI must provision Postgres/Redis for the Go test job) was also revised: unit tests
need no datastore, and the integration suite got its own job.

### 2.1 Job names are the contract

The `name:` value of each job is the identifier a GitHub ruleset lists as a required status
check. **Renaming a job silently drops it from that list** — the check keeps running and
stops gating. Any rename must update this section in the same change (023-FR-003).

| Job name | Trigger | Filter | Budget |
|---|---|---|---|
| `sqlc generate is up to date` | PR, push master | `sqlc` | ~1 min |
| `tygo generate is up to date` | PR, push master | `tygo` | ~1 min |
| `go vet` | PR, push master | `go` | ~2 min |
| `go test` | PR, push master | `go` | ~3 min |
| `frontend test (vitest)` | PR, push master | `web` | ~2 min |
| `frontend typecheck` | PR, push master | `web` | ~2 min |
| `shared types — no duplicates / no weakened fields / complete nullability` | PR, push master | `tygo` | ~1 min |
| `lint (go)` | PR, push master | `go` | ~2 min |
| `lint (web)` | PR, push master | `web` | ~1 min |
| `integration test` | PR, push master | `go` | ~5 min |
| `secret scan` | PR, push master | `any` | ~20 s |
| `vulnerability scan (go)` | PR, push master | `go` | ~1 min |
| `vulnerability scan (web)` | PR, push master | `web` | ~1 min |
| `build image (api)` | PR, push master | `go` | ~2 min warm / ~8 min cold |
| `build image (dashboard)` | PR, push master | `web` | ~1 min warm / ~4 min cold |

`e2e (playwright)` **was an entry and is gone** — see the notice in § 2. **Fifteen jobs
gate today.**

Two corrections to the count this table used to carry. It said "nine jobs" and omitted
`shared types — …`, which the workflow has defined and reported all along; ten gated
before 039. 039 then added the five supply-chain and build-integrity gates, bringing the
total to fifteen.

The five 039 gates follow the naming rules the rest of the table already uses: subject
first, scope in parentheses, and **no tool name in the check name**. `vulnerability scan
(go)` runs govulncheck and `secret scan` runs gitleaks, but neither says so — replacing
the tool must not force a ruleset edit, exactly as `lint (go)` does not say
`golangci-lint`.

`secret scan` is the only job on the `any` filter (`- '**'`). It is the one gate whose
verdict any file can change: a provider key pasted into a `specs/` markdown file is still
a provider key, and unlike a broken build it cannot be undone by a follow-up commit —
every clone and fork keeps a copy, so the only remedy is rotation. The ~20-second cost is
far below the ~20 runner-minutes that motivated the filtering design in the first place.

The two image jobs introduce **no new filter**. `apps/api/Dockerfile` COPYs only
`apps/api/**`, already the `go` anchor's first entry; `apps/dashboard/Dockerfile` COPYs
`pnpm-workspace.yaml`, `package.json`, `pnpm-lock.yaml`, `tsconfig.base.json`,
`packages/shared` and `apps/dashboard`, every one already in `web`. The `go` anchor also
carries `gateway/**`, so a gateway-only change rebuilds the API image for nothing. That is
the right way round: a false run costs two minutes, a false skip lets a broken image merge.

The set runs in parallel against a ≤10 minute wall-clock target. **`detect changed
areas` — the `changes` job — is deliberately not in the gating set** and must never be
listed in a ruleset: it is scaffolding, and requiring it would defeat the point, since the
filtered jobs already depend on it.

Two non-obvious filter memberships:

- `tygo` watches **both** `apps/api/**` and `packages/shared/src/generated.ts`, because
  `scripts/tygo-check.sh` reads Go DTOs and writes that TypeScript file.
- `web` does **not** need to watch `apps/api/internal/dto`. The tygo output is committed
  under `packages/shared/`, so a *regenerated* DTO change already matches `web` — and an
  *unregenerated* one is exactly what `tygo generate is up to date` exists to catch.

`lint (go)` reads its version from `apps/api/.golangci-version` exactly as the drift jobs read
their pins, and uses `golangci-lint-action` rather than `make lint-go` **only** for result
caching — the config and pin are identical, so the verdicts match.

`integration test` runs Postgres as **`pgvector/pgvector:pg16`, not stock postgres**, because
the schema has `vector(768)` columns, with `POSTGRES_DB: jobfinder_test` so no separate
`createdb` step is needed. Its run is two steps, and **the ordering is load-bearing**:

```bash
go test -tags integration ./internal/db/... -run TestMigrate   # 1. create the schema
go test -tags integration ./...                                # 2. everything else
```

`go test ./...` runs packages in parallel and the other integration-tagged packages assume an
existing schema, so a single combined invocation can start a dependent package before
migration finishes. The advisory lock in `internal/dbtest/lock.go` serialises `TRUNCATE`
between suites but does nothing for schema ordering. CI needs no
`COMPOSE_PROJECT_NAME`/`POSTGRES_HOST_PORT` handling — those exist to stop local worktrees
colliding on one host, and a runner is a fresh container.

The deleted `e2e (playwright)` job needed **no** database, Redis or Go backend: the specs
mock every API call through `page.route('**/api/…')` or assert headings and URLs only, and
`playwright.config.ts` starts the dev server via `webServer` with
`reuseExistingServer: !process.env.CI`. Recorded because it makes the job cheap to restore —
reinstating it is a workflow file, not infrastructure.

Re-runnability (023-FR-021) holds because no job depends on mutable external state: service
containers are fresh per run and no job contacts a live job board.

### 2.2 The ruleset to apply on upgrade

Not applicable today — `gh api repos/…/rulesets` returns `403 Upgrade to GitHub Pro or make
this repository public`. Recorded so the upgrade is one command rather than a rediscovery:

```bash
gh api -X POST repos/nonamecat19/job-finder/rulesets \
  -f name='master protection' -f target=branch -f enforcement=active \
  -f 'conditions[ref_name][include][]=refs/heads/master' \
  -f 'rules[][type]=deletion' \
  -f 'rules[][type]=non_fast_forward' \
  -f 'rules[][type]=pull_request' \
  -f 'rules[][type]=required_status_checks'
```

with `required_status_checks` listing exactly the fifteen job names in § 2.1 — **not** `e2e
(playwright)`, which no workflow reports any more — and
`pull_request.required_approving_review_count: 0` (023-FR-004 — one maintainer, so requiring
approval would deadlock the repository). Self-merge after green checks is the intended flow.

Until then 023-FR-002 is met as written: checks run and report before integration. What is
deferred — explicitly, within FR-002 itself — is the *host* refusing the merge button.

## 3. Local quality command (023-FR-007..015)

`make test-lint` = `lint-go` + `lint-web` + `test-go` + `test-react`, failing if any of the
four fails (023-FR-009).

- 023-FR-010: generated files are excluded from hand-written-code rules.
- 023-FR-011: each violation is reported with file, line and rule.
- 023-FR-012: adoption did not require clearing the entire pre-existing violation backlog in
  one change; the rule set was scoped so the gate could go green immediately.
- 023-FR-015: tools are version-pinned (`apps/api/.golangci-version`,
  `apps/api/.sqlc-version`) so a local run and a CI run reach the same verdict (023-SC-005: an
  author who sees local success is not surprised by CI).
- 023-FR-013 / 023-SC-010: the agent instructions must describe what the command covers
  **accurately**, and must not claim coverage for languages absent from the repository. There
  is no Python here and `test-lint` never checked any.
- 023-SC-004: violations reported within 60 seconds locally.

`make test-integration` and `make test-e2e` are separate targets — they need containers or a
browser — and are deliberately **not** part of `test-lint`.

**The coverage invariant**: `test-lint` must cover the union of every required CI check that
does not need infrastructure **and whose verdict depends only on the tree**. A check added
to CI without a matching local target breaks 023-SC-005 (≥95% local/CI agreement) and
authors stop trusting the local run.

### 3.1 The supply-chain gates and why they sit outside `test-lint` (039)

The second clause of the invariant above was added by 039 rather than quietly violated.
Four new targets exist, grouped under one alias:

```
make vuln-go    # scripts/govulncheck-check.sh
make vuln-web   # pnpm audit --audit-level=high --prod=false
make secrets    # gitleaks git . --redact --config .gitleaks.toml
make audit      # the three above, first non-zero wins (same shape as `make lint`)
make images     # docker build of both images, standalone
```

None is part of `test-lint`, and that is a decision, not an omission:

1. **They are not deterministic in time.** An advisory published this afternoon turns this
   morning's green run red with no code change. The property 023-SC-005 protects — that a
   passing local run predicts a passing CI run — was never on offer here, so folding them
   in would weaken the invariant rather than honour it.
2. **They need the network, and `images` needs Docker.** That is the same exemption
   `test-integration` and `test-e2e` already hold.

`make images` additionally costs 6–8 minutes cold, which would make the pre-push loop
slower than CI and push people to skip it entirely.

`make secrets` is the closest call — it is fast, offline and deterministic, and it would
fit `test-lint` on the letter of the invariant. It stays with the other three so that
"audit-class gate" is one coherent group with one entry point, rather than one gate hiding
inside `test-lint` and three outside it.

### 3.2 Responding to a failing supply-chain gate (039)

| Gate | What it means | What to do |
|---|---|---|
| `secret scan` | A credential shape appears in this pull request's commit range. | **Rotate the credential first** — it is in a pushed branch, so treat it as public. Then remove it from the change. Only if it is genuinely a false positive, add an allowlist entry to `.gitleaks.toml` with a comment giving the reason. |
| `vulnerability scan (go)` | A Go dependency has an advisory **reachable** from this module's code. | `cd apps/api && go get <module>@<fixed-version> && go mod tidy`. If no fix exists, add an expiring entry to `apps/api/.govulncheck-ignore`. |
| `vulnerability scan (web)` | A workspace dependency has an advisory at `high` or above. | Bump it. If no fix exists, add the id to `auditConfig.ignoreCves` in `pnpm-workspace.yaml` **and** a row in the table below. |
| `build image (api)` / `build image (dashboard)` | The Dockerfile no longer builds. | Fix the Dockerfile. **There is no suppression mechanism and there should not be** — a vulnerability or a secret match can be a false positive or an unfixable upstream fact, but an image that does not build is never either. |

**The Go exception file.** `apps/api/.govulncheck-ignore` takes one entry per line:

```
GO-2026-1234  2026-10-01  No fixed version upstream; reachable only from <path>
```

The expiry is the point. An expired entry **fails** the gate rather than lapsing into
silence, so the only way to keep an exception is to renew it in a reviewed diff. A
malformed line and an entry for an advisory that no longer appears both fail too — a dead
entry reads as live risk, and a silently skipped bad line means somebody believes an
advisory is suppressed when it is not. `scripts/govulncheck-check.sh --self-test` proves
all six parser failure modes without needing a real advisory.

**Reachability, not presence.** `govulncheck` fails the build only when the vulnerable
symbol is actually called from this module. Unreachable findings are printed and pass. At
the time 039 shipped there were three of them (klauspost/compress, x/crypto's unmaintained
openpgp package, x/net's dnsmessage) — real advisories against real dependencies that no
code path reaches. Failing on those would have made the gate noise inside a week.

**The web severity floor is `high`**, declared explicitly in the workflow rather than left
to a tool default. `moderate` was considered and rejected: the dashboard is a self-hosted
single-user front end with no untrusted multi-tenant input, and nearly every `moderate`
finding in a Vite/React devDependency tree is a build-time-only ReDoS advisory that a
deployed static bundle cannot reach. Starting at `high` keeps the gate credible. It can be
tightened later without a design change.

**Web advisory exceptions** — `auditConfig.ignoreCves` in `pnpm-workspace.yaml` (pnpm 11
no longer reads the `pnpm` field from `package.json` and warns that `auditConfig` there is
ignored). The array ships empty. Every id added to it needs a row here:

| Advisory | Package | Fixed version | Reason | Review by |
|---|---|---|---|---|
| *(none)* | | | | |

**The one-time full-history secret audit** (039-FR-016) ran at 484 commits and found
**15 findings, all false positives — no job-finder credential has ever been committed, so
nothing required rotation.** They were: the default `linkedin-client-id` rule matching the
`linkedinUrl` column alias in `contact.sql` and its sqlc output (8); `gcp-api-key` matching
Google Maps keys belonging to the *scraped sites* in the work.ua HTML parser fixtures (3);
`generic-api-key` on the `nodeFixtureKey` cross-language crypto test vector (2) and on a
`tokens:` struct field in the rephrase path (1); and a `Bearer <placeholder>` in a deleted
spec's example curl (1). Each has a narrow allowlist entry in `.gitleaks.toml` with the
reason inline.

Worth recording: `nodeFixtureKey` is a genuine 64-hex value and did **not** trip the
project's own `job-finder-config-encryption-key` rule, because that rule binds to the
`CONFIG_ENCRYPTION_KEY` variable name rather than matching bare hex. That binding is why
the rule does not also match every commit hash and checksum in the tree.

**What Dependabot cannot cover.** `.github/dependabot.yml` watches the Go module, the pnpm
workspace, the workflow's actions, and both Dockerfiles' base images. It does **not** read
compose files, so the images pinned in `docker-compose.yml` and `docker-compose.prod.yml`
— `pgvector/pgvector:pg16`, `redis:7-alpine`, `minio/minio:latest`,
`ghcr.io/flaresolverr/flaresolverr:latest`, the ClickHouse image and the Langfuse images —
stay manual. Two of those are pinned to `:latest`, which is a separate problem.

Its `gomod` entry points at `/apps/api`, not `/`. There is no root `go.mod`, and
**Dependabot reports no error when it finds no manifest at the configured directory** — a
wrong path produces silence, not a failure, so the misconfiguration stays invisible until
someone notices no Go updates have ever arrived.

`make lint-go` runs `scripts/golangci-check.sh` (the version guard) then `golangci-lint run`
over `apps/api` with `apps/api/.golangci.yml`, reporting `file:line: message (linter-name)`.
Its version-mismatch message mirrors `scripts/sqlc-check.sh` verbatim in structure — pinned
version, installed version, why it matters, install line.

`make lint-web` runs ESLint over `apps/dashboard` and `packages/shared` with the root flat
config, ignoring `**/dist/**`, `packages/shared/src/generated.ts` and `node_modules`. **A
missing `node_modules` fails with that instruction rather than silently passing.**

`make lint` = `lint-go` then `lint-web`, first non-zero wins. `make setup-hooks` sets
`core.hooksPath` and is idempotent.

**Nothing else may invoke a linter binary directly.** The constitution names `make` targets
as the canonical entry point precisely so the four callers cannot drift:

| Caller | Calls |
|---|---|
| Author, by hand | `make test-lint` before opening a PR; `make audit` when touching dependencies |
| `Stop` hook | `lint-go`/`test-go` and/or `lint-web`/`test-react`, scoped to changed paths |
| `PostToolUse` hooks | `make sqlc-generate`, `make tygo-generate` |
| CI | `make lint-go`, `make lint-web`, the two-step integration sequence |

023 also removed `package.json`'s `"test:python": "make test-python"` — the target did not
exist, so the script failed on invocation — along with the matching false claim in
`AGENTS.md`. That is 023-SC-010 in practice.

## 4. Automatic actions (023-FR-022..030)

Committed `.claude/settings.json` hooks, so they apply to every clone and worktree with no
per-user setup (023-FR-028):

| Trigger | Action |
|---|---|
| Edit `apps/api/internal/db/queries/*.sql` | Regenerate the typed DB layer (023-FR-022) |
| Edit `apps/api/internal/dto/*.go` | Regenerate shared type definitions (023-FR-023) |
| Edit Go source | `gofmt` + `go vet` on the affected package (023-FR-024) |
| Session end | Run the quality verification for whichever suites the session touched; a failure **blocks** the session from being reported complete (023-FR-025) |

Constraints that make them safe:

- 023-FR-026: results land **visibly in the working tree** for review. Nothing is rewritten
  at commit time.
- 023-FR-027: a missing tool fails with an actionable message naming the prerequisite, and
  leaves nothing half-done.
- 023-FR-029: actions are scoped to the files actually edited — editing one file never
  triggers repository-wide work.
- 023-FR-030: actions never trigger recursively on the files they themselves generate.
- 023-SC-011: session-end verification adds no more than 2 minutes, so it does not get
  disabled for being slow.

### 4.1 The hook scripts

**Every hook body is a standalone script under `scripts/hooks/`**; `.claude/settings.json`
only wires events to scripts. That is deliberate — if the agent-hook schema shifts in a future
version, breakage is detectable by running the script directly. Each is
`#!/usr/bin/env bash` with `set -euo pipefail`, checks for its tool first and **exits non-zero
with an install line if absent** (a missing tool must never read as a pass), is idempotent and
hand-runnable, and never writes outside the repository.

**Layer 1 — git hooks**, activated by `make setup-hooks`. `pre-commit` reads the current
branch and exits 1 on `master`, printing the branch-and-PR rule and the branch-creation
command. `pre-push` reads the `<local ref> <local sha> <remote ref> <remote sha>` lines on
stdin and exits 1 if any pushed ref is `refs/heads/master`. **Both gate destination only and
never inspect content**, and both must be no-ops on any other branch. `--no-verify` is the
023-FR-005 override for either.

**Layer 2 — agent hooks.** Four facts about the hooks API shaped the design: matchers filter
on **tool name only** (path filtering uses the per-entry `if` field); `PostToolUse` **cannot
block** — exit 2 is explicitly non-blocking for that event; `Stop` **can** block, via exit 2
or `{"decision":"block","reason":"…"}` on stdout; and hooks read a JSON object on stdin
carrying `tool_input.file_path`, `tool_input.command`, `cwd` and `session_id`.

| Hook | Bound to | Behaviour |
|---|---|---|
| `guard-master.sh` | `PreToolUse` on `Bash(git commit*)`, `Bash(git push*)` | Exit 2 on master — **blocks the tool call**, with stderr fed back to the agent: "On master. Create a branch first: `git checkout -b <nnn>-<slug>`" |
| `go-postedit.sh` | `PostToolUse` on `Edit(apps/api/**/*.go)` | `gofmt -w <file>`, `go vet ./<package>`. Always exit 0; reports through `hookSpecificOutput.additionalContext`. Scoped to the file's package, never the repository |
| `regen-sqlc.sh` | `PostToolUse` on `Edit(apps/api/internal/db/queries/*.sql)` | `make sqlc-generate`, refreshing `internal/db/sqlcgen/` in the working tree for review |
| `regen-tygo.sh` | `PostToolUse` on `Edit(apps/api/internal/dto/*.go)` | `make tygo-generate`, refreshing `packages/shared/src/generated.ts` |
| `session-verify.sh` | `Stop` (no matcher support — fires every time) | Scopes off `git diff --name-only`: Go paths → `make lint-go test-go`; dashboard/shared paths → `make lint-web test-react`; neither → immediate exit 0. Exit 2 **blocks the stop** |

> **`guard-master.sh` reads the branch of the checkout the command writes to** — a `git -C
> <dir>` or `cd <dir>` inside the command itself, else `cwd`, else `$CLAUDE_PROJECT_DIR` — not
> the session's branch. Agents routinely commit from a worktree while `$CLAUDE_PROJECT_DIR`
> still points at a main checkout sitting on master. Reading only the project dir gets it wrong
> in **both** directions: blocking commits bound for a feature branch, and passing commits that
> really do land on master whenever the session itself runs from a worktree.

> **`session-verify.sh` blocks at most once per `session_id`.** A blocking `Stop` re-enters the
> agent loop, which can end in another `Stop`. The script records a marker under the system
> temporary directory; a second consecutive failure reports without blocking, so **a session
> can always terminate.**

**No recursion (023-FR-030)**: regeneration writes to `internal/db/sqlcgen/` and
`packages/shared/src/generated.ts`, and neither path matches any hook's `if` filter, so a
regeneration cannot trigger another. **Re-verify this whenever a filter is widened.**

Only `.claude/settings.json` — the hook registry — is committed. `.claude/settings.local.json`
holds the maintainer's permission allowlist and is not.

## 5. Health and readiness (008)

| # | Requirement |
|---|---|
| 008-FR-001 | A **liveness** endpoint returning success whenever the process is running, independent of dependency state. |
| 008-FR-002 | A **readiness** endpoint checking Postgres, Redis and MinIO, reporting per-dependency status. |
| 008-FR-003 | Readiness returns a non-success HTTP status when any checked dependency is unreachable. |
| 008-FR-004 | Each dependency check is bounded by a timeout, so a hung dependency cannot hang the endpoint. |
| 008-FR-006 | `docker-compose.prod.yml` defines an api `healthcheck` using the readiness endpoint. |
| 008-FR-007 | The compose files define an ollama healthcheck using ollama's own status signal. |
| 008-FR-008 | The compose files define a minio healthcheck using minio's built-in health endpoint. |
| 008-FR-010 | New healthchecks follow the existing Postgres convention: 5 s interval, 3 s timeout, 10 retries. |
| 008-FR-011 | Services with startup-order dependencies use `depends_on` with `condition: service_healthy`. |

The operator-facing bar (008-SC-003): the root cause of a broken stack is identifiable from
`docker compose ps` health status alone, without reading logs. 008-SC-004: restarting one
unhealthy dependency lets dependents return to healthy with no manual intervention.

026-FR-007 adds to the same readiness report: the DB pool's configured capacity, connections
in use, and connections idle.

### 5.1 The endpoints

`GET /api/health` — liveness. Always `200 {"ok": true, "uptime": 1234.5}` while the process is
running and accepting connections.

`GET /api/health/ready` — readiness. Checks Postgres, Redis and MinIO with a **2 s
per-dependency timeout**. `200` when everything checked is ok, `503` otherwise:

```json
{
  "ok": false,
  "checks": {
    "postgres": {"status": "error", "error": "…", "latency_ms": 2001},
    "redis":    {"status": "ok", "latency_ms": 1},
    "minio":    {"status": "ok", "latency_ms": 8}
  },
  "pool": {
    "max_conns": 25, "acquired_conns": 4, "idle_conns": 6, "total_conns": 10,
    "empty_acquire_count": 0, "acquire_duration_ms": 0, "saturated": false
  }
}
```

`minio` reports `{"status": "disabled"}` with no `error` or `latency_ms` when `MINIO_ENDPOINT`
is unset, and **that does not affect the overall `ok`.**

Pool field semantics (026-FR-007): `max_conns` is the configured capacity after derivation;
`acquired_conns`, `idle_conns` and `total_conns` are gauges; `saturated` is
`acquired_conns >= max_conns` **at the instant of the request**. `empty_acquire_count` and
`acquire_duration_ms` are **cumulative counters since process start, not rates** — a single
reading cannot say whether the system is *currently* struggling, but two readings can. That is
a stated limitation; proper rate treatment belongs with a metrics system, which is what
026-FR-008 deferred.

Two decisions worth not re-litigating:

- **`ok` is not affected by pool saturation.** A saturated pool is a capacity signal, not an
  unreadiness signal — it is still serving requests, and flipping `ok` to false would take the
  process out of rotation for a load condition, making the load worse. Only a failing `Ping`
  sets `ok: false`.
- **When the pool dependency is nil, the `pool` key is omitted entirely** rather than emitted
  with zeroes. A zero-valued block is indistinguishable from a genuinely idle pool and would
  misreport `max_conns: 0`.

The handler's pool dependency is a separate interface from the existing `Pinger`:

```go
// Implemented by the Postgres pool only. Redis and MinIO have no equivalent,
// so this is deliberately not folded into Pinger.
type PoolStatter interface { PoolStats() db.PoolStats }
```

**Compose healthchecks** (008-FR-006..008, 008-FR-010 — 5 s interval, 3 s timeout, 10
retries):

| Service | `test` | Succeeds when |
|---|---|---|
| `api` | `curl -f http://localhost:<PORT>/api/health/ready` | curl exits 0, i.e. HTTP 2xx, i.e. readiness `ok: true` |
| `ollama` | `ollama list` | The CLI can reach the local API server |
| `minio` | a `/dev/tcp` probe of `/minio/health/live` | The first HTTP response line contains `200` |
| `postgres` | `pg_isready -U jobfinder` | Native Postgres readiness |

## 6. Queue monitoring (018)

Asynqmon, dev-only, at `http://localhost:8090`.

- 018-FR-001: lists all six queues — `ingest`, `match`, `generate`, `enrich`, `salary:infer`,
  `ghost:score`.
- 018-FR-002: per-queue live counts by state — pending, active, scheduled, retry, archived,
  completed.
- 018-FR-003: drill into a queue to list tasks with type, payload, state and, for failures,
  the error.
- 018-FR-004: retry, archive and delete, individually or in bulk.
- 018-FR-005: recent historical daily processed/failed counts per queue.
- 018-FR-006/007: its own port, distinct from the API, started with the rest of the dev stack
  (018-SC-004: one extra service, zero extra configuration).
- 018-FR-008: **local/dev network only — never exposed in production.** It is absent from
  `docker-compose.prod.yml`, and that is a security boundary, not an oversight. It offers
  unauthenticated task deletion.

Bars: any queue's pending/active/failed count readable within 5 seconds of opening it
(018-SC-001); a failed task's error found and retried in under 30 seconds without a script
(018-SC-002); state changes visible within 5 seconds (018-SC-003).

## 7. Database connection capacity (026)

The pool is sized explicitly, because the default silently sized it below what the workers
demand.

| # | Requirement |
|---|---|
| 026-FR-001 | Maximum pool size comes from explicit configuration, never the driver default. |
| 026-FR-002 | Minimum retained connections, maximum connection lifetime and maximum idle time are all configurable with documented defaults. |
| 026-FR-003 | The default maximum is **at least** the sum of all background worker pool sizes at their defaults, plus a reserve for interactive requests. |
| 026-FR-004 | Startup validates that configured concurrency cannot demand more connections than configured capacity allows. |
| 026-FR-013 | That validation accounts for the **highest concurrency reachable through runtime settings changes**, not only the values present at startup. |
| 026-FR-005 | Invalid capacity — zero, negative, or below the process minimum — is rejected at startup, naming the offending setting (026-SC-003). |
| 026-FR-006, 026-FR-008a | An interactive request competing with background work either obtains a connection or fails within a **bounded, configurable** wait, with an error identifying the cause. |
| 026-FR-009 | Sustained full saturation is logged, distinguishing capacity exhaustion from slow queries. |
| 026-FR-010 | Connections exceeding lifetime or idle limits are retired, never handed to a caller stale. |
| 026-FR-011 | After the database goes away and returns, full capacity is restored **without a process restart** (026-SC-006: within 60 seconds). |
| 026-FR-012 | Configuration documentation lists every setting with its default and effect. |

Bars: with every worker pool fully occupied, interactive requests complete within 150% of
their idle-system time (026-SC-001) and **zero** fail for want of a connection (026-SC-002).
The shipped defaults start cleanly with no capacity warning at any processor count from one
upward (026-SC-005).

**026-FR-008 is deferred by design** — exposing waiter counts and wait durations as
operational metrics was explicitly descoped, with no task raised. It is a known gap, not an
oversight; the readiness report (026-FR-007) is the interim answer.

### 7.1 The configuration keys

Seven keys, all optional, following the existing convention: `mapstructure` tag equal to the
env var name, default registered in `internal/config/defaults.go`, documented in
`apps/api/.env.example`.

| Key | Default | Effect |
|---|---|---|
| `DB_MAX_CONNS` | `0` (derive) | Maximum connections. `0` derives from worker concurrency + reserve — **25 at shipped defaults** |
| `DB_MIN_CONNS` | `2` | Kept open when idle, avoiding reconnect cost on the first request after quiet |
| `DB_MAX_CONN_LIFETIME` | `1h` | Maximum age before retirement |
| `DB_MAX_CONN_IDLE_TIME` | `30m` | Maximum idle before retirement — this is what retires connections silently dropped by intermediaries |
| `DB_ACQUIRE_TIMEOUT` | `5s` | How long an interactive request waits for a free connection before failing with a capacity error instead of hanging |
| `DB_SERVER_MAX_CONNS` | `100` | What the database server permits, **as declared by the operator — validated against, never queried** |
| `DB_INTERACTIVE_RESERVE` | `8` | Connections budgeted above total worker concurrency for interactive traffic |

> `DB_INTERACTIVE_RESERVE = 8` is **provisional and not derived from a latency model.**
> 026-SC-001 is what decides whether 8 is right: if the measured loaded/idle latency ratio
> exceeds 1.5, raise it and re-measure.

Derivation is `(sum of worker pool sizes) + 2 background + DB_INTERACTIVE_RESERVE`.

**Validation** (026-FR-004/005). Every message names the offending key and is prefixed
`config:` to match existing `config.Load` errors. Hard failures: any of the seven at or below
its floor; `DB_MIN_CONNS` exceeding the effective maximum; and — the load-bearing one — an
explicit `DB_MAX_CONNS` below what the workload requires:

```
config: DB_MAX_CONNS=%d is below the %d connections required by worker concurrency
(workers=%d background=2 reserve=%d). Raise DB_MAX_CONNS, or lower
AI_CONCURRENCY_CLOUD / INGEST_CONCURRENCY / ENRICH_CONCURRENCY.
```

Two conditions **warn** rather than fail: an effective maximum above `DB_SERVER_MAX_CONNS`
("connections may be refused under load"), and `DB_MAX_CONN_IDLE_TIME` exceeding
`DB_MAX_CONN_LIFETIME` ("idle retirement will never trigger").

On successful validation, exactly one info line makes the effective policy visible without
reading configuration:

```
level=INFO msg="db pool configured" max_conns=25 derived=true workers=15 background=2 \
  reserve=8 min_conns=2 lifetime=1h idle=30m acquire_timeout=5s
```

`derived=false` when `DB_MAX_CONNS` was set explicitly.

**On hosts with more than 25 cores, capacity went down.** The previous incidental default was
`max(4, NumCPU)`, unrelated to the workload; the derived 25 covers the whole workload plus
reserve. This was resolved deliberately in favour of the derived value, and `DB_MAX_CONNS`
exists for operators who disagree. An existing `.env` keeps working unchanged.

## 8. Measurable bars

**Quality gates (023)**

- 023-SC-001: zero changes reach the trunk without passing the declared check set — enforced
  by the local gate plus the maintainer declining to integrate red, **not by the host.**
- 023-SC-002: 100% of direct-to-trunk write attempts are rejected by the local gate, excluding
  deliberate use of the documented override.
- 023-SC-004: a style or static-analysis violation is reported within 60 seconds locally, so
  the gate is fast enough to run habitually rather than avoided.
- 023-SC-005: local and CI verdicts agree in ≥95% of cases.
- 023-SC-006: generated DB code and generated shared types are **never** stale relative to
  their sources in any change reaching the trunk.
- 023-SC-007: an agent editing a query definition or a DTO gets the generated output refreshed
  **without being told to**, in 100% of such edits.
- 023-SC-008: cross-service behaviour is exercised against real infrastructure on every change
  request — against zero automated runs before 023.
- 023-SC-012: the trunk can be restored from a broken state within an hour using the override,
  so the gate never makes the repository unrecoverable.

## 9. LLM observability collector (036)

A self-hosted Langfuse group — `langfuse-web`, `langfuse-worker` and `clickhouse` — records every
AI call the gateway routes. It reuses the platform's Postgres (separate logical database), Redis and
MinIO; ClickHouse is the one genuinely new stateful service.

The wiring, the callback contract and the coverage table live in
[`llm-routing.md` §7](llm-routing.md). This section is the operational half: what it holds, how to
stand it up, and what the retention promise actually is.

### 9.1 What it stores — read this before turning it on

**It stores prompt and response bodies.** For this platform that means the user's master profile:
legal name, employers, dates, achievements, contact details — in plain text, in a service with its
own database, its own UI and its own login.

Consequences that follow, and are enforced rather than advised:

- The operator UI is bound to **loopback only** (`127.0.0.1:3100`), in development as much as in
  production. Asserted by `TestCollectorUIIsBoundToLoopback`.
- Signup is **disabled** (`AUTH_DISABLE_SIGNUP=true`). An instance that accepts new accounts from
  anyone who can reach it is not inside any trust boundary worth the name. Enable signup only for
  the one-time creation of the first account, then disable it again.
- Telemetry is **off**. Nothing about this deployment leaves it.
- `LANGFUSE_HOST` is a **container-network** address (`http://langfuse-web:3000`). The published
  loopback port is for host-side verification only — the same trap `GATEWAY_URL` documents.
- The application container receives **no** `LANGFUSE_*` variable. Those live in the `litellm`
  container. Asserted by `TestApiContainerHoldsNoProviderCredentials`.

### 9.2 One-time setup

The collector's metadata needs its own logical database on the existing Postgres server — a separate
database, never a shared schema, and **never managed by goose**. Langfuse runs its own migrations,
and entangling them with the platform's sequential goose versions is a mess that only becomes
visible at the worst moment.

Creating it is a manual step, on purpose:

```bash
docker compose exec -T postgres createdb -U jobfinder langfuse
```

**An `docker-entrypoint-initdb.d` script will not do this for you.** The `pgdata` volume already
exists on every deployment that has ever run, and Postgres executes init scripts only on first
initialisation. A script added now would run on exactly the machines that do not need it.

Then set `LANGFUSE_DB_URL`, `LANGFUSE_CLICKHOUSE_URL`, `LANGFUSE_REDIS_URL`, the
`LANGFUSE_S3_*` group, `NEXTAUTH_URL`, `NEXTAUTH_SECRET` and `SALT` (see `.env.example`, where every
variable states what leaving it empty costs), bring the group up, create the first account, disable
signup again, and copy the project's API keys into `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` for
the `litellm` container.

### 9.3 Retention — enforced, not configured

Records older than `EVAL_PRUNE_RETENTION_DAYS` (default **30**) are deleted by a pruning job the
platform owns, running on the existing scheduler tick
(`internal/platform/observability`, driven from `jobsources/interfaces/worker/scheduler.go`).

It is a job rather than a setting because **automated retention is not available to an OSS
self-host** — the collector will keep everything forever unless something deletes it. Each run logs
what it deleted and a failure is logged as a failure, because a retention guarantee nobody can check
is how a window quietly stops being enforced while the documentation still claims it.

The job needs `EVAL_PRUNE_COLLECTOR_URL`, `EVAL_PRUNE_PUBLIC_KEY` and `EVAL_PRUNE_SECRET_KEY`.
These are deliberately **not** named `LANGFUSE_*`: the application must not be granted the
collector's ingestion variables, and this is a separate grant — a key so the platform can delete its
own records — that can be revoked on its own. It is still a real grant, and leaving these empty
while collection is on means **nothing is ever deleted**.

**Deleting a job or resetting a profile in the platform does not propagate to the collector.** The
prompt bodies of past runs survive in it until the window expires. There is no cascade, and building
one would mean the collector holding a foreign key into the platform's data. If a specific record
must go now, delete it in the collector's own UI.

### 9.4 The three standing questions (036-FR-012)

All three are answered in the collector's UI without running a benchmark, and all three depend on
`generation_name` being set to the task key — grouping by the `model` field instead groups by
serving deployment and collapses two stages that share a model into one bucket.

| Question | How it is answered |
|---|---|
| **What does a task cost over a period?** | Sum cost, grouped by `generation_name`, filtered to the date range. Read a *missing* cost as unpriced, never as free — an unpriced deployment yields no cost rather than zero. |
| **Where does the time go?** | Latency distribution (p50/p95) grouped by `generation_name`. Per-task, not per-model: `generation-summary` and `generation-select-premium` share a model and must stay separate buckets. |
| **How often is a task served by a fallback?** | Compare `metadata.model_group` (what was asked for) with `model` (what answered); the rate at which they differ is the non-primary-tier rate. **This needs no additional recorded field** — the two values are already on every record, which is why no degradation flag was added. |

### 9.5 Adding a new AI call path

Any change that adds one arrives with a coverage decision in the same change, recorded in
[`llm-routing.md` §7.3](llm-routing.md). A path added without one is a defect against 036-FR-013,
not a follow-up: "is this recorded?" is unanswerable after the fact without reading every call site.
