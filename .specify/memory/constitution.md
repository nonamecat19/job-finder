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
  - .specify/templates/plan-template.md ⚠ pending (generic Constitution Check gate references [PRINCIPLES]; verify wording still generic enough, no action forced)
  - .specify/templates/spec-template.md ✅ no principle-specific references found, no change needed
  - .specify/templates/tasks-template.md ✅ no principle-specific references found, no change needed
  - .claude/skills/speckit-*/SKILL.md ✅ generic, no agent-specific renames needed
Follow-up TODOs:
  - TODO(RATIFICATION_DATE): original adoption date unknown; using constitution creation date as placeholder-of-record until confirmed.
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
Cross-language boundaries (Go API ↔ React dashboard ↔ Python jobspy-sidecar) MUST go
through generated or explicitly shared types — sqlc for DB-to-Go, tygo for Go-to-TS, and
`packages/shared` as the single source of truth for TS-side DTOs/normalized job shapes.
Hand-maintained duplicate type definitions across apps are not permitted; regenerate
instead of hand-editing generated files.
Rationale: the stack spans three runtimes with no shared compiler; type drift between
services is the most common source of silent integration bugs in this codebase.

### IV. Test Discipline Per Language, Enforced at the Boundary
Each app tests in its native toolchain — `go test` for apps/api, `vitest` for the
dashboard, `pytest` for jobspy-sidecar — and integration/e2e paths (`test-integration`,
`test-e2e`) MUST exercise real Postgres/Redis via Docker Compose, not mocks, for
cross-service behavior. `make test-lint` (all three suites) MUST pass before a change
touching more than one app is considered done.
Rationale: matches the existing Makefile-enforced workflow; per-language suites keep
feedback fast, while Docker-backed integration tests catch the cross-service bugs unit
tests can't.

### V. Local-First, Self-Hosted by Default
Core scoring and generation MUST run against the local Ollama instance and self-hosted
Postgres/Redis; the system must remain fully operational with no calls to third-party
paid AI APIs for its primary matching/generation flow. External sources (Adzuna,
LinkedIn/Indeed/Glassdoor via JobSpy) are for job discovery only, not core inference.
Rationale: stated project goal is a self-hosted platform the user fully controls; a
hidden dependency on an external LLM API would silently break that guarantee and add
cost/privacy exposure users didn't opt into.

## Technology & Architecture Constraints

- Backend (`apps/api`): Go, sqlc for typed DB access, goose for migrations — migration
  version numbers MUST be unique and sequential; never reuse or duplicate a goose version.
- Dashboard (`apps/dashboard`): React + Vite + TanStack Query + dnd-kit + Tailwind.
- Job discovery sidecar (`apps/jobspy-sidecar`): Python FastAPI wrapping JobSpy; treated
  as best-effort/unstable upstream (scraping targets change), not a hard dependency for
  core functionality.
- `packages/shared`: shared TypeScript types (NormalizedJob, DTOs, JSON Resume subset) —
  the only place cross-app TS types are defined by hand; everything else generates from it
  or from Go via tygo.
- Data layer: Postgres with pgvector for embeddings, Redis/BullMQ-class queueing for async
  work (enrichment, generation jobs).
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
- Design/plan docs for non-trivial features are written under `plan/` before
  implementation begins, per existing repo convention (see prior `docs(plan): ...`
  commits); trivial fixes and refactors do not require a plan doc.
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

All feature plans and PRs touching `apps/api`, `apps/dashboard`, or `apps/jobspy-sidecar`
should be checked against the five Core Principles above before being marked ready for
review; deviations must be justified in the plan's Complexity Tracking section (or PR
description) rather than silently introduced.

**Version**: 1.0.0 | **Ratified**: TODO(RATIFICATION_DATE): original adoption date not recorded in repo history | **Last Amended**: 2026-07-16
