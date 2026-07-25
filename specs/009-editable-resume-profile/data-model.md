# Phase 1 Data Model: Fully Editable Resume Profile Tab

All types below are Go structs in `apps/api/internal/dto/resume.go`, tygo-generated into
`packages/shared/src/generated.ts` (Constitution Principle III). They describe the
*structured, typed view* of a resume that the client edits; the API translates to/from the
existing untyped `map[string]any` RendercvMaster on read/write (see research.md #2–#3).
These are additive DTOs — no existing DTO or DB column is removed.

## Resume

Top-level structured document for one Profile's resume content (maps to `cv.*` in the
RendercvMaster, minus `design`/`locale`/`settings`, which stay out of scope per Assumptions).

| Field | Type | Notes |
|---|---|---|
| `name` | `string` | Required — mirrors RenderCV's required `cv.name`. |
| `headline` | `*string` | Optional. |
| `location` | `*string` | Optional. |
| `email` | `*string` | Optional. |
| `phone` | `*string` | Optional. |
| `website` | `*string` | Optional. |
| `photo` | `*string` | Optional (path/URL), passthrough only — not edited via a dedicated UI in this feature. |
| `socialNetworks` | `[]SocialNetwork` | Ordered. |
| `customConnections` | `[]CustomConnection` | Ordered. |
| `sections` | `[]Section` | Ordered — this order **is** the section order (see research.md #2). |
| `unrecognized` | `map[string]any` | Any top-level `cv.*` keys not modeled above (FR-009). Opaque, round-tripped verbatim. |

## SocialNetwork

| Field | Type | Notes |
|---|---|---|
| `network` | `string` | e.g. "LinkedIn", "GitHub". |
| `username` | `string` | |

## CustomConnection

| Field | Type | Notes |
|---|---|---|
| `label` | `string` | |
| `value` | `string` | |

## Section

A named, ordered group of same-typed entries (FR-005, FR-006).

| Field | Type | Notes |
|---|---|---|
| `name` | `string` | User-defined; arbitrary display name (e.g. "Experience", "Selected Honors"). |
| `entryType` | `EntryType` (string enum) | One of the 9 canonical types (research.md #1). Fixed once entries exist; a section holds one entry type at a time, matching RenderCV's model. |
| `entries` | `[]Entry` | Ordered — this order **is** the entry order within the section. |

`EntryType` enum values: `education`, `experience`, `normal`, `publication`, `one_line`,
`bullet`, `numbered`, `reversed_numbered`, `text`.

## Entry

A tagged union keyed by the parent Section's `entryType`. Represented in Go as one struct
with all optional fields (tygo-friendly) rather than a Go interface, so the generated TS
type is a plain discriminated-friendly object; the client only reads/writes the fields
relevant to its section's `entryType`.

| Field | Type | Used by entry type(s) | Notes |
|---|---|---|---|
| `institution` | `*string` | education | |
| `area` | `*string` | education | |
| `degree` | `*string` | education | |
| `company` | `*string` | experience | |
| `position` | `*string` | experience | |
| `name` | `*string` | normal (project) | |
| `title` | `*string` | publication | |
| `authors` | `[]string` | publication | Ordered. |
| `doi` | `*string` | publication | |
| `url` | `*string` | publication, normal | |
| `journal` | `*string` | publication | |
| `label` | `*string` | one_line | |
| `details` | `*string` | one_line | |
| `bullet` | `*string` | bullet | |
| `number` | `*string` | numbered | |
| `reversedNumber` | `*string` | reversed_numbered | Field name is `reversed_number` in RenderCV YAML; note the naming is a RenderCV quirk (value is a talk title/description, not a number) and is preserved as-is for round-trip fidelity. |
| `text` | `*string` | text | Plain markdown string entry. |
| `date` | `*string` | education, experience, normal, publication | Single free-form date (RenderCV allows either this or start/end). |
| `startDate` | `*string` | education, experience, normal | |
| `endDate` | `*string` | education, experience, normal | `"present"` is a valid literal value. |
| `location` | `*string` | education, experience, normal | |
| `summary` | `*string` | education, experience, normal, publication | |
| `highlights` | `[]string` | education, experience, normal | Ordered. |
| `unrecognized` | `map[string]any` | any | Any fields present in imported data not covered above (FR-009). |

**Validation rules** (enforced by API before persistence, surfaced inline per FR-007):
- `Resume.name` MUST be non-empty (mirrors RenderCV's `cv.name` requirement).
- If both `startDate` and `endDate` are set on an entry, `endDate` MUST NOT be before
  `startDate` (unless `endDate == "present"`).
- `Section.name` MUST be non-empty.
- An `Entry` MUST NOT be persisted with all relevant-to-its-type fields empty (prevents
  silently saving blank entries) — client blocks save and points at the entry.

## UnrecognizedSection

**Decision**: rather than a separate type, an unrecognized section is simply a `Section`
whose `entryType` doesn't match one of the 9 known values; its `entries[].unrecognized`
carries the full raw entry data. This avoids a parallel "fallback section" type and keeps
one Section list to reorder (research.md #4).

## Relationship to existing types

- `Resume` is additively attached to the existing `ProfileDto`
  (`packages/shared/src/generated.ts`) as a new typed field (e.g. `resume: Resume`),
  alongside the existing `rendercvFull: any` (which may remain for backward-compatible raw
  access, or be superseded — a Phase 2 tasks-level decision, not a data-model change).
- No changes to the `Profile` DB table or its columns; `Resume` is a request/response
  shape only, translated to/from `map[string]any` at the API boundary
  (`internal/generation/resume_mapping.go`) before touching `rendercvYaml`/`rendercvConfig`.
