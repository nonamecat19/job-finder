---
title: DTOs and contracts
sidebar_position: 3
description: internal/dto, tygo generation, the hand-maintained shared index, and how contract drift is caught.
---

# DTOs and contracts

## The contract chain

```mermaid
flowchart LR
    DOM["domain types"] -->|handler mapping| DTO["internal/dto/*.go"]
    DTO -->|tygo generate| GEN["packages/shared/src/generated.ts"]
    DTO -.->|hand-mirrored| IDX["packages/shared/src/index.ts"]
    GEN --> BUILD["pnpm --filter @job-finder/shared build"]
    IDX --> BUILD
    BUILD --> DIST["shared/dist"]
    DIST --> DASH["apps/dashboard imports @job-finder/shared"]
```

Two paths leave `internal/dto`, and only one is automatic. That asymmetry is the single
most common source of contract bugs in this repo, so it gets its own rule in `AGENTS.md`:

> Go DTO field names/JSON tags in `apps/api/internal/dto` must match
> `packages/shared/src/index.ts` field-for-field — `index.ts` is hand-maintained (not
> auto-imported from the tygo-generated `generated.ts`), so update both when adding a DTO
> field.

## What lives in `internal/dto`

| File | Covers |
| --- | --- |
| `dto.go` | core shared types |
| `jobs.go` | job list/detail payloads |
| `profiles.go`, `resume.go` | profile and resume documents |
| `documents.go` | generated documents |
| `activity.go`, `queue_backlog.go` | activity runs and queue stats |
| `settings.go` | LLM and AI-feature settings |
| `analysis.go`, `intel.go`, `postage.go` | scoring, company intel, analytics |

## tygo configuration

```yaml
# apps/api/tygo.yaml
packages:
  - path: "github.com/job-finder/api/internal/dto"
    output_path: "../../packages/shared/src/generated.ts"
    type_mappings:
      "time.Time": "string"
```

One mapping matters: `time.Time` becomes `string`, because the wire format is JSON and the
dashboard parses timestamps itself (`apps/dashboard/src/lib/time.ts`).

Regenerate with:

```bash
cd apps/api && tygo generate     # or: just tygo-generate
```

## The hand-maintained side

`packages/shared/src/index.ts` declares the enums and interfaces the dashboard imports —
`APPLICATION_STATUSES`, `ENTRY_TYPES`, and narrowing aliases over the generated shapes.
It is where const-tuple enums live, which Go cannot express and tygo therefore cannot
generate:

```ts
export const APPLICATION_STATUSES = [
  'found', 'shortlisted', 'docs_generated', 'applied',
  'interview', 'offer', 'rejected',
] as const;
export type ApplicationStatus = (typeof APPLICATION_STATUSES)[number];
```

Those strings must match the Go status vocabulary (`internal/domain/job.go`).

## Drift detection

```mermaid
sequenceDiagram
    participant Dev
    participant CI as GitHub Actions
    participant T as tygo (pinned version)
    participant G as git diff
    Dev->>CI: push
    CI->>CI: read apps/api/.tygo-version
    CI->>T: go install tygo@version
    CI->>T: ./scripts/tygo-check.sh
    T->>G: regenerate and diff generated.ts
    alt diff is empty
        G-->>CI: pass
    else diff is non-empty
        G-->>CI: fail — run just tygo-generate and commit
    end
```

The same pattern guards SQL: `sqlc-check.sh` with `apps/api/.sqlc-version`. Both jobs are
in `.github/workflows/api-ci.yml`.

:::warning What CI cannot catch
`index.ts` is not generated, so **nothing** verifies it against the Go DTOs. A missing
field there surfaces as `undefined` at runtime. When you add a DTO field, edit both files
in the same commit.
:::

## Adding a field — the checklist

```mermaid
flowchart TD
    A["Add field to internal/dto struct with a JSON tag"] --> B["Map it in the handler"]
    B --> C["just tygo-generate"]
    C --> D["Mirror it in packages/shared/src/index.ts"]
    D --> E["pnpm --filter @job-finder/shared build"]
    E --> F["Use it in the dashboard"]
    F --> G["Commit generated.ts alongside the source change"]
```

## Domain vs DTO

The `Job` domain type (`internal/domain/job.go:5-35`) carries fields the wire never sees —
`Raw []byte`, `Embedding []float32`, `EmbeddingHash *string`. Keeping them out of the DTO
is deliberate: a 768-dimension vector has no business in a feed response, and the raw
provider payload is for reprocessing, not for the client.

| Concern | Domain | DTO |
| --- | --- | --- |
| Embeddings and hashes | yes | no |
| Raw provider payload | yes | no |
| Nullable pointers | yes | yes, as optional JSON |
| Derived display fields | no | yes |
