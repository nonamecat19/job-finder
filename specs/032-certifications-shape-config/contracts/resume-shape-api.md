# Contract: `/v1/settings/resume-shape`

**Feature**: 032-certifications-shape-config

No new endpoints. The three existing methods gain three fields in their shared request/
response body. The change is **additive**; no field is renamed, retyped or removed.

## Body: `ResumeShapeConfigDto`

Both request and response body for every method below. PUT replaces the whole config, so
every field must be present.

```jsonc
{
  "summaryLines": 4,
  "skillsEnabled": true,
  "skillsMaxGroups": 0,
  "experienceBulletsMin": 8,
  "experienceBulletsMax": 10,
  "targetPages": 2,
  "projectsEnabled": true,
  "projectsMin": 0,
  "projectsMax": 0,
  "projectBulletsMax": 0,

  // added by this feature
  "certificationsEnabled": true,
  "certificationsMin": 0,
  "certificationsMax": 0
}
```

### New fields

| Field | Type | Range | Default | Meaning |
|---|---|---|---|---|
| `certificationsEnabled` | boolean | — | `true` | Certifications section renders |
| `certificationsMin` | integer | 0–20 | `0` | Target floor. `0` = no minimum |
| `certificationsMax` | integer | 0–20 | `0` | Hard cap. `0` = unlimited |

The `0 = unlimited` convention already documented for `skillsMaxGroups`, `projectsMin`,
`projectsMax` and `projectBulletsMax` extends to `certificationsMin` and
`certificationsMax`. The DTO doc comment must be updated to name them.

## `GET /v1/settings/resume-shape`

**200** — current config, now including the three new fields. Served from the in-memory
cache.

## `PUT /v1/settings/resume-shape`

Replaces the whole config.

**200** — the stored config as persisted.

**400** — validation failure. The whole update is rejected and nothing is written; a
subsequent GET returns the previous values unchanged (FR-008).

New 400 messages introduced by this feature:

| Condition | Message |
|---|---|
| `certificationsMin` outside 0–20 | `certificationsMin must be between 0 and 20` |
| `certificationsMax` outside 0–20 | `certificationsMax must be between 0 and 20` |
| `certificationsMin > certificationsMax` (when max > 0) | `certificationsMin must be <= certificationsMax` |
| `certificationsMin > 0` while `certificationsEnabled` is false | `certificationsMin > 0 requires certificationsEnabled` |

Message forms mirror the existing projects messages exactly, so clients that surface
`error` verbatim need no change.

Validation runs twice by design — in the handler to produce a 400 rather than a 500, and
in the service to make the write atomic. Both call the same
`domain.ShapeConfig.Validate()`, so the new rules take effect in both places from one
edit.

## `DELETE /v1/settings/resume-shape`

Resets to defaults.

**200** — the documented defaults, now including `certificationsEnabled: true`,
`certificationsMin: 0`, `certificationsMax: 0` (FR-011).

## Backward compatibility

| Scenario | Behaviour |
|---|---|
| Old client PUTs a body without the new fields | Go decodes missing booleans as `false` and missing ints as `0` — so `certificationsEnabled` silently becomes **false**, disabling the section. This is a real hazard of a whole-config PUT. |
| New client, old server | Server ignores unknown fields; the new settings have no effect. |
| Upgraded DB, never edited | Migration defaults apply; behaviour identical to pre-feature (SC-004). |

The first row deserves attention during implementation. The dashboard always round-trips
the full config it received from GET, so it is unaffected. Any external/scripted client
that PUTs a hand-written body would disable certifications unintentionally. Options, to
be settled in tasks: accept it (consistent with how `skillsEnabled`/`projectsEnabled`
already behave on this endpoint — this feature does not make the situation worse), or
decode into a pointer-field struct and treat absent booleans as "keep current". The
existing precedent is to accept it; deviating would make certifications behave
differently from the other two toggles on the same endpoint.

## Generated type contract (Constitution III)

`packages/shared/src/generated.ts` is tygo output from `apps/api/internal/dto`. Sequence:

1. Edit `apps/api/internal/dto/settings.go`.
2. `make tygo-generate`.
3. `pnpm --filter @job-finder/shared build`.
4. Dashboard typechecks against the new fields.

`make tygo-check` fails CI if the committed generated file drifts from a fresh
generation. `packages/shared/src/index.ts` already re-exports `ResumeShapeConfigDto`
(commit `aee564a`), so no export change is needed.
