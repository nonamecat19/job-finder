<!--
Sync Impact Report
==================
Version change: [TEMPLATE] → 1.0.0 (initial ratification)
Modified principles: n/a (first fill of template)
Added sections:
  - Core Principles (I–V, all newly authored)
  - Technology & Architecture Constraints
  - Development Workflow & Quality Gates
  - Governance
Removed sections: none
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ Constitution Check gate is generic ("Gates determined based on constitution file"); no principle-specific references, no change needed
  - .specify/templates/spec-template.md ✅ no principle-specific references found, no change needed
  - .specify/templates/tasks-template.md ✅ no principle-specific references found, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ generic, no agent-specific renames needed
Follow-up TODOs: none
Notes:
  - 2026-07-23: constitution scaffolding installed at repo root (.specify/ previously existed only in agent worktrees). RATIFICATION_DATE resolved to the initial-authoring date of v1.0.0 (matches Last Amended for the first version).

--------------------------------------------------------------------------

Version change: 1.0.0 → 1.0.1 (PATCH — wording correction, no principle
  added, removed, or redefined)
Modified principles: none. Development Workflow & Quality Gates corrected:
  "Design/plan docs ... are written under `plan/`" → written at
  `specs/<nnn>-<slug>/plan.md`. That directory never existed in this
  repository; the correction brings the document into line with actual
  practice (feature 024-agent-context-consolidation, FR-015/FR-017).
Added sections: none
Removed sections: none
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ re-checked, no `plan/`-directory reference, no change needed
  - .specify/templates/spec-template.md ✅ re-checked, no change needed
  - .specify/templates/tasks-template.md ✅ re-checked, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ re-checked, no `plan/`-directory reference found, no change needed
Follow-up TODOs: none

--------------------------------------------------------------------------

Version change: 1.0.1 → 1.0.2 (PATCH — factual corrections, no principle
  added, removed, or redefined)
Modified principles: none.
  - Technology & Architecture Constraints, data layer: "Redis/BullMQ-class
    queueing" → asynq on Redis, named directly. BullMQ was the pre-Go stack;
    the Go port uses hibiken/asynq with one queue per task type
    (internal/queue/queue.go). The old wording described a stack that is not
    in the tree.
  - Principle V, external-inference wording: clarified that hosted inference
    reached through the LiteLLM gateway is permitted so long as every task
    chain terminates at local Ollama, matching what
    030-litellm-model-routing shipped (FR-008/FR-009). The guarantee being
    protected — the system stays fully operational with no third-party AI
    call — is unchanged; only the description of how it is enforced.
  - Development Workflow & Quality Gates: `specs/<nnn>-<slug>/plan.md`
    remains where a plan is written during a feature, with a note that the
    plan is removed once the feature ships and its durable requirements are
    folded into specs/domains/. See specs/README.md.
Added sections: none
Removed sections: none
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ re-checked, no queue-technology or
    provider reference, no change needed
  - .specify/templates/spec-template.md ✅ re-checked, no change needed
  - .specify/templates/tasks-template.md ✅ re-checked, no change needed
  - .specify/templates/checklist-template.md ✅ re-checked, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ re-checked, no queue-technology or
    provider reference found, no change needed
Follow-up TODOs: none

--------------------------------------------------------------------------

Version change: 1.0.2 → 1.0.3 (PATCH — factual correction, no principle
  added, removed, or redefined)
