# Implementation Plan: Resume Generation Strictness & Model Improvement

**Branch**: `033-resume-gen-strictness` | **Date**: 2026-08-07 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/033-resume-gen-strictness/spec.md`

## Summary

Close the strictness gaps in AI resume generation at three layers: (1) the verifier — extend
`VerifyRendercvGrounding` to enforce skill tokens and highlight word-overlap at the default
(moderate) grounding level on the primary tailoring pass, and run
`DropUngroundedSkillTokens` after the primary merge, not only after page-fitting; (2) the
prompt — remove stale references to removed struct fields and upgrade the structured-output
mode from `json_object` to `json_schema` with `strict: true` for the `generation` task,
constraining the model at the API level; (3) the model — re-evaluate every model in the
generation chain against the strictness rules and pick the primary from data, not from a
general-quality hunch. Technical approach is in [research.md](research.md); data/contracts in
[data-model.md](data-model.md) and [contracts/contracts.md](contracts/contracts.md).

## Technical Context

**Language/Version**: Go 1.24 (`apps/api`), TypeScript 5.x (`apps/dashboard` — not touched by
this feature).

**Primary Dependencies**: `github.com/invopop/jsonschema` (schema generation, already used in
`port.go`), `hibiken/asynq` (workers, unchanged), LiteLLM proxy (gateway, config-time
verification of `json_schema` support).

**Storage**: Postgres + pgvector — **no schema changes**. The feature is in-memory validation,
prompt construction, and request shaping.

**Testing**: `go test` for unit tests (`apps/api/internal/generation/domain`,
`apps/api/internal/platform/llm`), `vitest` for the dashboard (unchanged). `make test-lint`
is the merge gate. Integration tests are not strictly required — the strictness checks are
pure functions over in-memory documents.

**Target Platform**: Linux server (Docker Compose stack). No platform change.

**Project Type**: web-service (Go backend + React frontend). This feature is backend-only.

**Performance Goals**: The strictness checks add ≤10% to median tailoring run time (SC-006);
median local-model run still under 60s (020-SC-007). The checks are deterministic, no extra LLM
round-trips beyond the existing grounding loop.

**Constraints**: Constitution V — the chain still terminates at local Ollama; no new
third-party dependency. 030-C5 — every model in the generation chain must support
`json_schema` strict mode, verified at config time.

**Scale/Scope**: ~5 Go files edited (grounding, structure, prompt, gateway adapter, service
loop), ~3 new test files, 1 gateway config edit (primary model), 1 benchmark fixture. No
dashboard changes, no DTO changes, no migrations.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| I. No Auto-Apply | ✅ Pass | Tailoring produces a local draft; no submission path is touched. |
| II. Grounded Generation | ✅ **Strengthened** | This feature *is* the strengthening: closes the moderate-level skill-token and highlight-drift gaps. |
| III. Typed Contracts | ✅ Pass | `chatRequest.ResponseFormat` type change is Go-internal. `TailoredSections` struct unchanged. No tygo regeneration. |
| IV. Test Discipline | ✅ Pass | New grounding checks get Go unit tests; the benchmark is a Go test fixture. `make test-lint` is the gate. |
| V. Local-First, Self-Hosted | ✅ Pass | The chain still terminates at local Ollama. The model re-evaluation chooses among existing chain entries; no new third-party API. |

No violations. No Complexity Tracking entries needed.

## Project Structure

### Documentation (this feature)

```text
specs/033-resume-gen-strictness/
├── plan.md              # This file
├── research.md          # Phase 0 — R1-R7, decisions and rationale
├── data-model.md        # Phase 1 — extended entities, new in-memory types, state transitions
├── quickstart.md        # Phase 1 — 7 validation scenarios
├── contracts/
│   └── contracts.md     # Phase 1 — C1-C10, internal + external contracts
└── tasks.md             # Phase 2 output (/speckit-tasks command - NOT created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── generation/
│   │   ├── application/
│   │   │   ├── rendercv_llm.go        # buildSelectPrompt cleanup, MaxTokens, ResponseMode
│   │   │   └── service.go             # grounding loop: DropUngroundedSkillTokens on primary pass,
│   │   │                              #   extended VerifyRendercvGrounding call, highlight-drift strip
│   │   └── domain/
│   │       ├── rendercv.go            # (no struct change) DropUngroundedSkillTokens already exists
│   │       ├── rendercv_grounding.go  # VerifyRendercvGrounding extended (all levels), adjacency
│   │       └── rendercv_structure.go   # StripUngroundedHighlights (new), mirrors StripStructureViolations
│   └── platform/llm/
│       ├── domain/
│       │   └── port.go                # CompleteOptions.ResponseMode (new), ResponseMode enum
│       └── infrastructure/gateway/
│           └── gateway.go             # chatRequest.ResponseFormat upgraded to struct,
│                                      #   CompleteJSON builds json_schema for strict mode
└── internal/generation/domain/*_test.go   # unit tests for all new/extended checks

gateway/
└── config.yaml                        # generation primary model (post-benchmark), verified json_schema support
```

**Structure Decision**: Backend-only change inside the existing feature-module layout
(`internal/generation/{application,domain}` + `internal/platform/llm/{domain,infrastructure/gateway}`).
No new packages, no new HTTP handlers, no dashboard changes. Matches the codebase-structure
domain rule (handlers stay in feature packages; `internal/httpapi` is router-only).

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. Table is empty.