# Feature Specification: Enforced Workflow Quality Gates

**Feature Branch**: `023-workflow-quality-gates`

**Created**: 2026-07-28

**Status**: Draft

**Input**: User description: "Harden the AI agent development workflow with real, enforced quality gates (Tier A). Scope: (1) Agent work must never land directly on master — every agent-authored change goes on a feature branch and merges via a PR whose CI is green; master gets branch protection. (2) Make `make test-lint` actually lint: add a golangci-lint config for apps/api and an ESLint config for apps/dashboard, expose `lint-go` and `lint-web` make targets, and redefine `test-lint` as lint-go + lint-web + test-go + test-react; fix AGENTS.md which currently claims the suite covers Python (it does not). (3) Add CI jobs that run the integration suite (`go test -tags integration` against real Postgres+Redis service containers) and the Playwright e2e suite, so Constitution Principle IV is actually enforced instead of only asserted; e2e may run on a nightly schedule if too slow for every PR. (4) Add committed project-level Claude Code hooks in .claude/settings.json: PostToolUse on Go file edits runs gofmt and go vet; PostToolUse on apps/api/internal/db/queries/*.sql runs `make sqlc-generate`; PostToolUse on apps/api/internal/dto/*.go runs `make tygo-generate`; Stop hook runs `make test-lint` and blocks on failure. Goal: codegen drift and unformatted/unlinted/untested code become structurally impossible rather than a thing the agent must remember. Success is measured by: no direct-to-master commits, CI red before merge instead of after, and sqlc/tygo generated files never stale in a commit."

## Problem Statement

Today the repository's quality gates are asserted but not enforced. All 309 commits have landed directly on the trunk; automated checks run only *after* a change is already there, so a broken change is discovered after it has become the shared baseline. The command the project documents as its merge gate performs no style or static analysis at all — it is an alias for the two unit-test suites, and no linter configuration exists for either application. The integration and end-to-end suites exist as make targets but no automated run ever invokes them, despite the project constitution requiring that cross-service behaviour be exercised against real infrastructure. Nothing mechanically prevents an author — human or AI agent — from committing generated code that no longer matches its source, unformatted code, or code that fails the test suite.

The consequence is that correctness depends on the author remembering a checklist. AI coding agents are especially poor at this: they optimise for the visible task and skip the invisible ritual. This feature converts the project's stated quality rules into mechanisms that fail loudly and early.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Changes are proven before they become the baseline (Priority: P1)

An author — a person or an AI coding agent — finishes a change. Instead of writing it onto the trunk, they place it on a dedicated branch and open a change request. Automated checks run against the change while it is still isolated, and report their result before anyone considers integrating. If the checks fail, the trunk is unaffected and the author fixes the change on the branch.

**Why this priority**: This is the single structural change that makes every other gate meaningful. Without isolation before integration, every other check is a post-mortem rather than a gate. It also bounds the blast radius of an AI agent's mistake to a branch nobody else is standing on.

**Independent Test**: Attempt to write a change directly to the trunk; the attempt is rejected. Open a change request containing a deliberately failing test; verify the failure is visible before integration. This delivers value on its own even if no new checks are added, because the existing checks stop being post-mortems.

**Acceptance Scenarios**:

1. **Given** an author has a completed change, **When** they attempt to write it directly onto the trunk, **Then** the write is rejected and the author is directed to use a branch and change request.
2. **Given** an author needs to repair a broken trunk, **When** they invoke the documented override, **Then** the write succeeds and leaves a visible trace.
3. **Given** a change request whose automated checks have failed, **When** the author views it, **Then** the failing check is identified before integration, and the trunk still holds the last good state.
4. **Given** a change request whose automated checks have all passed, **When** the author integrates it, **Then** the change joins the trunk successfully.
5. **Given** an AI agent is asked to implement a feature, **When** it begins work, **Then** it creates a branch first and never writes to the trunk, because the repository's agent instructions state this as a rule and the local gate rejects direct writes regardless.
6. **Given** the hosting plan is later upgraded, **When** the recorded configuration is applied, **Then** integration of a change request with pending or failing checks becomes mechanically impossible, with no other change to the workflow.

---

