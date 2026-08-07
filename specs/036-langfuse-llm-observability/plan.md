# Implementation Plan: Self-Hosted LLM Observability

**Branch**: `036-langfuse-llm-observability` | **Date**: 2026-08-07 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/036-langfuse-llm-observability/spec.md`

## Summary

Every AI call in this platform already passes through one LiteLLM proxy that knows the requested task
key, the model that actually served it, the token counts and the cost. None of that is retained. Cost
per resume is known only because someone ran a benchmark by hand on 2026-08-07.

Point the proxy's success and failure callbacks at a self-hosted Langfuse v4 stack. That is US1 and it
costs zero lines of Go: `gateway/config.yaml` gains a callback list, the compose files gain a service
group, `.env.example` gains variables.

US2 — grouping one resume run's calls into one trace, *and* making reporting group by task rather
than by serving model — is where the application must speak. The proxy cannot know which calls belong
together, and the collector records the served deployment rather than the requested key, so both need
request metadata: `existing_trace_id` for correlation, `generation_name`/`tags` for the task key.

**This plan was revised after an audit.** Four premises were wrong, and the corrections changed the
shape of the work rather than its wording:

- The collector groups by **served deployment**, not requested task key — so per-task reporting is
  not free and no longer sits in US1's config-only scope (R6).
- Automated retention is **enterprise-only**; an OSS self-host prunes for itself, so FR-008 now
  carries a job and tests that did not exist (R7).
- The US2 call sites are **six free functions in `rendercv_llm.go`** plus one in `service.go`, none
  holding an `*activity.Recorder` — a signature change, not a threaded field (R5).
- `docker-compose.prod.yml` has **no `litellm` service**, and its `api` service ingests the whole
  `.env`, which already exposes every provider key.

Two things still gate the design: the collector must never be able to slow or fail an inference call,
and no prompt body may leave the deployment.

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`); no dashboard change

**Primary Dependencies**: LiteLLM proxy (`ghcr.io/berriai/litellm:main-stable`) for callbacks;
Langfuse v2 container for collection; PostgreSQL 16 for the collector's own store

**Storage**: Langfuse v4's own stack — ClickHouse (new), plus Redis and S3-compatible blob storage,
both of which the deployment already runs (Redis, MinIO), plus a separate logical Postgres database
for its metadata. The platform's own schema, `sqlc` models and goose migrations are untouched.
ClickHouse is the one genuinely new stateful service and its cost is accepted deliberately
(research R2).

**Testing**: `go test` for the metadata threading (US2) and for the "unconfigured collector changes
nothing" guarantee; the collector itself is verified through quickstart scenarios, including a
deliberate dead-collector run

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Deployment/configuration change plus a small typed addition to the LLM port

**Performance Goals**: < 50 ms median added latency per AI call; zero added latency when the
collector is unreachable

**Constraints**: no prompt or completion content leaves the deployment; collector credentials stay in
the collector and proxy containers' environment and are unreadable from the application; the platform
must start, run and pass its suites with observability entirely unconfigured

**Scale/Scope**: single-user deployment, on the order of hundreds of AI calls per day — the reason
Langfuse v2's single container beats v3's ClickHouse stack (research R2)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Assessment |
|---|---|
| **I. No Auto-Apply** | Untouched. Observability adds no outbound path to any employer and no submission capability. **PASS** |
| **II. Grounded Generation** | Indirectly strengthened. Retained prompt/completion pairs are what make a grounding complaint investigable after the fact instead of unreproducible. No generation behaviour changes. **PASS** |
| **III. Typed Contracts** | US2 adds one field to `CompleteOptions`, a Go-internal type. Nothing crosses to TypeScript, so no tygo regeneration and no hand-duplicated type. **PASS** |
| **IV. Test Discipline** | Go tests cover the metadata threading and the unconfigured path; the collector's behaviour under failure is a quickstart scenario because it needs a real dead container, which a unit test cannot express. Only `apps/api` changes, so `go test` is the gate; `make test-lint` is not required by the boundary rule but is cheap and listed in Polish. **PASS** |
| **V. Local-First** | Self-hosted, in-stack, no third-party egress (FR-005), inference unaffected when absent (FR-004, FR-015). **But two qualifications the first pass missed.** (a) Principle V's credential clause — "provider credentials MUST stay in the gateway container's environment and MUST NOT be readable through the application" — is *already violated*: `docker-compose.prod.yml:27` gives the `api` service `env_file: .env`, exposing every provider key. FR-007 now requires fixing that channel rather than adding a second secret to it. (b) Co-locating collector storage with the platform's own datastores means a healthy collector filling the disk can stop inference — a failure mode the FR-004 gate does not test. **CONDITIONAL PASS**: contingent on FR-007's channel fix and on the collector having bounded, separately-provisioned storage. |

