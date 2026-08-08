# Implementation Plan: User-Selectable Summary Model

**Branch**: `034-resume-model-choice` | **Date**: 2026-08-08 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/034-resume-model-choice/spec.md`, **rescoped** per the
standing note at the top of that file.

## Summary

The spec asked for a user-facing choice of "the model that writes my resume". Between the spec being
written and this plan, 035 split resume generation into five stages served by different models, so
"the" generation model no longer exists. Planning the spec as written would produce a settings panel
with five dropdowns nobody wants.

The choice that carries real quality and real cost is **the summary stage**, and only that. The
mechanical stages — analyze and select — are where 035 measured the economy model performing as well
as the premium one at a fraction of the price; exposing them offers the user a way to spend more
money for no improvement. The summary is the part 035 found the economy model actually failing at,
and it is the part a human reads first.

So: a curated four-option choice over the summary stage, persisted as the user's default, applied to
one run, and recorded on the result.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`), TypeScript/React (`apps/dashboard`)

**Primary Dependencies**: none added.

**Storage**: one new singleton settings row, migration `00039`. Modelled exactly on
`ResumeShapeSetting` (00034) — same singleton guard, same seeded default.

**Testing**: `go test` for the catalogue, the service, the resolution seam and the HTTP surface;
vitest for the selector. The 038 eval gate covers the generation path this feature touches.

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Full-stack vertical slice (migration → service → HTTP → DTO → dashboard)

**Constraints**: a user who never opens the selector must get byte-identical behaviour to today
(spec AC3); the chain must still terminate at `local` (Constitution V); no run may fail because an
option's task key is unconfigured.

## Constitution Check

| Principle | Assessment |
|---|---|
| **I. No Auto-Apply** | Untouched — this changes which model writes a summary, not what is sent anywhere. **PASS** |
| **II. Grounded Generation** | Unaffected by construction: the option changes *which* provider serves the summary stage, not the grounding checks applied to its output. Every option's output goes through the same `VerifyRendercvGrounding` and the same summary-substitution path. A cheaper option that fabricates more is caught by the same machinery, and 038's corpus is what would measure that. **PASS** |
| **III. Typed Contracts** | New DTO crosses to TypeScript, so `make tygo-generate` is required and `packages/shared/src/generated.ts` changes. **PASS with an action** (T014). |
| **IV. Test Discipline** | Catalogue, service, resolution and handler tested; the selector tested in vitest. **PASS** |
| **V. Local-First** | The self-hosted option is always offered and always selectable. Every hosted option's task key gets a fallback chain terminating at `local`, asserted by the existing `gateway_config_test.go` invariants once the keys join `requestedGenerationGroups`. An option whose key is absent from the gateway resolves to local rather than failing. **PASS** |

## Project Structure

```text
apps/api/
├── internal/db/migrations/00039_summary_model_setting.sql   # NEW
├── internal/db/queries/summarymodelsetting.sql              # NEW
├── internal/generation/domain/summary_option.go             # NEW: the catalogue
├── internal/generation/application/service.go               # summary option resolution
├── internal/summarymodel/service.go                         # NEW: settings service
├── internal/summarymodel/interfaces/http/handler.go         # NEW
├── internal/dto/                                            # NEW DTOs
└── cmd/server/compose.go                                    # wiring: one router per option
gateway/config.yaml                                          # two new task keys + chains
apps/dashboard/src/                                          # the selector + result badge
```

**Structure Decision**: mirror `resumeshape` exactly. It is the same shape of problem — a singleton
user setting the generation pipeline reads through a narrow port — and it already solved the two
things that matter: the pipeline resolves the value **once at the top of a run**, so a settings
change mid-run cannot alter the document being generated; and the port is defined in the generation
package so that package never imports the settings package.

## Key Decisions

**D1 — Four options, three hosted plus self-hosted.** `standard` (the current default, task key
`generation-summary`, unchanged), `premium`, `fast`, and `local`. Spec AC1 asks for 3-5 plus the
self-hosted option; four total is inside that and each one is a choice a user can actually reason
about. More options is not more value here — it is a longer menu over the same three price points.

**D2 — `standard` maps to the existing `generation-summary` key, untouched.** This is what makes
AC3 true by construction rather than by testing: a user who never discovers the selector sends the
same task key to the same chain as today. If `standard` had been given a new key, AC3 would depend
on the two chains being kept in sync forever.

**D3 — The catalogue is Go, not a database table.** An option is a task key plus prose describing
what it is good at. The task key must exist in `gateway/config.yaml`, which is a deployment artifact
reviewed in code — so a database row could name a key the gateway has never heard of and the
mismatch would surface as a runtime fallback nobody noticed. Only the *choice* is persisted; the
menu is code, and the config test asserts every option's key is a declared, chained group.

**D4 — A run carries the choice; choosing also persists it.** Spec AC2 wants the run to use what was
picked, AC4 wants the pick remembered. One field on the tailoring request, applied to that run and
written through to the setting, satisfies both without a separate "save" action.

**D5 — An unconfigured option degrades to local, it does not fail.** With no gateway configured
every router already resolves to the local model — that is the pre-split behaviour and Constitution
V's whole point. An option is a routing preference, and a preference that can fail a run is a
liability.

## Complexity Tracking

| Decision | Cost accepted | Alternative rejected |
|---|---|---|
| **A new settings table for one column** | A migration, two queries, a service and a handler to store a single enum value. | *Reusing `ResumeShapeSetting`*: it is the resume's **shape** — lines, bullets, pages. A model choice is not shape, and the resume-shape DTO already crosses to TypeScript with a settled meaning. Widening it would make one row mean two unrelated things and one endpoint reset both. |
| **Per-option routers built at wiring time** | `compose.go` constructs one `Router` per option rather than one for the stage, and the generation service holds a map instead of a single provider. | *Constructing a router per run from the option*: puts gateway/local plumbing inside the generation service, which is the layering `GenerationRouters` exists to avoid. |
