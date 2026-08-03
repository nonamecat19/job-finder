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
  (`scripts/hooks/guard-master.sh`) stops the agent earlier still.
  `specs/archive/023-workflow-quality-gates/contracts/required-checks.md` records the ruleset
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

`.github/workflows/e2e.yml` runs the Playwright suite on a schedule and on demand
(023-FR-019); failures are surfaced without polling (023-FR-020).

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
