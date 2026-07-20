# Spec Quality Checklist: Ghost-Job Detector

Applied to [spec.md](../spec.md) on 2026-07-20.

## Content quality

- [x] No implementation detail in the spec — table names, hashing algorithms, and package layout live in [data-model.md](../data-model.md) / [research.md](../research.md) / [plan.md](../plan.md), not in requirements.
- [x] Written for a reader deciding whether the feature is worth building, not for a reader implementing it.
- [x] Every mandatory section present: User Scenarios & Testing, Requirements, Success Criteria.
- [x] Section order matches spec 001.

## Requirement completeness

- [x] No `[NEEDS CLARIFICATION]` markers remain. The three decisions that could have been open — separate table, badge-only policy, 50/80 bands — were user-approved before drafting and are recorded under Assumptions.
- [x] All 20 functional requirements are testable and unambiguous.
- [x] Success criteria (SC-001…SC-011) are measurable and technology-agnostic — none names a table, language, or library.
- [x] Every user story has an Independent Test and numbered acceptance scenarios.
- [x] Edge cases enumerated: no `postedAt`, one-off company, legitimate agency cross-post, unparseable company, empty description, malformed model output, deleted job, all-signals-unknown.
- [x] Scope bounded: scoring, badge, panel, manual refresh. No auto-action, no scheduler, no employer contact.
- [x] Dependencies and assumptions stated explicitly, including the ones that weaken the feature (always-hiring is noisy on a fresh install; every signal is a proxy with an innocent explanation).

## Constitution alignment

- [x] **I. No Auto-Apply** — FR-015 and FR-016 make badge-only an explicit requirement, not an implementation choice. SC-007 makes it measurable.
- [x] **II. Grounded Generation** — FR-019 forbids explanation content not traceable to a measured signal.
- [x] **III. Typed Contracts** — no hand-written cross-language type is implied; enforced in plan.md.
- [x] **IV. Test Discipline** — quickstart Levels 1-4 map to the spec's requirements and success criteria; `make test-lint` named as the binding gate since the change crosses two apps.
- [x] **V. Local-First** — FR-020 requires local inference.

## Traceability

- [x] Each user story maps to functional requirements: US1 → FR-012/FR-017, US2 → FR-013/FR-011, US3 → FR-014.
- [x] Each success criterion traces to at least one requirement.
- [x] Each edge case has a matching row in quickstart Level 4.

## Notes

- The 50/80 thresholds are product constants chosen without a calibration set. That is recorded honestly in Assumptions rather than presented as derived, and they are expected to be tuned once real scores exist.
- The always-hiring signal's dependence on the user having worked their pipeline is a real weakness of the feature, documented in research Decision 4 rather than smoothed over.
