# Implementation Plan: Fully Editable Resume Profile Tab

**Branch**: `009-editable-resume-profile` | **Date**: 2026-07-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/009-editable-resume-profile/spec.md`

## Summary

Rewrite the dashboard Profile tab so the whole resume — identity fields plus all nine
RenderCV entry types (education, experience, normal/project, publication, one-line/skill,
bullet, numbered, reversed-numbered, text) — is directly, structurally editable, with
add/edit/delete/reorder for both entries and sections. Uploading a RenderCV YAML config
remains a one-time, optional pre-fill convenience, not a requirement, and overwrite is
gated behind confirmation. Storage stays on the existing `Profile.rendercvConfig` (jsonb)
/ `Profile.rendercvYaml` (text) columns: the client edits a typed, structured document; the
API reconstructs the master YAML map from structured edits, runs it back through the
already-in-flight `PrepareMasterForMarshal`/`_order`-preserving pipeline in
`apps/api/internal/generation`, and persists both columns from that single canonical
representation. No new tables or migrations are required.

## Technical Context

**Language/Version**: Go (apps/api, matches existing module), TypeScript 5 / React 19 (apps/dashboard)

**Primary Dependencies**: chi-style HTTP mux + sqlc + goose + tygo (backend, existing);
React + Vite + TanStack Query + Tailwind v4 + `@dnd-kit/core` (dashboard, existing).
New: `@dnd-kit/sortable` + `@dnd-kit/utilities` for entry/section reordering (same
`@dnd-kit` family already in use, no new drag-and-drop library introduced).

**Storage**: PostgreSQL — reuses existing `Profile.rendercvConfig` (jsonb) and
`Profile.rendercvYaml` (text) columns from migration `00005_profile_rendercv.sql`. No
schema change.

**Testing**: `go test ./...` for apps/api (new handler + mapping unit tests); `vitest run`
for apps/dashboard (new form/section/entry component tests); `make test-lint` before merge
since the change spans both apps, per Constitution Principle IV.

**Target Platform**: Existing Docker Compose dev/prod stack; browser-based dashboard, Linux-hosted Go API.

**Project Type**: Web application (existing two-app layout: `apps/api` + `apps/dashboard`, shared types in `packages/shared`).

**Performance Goals**: Interactive form UX only — no throughput/latency target beyond
"stays responsive with 50+ entries in a section" (spec edge case), verified qualitatively,
not benchmarked.

**Constraints**:
- Must not require a config file for any editing capability (FR-001, FR-002).
- Must not clobber the in-flight `_order` / `PrepareMasterForMarshal` section-ordering work
  in `apps/api/internal/generation/rendercv.go` — structured edits must round-trip through
  it, not bypass it.
- Must preserve unrecognized/unmapped config data via a fallback editor rather than
  drop it (FR-009).
- Must gate destructive actions and config-overwrite-on-reupload behind explicit
  confirmation (FR-010, FR-011).

**Scale/Scope**: Single-user, self-hosted; one resume document per Profile row; a handful
of Profile rows per user. No multi-tenant or high-concurrency concerns.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — N/A. This feature only edits resume source data; it has no
  code path that submits applications or contacts employers. PASS.
- **II. Grounded Generation** — N/A directly (no LLM call in this feature), but relevant
  downstream: this resume data *is* the "master profile data" other generation features
  must stay grounded in (per constitution). Plan does not change how generation consumes
  the profile, only how the profile is authored. PASS.
- **III. Typed Contracts Across Service Boundaries** — Currently `ProfileDto.rendercvFull`
  is untyped `any` in `packages/shared/src/generated.ts`; this is the gate most at risk.
  Plan requires new typed Go DTOs (one struct per RenderCV entry type + Section + Resume
  document) under `apps/api/internal/dto`, regenerated to TS via `tygo generate`
  (`apps/api/tygo.yaml` → `packages/shared/src/generated.ts`) so the dashboard never
  hand-declares these shapes. PASS, contingent on Phase 1 data-model actually being
  expressed as tygo-eligible Go structs (addressed in data-model.md).
- **IV. Test Discipline Per Language, Enforced at the Boundary** — Plan adds Go unit tests
  for structured-edit → YAML-map reconstruction and Go handler tests (`go test`), and
  Vitest component tests for the new form/section/entry UI. `make test-lint` required
  before merge since both apps change. PASS.
- **V. Local-First, Self-Hosted by Default** — N/A. No external AI/API calls introduced. PASS.

No violations requiring Complexity Tracking — the design deliberately keeps the existing
JSONB-blob-per-profile storage model rather than introducing relational per-entry tables,
to avoid unrequested architectural expansion.

## Project Structure

### Documentation (this feature)

```text
specs/009-editable-resume-profile/
├── plan.md              # This file
├── research.md           # Phase 0 output
├── data-model.md         # Phase 1 output
├── quickstart.md         # Phase 1 output
├── contracts/            # Phase 1 output
│   └── profile-resume-api.md
└── tasks.md              # Phase 2 output (/speckit-tasks — not created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── dto/
│   │   └── resume.go                # NEW: typed Go structs for Resume/Section/each of
│   │                                 #   the 9 RenderCV entry types + tygo-generated TS
│   ├── generation/
│   │   ├── rendercv.go               # EXISTING (in-flight): _order / PrepareMasterForMarshal
│   │   ├── rendercv_config.go        # EXISTING (in-flight): ParseRendercv + order capture
│   │   └── resume_mapping.go         # NEW: structured dto.Resume <-> map[string]any
│   │                                 #   (RendercvMaster["cv"]) conversion, built on top of
│   │                                 #   ParseRendercv/PrepareMasterForMarshal, preserving
│   │                                 #   unrecognized fields per section/entry
│   ├── profile/
│   │   └── service.go                # EXTEND: structured update path (build YAML from
│   │                                 #   dto.Resume, reuse existing Create/Update persistence)
│   └── httpapi/
│       └── profiles.go               # EXTEND: routes for structured resume read/update
│                                     #   distinct from the existing whole-YAML upload route
└── tygo.yaml                         # EXISTING: add apps/api/internal/dto to generated packages if not already covered

