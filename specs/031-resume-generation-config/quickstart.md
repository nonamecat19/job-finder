# Quickstart: Validating Configurable Resume Generation Shape

**Feature**: `031-resume-generation-config` | **Date**: 2026-08-02

Runnable validation scenarios proving the feature end to end. Details of types and endpoints live in [data-model.md](./data-model.md) and [contracts/settings-resume-shape.md](./contracts/settings-resume-shape.md); this file is the run guide.

---

## Prerequisites

- Docker Compose stack available (`make up` brings up Postgres + Redis + the API).
- A master resume with a `projects` section — `resume/resume.yaml` in this repo already has 4 project entries and works as the fixture.
- A local LLM reachable through the configured routing (constitution V) for the scenarios that actually generate.

> Long-lived processes (`make up`, the API server) must be started through `process-hive`, not backgrounded ad hoc.

## Setup

```bash
make up                 # stack up, migrations applied (00034 seeds the default row)
make sqlc-generate      # after adding queries/resumeshapesetting.sql
make tygo-generate      # after adding ResumeShapeConfigDto
```

---

## Scenario 1 — Defaults are unchanged behaviour (FR-003, SC-002)

```bash
curl -s localhost:8080/v1/settings/resume-shape | jq
```

**Expect**: the exact default payload from the contract (`summaryLines: 4`, `targetPages: 2`, every `*Max` sentinel `0`).

Then generate a resume against a known vacancy with the config untouched.

**Expect**: 2 pages; summary ~3-4 sentences; 8-10 bullets per role; all master projects present with their bullets verbatim. Output is indistinguishable from a pre-feature generation — this is the regression guard for SC-002.

---

## Scenario 2 — Short resume (User Story 1, FR-007/FR-010/FR-011, SC-004/SC-005)

```bash
curl -s -X PUT localhost:8080/v1/settings/resume-shape \
  -H 'content-type: application/json' \
  -d '{"summaryLines":2,"skillsEnabled":true,"skillsMaxGroups":3,
       "experienceBulletsMin":4,"experienceBulletsMax":5,"targetPages":1,
       "projectsEnabled":false,"projectsMin":0,"projectsMax":0,"projectBulletsMax":0}' | jq
```

Generate a resume.

**Expect**:
- Summary ≈ 2 lines (±1 line — approximate by design).
- Every experience entry has 4-5 bullets; none exceeds 5 (the max is a hard clamp).
- At most 3 skill groups.
- No projects section, and no orphan heading.
- Rendered PDF is 1 page.
- Activity trace shows the resolved config and the achieved page count.

---

## Scenario 3 — Projects limiting (User Story 2, FR-012/FR-013/FR-018/FR-019, SC-007)

```bash
curl -s -X PUT localhost:8080/v1/settings/resume-shape \
  -H 'content-type: application/json' \
  -d '{"summaryLines":4,"skillsEnabled":true,"skillsMaxGroups":0,
       "experienceBulletsMin":8,"experienceBulletsMax":10,"targetPages":2,
       "projectsEnabled":true,"projectsMin":3,"projectsMax":4,"projectBulletsMax":2}' | jq
```

Generate a resume against a master with more projects than the cap (add two extra entries to the fixture to exercise truncation).

**Expect**:
- Exactly 3-4 project entries in the output.
- Each project shows at most 2 bullets.
- Every project's `name`, `url`, `start_date`, `end_date` are byte-identical to the master — diff them to confirm (FR-018).
- Projects appear in the master's section position and the retained entries keep master order (FR-019).
- Bullets are drawn only from that project's own master bullets (FR-018) — cross-project bleed is a grounding failure.

---

## Scenario 4 — Section disable does not break grounding (User Story 3, FR-020, SC-006)

```bash
curl -s -X PUT localhost:8080/v1/settings/resume-shape \
  -H 'content-type: application/json' \
  -d '{"summaryLines":4,"skillsEnabled":false,"skillsMaxGroups":0,
       "experienceBulletsMin":8,"experienceBulletsMax":10,"targetPages":2,
       "projectsEnabled":true,"projectsMin":0,"projectsMax":0,"projectBulletsMax":0}' | jq
```

Generate, then re-enable and regenerate.

**Expect**: no skills section and no orphan heading while disabled; every other section still present in master order; generation succeeds with **zero** grounding or structure violations; skills return on regeneration after re-enabling.

---

## Scenario 5 — Validation is all-or-nothing (FR-004)

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X PUT localhost:8080/v1/settings/resume-shape \
  -H 'content-type: application/json' \
  -d '{"summaryLines":4,"skillsEnabled":true,"skillsMaxGroups":0,
       "experienceBulletsMin":8,"experienceBulletsMax":10,"targetPages":9,
       "projectsEnabled":true,"projectsMin":0,"projectsMax":0,"projectBulletsMax":0}'
curl -s localhost:8080/v1/settings/resume-shape | jq .targetPages
```

**Expect**: `400`, an error naming `targetPages` and the range `1 and 3`, and the follow-up `GET` still returning the **previous** `targetPages` — nothing partially stored.

---

## Scenario 6 — Reset (User Story 4, FR-005, SC-009)

```bash
curl -s -X DELETE localhost:8080/v1/settings/resume-shape | jq
```

**Expect**: the default payload; a following `GET` agrees; the next generated resume matches Scenario 1's output.

---

## Scenario 7 — Shortfall without fabrication (FR-017, edge case)

Set `experienceBulletsMin: 15` against a master where some role has fewer bullets, and generate.

**Expect**: generation succeeds; the short role carries only the bullets that exist; **no invented bullets**; the activity trace records a shortfall entry naming the path, the requested count and the available count.

---

## Scenario 8 — Unreachable page target (FR-021, edge case)

Set `targetPages: 1` with `experienceBulletsMin/Max: 15/20` and `summaryLines: 12` — deliberately contradictory — and generate.

**Expect**: generation **completes** rather than failing; the page target wins over the length targets (FR-016); the activity trace records both the conflict and the final page count.

---

## Automated suites

```bash
make test-go            # domain shape unit tests, merge, grounding, page loop, handler tests
make test-react         # ResumeShapeCard vitest
make test-integration   # Docker-backed API + DB round-trip of the settings endpoints
make sqlc-check         # generated DB code in sync
make tygo-check         # generated TS in sync with the Go DTO
make test-lint          # required gate before this change is done (constitution IV)
```

## Definition of done for validation

- Scenarios 1-8 behave as described against a live stack.
- `make test-lint`, `make sqlc-check`, `make tygo-check` all pass.
- Scenario 1 confirms no behavioural drift for users who never open the settings card.
