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
dashboard — and integration/e2e paths (`test-integration`,
`test-e2e`) MUST exercise real Postgres/Redis via Docker Compose, not mocks, for
cross-service behavior. `make test-lint` (both suites) MUST pass before a change
touching more than one app is considered done.
Rationale: matches the existing Makefile-enforced workflow; per-language suites keep
feedback fast, while Docker-backed integration tests catch the cross-service bugs unit
tests can't.

### V. Local-First, Self-Hosted by Default
Core scoring and generation MUST remain fully operational against the local Ollama
instance and self-hosted Postgres/Redis, with no calls to third-party AI APIs. Hosted
inference reached through the LiteLLM gateway is permitted as an optimisation, but every
task's routing chain MUST terminate at the local model, and the system MUST serve AI tasks
locally when the gateway is unconfigured or unreachable. Provider credentials MUST stay in
the gateway container's environment and MUST NOT be readable through the application.
External job sources (Adzuna, Jooble, Indeed, Glassdoor, ...) are for job discovery only,
never core inference.
Rationale: stated project goal is a self-hosted platform the user fully controls; a
hidden dependency on an external LLM API would silently break that guarantee and add
cost/privacy exposure users didn't opt into.

## Technology & Architecture Constraints

- Backend (`apps/api`): Go, sqlc for typed DB access, goose for migrations — migration
  version numbers MUST be unique and sequential; never reuse or duplicate a goose version.
- Dashboard (`apps/dashboard`): React + Vite + TanStack Query + dnd-kit + Tailwind.
- Scraping-based job sources: treated as best-effort/unstable upstream (scraping targets
  change), not a hard dependency for core functionality.
- `packages/shared`: shared TypeScript types (NormalizedJob, DTOs, JSON Resume subset) —
  the only place cross-app TS types are defined by hand; everything else generates from it
  or from Go via tygo.
- Data layer: Postgres with pgvector for embeddings; asynq on Redis for async work, with
  one dedicated queue per task type (ingest, match, generate, enrich, salary, ghost).
- Full stack ships via Docker Compose (`docker-compose.yml` dev, `docker-compose.prod.yml`
  prod); GPU is recommended, not required, for the Ollama model runtime.

## Development Workflow & Quality Gates

- `pnpm install` + `pnpm --filter @job-finder/shared build` before other workspace
  packages, since dashboard/api tooling (tygo-generated types) depend on shared being
  built first.
- Use `make` targets as the canonical entry points for dev/test/seed operations
  (`make up`, `make dev`, `make test`, `make test-integration`, `make test-e2e`,
  `make seed`) rather than ad hoc docker/pnpm invocations, so CI and local runs stay
  aligned.
- Design/plan docs for non-trivial features are written at
  `specs/<nnn>-<slug>/plan.md` before implementation begins; trivial fixes and
  refactors do not require a plan doc. Once a feature ships, its durable requirements
  and interface contracts are folded into `specs/domains/` and the whole
  `specs/<nnn>-<slug>/` directory is removed — there is no archive, so exactly one
  copy of every binding rule exists. Originals stay recoverable from git history.
  See `specs/README.md`.
- A change is not "done" until its own language's test suite passes locally; changes
  crossing app boundaries additionally require `make test-lint` before merge.

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

**Version**: 1.0.3 | **Ratified**: 2026-07-16 | **Last Amended**: 2026-08-04
