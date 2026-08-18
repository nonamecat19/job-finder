---
title: Testing philosophy
sidebar_position: 6
description: What is a unit test, what needs Postgres, what is an integration or live smoke test, and what CI actually gates.
---

# Testing philosophy

## Rule: the default test needs nothing but Go

Tests live next to the code (`foo.go` / `foo_test.go`) and the vast majority run with
plain `go test ./...` — no database, no Redis, no network. That is possible only because
services depend on ports they declared themselves
([DI](/principles/dependency-injection)), so a fake is a struct with the six methods the
service actually calls.

```mermaid
flowchart TD
    U["Unit tests: go test ./..."] --> CI1["CI job: go-test"]
    D["DB-backed tests: internal/dbtest"] --> LOC["make test-integration"]
    I["Integration tests: -tags integration"] --> LOC
    L["Live smoke tests: library adapters/live, internal/*/application/live_test.go"] --> MAN["Manual, opt-in"]
    F["Frontend: vitest"] --> CI2["CI job: frontend-test"]
    E["E2E: playwright"] --> LOC2["make test-e2e"]
```

## The layers

| Layer | Location | Needs | Run by |
| --- | --- | --- | --- |
| Unit | `internal/**/ *_test.go` | nothing | `make test-go`, CI `go-test` |
| Repository / DB | helpers in `internal/dbtest` | a Docker daemon (Postgres container) | `make test-integration` |
| Integration | files behind `//go:build integration` | a Docker daemon (Postgres, RabbitMQ, ClickHouse, LiteLLM containers) | `make test-integration` |
| Live smoke | `adapters/live/live_test.go` (library), `internal/*/application/live_test.go` | real network + credentials | manual |
| Frontend unit | `apps/dashboard/**/*.test.ts(x)` | jsdom | `make test-react`, CI `frontend-test` |
| E2E | `apps/dashboard/tests` (Playwright) | full stack up | `make test-e2e` |

:::warning Live tests are opt-in on purpose
The library's `adapters/live/live_test.go` hits real job boards. It exists so an adapter break is diagnosable,
not to run on every commit — CI never executes it.
:::

## Rule: adapters are tested against captured fixtures

Every adapter under `internal/jobsources/adapters` has a sibling `_test.go`, and the
package carries a `testdata/` directory of recorded responses. A parser regression is
caught deterministically and offline; the live test is only the tripwire for "the site
changed its HTML".

```mermaid
sequenceDiagram
    participant T as adapter_test.go
    participant FX as testdata fixture
    participant A as Adapter
    participant N as NormalizedJob
    T->>FX: load recorded HTML/JSON
    T->>A: Parse(fixture)
    A-->>N: normalized jobs
    T->>N: assert title, company, url, dedupeKey
```

## Rule: test the boundary you own

- **HTTP handlers** are tested through the router with `httptest` — see
  `internal/*/interfaces/http/*_test.go` (for example `jobsources/interfaces/http/sources_test.go`) —
  so route wiring, decoding and status mapping are covered together.
- **Policies and middleware** get table tests: `internal/queue/policy_test.go` asserts
  that invalid concurrency and liveness settings are rejected at startup.
- **Routing logic** gets its own test: `internal/llm/router_test.go` covers the
  fallback-to-Ollama path when a credential is absent.

## Rule: startup validation is a test you get for free

Because `PoliciesFromConfig` validates at boot (`internal/queue/policy.go:96-120`), a bad
`AI_CONCURRENCY_LOCAL` fails immediately with a named message rather than producing a
subtly wrong worker pool. Prefer moving a class of bug into startup validation over
writing a test that asserts the buggy behaviour is absent.

## Rule: CI is the gate, and it gates generated code too

`.github/workflows/api-ci.yml` runs six independent jobs on every PR:

```mermaid
flowchart LR
    PR["Pull request"] --> A["sqlc-drift"]
    PR --> B["tygo-drift"]
    PR --> C["go-vet"]
    PR --> D["go-test"]
    PR --> E["frontend-test"]
    PR --> F["frontend-typecheck"]
    A --> M{"all green?"}
    B --> M
    C --> M
    D --> M
    E --> M
    F --> M
    M -->|yes| MERGE["mergeable"]
    M -->|no| FIX["fix and push"]
```

`sqlc-drift` and `tygo-drift` re-run the generators with the pinned versions from
`apps/api/.sqlc-version` / `.tygo-version` and diff the result. Forgetting
`make sqlc-generate` is a red build, not a mystery at runtime.

## Writing a new test — the decision

```mermaid
flowchart TD
    Q1{"Does it touch SQL you wrote?"} -->|yes| DBT["dbtest + real Postgres"]
    Q1 -->|no| Q2{"Does it parse a third-party payload?"}
    Q2 -->|yes| FIX2["fixture in testdata + unit test"]
    Q2 -->|no| Q3{"Does it cross an HTTP boundary you own?"}
    Q3 -->|yes| HT["httptest against the router"]
    Q3 -->|no| UT["plain unit test with a fake port"]
```
