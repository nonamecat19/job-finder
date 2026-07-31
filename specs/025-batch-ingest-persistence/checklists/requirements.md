# Specification Quality Checklist: Batched, Atomic Ingest Persistence

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-07-30
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
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

- Validation pass 1: FR-007 originally named a specific index type and FR-002/FR-003 named a
  specific bulk-copy mechanism. Both were rewritten to state the required property (index-served,
  bulk rather than per-row) without prescribing the mechanism, which belongs in `plan.md`.
- Validation pass 1: SC-002 originally stated a round-trip count in database-vendor terms; it now
  states a bounded interaction count per chunk, which is verifiable from query logs without
  assuming a particular driver.
- Scope deliberately excludes changing the posting-identity rule (see Assumptions) — doing so
  would orphan every stored posting and is a separate feature.
- Items marked incomplete require spec updates before `/speckit-clarify` or `/speckit-plan`.

## Post-analysis resolutions (2026-07-30)

`/speckit-analyze` raised three actionable findings. All resolved:

- **SC-001/SC-005 were unverifiable-by-default** (HIGH). Both are ratios against a pre-change
  baseline that cannot be captured retroactively. T002 is now a hard gate — no Phase 2 task starts
  until the three baseline numbers exist — with an explicit instruction to declare the criteria
  unmeasurable rather than claim them later if the baseline cannot be taken.
- **Two different interaction budgets** (MEDIUM). SC-002 says ≤10 per chunk, data-model.md §4
  guarantees ≤6. T024 now asserts 6, so a regression from 6 to 9 fails the test instead of passing
  silently until the acceptance criterion itself breaks.
- **Concurrency semantics changed without being stated** (LOW). When two runs race, the loser now
  records no repeat sighting. Added to spec Assumptions, since the sighting count feeds the
  ghost-job detector.
- `contracts/queries.md §6` remains provisional on `ActivityRun`'s exact columns by design; T006
  verifies against the real schema before T007 writes the query.