### User Story 2 - The documented quality command actually checks quality (Priority: P1)

An author runs the single command the project documents as "is my change acceptable". That command checks code style and common defect patterns in both applications, in addition to running both unit-test suites. When the command reports success, the author can trust that a change-request check will not subsequently fail for a style or static-analysis reason. When it reports failure, the report identifies the file, the location, and the rule that was violated.

**Why this priority**: The project constitution names this command as the boundary-crossing merge gate, and the repository's agent instructions tell agents to run it. Both currently point at a command that silently checks less than it claims. A gate that reports success without checking is worse than no gate, because it manufactures false confidence.

**Independent Test**: Introduce a deliberate style violation and a deliberate static-analysis violation into each application, run the quality command, and confirm each violation is reported with its location. This is testable and valuable independently of the branch workflow.

**Acceptance Scenarios**:

1. **Given** the backend application contains code with an unused variable or a suspicious construct, **When** the author runs the quality command, **Then** it fails and names the offending file and line.
2. **Given** the dashboard application contains code violating the configured style or correctness rules, **When** the author runs the quality command, **Then** it fails and names the offending file and line.
3. **Given** both applications are clean, **When** the author runs the quality command, **Then** it runs style checks and both unit-test suites and reports success.
4. **Given** an author wants to check only one application, **When** they run that application's dedicated style-check command, **Then** only that application is checked.
5. **Given** the repository's agent instructions describe what the quality command covers, **When** a reader consults them, **Then** the description matches what the command actually runs, including no claim of coverage for languages the repository does not contain.

---

### User Story 3 - Cross-service behaviour is verified against real infrastructure (Priority: P2)

Automated checks on a change request exercise the backend's integration suite against a real database and a real queue, not substitutes. A separate automated run exercises the dashboard end-to-end against a running stack. A change that breaks a migration, a query, or a cross-application contract is caught by automation rather than by a person noticing later.

**Why this priority**: The project constitution requires this explicitly, and it is the category of defect most likely to be produced by an AI agent and least likely to be caught by unit tests — schema drift, migration ordering, and contract mismatch between the two runtimes. It is P2 rather than P1 only because it depends on the change-request workflow existing first to be worth anything.

**Independent Test**: Introduce a migration that conflicts with an existing query, open a change request, and confirm the integration check fails. Introduce a change that breaks a primary dashboard flow and confirm the end-to-end check fails.

**Acceptance Scenarios**:

1. **Given** a change request that alters a database migration or query, **When** automated checks run, **Then** the integration suite executes against a real database and queue and its result gates integration.
2. **Given** the integration suite requires a database that does not yet exist, **When** the automated check starts, **Then** it provisions the database and waits for it to accept connections before running tests.
3. **Given** a change breaks a primary user flow in the dashboard, **When** the end-to-end suite runs, **Then** it fails and identifies the failing flow.
4. **Given** the end-to-end suite is too slow to run on every change request, **When** it is scheduled instead, **Then** it runs at least daily and on demand, and its failures are surfaced to the author.
5. **Given** an automated check fails because of infrastructure flakiness rather than the change, **When** the author re-runs the check, **Then** it can be re-run without altering the change.

---

### User Story 4 - Generated artefacts cannot go stale and code cannot be left unformatted (Priority: P2)

While an AI agent works, the environment repairs and verifies the things the agent would otherwise forget. Editing a database query regenerates the typed database layer. Editing a backend data-transfer type regenerates the shared type definitions consumed by the dashboard. Editing backend source formats it and runs static analysis on it. When the agent finishes a work session, the quality command runs and a failure prevents the session from being reported as complete.

**Why this priority**: The two most frequently observed drift classes in this repository are stale generated database code and stale generated shared types — both are already policed by change-request checks, so today they cause a failed check and a round trip. Repairing them at edit time removes the round trip entirely and shortens the feedback loop from minutes to seconds. It is P2 because the change-request checks already catch these; this story makes catching them cheaper, not newly possible.

**Independent Test**: Edit a database query file without regenerating, and confirm the typed database layer is regenerated automatically. Edit a backend data-transfer type and confirm shared types are regenerated. Write badly formatted backend code and confirm it is formatted. End a work session with a failing test and confirm the session is not reported complete.

