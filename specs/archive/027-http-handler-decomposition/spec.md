> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/codebase-structure.md`](../../domains/codebase-structure.md) — read that first.

---
# Feature Specification: HTTP Handler Decomposition into Feature Modules

**Feature Branch**: `027-http-handler-decomposition`

**Created**: 2026-07-30

**Status**: Clarified

**Input**: User description: "Split the single flat HTTP package that holds every feature's handlers into per-feature interface layers, leaving the shared package responsible only for routing and shared response helpers."

## Clarifications

### Session 2026-07-30

Resolved with recommended defaults.

- Q: Does a feature with one small endpoint still get a full adapter layer? → A: **Yes.** Recorded
  in Assumptions already; restated here as a decision. Consistency is the entire point — a layout
  with per-feature exceptions is the layout that drifts.
- Q: What happens to handlers whose only dependency is the shared DTO package? → A: **They move
  anyway.** Six of them (`activity`, `contacts`, `hosts`, `notifications`, `postage`, `sources`)
  import nothing but `dto`, which makes them the *easiest* to move, not candidates for staying.
- Q: Where do handlers that reach directly into data access go? → A: **They move, and the
  violation is fixed in the same task, not carried across.** One handler (`roster`) imports
  `db/sqlcgen` and `dbutil` directly. Moving it unchanged would install a layering violation
  inside the feature's own adapter layer, which is worse than leaving it where it is.
- Q: Is the shared DTO package also decomposed? → A: **No — out of scope.** `internal/dto` is a
  cross-cutting contract package consumed by handlers, workers and the tygo generator. Splitting it
  is a separate question entangled with the `packages/shared` duplication problem, and bundling it
  here would make this change unreviewable.
- Q: How is the regression guard implemented? → A: **`depguard` rules in the existing
  `apps/api/.golangci.yml`, plus one placement test.** The linter is already pinned, already wired
  into `make lint-go`, and already gates CI, so the import rules cost nothing new. But `depguard`
  matches import paths, not file locations, so it cannot catch a handler placed inside a feature
  module yet outside its adapter layer — which is half of FR-011. That half is covered by a small
  Go test that walks the module tree and fails on any file importing the router library from
  outside an `interfaces/` package. Roughly twenty lines, runs in the existing suite, no new
  tooling. Relying on `depguard` alone was rejected: it would leave a numbered requirement
  unenforced while appearing to satisfy it.
- Q: What is the adapter package named, given it collides with the standard library's HTTP
  package? → A: **Directory `interfaces/http`, package name `http`, standard library imported
  normally.** This is legal and unambiguous: an import name is file-scoped and the package never
  refers to itself by name, so `http.ResponseWriter` resolves to the standard library inside a
  package that is itself called `http`. Decided here rather than during implementation because it
  applies to all nineteen destination packages and a mid-migration reversal would rename every
  package moved so far. Verify at the first move; if a linter objects, the single fallback is
  `interfaces/httpapi` applied uniformly, decided once.

## Problem Statement

Every HTTP endpoint in the backend lives in one package. That package holds twenty-six source files covering twenty-three separate feature areas, and to do so it imports twenty-four other internal packages — effectively every feature the product has. It is the single point at which all features meet.

This creates a hub. Adding an endpoint to any one feature means editing a package shared by all of them, so unrelated work collides there, and a change to any feature's public surface pulls the shared package's compile graph along with it. It also inverts the layering the rest of the codebase is moving toward: most feature modules already separate their core logic from their adapters, and five of them already have a dedicated adapter layer — but that layer contains only their *background-worker* adapter. Their HTTP adapter, which is the same kind of thing, sits outside the module entirely. The result is a codebase where half of each feature's edge lives inside the feature and half lives in a package that belongs to no feature.

The practical costs are ordinary but constant: a reader tracing one feature must open two distant locations; a reviewer cannot see a feature's change as one diff; a test for one feature's endpoints compiles the whole hub; and the coupling grows automatically, because the path of least resistance for a new endpoint is to add one more file to the pile.

The routing mechanism already supports the fix. Routes are contributed by handlers that each expose a mount function, and the router simply accepts a list of them, so where a handler physically lives is already irrelevant to how it is wired. Nothing structural blocks this change — only the accumulated placement.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A feature's endpoints live with the feature (Priority: P1)

A developer working on one feature finds that feature's request handling, its core logic, and its data access in one place. Adding, changing, or removing an endpoint for that feature touches only that feature's files plus the one line that registers it with the router. A reviewer sees the whole change as one coherent diff.

**Why this priority**: This is the maintainability outcome the whole feature exists to produce. It is independently valuable even if applied to a single feature first, which is also how it should be delivered.

**Independent Test**: Pick one feature, move its request handling into the feature's own adapter layer, and confirm that adding a new endpoint to it requires no edit to any shared package other than the router registration line — and that no other feature's files are touched.

**Acceptance Scenarios**:

1. **Given** a developer is adding an endpoint to an existing feature, **When** they make the change, **Then** they edit only that feature's own files plus the single registration point.
2. **Given** a developer is reading one feature, **When** they open that feature's directory, **Then** they find its request handling alongside its logic, without navigating to a shared package.
3. **Given** a feature is removed entirely, **When** its directory is deleted and its registration removed, **Then** no orphaned request-handling code remains anywhere else.
4. **Given** two developers change two different features at the same time, **When** their changes are integrated, **Then** they do not conflict in a shared handler package.
5. **Given** a feature already has an adapter layer for background work, **When** its request handling is moved, **Then** both adapters sit side by side under that same layer, in a consistent arrangement across features.

---

### User Story 2 - The shared package has one narrow job (Priority: P1)

The remaining shared package is responsible only for assembling routes and for the helpers every handler uses to write responses and errors. It no longer knows what features exist. A reader can understand it in a few minutes.

**Why this priority**: Equal to Story 1, because leaving the shared package broad would mean the handlers moved but the coupling stayed. The measurable end state is that the shared package stops importing feature packages.

**Independent Test**: Inspect the shared package's dependencies after the change and confirm it imports no feature package. Confirm every feature package depends on the shared helpers and not the reverse.

**Acceptance Scenarios**:

1. **Given** the decomposition is complete, **When** the shared routing package's dependencies are inspected, **Then** it imports no feature package.
2. **Given** a feature needs to write a response or an error, **When** it does so, **Then** it uses the shared helpers rather than duplicating that logic.
3. **Given** a new feature is added, **When** it contributes routes, **Then** it does so through the existing registration mechanism without modifying the shared package.
4. **Given** cross-cutting request behaviour such as logging, request identification or error recovery changes, **When** it is updated, **Then** it is updated in one place and applies to every feature.
5. **Given** a developer inspects the dependency direction between the shared package and the features, **When** they check it, **Then** dependencies point from features to shared, never the reverse, and this is checked automatically rather than by convention.

---

### User Story 3 - The externally visible API is unchanged (Priority: P1)

A user of the dashboard, or of the API directly, notices nothing. Every route responds at the same path, with the same methods, the same request and response shapes, and the same error format and status codes as before.

**Why this priority**: Equal priority because this is a pure restructuring; any observable change is a defect, not a feature. Stating it as a story makes it something that gets tested rather than assumed.

**Independent Test**: Capture the complete set of routes, methods, and representative responses before the change; replay them after; confirm no difference.

**Acceptance Scenarios**:

1. **Given** the complete set of routes before the change, **When** the same set is enumerated after, **Then** the paths and methods are identical, including both versioned and unversioned mounts.
2. **Given** a request that succeeds today, **When** it is repeated after the change, **Then** the status code and response body are identical.
3. **Given** a request that fails today, **When** it is repeated after the change, **Then** the status code and error body are identical.
4. **Given** the dashboard's end-to-end suite passes before the change, **When** it runs after, **Then** it passes unmodified.
5. **Given** a route that does not exist, **When** it is requested after the change, **Then** the not-found response is unchanged.

---

### User Story 4 - The arrangement cannot silently regress (Priority: P2)

Once features own their request handling, a later change cannot quietly put a new handler back in the shared package or make the shared package depend on a feature again. The build reports the violation.

**Why this priority**: Lower than the restructuring itself, but this is what makes the work permanent rather than a state the codebase drifts out of within a few months — which is exactly how the current situation arose.

**Independent Test**: Deliberately add a dependency from the shared package to a feature package and confirm the automated check fails and names the offending import.

**Acceptance Scenarios**:

1. **Given** a change introduces a dependency from the shared routing package to a feature package, **When** the automated checks run, **Then** they fail and identify the offending dependency.
2. **Given** a change places request handling outside a feature's own adapter layer, **When** the automated checks run, **Then** they fail and identify where it should live.
3. **Given** a change respects the arrangement, **When** the automated checks run, **Then** they pass without the developer needing to know the rule in advance.
4. **Given** the project documents its module arrangement, **When** a developer or AI agent consults that documentation, **Then** it describes the arrangement the automated check enforces, with no contradiction between them.

---

### Edge Cases

- Two features currently share a helper that is specific to neither. It must move to the shared helpers or be duplicated deliberately, not left in the shared package as a feature-shaped remnant.
- A handler currently reaches into another feature's logic directly. Moving it must not make one feature's adapter depend on another feature's internals; the dependency must go through that feature's own boundary.
- A handler currently constructs data access directly rather than going through its feature's boundary. Moving it must not carry that shortcut into the feature's adapter layer.
- Feature modules that do not yet have an adapter layer must gain one in the same arrangement as those that do, rather than being left flat.
- A test currently living in the shared package covers a specific feature's endpoints; it must move with that feature and continue to run.
- The routing package mounts every route twice, at a versioned and an unversioned path. Both must continue to be produced from a single registration per feature.
- A feature has exactly one small endpoint, so moving it produces a near-empty package. Consistency should win over file-count minimisation, but the decision must be stated rather than made silently per feature.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Each feature's request-handling code MUST reside within that feature's own module, under the same adapter layer that already holds its background-work adapter where one exists.
- **FR-002**: Feature modules that do not yet have an adapter layer MUST gain one in the same arrangement, so the layout is uniform across features.
- **FR-003**: The shared routing package MUST retain responsibility only for assembling routes, for cross-cutting request behaviour, and for shared response and error helpers.
- **FR-004**: The shared routing package MUST NOT depend on any feature module after the change.
- **FR-005**: Every feature MUST contribute its routes through the existing registration mechanism, requiring no modification of the shared package to add a feature.
- **FR-006**: The set of routes, their methods, their request and response shapes, their status codes and their error format MUST be unchanged by this feature.
- **FR-007**: Both the versioned and unversioned route mounts MUST continue to be produced from a single registration per feature.
- **FR-008**: Response and error writing MUST continue to go through shared helpers rather than being duplicated per feature.
- **FR-009**: Tests covering a feature's endpoints MUST move with that feature and MUST continue to run.
- **FR-010**: An automated check MUST fail a change that introduces a dependency from the shared routing package to a feature module.
- **FR-011**: An automated check MUST fail a change that places feature request handling outside the feature's own adapter layer. This has two halves, and they need two mechanisms: handling placed in the *shared routing package* is caught by an import rule, but handling placed *inside a feature module yet outside its adapter layer* is a question of file location, which an import rule cannot express. Both halves MUST be enforced.
- **FR-012**: A feature's request-handling adapter MUST NOT depend on another feature's internals; any cross-feature need MUST go through that feature's own boundary.
- **FR-013**: The project's agent- and contributor-facing documentation MUST describe the arrangement, and MUST match what the automated check enforces.
- **FR-014**: The move MUST be deliverable one feature at a time, with the system fully working after each feature is moved.

### Key Entities

- **Feature module**: A self-contained product capability, owning its core logic, its data-access boundary, and — after this change — both its background-work adapter and its request-handling adapter.
- **Adapter layer**: The part of a feature module that translates between the outside world and the feature's core logic. Currently holds only background-work adapters; gains request-handling adapters.
- **Shared routing package**: The narrow remaining package that assembles routes, applies cross-cutting request behaviour, and provides response and error helpers. Depends on no feature.
- **Route registration**: The single contribution point through which a feature adds its routes to the router, unchanged in mechanism by this feature.
- **Arrangement check**: The automated dependency rule that keeps the layering from regressing.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The shared routing package depends on zero feature modules, down from twenty-four.
- **SC-002**: Adding an endpoint to an existing feature requires editing files in exactly one feature directory, plus at most one registration line.
- **SC-003**: Every route, method, status code and response shape available before the change is available after it, verified by an automated comparison with zero differences.
- **SC-004**: The dashboard's end-to-end suite passes unmodified after the change.
- **SC-005**: A deliberately introduced dependency from the shared package to a feature is rejected by automated checks in 100% of attempts.
- **SC-006**: Every feature module presents the same internal arrangement, with no feature left flat.
- **SC-007**: The work is delivered in increments, each of which leaves the system fully working and independently mergeable.

## Assumptions

- This is a restructuring with no change to observable behaviour; any behavioural change discovered during the work is a pre-existing defect to be reported separately, not fixed silently as part of the move.
- The existing route-registration mechanism is adequate and is not redesigned by this feature.
- Shared response and error helpers stay shared; duplicating them per feature is explicitly rejected.
- The naming of each feature's adapter layer follows the convention already established by the five modules that have one, rather than introducing a new name.
- Cross-cutting request behaviour — logging, request identification, error recovery, cross-origin policy — remains centralised and is not distributed to features.
- Authentication is out of scope: the API has none today, and adding it is a separate concern that this restructuring should make easier rather than pre-empt.
- The consistent-layout question raised in Edge Cases — whether a single-endpoint feature still gets a full adapter layer — is resolved in favour of consistency, and is recorded here rather than decided per feature during implementation.
- Delivery is incremental by feature; a single large change moving all twenty-three feature areas at once is explicitly rejected as unreviewable.