**Post-Phase-1 re-check**: the design now adds a service group (web, worker, ClickHouse), two
`CompleteOptions` fields, seven call sites, and a pruning job. That is materially more than the
"one container and one field" the first pass claimed — the growth came entirely from checking
premises rather than from scope creep, and each addition traces to a specific corrected assumption
(R2, R6, R7). Complexity Tracking below records the two that warrant justification.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| New stateful service (ClickHouse) for a few hundred calls/day | v4 requires it, and v4 is the only version that is current and supported (research R2) | v2 is one container but two majors behind, and could not satisfy FR-008 or FR-012 anyway — the reason this decision was redone |
| A pruning job the platform owns and tests | The collector's automated retention is enterprise-only; OSS self-hosts prune themselves (research R7) | Configuring a retention window — the original plan — is not available at any version. Dropping the window leaves the user's employment history accumulating indefinitely |

## Project Structure

### Documentation (this feature)

```text
specs/036-langfuse-llm-observability/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── contracts.md     # Phase 1 output
├── checklists/
│   └── requirements.md
└── spec.md
```

### Source Code (repository root)

```text
docker-compose.yml                       # + langfuse-web, langfuse-worker, clickhouse
docker-compose.prod.yml                  # NOTE: has NO litellm service today. Adding the
                                         #   collector to prod requires adding the gateway first,
                                         #   or a commented decision that prod has no gateway.
                                         #   Also: `api` uses `env_file: .env` (see FR-007).
gateway/config.yaml                      # + litellm_settings.success_callback / failure_callback
.env.example                             # + LANGFUSE_* / CLICKHOUSE_* variables, empty defaults

apps/api/internal/platform/llm/
├── domain/port.go                       # US2: + CompleteOptions.TraceID and TaskKey metadata
└── infrastructure/gateway/gateway.go    # US2: emit existing_trace_id + generation_name + tags
                                         #   NOTE: usageFrom already reads x-litellm-model-group,
                                         #   x-litellm-response-cost, x-litellm-attempted-fallbacks

apps/api/internal/generation/application/
├── rendercv_llm.go                      # US2: SIX call sites — analyzeVacancy :68,
│                                        #   selectContent :299, writeSummary :352,
│                                        #   structure re-tailor :372, expandContent :386,
│                                        #   condenseContent :476. All are free functions with
│                                        #   no *activity.Recorder in scope: signature change.
└── service.go                           # US2: the cover-letter call, seventh site

apps/api/internal/matching/application/
└── service.go                           # US2: the match path

apps/api/internal/platform/llm/
└── gateway_config_test.go               # ALREADY EXISTS — extend, do not create

apps/api/internal/<pruning>/             # FR-008: scheduled prune, on the existing robfig/cron

specs/domains/
├── llm-routing.md                       # § observability: coverage, retention, retune procedure
└── platform-operations.md               # collector as an operated service
```

**Structure Decision**: existing web-service layout. No new app, no migration in the platform's own
schema. The collector is a peer service group in the Compose stack. The application changes are: two
optional metadata fields on the LLM port, seven call sites that set them, and a pruning job — the
last of which exists only because the collector's own retention is not available to an OSS
self-host (research R7).

## Phase 0: Research

See [research.md](./research.md). Seven decisions, revised 2026-08-07 after audit; its corrections log
records what was wrong. The load-bearing ones:

- **R1/R2**: self-hosted Langfuse **v4**, never the hosted SaaS (Principle V, and prompts carry the
  user's PII). ClickHouse is accepted as the one genuinely new service; v2 was rejected on age and on
  being unable to meet the requirements that had been resting on it.
- **R3**: proxy callbacks are what make US1 config-only, and they define the coverage gap FR-013 must
  document. Embeddings bypass the proxy **unconditionally** (`router.go:71-73`), not conditionally.
- **R4**: the dead-collector scenario is a required test, not an assumption — precedent: 030's
  `request_timeout`, documented as enforced, observed not to be, at the cost of an 830-second hang.
  Verified support: the integration dispatches callbacks on an executor, off the request path.
- **R5**: correlation uses **`existing_trace_id`**; plain `trace_id` overwrites the trace on every
  call. Seven call sites, six of them free functions.
- **R6**: per-task grouping must be **created** via `generation_name`/`tags`. The original claim that
  it came free was false and inverted the reporting design.
- **R7**: 30-day retention enforced by a **pruning job this project owns**. The configuration setting
  the first pass specified does not exist for OSS self-hosts at any version.

## Phase 1: Design

- [data-model.md](./data-model.md) — the call record and run trace as the collector stores them, the
  `CompleteOptions` addition, and the field-by-field mapping from proxy response to record.
- [contracts/contracts.md](./contracts/contracts.md) — the callback configuration contract, the
  environment contract, the metadata field on the outbound chat request, and the coverage statement.
- [quickstart.md](./quickstart.md) — runnable validation: bring the collector up, exercise every task
  key, force a fallback, kill the collector mid-run, and run the whole stack with it unconfigured.