**Acceptance Scenarios**:

1. **Given** an agent edits a database query definition, **When** the edit completes, **Then** the generated typed database layer is regenerated without the agent being asked.
2. **Given** an agent edits a backend data-transfer type, **When** the edit completes, **Then** the generated shared type definitions are regenerated without the agent being asked.
3. **Given** an agent writes backend source that does not match the language's canonical formatting, **When** the edit completes, **Then** the file is reformatted and static analysis runs on the affected package.
4. **Given** an agent's work has left the test suite failing, **When** the agent attempts to end its work session, **Then** the session is blocked and the failure is reported back to the agent for repair.
5. **Given** an automatic regeneration or check fails because a required tool is not installed, **When** the failure occurs, **Then** the author is told which tool is missing and how to install it, and the failure does not silently corrupt any file.
6. **Given** these behaviours are defined in repository configuration, **When** a new contributor or agent clones the repository, **Then** the behaviours apply without per-machine setup, because the configuration is committed rather than local-only.

---

### Edge Cases

- **Emergency repair of a broken trunk**: if the trunk is broken and the change-request path is itself blocked, a documented, auditable override path must exist so the repository is never unrecoverable. The override must be a deliberate, visible act rather than a silent bypass.
- **Solo maintainer**: the repository has one maintainer, so a change request cannot require approval from a second person. The gate is the automated checks, not human review; a maintainer may integrate their own change request once checks pass.
- **Pre-existing violations**: enabling linters on an existing codebase will surface a backlog of violations. Adopting the gate must not require fixing every historical violation in the same change, or the feature will never land.
- **Automatic regeneration produces a change the author did not expect**: regeneration must be visible in the working tree so the author reviews it before it is committed, never applied invisibly at commit time.
- **Session-end check is slow**: if the full suite takes minutes, running it at every session end will be abandoned by its users. The check must be fast enough to tolerate or scoped to what was touched.
- **Tooling absent in the environment**: linters and code generators are external binaries. Every gate must degrade to a clear, actionable error rather than a silent pass when its tool is missing.
- **Concurrent worktrees**: the repository uses multiple git worktrees sharing one host. Checks that provision infrastructure must not collide across worktrees.
- **Generated files failing the linter**: machine-generated code should not be held to hand-written code's style rules; linters must exclude generated output or the gate will produce permanent, unfixable noise.
- **In-flight work at adoption time**: uncommitted or branch-resident work that predates the gate must have a defined path onto the trunk under the new rules.

## Requirements *(mandatory)*

### Functional Requirements

#### Isolation and integration control

- **FR-001**: The trunk MUST reject direct writes; all changes MUST arrive via a change request. Enforcement is local (see Assumptions): the rejection happens in the author's own environment, and the bypass in FR-005 is the same mechanism that provides the override.
- **FR-002**: Every automated check MUST run on a change request and MUST report its result before integration, so a failing change is visibly red while it is still isolated. **Mechanically preventing integration of a red change request is out of scope for this feature** — the hosting plan does not offer it (see Assumptions). The gate is therefore: checks run, results are visible, and the maintainer does not integrate red. The configuration that would enforce this mechanically MUST be recorded, ready to apply, so the upgrade is a single action rather than a rediscovery.
- **FR-003**: The set of checks intended to gate integration MUST be explicitly declared in one place, and adding or renaming one MUST be a deliberate act rather than an accident of naming. This declaration is what the recorded configuration in FR-002 consumes.
- **FR-004**: When mechanical enforcement is adopted, it MUST NOT require a second person's approval, so the workflow does not deadlock on a one-person project. Until then this is satisfied trivially.
- **FR-005**: A documented override path MUST exist for emergency repair, and using it MUST leave a visible trace.
- **FR-006**: The repository's agent instructions MUST state that agents create a branch and open a change request, and never write to the trunk.

#### Style and static analysis

