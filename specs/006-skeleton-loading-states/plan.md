# Implementation Plan: Skeleton Loading States

**Branch**: `006-skeleton-loading-states` | **Date**: 2026-07-24 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-skeleton-loading-states/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command; its definition describes the execution workflow.

## Summary

Replace the dashboard's generic `Spinner` component and inline "Loading…" text with a shared family of skeleton primitives (line/block/circle, shimmer animation) that mirror each page/panel's real layout. Migrate every loading surface (feed, tracker, status, sources, contacts, profile, settings, job-detail sub-panels) to the new primitives, keep `EmptyState`/`ErrorState` untouched, and scope loading to the specific re-fetching section.

## Technical Context

**Language/Version**: TypeScript 5.6, React 19

**Primary Dependencies**: Vite 6, TanStack Query 5, Tailwind CSS 4, `class-variance-authority`, `clsx`/`tailwind-merge` (existing `cn` helper), Radix UI primitives — no new runtime dependency required

**Storage**: N/A (presentational only, no data model change)

**Testing**: Vitest + @testing-library/react (existing `apps/dashboard` unit/component suite); Playwright for `test:e2e` if visual/layout regression coverage is desired

**Target Platform**: Web (dashboard SPA), existing supported browsers

**Project Type**: Web application — frontend-only change within `apps/dashboard`

**Performance Goals**: No regression to page interactivity; skeleton render must not block TanStack Query cache/paint (pure CSS animation, no JS-driven frame loop)

**Constraints**: No layout shift between skeleton and loaded content (CLS-neutral); shared animation timing/tokens across all instances; must not alter existing `EmptyState`/`ErrorState` behavior or timing

**Scale/Scope**: ~25 files in `apps/dashboard/src/features/**` and `apps/dashboard/src/components/ui.tsx` currently importing `Spinner` or rendering inline loading text

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **I. No Auto-Apply, Ever** — N/A. Purely presentational loading-state change; no application-submission code path touched.
- **II. Grounded Generation** — N/A. No LLM-generated content involved.
- **III. Typed Contracts Across Service Boundaries** — PASS. No new cross-language boundary; skeleton primitives are dashboard-internal React components, no `packages/shared` or sqlc/tygo changes needed.
- **IV. Test Discipline Per Language, Enforced at the Boundary** — PASS (must maintain). Change is single-app (`apps/dashboard`), so `vitest` suite is the required gate; no cross-service integration/e2e requirement triggered since no API/DB behavior changes. Existing component tests referencing `Spinner`/loading text must be updated alongside the migration.
- **V. Local-First, Self-Hosted by Default** — N/A. No AI/inference path involved.

No violations. Complexity Tracking table not needed.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
apps/dashboard/
├── src/
│   ├── components/
│   │   ├── ui.tsx                 # add Skeleton primitives here (SkeletonLine, SkeletonBlock, SkeletonCircle); Spinner stays only if still used elsewhere, else removed
│   │   └── ui.test.tsx            # existing/new tests for new primitives (create if absent)
│   └── features/
│       ├── feed/FeedPage.tsx              # replace Spinner with skeleton job-card list
│       ├── tracker/TrackerPage.tsx        # replace Spinner with skeleton row list
│       ├── status/StatusPage.tsx          # replace Spinner with skeleton activity list
│       ├── sources/SourcesPage.tsx        # replace Spinner with skeleton
│       ├── contacts/ContactsPage.tsx      # replace Spinner with skeleton
│       ├── profile/ProfilePage.tsx        # replace Spinner with skeleton
│       ├── settings/SettingsPage.tsx, AiFeatureSettingsCard.tsx, LlmSettingsCard.tsx
│       ├── tailor/TailorPage.tsx
│       └── job-detail/                    # CoachPanel, OutreachPanel, ReferralPathsCard,
│                                           # CompanyIntelCard, GhostSignalPanel, KeywordDiffPanel,
│                                           # PrepPackPanel, ContactLine, PostAgeSignal, JobDetailPage
└── (each *.test.tsx co-located with the file above updated for new markup)
```

**Structure Decision**: Frontend-only change inside the existing `apps/dashboard` React app (this repo's monorepo already separates `apps/api` (Go), `apps/dashboard` (React), `apps/jobspy-sidecar` (Python), `packages/shared` (TS types)). New skeleton primitives live in the existing shared `components/ui.tsx` module alongside `Spinner`/`EmptyState`/`ErrorState`; no new top-level directory, no `packages/shared` change, no backend touch.

## Complexity Tracking

*No constitution violations — table not needed.*
