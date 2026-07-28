---
title: Principles overview
sidebar_position: 1
description: The design rules this codebase actually follows, and where each one is enforced.
---

# Principles overview

This section states the rules the codebase plays by. Each principle is written as
*rule → why → where it shows up → when to break it*. Nothing here is aspirational: every
rule is one you can verify by reading the file it cites.

## The ten rules

```mermaid
mindmap
  root(("Job Finder principles"))
    Structure
      "One process, many goroutines"
      "Composition root owns wiring"
      "Ports and adapters"
    Data
      "SQL is the source of truth"
      "Generated code is committed"
      "Contracts mirror field for field"
    Behaviour
      "Degrade, never disappear"
      "Async by default"
      "Be a good web citizen"
    Discipline
      "Spec before code"
      "CI is the gate"
```

| # | Principle | Enforced in | Detail |
| --- | --- | --- | --- |
| 1 | One process, many goroutines | `cmd/server/main.go:41-56` | [Architectural](/principles/architectural-principles) |
| 2 | The composition root owns all wiring | `cmd/server/compose*.go` | [DI](/principles/dependency-injection) |
| 3 | Ports and adapters at every I/O edge | `internal/*/ports.go` | [DI](/principles/dependency-injection) |
| 4 | SQL is the source of truth; sqlc generates the access layer | `internal/db/queries/*.sql` | [Domain modeling](/principles/domain-modeling) |
| 5 | Generated code is committed and drift-checked | `scripts/sqlc-check.sh`, `scripts/tygo-check.sh` | [Coding conventions](/principles/coding-conventions) |
| 6 | Go DTOs and TypeScript types match field-for-field | `internal/dto`, `packages/shared/src/index.ts` | [Domain modeling](/principles/domain-modeling) |
| 7 | Every AI feature degrades gracefully | `internal/llm/router.go:79-90` | [Architectural](/principles/architectural-principles) |
| 8 | Slow work is a task, never an HTTP request | `internal/queue/queue.go:15-20` | [Architectural](/principles/architectural-principles) |
| 9 | Be a good web citizen when scraping | `internal/retrieval/*` | [Architectural](/principles/architectural-principles) |
| 10 | Errors are typed values mapped once at the edge | `internal/apperr` | [Error handling](/principles/error-handling) |

## How the principles reinforce each other

```mermaid
flowchart TD
    P4["SQL is the source of truth"] --> P5["Generated code is committed"]
    P5 --> P6["Contracts match field for field"]
    P6 --> CI["CI drift jobs fail the PR"]
    P3["Ports and adapters"] --> P2["Composition root owns wiring"]
    P2 --> P1["One process, many goroutines"]
    P3 --> P7["AI degrades gracefully"]
    P8["Async by default"] --> P9["Good web citizen"]
    P10["Typed errors"] --> P7
    CI --> SHIP["Merged"]
    P1 --> SHIP
```

The chain matters: because the schema generates the Go types, and the Go DTOs generate
the TypeScript types, a schema change that nobody propagated **cannot** reach `master` —
`sqlc-drift` and `tygo-drift` in `.github/workflows/api-ci.yml` fail first.

## Pages in this section

- [Architectural principles](/principles/architectural-principles) — process shape, async, degradation, scraping ethics
- [Domain modeling](/principles/domain-modeling) — three type layers and the rules for crossing them
- [Error handling](/principles/error-handling) — `apperr` kinds, retryable vs terminal, HTTP mapping
- [Dependency injection](/principles/dependency-injection) — constructor injection, ports, the composition root
- [Testing philosophy](/principles/testing-philosophy) — what is unit, what needs Postgres, what is a live smoke test
- [Coding conventions](/principles/coding-conventions) — naming, file layout, generated code, comments
