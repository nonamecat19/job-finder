# Phase 0 Research: Fully Editable Resume Profile Tab

No items in Technical Context were marked NEEDS CLARIFICATION — the stack is fixed by the
project constitution and existing code. This document instead resolves the implementation
choices needed before Phase 1 design, each framed as a decision with rationale and
rejected alternatives.

## 1. RenderCV entry type inventory

**Decision**: Support exactly the 9 canonical RenderCV entry types, matching the sample
fixture and RenderCV's own schema comment ("You can choose any of the 9 entry types"):
`EducationEntry`, `ExperienceEntry`, `NormalEntry` (used for projects), `PublicationEntry`,
`OneLineEntry` (used for skills: label/details), `BulletEntry` (e.g. honors), `NumberedEntry`
(e.g. patents: `number` field), `ReversedNumberedEntry` (e.g. invited talks:
`reversed_number` field), `TextEntry` (plain markdown strings, e.g. an intro/welcome section).

**Rationale**: `apps/api/internal/generation/testdata/sample_rendercv.yaml` demonstrates all
9 shapes under differently-named sections (a section's *name* is arbitrary; its *entry type*
is determined by which fields its entries contain). FR-003 requires dedicated structured
forms for "at minimum" experience/education/skills/projects/publications/patents/talks —
covering all 9 canonical types satisfies FR-003 fully rather than partially, per the
scope decision already made with the user (all entry types get structured forms, not a
common-subset + fallback split).

**Alternatives considered**: Inferring entry type per-section from field presence only
at parse time (no explicit type tag) — rejected as the sole mechanism because the UI needs
the user to be able to declare the type for a *brand-new* empty section (User Story 1),
where no fields exist yet to infer from. Decision: the client records/display an explicit
entry-type per section (chosen at section-creation time), while import still infers type
from an uploaded config's existing field shapes.

## 2. Reconciling structured edits with the in-flight order-preservation work

**Decision**: New `resume_mapping.go` converts between `dto.Resume` (typed) and the
existing `map[string]any` `RendercvMaster` shape by operating *through*
`ParseRendercv`/`PrepareMasterForMarshal`, not around them. Structured edits are applied by:
loading the current master map, replacing only `cv.sections` (and the identity fields)
with values derived from the structured edit, preserving the `_order` key semantics already
implemented, then calling the existing YAML marshal path unchanged.

**Rationale**: `rendercv.go`/`rendercv_config.go` currently have uncommitted local changes
whose entire purpose is preserving `cv.sections` key order through a parse→tailor→render
round trip via a synthetic `_order` key and `protectedSections`. Building structured editing
as a second, parallel code path that also marshals YAML would create two divergent
YAML-writing implementations and risk silently breaking order preservation. Routing through
the same map-shaped representation keeps a single source of truth for marshaling.

**Alternatives considered**: Have the frontend own an explicit `order` field per section/
entry as a first-class part of `dto.Resume` and drop the `_order`-in-map convention
entirely — rejected for this feature because it would require modifying already-in-flight,
uncommitted work in a way that's out of scope for this spec; instead `dto.Resume`'s own
section/entry list order *is* the order (arrays are already ordered), and the mapping layer
is responsible for translating that array order into the `_order` convention the existing
marshal path expects.

## 3. Persisting structured edits (no schema change)

**Decision**: A structured edit is saved by reconstructing the full RenderCV YAML text via
`PrepareMasterForMarshal` + existing YAML marshal, then calling the existing
`Update`/`Create` persistence path (`rendercvYaml` text column, from which
`rendercvConfig` jsonb is re-derived via `ParseRendercv`, same as today for file upload).

**Rationale**: `apps/api/internal/profile/service.go`'s `Create` already treats
`rendercvYaml` as the canonical input and derives `rendercvConfig` from it via
`ParseRendercv`. Reusing this exact flow for structured edits means both entry points
(file upload and form editing) converge on one persistence path, so the two columns can
never drift out of sync with each other.

**Alternatives considered**: Writing `rendercvConfig` jsonb directly from structured edits
and leaving `rendercvYaml` stale/unregenerated — rejected because `rendercvYaml` is used
elsewhere as the literal file representation (e.g. re-download/re-render), and letting it
go stale after in-app edits would violate FR-008's "reliably persisted" expectation and
reintroduce exactly the two-sources-of-truth problem this design avoids.

## 4. Unrecognized data preservation (FR-009)

**Decision**: The mapping layer never deletes keys it doesn't recognize. Any section whose
entries don't match one of the 9 typed shapes (or any per-entry field the mapping layer
doesn't know about) is retained verbatim as opaque JSON, surfaced to a
`UnrecognizedEntryFallbackForm` component that lets the user view/edit it as raw key-value
pairs without the app claiming to "understand" its structure.

**Rationale**: Directly required by FR-009 and the "config upload with unrecognized
fields" edge case. Silent data loss on import is called out explicitly as unacceptable.

**Alternatives considered**: Rejecting import of any config containing unrecognized data —
rejected as it directly contradicts "config file optional/pre-fill convenience" framing;
users with valid-but-unusual configs must not be blocked from importing.

## 5. Reordering UI mechanism

**Decision**: Add `@dnd-kit/sortable` and `@dnd-kit/utilities` (same `@dnd-kit` family as
the already-present `@dnd-kit/core` dependency) for both entry-within-section and
section-within-resume drag reordering, with visible up/down move buttons as a
non-drag fallback for keyboard/accessibility parity.

**Rationale**: `@dnd-kit/core` is already a dashboard dependency (per Technical Context),
so `@dnd-kit/sortable` is an incremental addition within an already-adopted library family
rather than a new dependency choice. Up/down buttons address FR-013's discoverability bar
and basic accessibility without requiring a full drag-and-drop interaction to reorder.

**Alternatives considered**: Building custom drag handling — rejected, reinvents what
`@dnd-kit/sortable` already provides and increases surface area for bugs in a UX-sensitive
interaction.

## 6. Form state management

**Decision**: Use local component state (`useState`/`useReducer`) plus existing
`@tanstack/react-query` mutations, matching the codebase's current pattern (no form
library like `react-hook-form` or `formik` is currently a dashboard dependency).

**Rationale**: Introducing a new form library is a dependency addition not required by the
spec; the existing `hooks.ts` pattern (`useUpdateProfile`, `useUploadConfig` via React
Query) already fits a "load structured resume, mutate, save" flow without one. Per-field
inline validation (FR-007) can be implemented with plain state and derived validation
functions given the bounded, well-typed entry shapes from research item #1.

**Alternatives considered**: Adding `react-hook-form` for validation ergonomics — rejected
as an unnecessary new dependency; the entry/section data shapes are small and finite (9
known types), not deeply dynamic, so a dedicated form library's main benefit
(performance on large dynamic forms) doesn't apply strongly here.