- **FR-007**: The backend application MUST have a committed style and static-analysis configuration, and a dedicated command that runs it.
- **FR-008**: The dashboard application MUST have a committed style and static-analysis configuration, and a dedicated command that runs it.
- **FR-009**: The project's documented quality command MUST run both style checks and both unit-test suites, and MUST fail if any of the four fails.
- **FR-010**: Style and static-analysis configurations MUST exclude machine-generated files from hand-written-code rules.
- **FR-011**: Style checks MUST report each violation with file, line, and the rule violated.
- **FR-012**: Initial adoption MUST NOT require clearing the entire backlog of pre-existing violations in the same change; the rule set MUST be scoped so the gate can be turned on immediately and tightened later. The backlog MUST be measured per rule before the rule set is fixed, and the scoping decision MUST follow a stated threshold rather than judgement in the moment: **every rule with zero violations is enabled; a rule with 1–30 violations is fixed and enabled, up to a total of 80 violations fixed across both languages; any rule exceeding either limit is deferred with its measured count recorded in the configuration.** The thresholds exist so the decision is reproducible and so the feature cannot quietly expand into a cleanup project.
- **FR-013**: The repository's agent instructions MUST accurately describe what the quality command covers, and MUST NOT claim coverage for languages absent from the repository.
- **FR-014**: Style checks MUST run on every change request and appear in the declared gating set of FR-003.
- **FR-015**: Style and static-analysis tools MUST be version-pinned so a local run and an automated run reach the same verdict.

#### Real-infrastructure verification

- **FR-016**: Automated checks MUST run the backend integration suite against a real database and a real queue, provisioned for the run.
- **FR-017**: The integration check MUST create any database the suite requires and wait for readiness before running tests, so it does not fail on a cold environment.
- **FR-018**: The integration check MUST run on every change request and appear in the declared gating set of FR-003. Like every other check, it cannot mechanically block integration until the hosting plan supports it (FR-002).
- **FR-019**: The end-to-end suite MUST run automatically on a recurring schedule of at least once daily, and MUST be triggerable on demand.
- **FR-020**: End-to-end failures MUST be surfaced where the maintainer will see them without polling.
- **FR-021**: Any automated check MUST be re-runnable without modifying the change under test.

#### Edit-time repair and session-end verification

- **FR-022**: Editing a database query definition MUST trigger regeneration of the typed database layer.
- **FR-023**: Editing a backend data-transfer type MUST trigger regeneration of the shared type definitions.
- **FR-024**: Editing backend source MUST trigger canonical formatting of that source and static analysis of the affected scope.
- **FR-025**: Ending an agent work session MUST run the project's quality verification, and a failure MUST block the session from being reported complete and MUST report the failure back to the agent.
- **FR-026**: Automatic actions MUST leave their results visible in the working tree for review, and MUST NOT rewrite files at commit time.
- **FR-027**: Every automatic action MUST fail with an actionable message naming the missing prerequisite when its tool is unavailable, and MUST NOT leave a file partially written.
- **FR-028**: These automatic behaviours MUST be defined in committed repository configuration so they apply to every clone and every worktree without per-machine setup.
- **FR-029**: Automatic actions MUST be scoped to the files actually edited, so that editing one file does not trigger repository-wide work.
- **FR-030**: Automatic actions MUST NOT trigger recursively on the files they themselves generate.

### Key Entities

