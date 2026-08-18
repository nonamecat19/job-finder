# Specification Quality Checklist: Dedicated AI Orchestration Service

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-08-18
**Feature**: [spec.md](../spec.md)

## Content Quality

- [~] No implementation details (languages, frameworks, APIs) — requirements and criteria are
      clean; a single *Mandated Technologies* section names the maintainer-chosen stack by
      design (see Notes)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

- **Deviation from the template, deliberate**: `spec.md` carries a *Mandated Technologies*
  section naming LangChain, LangGraph, Langfuse, RabbitMQ and Python. The template bans
  implementation detail, and normally these belong in `plan.md` — but the maintainer named
  them as the request itself, so they are constraints on planning, not outcomes of it. A spec
  that omitted them would let planning select something else without contradicting anything
  written down. The requirements, acceptance scenarios and success criteria remain
  technology-agnostic and testable; only the selection is pinned, and it is pinned in one
  section rather than scattered through the requirements.
- **Clarified 2026-08-18** (5 questions, all answered — see `spec.md` § Clarifications):
  1. **Scope is total** — all fourteen task keys migrate, embeddings included; no LLM call
     path remains in the backend (FR-019a).
  2. **Routing layer retained unchanged** — the orchestration service is a client of the
     gateway, by task key only, with no provider credentials and no bypass path
     (FR-009 – FR-011).
  3. **Event-driven backbone replaces the job queue** — every queue, AI and non-AI, migrates
     to a message broker; services communicate by work and result events; no synchronous
     backend→service call for queued work (FR-026 – FR-038, US5).
  4. **Definitions live in-repo** — prompts and workflows are committed, never fetched at
     runtime (FR-015a).
  5. **Trace payloads purge at 30 days**, metrics retained indefinitely (FR-018 – FR-018b).
- **Scope expansion recorded, with the coupling named**: the broker migration arrived during
  clarification and is far larger than the AI migration that motivated it. It touches every
  queue and worker — ingestion, enrichment, notification, scheduling — and replaces the
  retry/backoff/stuck-run machinery in `internal/queue/policy.go` that non-AI paths depend
  on. The spec keeps it in one feature per the maintainer's decision, sequenced as a
  prerequisite (US5, P1) proven on non-AI work before any AI capability moves. Planning
  should treat it as a separable phase with its own rollback story, and should weigh
  splitting it into its own feature directory if the task count justifies it.
- **New-language cost accepted** — a third runtime enters the stack, requiring amendments to
  the constitution's technology constraints (§ *Technology & Architecture Constraints*) and
  per-language test discipline (Principle IV) before implementation.
- **Constitution conflicts, flagged for planning** — two now, both amendments rather than
  implementation details:
  1. Principle V (*Self-Hosted Control Plane, Single Inference Path*) is preserved by
     FR-009 – FR-011, but § *Technology & Architecture Constraints* names only Go and
     TypeScript runtimes, and `AGENTS.md` states "No Python is in this repository".
  2. The same section pins "asynq on Redis for async work, with one dedicated queue per task
     type". FR-026 removes it. The constraint must be rewritten before implementation, not
     silently contradicted.