Modified principles: none.
  - Development Workflow & Quality Gates: removed the claim that a shipped
    feature's spec.md is "archived under specs/archive/". specs/archive/ was
    deleted; every archived spec's durable requirements and interface
    contracts were folded into specs/domains/*.md and the feature directory
    is now removed outright on ship. Keeping a second copy of a still-binding
    rule is the drift this repository already fixed once for shared types
    (024) and for context documents (024-FR-010) — the same reasoning applies
    to specs. Originals remain recoverable from git history. See
    specs/README.md.
Added sections: none
Removed sections: none
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ re-checked, no specs/archive
    reference, no change needed
  - .specify/templates/spec-template.md ✅ re-checked, no change needed
  - .specify/templates/tasks-template.md ✅ re-checked, no change needed
  - .specify/templates/checklist-template.md ✅ re-checked, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ re-checked, no specs/archive
    reference found, no change needed
Follow-up TODOs: none

--------------------------------------------------------------------------

Version change: 1.0.3 → 2.0.0 (MAJOR — a principle is redefined, not
  clarified)
Modified principles:
  - **Principle V, "Local-First, Self-Hosted by Default" → "Self-Hosted
    Control Plane, Single Inference Path".** The local-first inference
    guarantee is removed. Before this amendment the principle required core
    scoring and generation to remain fully operational against a local
    Ollama instance with no calls to third-party AI APIs, and required the
    system to serve AI tasks locally when the gateway was unconfigured or
    unreachable. Both requirements are void.

    Why. The guarantee was costing more than it bought. Honouring it meant
    two inference paths in the application — a gateway path and a direct
    local path — with different failover, different cost accounting and,
    for the local path, no observability at all. The second path was the
    default in the shipped example environment, so the system most operators
    actually ran was the one nothing could see into. Embeddings never used
    the gateway on either path, which made "every AI call is recorded"
    untrue in a way that read as true.

    A second, quieter reason: the local tier had already stopped being
    local. `OLLAMA_URL` defaulted to `https://ollama.com` with an
    `OLLAMA_KEY`, so the terminal tier of every failover chain was a
    third-party API wearing the word "local". The principle was protecting
    a property the configuration had not had for some time.

    What replaces it. Provider credentials, routing policy and the proxy
    itself stay self-hosted and in-repository; the application holds no
    provider credential and reaches no provider directly. Availability is
    protected by requiring every task's chain to span at least two distinct
    providers, rather than by a local terminal tier.

    What is knowingly given up, stated so no future reader has to infer it:
    the platform can no longer score or generate anything without reaching a
    third-party provider, and prompt content — profile data, resume content,
    posting text — leaves the deployment on every AI request, with no
    configuration under which it does not. That is a real loss of the
    original product promise and it was accepted deliberately, not
    discovered. See specs/044-litellm-only-routing/spec.md and its
    Clarifications section for the decision record.
Added sections: none
Removed sections: none
Templates requiring updates:
  - .specify/templates/plan-template.md ✅ re-checked, no provider or
    local-first reference, no change needed
  - .specify/templates/spec-template.md ✅ re-checked, no change needed
  - .specify/templates/tasks-template.md ✅ re-checked, no change needed
  - .specify/templates/checklist-template.md ✅ re-checked, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ re-checked, no provider or
    local-first reference found, no change needed
Follow-up TODOs: the documents that describe the old guarantee are updated
  in the same feature, not here — specs/domains/llm-routing.md,
  docs/docs/ai/*, .env.example, docker-compose.prod.yml, README.md.
--------------------------------------------------------------------------

Version change: 2.0.0 → 2.1.0 (MINOR — guidance materially expanded; no
  principle removed or redefined)
Modified principles:
  - **Principle IV, "Test Discipline Per Language"** — `pytest` for apps/ai
    added alongside `go test` and `vitest`; the integration-test clause now
    names "a real message broker" rather than Redis; `just audit` is stated
    as covering every language's dependency surface. The principle itself —
    native toolchain per app, real dependencies for cross-service paths,
    enforced at the merge gate — is unchanged. A third runtime enters the
    repository and the gate must see it.
Modified sections:
  - **Technology & Architecture Constraints, data layer**: "asynq on Redis
    for async work, with one dedicated queue per task type" → RabbitMQ, one
    durable queue per work type plus a dead-letter queue per work type, with
    Redis explicitly demoted to caching and rate-limit state. The
    one-queue-per-work-type rule survives the broker change; what changes is
    what implements it, and that services now communicate by events rather
    than by one process consuming its own job list.
  - **Technology & Architecture Constraints**: `apps/ai` added as a third
    runtime — Python, LangChain/LangGraph, Langfuse — with its boundaries
    stated where the constraint lives rather than only in a feature spec: no
    persistence, no database or provider credential, gateway-only model
    access, in-repository prompts.
Added sections: none
Removed sections: none

Why now, before the code. This amendment lands ahead of the implementation
it describes, which inverts the usual order and is deliberate: spec 047
cannot be implemented without contradicting the constraints as written, and
a plan whose first task violates the constitution is a plan that teaches
people to ignore it. The constraints section is prescriptive — it says what
MUST be built — so it is the correct place for a decided-but-unbuilt rule.

What is NOT amended here, and why. Every other asynq reference in the
repository — README.md, docs/docs/async/*, docs/docs/architecture/*,
specs/domains/*.md — describes the system as it currently runs, and asynq is
still the live dispatch mechanism at the time of this amendment. specs/README
draws the line: specs say what must be true, docs say how it works. Those
documents become false at the moment the migration lands and are corrected
in that change, not in this one. A doc that describes a system nobody has
built yet is worse than one that is merely out of date.

Templates requiring updates:
  - .specify/templates/plan-template.md ✅ re-checked, Constitution Check
    gate is generic, no queue-technology or runtime reference, no change
  - .specify/templates/spec-template.md ✅ re-checked, no change needed
  - .specify/templates/tasks-template.md ✅ re-checked, no change needed
  - .specify/templates/checklist-template.md ✅ re-checked, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ re-checked, no queue-technology or
    runtime reference found, no change needed
Follow-up TODOs: AGENTS.md ("No Python is in this repository", the
  `test-lint` description, and the apps/api "asynq workers" line), README.md,
  docs/docs/async/*, docs/docs/architecture/* and the affected
  specs/domains/*.md are corrected in the feature that removes asynq — spec
  047, phases 2 and 4 — not here. Tracked in
  specs/047-langchain-ai-service/contracts/configuration.md K6.
-->

# job-finder Constitution

## Core Principles

### I. No Auto-Apply, Ever (NON-NEGOTIABLE)
The system discovers, scores, and drafts application materials, but a human MUST review
and manually submit every application. No code path may submit an application, message an
employer, or otherwise act on a job listing on the user's behalf without an explicit,
per-application user action immediately preceding it.
Rationale: this is the product's foundational trust boundary (stated in README as a
non-negotiable design goal); silently automating submission would misrepresent candidates
and expose users to irreversible, hard-to-detect harm.

### II. Grounded Generation
Any LLM-generated content (resume bullets, cover letters, tailored summaries) MUST be
derived from and traceable to the user's actual master profile data or the source job
posting — never fabricated experience, skills, or credentials. Prompts and post-processing
MUST make it possible to trace generated claims back to source fields.
Rationale: resumes and cover letters are used in real hiring decisions; hallucinated
content damages user credibility and is the single biggest trust risk for an AI-assisted
job tool.

### III. Typed Contracts Across Service Boundaries
Cross-language boundaries (Go API ↔ React dashboard) MUST go
through generated or explicitly shared types — sqlc for DB-to-Go, tygo for Go-to-TS, and
`packages/shared` as the single source of truth for TS-side DTOs/normalized job shapes.
Hand-maintained duplicate type definitions across apps are not permitted; regenerate
instead of hand-editing generated files.
Rationale: the stack spans two runtimes with no shared compiler; type drift between
services is the most common source of silent integration bugs in this codebase.

### IV. Test Discipline Per Language, Enforced at the Boundary
Each app tests in its native toolchain — `go test` for apps/api, `vitest` for the
dashboard, `pytest` for apps/ai — and integration/e2e paths (`test-integration`,
`test-e2e`) MUST exercise real Postgres and a real message broker via Docker Compose, not
mocks, for cross-service behavior. `just test-lint` (every language's suite) MUST pass
before a change touching more than one app is considered done, and `just audit` MUST cover
every language's dependency surface — a runtime whose dependencies no gate inspects is a
supply-chain hole, not a new app.
Rationale: matches the existing Justfile-enforced workflow; per-language suites keep
feedback fast, while Docker-backed integration tests catch the cross-service bugs unit
tests can't.

### V. Self-Hosted Control Plane, Single Inference Path
Data and control stay self-hosted: Postgres, Redis, document storage and the LiteLLM
routing service all run inside the deployment, and routing policy lives in-repository as
reviewed configuration. Inference does not. The application MUST reach AI providers
through the self-hosted gateway and MUST NOT call any provider directly — one path, so
that every AI request is recorded, costed and attributed without exception. Provider
credentials MUST stay in the gateway container's environment and MUST NOT be readable
through the application. Availability MUST be protected by chain diversity: every task's
routing chain MUST span at least two distinct providers. External job sources (Adzuna,
Jooble, Indeed, Glassdoor, ...) are for job discovery only, never core inference.

This principle no longer promises offline operation. Core scoring and generation depend on
at least one third-party provider being reachable, and prompt content — including profile
data and generated application materials — leaves the deployment on every AI request, with
no configuration under which it does not. Documentation presented to operators MUST state
that plainly rather than implying otherwise.

Rationale: the previous local-first guarantee required a second inference path inside the
application, which carried its own failover, its own cost accounting and no observability —
and it was the path the default configuration actually used. One audited path is worth more
than a fallback nobody could see into. What users control is what this project can
genuinely keep controlling: their data, their routing policy, their credentials.

## Technology & Architecture Constraints

- Backend (`apps/api`): Go, sqlc for typed DB access, goose for migrations — migration
  version numbers MUST be unique and sequential; never reuse or duplicate a goose version.
- Dashboard (`apps/dashboard`): React + Vite + TanStack Query + dnd-kit + Tailwind.
- AI orchestration (`apps/ai`): Python, LangChain and LangGraph for prompt assembly, step
  sequencing and bounded tool loops, Langfuse for run-level tracing. It owns no persistence
  and holds no database or provider credential; every model call goes through the gateway
  by task key (Principle V). Prompts and workflow definitions live in-repository and are
  changed by commit, never fetched at runtime.
- Scraping-based job sources: treated as best-effort/unstable upstream (scraping targets
  change), not a hard dependency for core functionality.
- `packages/shared`: shared TypeScript types (NormalizedJob, DTOs, JSON Resume subset) —
  the only place cross-app TS types are defined by hand; everything else generates from it
  or from Go via tygo.
- Data layer: Postgres with pgvector for embeddings; RabbitMQ for asynchronous work, with
  one dedicated durable queue per work type (ingest, match, generate, enrich, salary,
  ghost) and a dead-letter queue per work type. Redis is for caching and rate-limit state
  only — it is not a queue backend. Services communicate by publishing and consuming
  events; a service MUST NOT be called synchronously to perform queued work.
- Full stack ships via Docker Compose (`docker-compose.yml` dev, `docker-compose.prod.yml`
  prod); GPU is recommended, not required, for the Ollama model runtime.

## Development Workflow & Quality Gates

- `pnpm install` + `pnpm --filter @job-finder/shared build` before other workspace
  packages, since dashboard/api tooling (tygo-generated types) depend on shared being
  built first.
- Use `make` targets as the canonical entry points for dev/test/seed operations
  (`just up`, `just dev`, `just test`, `just test-integration`, `just test-e2e`,
  `just seed`) rather than ad hoc docker/pnpm invocations, so CI and local runs stay
  aligned.
- Design/plan docs for non-trivial features are written at
  `specs/<nnn>-<slug>/plan.md` before implementation begins; trivial fixes and
  refactors do not require a plan doc. Once a feature ships, its durable requirements
  and interface contracts are folded into `specs/domains/` and the whole
  `specs/<nnn>-<slug>/` directory is removed — there is no archive, so exactly one
  copy of every binding rule exists. Originals stay recoverable from git history.
  See `specs/README.md`.
- A change is not "done" until its own language's test suite passes locally; changes
  crossing app boundaries additionally require `just test-lint` before merge.

## Governance

This constitution supersedes ad hoc conventions when the two conflict. Amendments are made
by editing this file directly, updating the Sync Impact Report header, and bumping
**CONSTITUTION_VERSION** per semantic versioning: MAJOR for removing or redefining a
principle, MINOR for adding a principle or materially expanding guidance, PATCH for
wording/clarification only. Any amendment MUST re-check `.specify/templates/*.md` and the
installed `speckit-*` commands/skills for now-stale references and update them in the same
change.

All feature plans and PRs touching `apps/api`, `apps/dashboard`, or `packages/shared`
should be checked against the five Core Principles above before being marked ready for
review; deviations must be justified in the plan's Complexity Tracking section (or PR
description) rather than silently introduced.

**Version**: 2.1.0 | **Ratified**: 2026-07-16 | **Last Amended**: 2026-08-18
