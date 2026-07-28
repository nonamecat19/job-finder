---
title: Coding conventions
sidebar_position: 7
description: File layout, naming, comment style, generated code, and the spec-driven workflow this repo follows.
---

# Coding conventions

## Package layout

`apps/api/internal/<domain>` — one package per business capability, flat, named after the
thing rather than the layer. There is no `services/`, `handlers/`, `models/` split.

```mermaid
flowchart TD
    subgraph pkg["internal/matching (typical package shape)"]
        S["service.go — the use case"]
        P["ports.go — outbound interfaces"]
        R["repository.go — the adapter"]
        H["handler.go — asynq task handler"]
        T["*_test.go — colocated tests"]
    end
    HTTP["internal/httpapi/*.go"] --> S
    S --> P
    R -.implements.-> P
    H --> S
```

Recurring filenames and what they always mean:

| File | Meaning |
| --- | --- |
| `ports.go` | interfaces this package needs from the outside |
| `service.go` | the use case, no transport concerns |
| `handler.go` | an asynq task handler (`ProcessTask`) |
| `repository.go` | the persistence adapter |
| `<name>_test.go` | tests for `<name>.go`, same package |

## Comment style: comments explain *why*, and cite the spec

The prevailing style is a doc comment that records the decision, not the mechanics. From
`internal/queue/queue.go:22-37`:

> These are deliberately separate asynq *queues* … A single asynq.Server's `Queues` map
> only controls priority weighting *within* one shared worker pool, not a hard per-queue
> concurrency ceiling, so each task type still needs its own server.

Comments frequently cite the spec that produced the behaviour — `019-ai-job-throughput`,
`001-cerebras-model-toggle`, `FR-008`, `research.md R3`. Keep that habit: it is how the
`specs/` tree stays connected to the code.

## Naming

- Exported constants for wire values are grouped and prefixed by kind: `TypeIngest`,
  `QueueIngest`, `RungDirect`, `TaskProviderOllama`.
- Sentinel errors are `Err<Condition>` and are compared with `errors.Is`, never by string.
- Payload structs are `<Task>Payload` with JSON tags in camelCase to match the TS side.
- Config fields carry `mapstructure:"UPPER_SNAKE"` tags matching the environment variable
  exactly (`internal/config/config.go`).

## Generated code is committed and never hand-edited

```mermaid
flowchart LR
    SQL["internal/db/queries/*.sql"] -->|sqlc| GEN["internal/db/sqlcgen"]
    MIG["internal/db/migrations/*.sql"] -->|sqlc| GEN
    DTO["internal/dto/*.go"] -->|tygo| TS["packages/shared/src/generated.ts"]
    GEN --> COMMIT["git commit"]
    TS --> COMMIT
    COMMIT --> CIJ["CI: sqlc-check.sh / tygo-check.sh"]
```

Rules:

1. Edit the `.sql` or the DTO, then run `make sqlc-generate` / `make tygo-generate`.
2. Commit the regenerated output in the same commit as its source.
3. Tool versions are pinned in `apps/api/.sqlc-version` and `apps/api/.tygo-version` so
   local and CI emit byte-identical code.
4. `packages/shared/src/index.ts` is **hand-maintained** and must be updated alongside —
   it does not re-export `generated.ts` (`AGENTS.md`).

## Adding an HTTP handler

Handlers expose a `Mount(r chi.Router)` method and are registered as variadic mounts in
`buildServers` (`cmd/server/servers.go:66-73`) — never by editing `router.go`.

```go
// internal/httpapi/hosts.go
func (h *HostsHandler) Mount(r chi.Router) {
    r.Get("/hosts/{host}/retrieval-status", h.retrievalStatus)
    r.Post("/hosts/{host}/clear-cookies", h.clearCookies)
}
```

`NewRouter` mounts every handler under both `/api` and `/api/v1`
(`internal/httpapi/router.go:38-39`), so paths inside a `Mount` must **not** carry a
version prefix of their own.

## The spec-driven workflow

```mermaid
flowchart LR
    IDEA["Idea"] --> SPEC["specs/NNN-name/spec.md"]
    SPEC --> PLAN["plan.md + research.md"]
    PLAN --> TASKS["tasks.md"]
    TASKS --> CODE["Implementation"]
    CODE --> CHECK["checklists/"]
    CHECK --> PR["PR with FR citations in comments"]
```

Feature directories are numbered (`specs/001-cerebras-model-toggle` …
`specs/019-ai-job-throughput`) and carry `spec.md`, `plan.md`, `tasks.md`, plus
`checklists/` and `contracts/` where relevant. Tooling lives in `.specify/`.

## Commits

From `AGENTS.md`: conventional commits (`feat:`, `fix:`, `chore:`, `docs:`), created after
a completed feature or refactor, containing only files you changed.

## Frontend conventions

- Feature-sliced: `apps/dashboard/src/features/<slice>` owns its pages and components.
- All server access goes through `src/lib/api.ts`; query keys are centralised in
  `src/lib/queryKeys.ts`.
- Shared primitives live in `src/components/ui.tsx`; layout in `src/components/layout`.
- Types come from `@job-finder/shared`, never redeclared locally.
