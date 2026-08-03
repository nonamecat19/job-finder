# Contract: Resume Shape Settings API

**Feature**: `031-resume-generation-config`
**Mounted by**: `apps/api/internal/resumeshape/interfaces/http/resume_shape.go`, alongside the existing `/v1/settings/ai-features` routes.
**Shared type**: `ResumeShapeConfigDto` in `packages/shared/src/generated.ts` (tygo-generated from `internal/dto`; never hand-edited — constitution III).

---

## Payload

`ResumeShapeConfigDto` — identical on request and response.

```json
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
  "projectBulletsMax": 0
}
```

The values above are also the defaults (FR-003). `0` means "unlimited / no limit" for `skillsMaxGroups`, `projectsMin`, `projectsMax` and `projectBulletsMax`.

### Field constraints

| Field | Type | Valid range | Notes |
|-------|------|-------------|-------|
| `summaryLines` | int | 1–12 | Approximate target (FR-007) |
| `skillsEnabled` | bool | — | `false` removes the section entirely (FR-009) |
| `skillsMaxGroups` | int | 0–20 | `0` = keep all groups |
| `experienceBulletsMin` | int | 1–20 | Must be `<= experienceBulletsMax` |
| `experienceBulletsMax` | int | 1–20 | Hard cap (FR-014) |
| `targetPages` | int | 1–3 | |
| `projectsEnabled` | bool | — | `false` removes the section entirely |
| `projectsMin` | int | 0–20 | `> 0` requires `projectsEnabled: true`; must be `<= projectsMax` when `projectsMax > 0` |
| `projectsMax` | int | 0–20 | `0` = include all master projects |
| `projectBulletsMax` | int | 0–10 | `0` = keep every bullet |

---

## `GET /v1/settings/resume-shape`

Returns the current config.

- **200** — `ResumeShapeConfigDto`

Never 404s: the singleton row is seeded by migration `00034`, and the service falls back to `DefaultShapeConfig()` if the row is somehow absent.

---

## `PUT /v1/settings/resume-shape`

Replaces the whole config. Full-payload replacement, not a patch — FR-004 requires all-or-nothing validation, so every field must be present.

- **Request**: `ResumeShapeConfigDto`
- **200** — `ResumeShapeConfigDto` (the persisted values)
- **400** — `{"error": "..."}` when the body is unparseable or any field is out of range. The message names the offending field and its valid range, e.g. `targetPages must be between 1 and 3`, `experienceBulletsMin must be <= experienceBulletsMax`, `projectsMin > 0 requires projectsEnabled`.
- **500** — `{"error": "..."}` on a persistence failure.

**Ordering guarantee**: validation runs before any write. On 400, nothing is stored and the in-memory cache is untouched — a subsequent `GET` returns the pre-request values (FR-004).

**Effect**: applies to every generation **started after** the response. A generation already in flight completes with the config it resolved at its start (spec edge case).

---

## `DELETE /v1/settings/resume-shape`

Resets to documented defaults (FR-005).

- **200** — `ResumeShapeConfigDto` containing the defaults.

Idempotent: deleting an already-default config returns the same body.

---

## Error format

Reuses `httpx.WriteError`, matching every other endpoint in the API:

```json
{ "error": "targetPages must be between 1 and 3" }
```

---

## Dashboard client contract

`apps/dashboard/src/lib/api.ts`, added to the existing `settings` object:

```text
settings.getResumeShape()            -> Promise<ResumeShapeConfigDto>
settings.putResumeShape(body)        -> Promise<ResumeShapeConfigDto>
settings.resetResumeShape()          -> Promise<ResumeShapeConfigDto>
```

Query keys (`lib/queryKeys.ts`): `resumeShape.all = ['resumeShape']`, `resumeShape.get = ['resumeShape', 'get']`. Mutations invalidate `resumeShape.all`, matching the `aiFeatures` pattern.

---

## Contract test checklist

- `GET` on a fresh database returns exactly `DefaultShapeConfig()`.
- `PUT` with a valid body returns the persisted values; a following `GET` returns the same.
- `PUT` with `targetPages: 4` → 400, message names `targetPages` and the range `1 and 3`; a following `GET` still returns the previous values.
- `PUT` with `experienceBulletsMin: 12, experienceBulletsMax: 8` → 400 naming the cross-field rule.
- `PUT` with `projectsEnabled: false, projectsMin: 2` → 400 naming the dependency.
- `DELETE` after a `PUT` returns the defaults; a following `GET` returns the defaults.
- Round-trip: every field set to a non-default in-range value survives `PUT` → `GET` unchanged.
- `scripts/tygo-check.sh` passes (generated TS is in sync with the Go DTO).