apps/dashboard/
└── src/features/profile/
    ├── ProfilePage.tsx                # REWRITE: orchestrates identity form + section list
    ├── hooks.ts                       # EXTEND: structured resume query/mutation hooks
    ├── components/
    │   ├── IdentityForm.tsx           # NEW: name/headline/location/contact/social links
    │   ├── SectionList.tsx            # NEW: reorderable section list (dnd-kit sortable)
    │   ├── SectionEditor.tsx          # NEW: section rename/delete/add-entry chrome
    │   ├── entries/
    │   │   ├── EducationEntryForm.tsx
    │   │   ├── ExperienceEntryForm.tsx
    │   │   ├── NormalEntryForm.tsx        # projects
    │   │   ├── PublicationEntryForm.tsx
    │   │   ├── OneLineEntryForm.tsx       # skills
    │   │   ├── BulletEntryForm.tsx        # e.g. honors
    │   │   ├── NumberedEntryForm.tsx      # e.g. patents
    │   │   ├── ReversedNumberedEntryForm.tsx # e.g. invited talks
    │   │   ├── TextEntryForm.tsx
    │   │   └── UnrecognizedEntryFallbackForm.tsx  # FR-009
    │   └── ConfirmDialog.tsx          # NEW: shared confirm for delete + reupload-overwrite
    └── uploadConfig (existing flow retained, now explicitly optional pre-fill only)

packages/shared/src/generated.ts        # REGENERATED via `tygo generate`, not hand-edited
```

**Structure Decision**: Existing two-app web layout (Option 2) is reused as-is —
`apps/api` (Go) + `apps/dashboard` (React) + `packages/shared` (generated TS DTOs). No new
apps, packages, or services. All new backend logic is added to existing
`internal/dto`, `internal/generation`, `internal/profile`, `internal/httpapi` packages;
all new frontend logic is added under the existing `features/profile` module.

## Complexity Tracking

*No entries — Constitution Check has no unjustified violations.*