- **Change request**: a proposed set of changes held outside the trunk, carrying the reported results of the automated checks that inform whether it should be integrated.
- **Declared gating set**: the named automated verifications that must be green before a change is integrated, recorded in exactly one place. It is the machine-readable form of the project's definition of "done" — and the input the host-side rule consumes once the hosting plan supports one. Today it is enforced by the maintainer reading it, not by the host.
- **Quality command**: the single documented entry point an author or agent runs to answer "is my change acceptable"; its coverage must equal the coverage of the declared gating set minus the checks that need infrastructure, or the local and automated verdicts will disagree.
- **Edit-time action**: an automatic repair or verification bound to a class of file edit, executing while the author is still working.
- **Session-end verification**: an automatic check bound to the end of an agent work session, gating the claim that work is finished.
- **Override**: a deliberate, traceable bypass of the local gate, reserved for restoring a broken trunk. Because enforcement is client-side, the override is the same mechanism that makes the gate bypassable — which is why it must leave a trace rather than be made impossible.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero changes reach the trunk without having passed the declared check set, measured across all changes following adoption. Enforced by the local gate plus the maintainer declining to integrate red, not by the host.
- **SC-002**: 100% of attempts to write directly to the trunk are rejected by the local gate, excluding deliberate use of the documented override.
- **SC-003**: A change containing a style violation, a static-analysis violation, a failing unit test, a failing integration test, or stale generated output produces a **visibly failing check** on its change request in every case, before integration.
- **SC-004**: A style or static-analysis violation is reported to the author within 60 seconds of running the quality command locally, so the gate is fast enough to run habitually rather than avoided.
- **SC-005**: An author who runs the quality command locally and sees success is not subsequently surprised by a failing style, static-analysis, or unit-test check on their change request — local and automated results agree in at least 95% of cases.
- **SC-006**: Generated database code and generated shared type definitions are never stale relative to their sources in any change that reaches the trunk.
- **SC-007**: An agent editing a query definition or a data-transfer type has the corresponding generated output refreshed without being instructed to do so, in 100% of such edits.
- **SC-008**: Cross-service behaviour is exercised against real infrastructure on every change request, versus zero automated runs today.
- **SC-009**: The end-to-end suite runs at least once per day without human initiation.
- **SC-010**: The repository's agent instructions contain no statement about the quality command's coverage that the command does not satisfy.
- **SC-011**: Session-end verification adds no more than 2 minutes to an agent work session, so it is not disabled by its users for being slow.
- **SC-012**: The trunk can be restored from a broken state within one hour using the documented override, so the gate never makes the repository unrecoverable.

## Assumptions

- **The host cannot block a merge on this project's plan.** The repository is private on a free tier, where both the ruleset and branch-protection APIs refuse with "Upgrade to GitHub Pro or make this repository public". Enforcement is therefore client-side: committed git hooks reject commits and pushes to the trunk, and an agent-level hook stops the attempt earlier still. Automated checks still run on every change request and report their result; what is missing is the host refusing the merge button. The exact configuration to apply after an upgrade is recorded as a ready-to-run command. This was a deliberate choice over paying for the plan or making the repository public.
- The client-side gate is bypassable by design — the same bypass is the emergency override of FR-005, and its use is visible in shell history and in an agent transcript.
- The project has a single maintainer, so integration is gated by automation and by that maintainer's judgement rather than by peer approval; self-integration after green checks is the intended flow.
- Style and static-analysis tooling for both languages will be pinned to explicit versions, consistent with how the project already pins its code generators, so local and automated runs produce identical verdicts.
- The initial linter rule sets will be conservative — correctness and defect-pattern rules first, aggressive stylistic rules deferred — and scoped by the numeric thresholds in FR-012 so the gate can be enabled without a large cleanup change blocking it.
- The end-to-end suite turned out to need no database, queue or backend — it mocks its API calls — so it runs on every change request **and** on a daily schedule. An earlier draft of this spec assumed it was too slow for per-request use; measurement contradicted that, and the delivered behaviour exceeds FR-019. The integration suite is fast enough to run on every change request.
- Session-end verification will be scoped to the applications actually touched during the session rather than always running everything, in order to meet the 2-minute budget.
- Edit-time regeneration relies on the code generators already pinned and used by the project; no new generator is introduced.
- Automatic actions run in the author's local environment; when a required tool is absent the action reports the gap rather than attempting to install anything.
- Existing worktree isolation for provisioned infrastructure is assumed to remain in force, so concurrent checks do not collide.
- Work already in flight when the gate is adopted will be moved onto a branch and integrated through a change request like any other change.
- The project constitution is the authority on what must be verified; this feature adds no new rules, it only converts existing rules from prose into mechanism.

## Out of Scope

- Resolving the contradiction between the constitution and the agent instructions regarding hand-maintained shared type definitions, and consolidating the project onto a single agent tooling stack — these are the subject of a separate feature.
- Restructuring the specification pipeline, spec lifecycle tracking, worktree and branch cleanup, and permission-list hygiene — deferred.
- Fixing the existing backlog of style violations beyond what is required to make the gate pass on a conservative rule set.
- Adding new test coverage; this feature runs the suites that already exist.
- Changing what the integration or end-to-end suites assert.
