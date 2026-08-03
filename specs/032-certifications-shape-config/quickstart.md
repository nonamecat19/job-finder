# Quickstart: Validating Certifications Shape Config

**Feature**: 032-certifications-shape-config

How to prove this feature works end to end. Run top to bottom after implementation.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # must precede dashboard/api tooling
make up                                  # Postgres + Redis via Docker Compose
```

## 1. Codegen is in sync

Both gates must pass before anything else — they catch a hand-edited generated file or a
forgotten regeneration.

```bash
make sqlc-generate && make sqlc-check
make tygo-generate && make tygo-check
```

**Expected**: both clean, no diff. `packages/shared/src/generated.ts` now contains
`certificationsEnabled`, `certificationsMin`, `certificationsMax` on
`ResumeShapeConfigDto`.

## 2. Migration applies and preserves behaviour

```bash
make up   # goose runs 00035 on start
```

Then verify the singleton row backfilled to behaviour-preserving values:

```sql
SELECT "certificationsEnabled", "certificationsMin", "certificationsMax"
FROM "ResumeShapeSetting" WHERE "id" = 'default';
```

**Expected**: `true, 0, 0` — section rendered, no cap, no minimum (FR-010, SC-004).

Confirm the down migration is clean by rolling back and forward once.

## 3. Unit suites

```bash
make test-go
make test-react
```

**Expected**: green, including new cases for

- `Validate()` — each new range bound and both cross-field rules (see
  [contracts/resume-shape-api.md](contracts/resume-shape-api.md) for exact messages)
- `DefaultShapeConfig()` — the three new defaults
- `ApplySectionToggles` — certifications removed from both `cv.sections` and `_order`
  when disabled; untouched when enabled; no error when the section is absent
- `ApplyHardLimits` — truncates to `CertificationsMax` preserving authored order; keeps
  all when max is `0`; emits a `cv.sections.certifications` shortfall when fewer than
  `CertificationsMin` exist; never pads
- `ResumeShapeCard` — the `dirty` flag flips on a `certificationsEnabled`-only edit
  (the one change the TS compiler cannot catch — see data-model.md)

## 4. API contract by hand

```bash
curl -s localhost:8080/v1/settings/resume-shape | jq
```

**Expected**: three new fields present with defaults.

Reject an invalid config — min above max:

```bash
curl -s -X PUT localhost:8080/v1/settings/resume-shape \
  -H 'content-type: application/json' \
  -d '{"summaryLines":4,"skillsEnabled":true,"skillsMaxGroups":0,
       "experienceBulletsMin":8,"experienceBulletsMax":10,"targetPages":2,
       "projectsEnabled":true,"projectsMin":0,"projectsMax":0,"projectBulletsMax":0,
       "certificationsEnabled":true,"certificationsMin":5,"certificationsMax":2}' | jq
```

**Expected**: `400`, `certificationsMin must be <= certificationsMax`. Re-run the GET and
confirm **nothing changed** — this is the atomicity guarantee in FR-008.

Repeat with `"certificationsMin":3,"certificationsEnabled":false` →
`certificationsMin > 0 requires certificationsEnabled`.

Reset:

```bash
curl -s -X DELETE localhost:8080/v1/settings/resume-shape | jq
```

**Expected**: defaults returned, including the three new fields (FR-011).

## 5. Integration: generation honours the settings

```bash
make test-integration
```

Needs a profile fixture with a certifications section — **none exists in the repo
today**, so one must be added as part of this feature. Cases to cover:

| Config | Expected rendered output |
|---|---|
| defaults | certifications present, all entries, positioned between education and publications (FR-013) |
| `certificationsEnabled: false` | no certifications section; no gap in section order; master profile unmodified (FR-004) |
| `certificationsMax: 3`, profile has 8 | exactly 3, the first 3 in authored order (FR-005, FR-015) |
| `certificationsMax: 3`, profile has 2 | both, nothing invented (FR-006) |
| `certificationsMin: 4`, profile has 2 | both rendered, run records a `cv.sections.certifications` shortfall of 4 requested / 2 available, generation succeeds (SC-007) |
| profile has no certifications section | generation succeeds either toggle state, no empty heading |

Also confirm the activity record's shape metadata echoes the three new keys, so a past
document can be explained by the settings that produced it.

## 6. Cross-app gate

This feature touches `apps/api`, `apps/dashboard` and `packages/shared`, so the
constitution requires:

```bash
make test-lint
```

**Expected**: green. This is the "done" bar for a multi-app change.

## 7. Manual UI check

```bash
make dev
```

Settings → resume shape card. Confirm:

- "Include certifications section" checkbox sits with the skills and projects toggles
- "Min certifications" and "Max certifications" rows appear in the numeric grid
- toggling **only** the certifications checkbox enables the save button
- saving, reloading the page, and seeing the values persist
- an out-of-range value surfaces the server's error message

## What is NOT validated here

Vacancy-relevance selection of certifications is out of scope (research D3, spec FR-015).
A cap truncates in authored order. There is deliberately no test asserting that the
"most relevant" certifications survive, because the feature makes no such promise.
