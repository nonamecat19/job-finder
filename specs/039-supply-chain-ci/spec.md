# Feature Specification: Supply-chain and build-integrity CI gates

**Feature Branch**: `039-supply-chain-ci`

**Created**: 2026-08-08

**Status**: Draft

**Input**: User description: "Supply-chain and build-integrity CI gates. Add four missing merge gates to the existing GitHub Actions workflow so dependency drift, known vulnerabilities, leaked secrets, and broken container builds cannot merge unnoticed: (1) Dependabot config covering the Go module in apps/api, the pnpm workspace, GitHub Actions versions, and the Docker base images; (2) a govulncheck job for apps/api and a pnpm audit gate for the workspace; (3) a gitleaks secret-scan job over the diff, given the repo handles CONFIG_ENCRYPTION_KEY plus four LLM provider API keys; (4) a job that actually builds both container images (apps/api/Dockerfile, apps/dashboard/Dockerfile) so a broken Dockerfile cannot merge green. Must respect the existing dorny/paths-filter change-detection design in .github/workflows/api-ci.yml — jobs skip (not vanish) when irrelevant paths change, so required checks still report. Pure config/CI work, no application code."

## Problem

The merge gate today proves that the code compiles, lints, type-checks, and passes its
tests. It proves nothing about the *supply chain* around that code or about whether the
thing that actually ships — a container image — can still be built.

Four classes of defect can reach `master` today with every check green:

1. **Dependency drift.** 82 direct Go module requirements plus a pnpm workspace, all
   updated only when somebody remembers. No automation opens update pull requests, so
   the tree silently ages between manual sweeps.
2. **Known vulnerabilities.** Nothing checks a dependency against a published advisory.
   A CVE disclosed against a pinned version is invisible until a human reads a
   newsletter.
3. **Leaked secrets.** The project handles `CONFIG_ENCRYPTION_KEY` and four LLM provider
   API keys (`CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY`).
   `.env` is gitignored and `gateway/config.yaml` has a test asserting no literal
   credential appears in *that one file*, but no gate covers the rest of the tree, a new
   fixture, a pasted log line, or a docs snippet.
4. **Broken container builds.** Both Dockerfiles are exercised only by a human running
   `docker compose up`. A change that breaks `apps/api/Dockerfile` or
   `apps/dashboard/Dockerfile` merges green and is discovered at deploy time.

The existing workflow's change-detection design (`dorny/paths-filter` feeding `if:`
conditions, deliberately *not* a top-level `on: paths:` filter, so skipped jobs still
report a status) is load-bearing for the branch-protection ruleset recorded in
`specs/domains/platform-operations.md` § 2.2. Any new gate has to participate in that
design rather than work around it.

## User Scenarios & Testing *(mandatory)*

The "user" for this feature is the maintainer of the repository and any coding agent
working in it. The product surface is the pull-request check list.

### User Story 1 - A broken container build is caught before merge (Priority: P1)

A contributor changes a build-time dependency, a Go build tag, an nginx config path, or
the shape of the pnpm workspace, and the change compiles and tests fine but breaks the
container build. Today that merges green. With this feature the pull request shows a
failed image-build check naming which of the two images failed and at which build stage.

**Why this priority**: it is the only one of the four that catches a defect the current
gate actively hides — a green merge that cannot be deployed. The other three surface
risk that is at least *latent* rather than immediately breaking.

**Independent Test**: open a pull request that deliberately breaks one Dockerfile (for
example, reference a build stage that does not exist). The image-build check must fail
and name that image; every other check must be unaffected.

**Acceptance Scenarios**:

1. **Given** a pull request whose changes break `apps/api/Dockerfile`, **When** CI runs,
   **Then** the image-build check fails and its log identifies the API image and the
   failing build step.
2. **Given** a pull request that touches only `specs/**` or `docs/**`, **When** CI runs,
   **Then** the image-build check is reported as skipped, not as missing or failed.
3. **Given** a pull request that touches only `apps/dashboard/**`, **When** CI runs,
   **Then** the dashboard image is built and the API image build is skipped.

---

### User Story 2 - A known vulnerability blocks the merge (Priority: P1)

A dependency in use has a published advisory affecting the pinned version. The
contributor learns this from a failing check on their own pull request, with the
advisory identifier and the affected symbol or package named, rather than from an
incident months later.

**Why this priority**: equal-first because it is the gate whose absence has unbounded
downside — an exploitable dependency shipped into a self-hosted deployment that handles
the user's résumé data and provider credentials.

**Independent Test**: pin a dependency to a version with a known published advisory on a
scratch branch; the vulnerability check must fail and name the advisory. Revert; it must
pass.

**Acceptance Scenarios**:

1. **Given** the Go module depends on a version with a published advisory that the code
   actually reaches, **When** CI runs, **Then** the Go vulnerability check fails and
   names the advisory identifier and the calling path.
2. **Given** an advisory exists against a Go dependency but no reachable call path into
   the vulnerable symbol, **When** CI runs, **Then** the check passes and the finding is
   visible in the log as informational rather than failing the build.
3. **Given** a workspace JavaScript dependency has an advisory at or above the configured
   severity floor, **When** CI runs, **Then** the JavaScript audit check fails and names
   the package and severity.
4. **Given** an advisory exists only below the severity floor, **When** CI runs, **Then**
   the audit check passes and lists the finding without failing.

---

### User Story 3 - A committed secret never reaches master (Priority: P1)

A contributor pastes a real provider key into a fixture, a docs example, a debug log, or
a compose override. The secret-scan check fails on that pull request, so the key never
lands in `master`'s history where rotation would be the only remedy.

**Why this priority**: equal-first because it is the only failure mode in this feature
that is *irreversible*. A broken build is fixed by a commit; a vulnerable dependency is
fixed by a bump; a key published to git history must be rotated, and every clone and
fork keeps a copy.

**Independent Test**: open a pull request adding a file containing a syntactically valid
provider key of a shape the scanner recognises. The secret-scan check must fail and name
the file, the line, and the rule that matched.

**Acceptance Scenarios**:

1. **Given** a pull request that adds a recognised secret pattern to any tracked file,
   **When** CI runs, **Then** the secret-scan check fails and names file, line, and rule.
2. **Given** a pull request touching only files with no secret-like content, **When** CI
   runs, **Then** the secret-scan check passes.
3. **Given** a known false positive (a documentation placeholder, a test fixture that
   must look key-shaped), **When** it is recorded in the scanner's allowlist with a
   reason, **Then** the check passes and the allowlist entry is reviewable in the diff.
4. **Given** a secret already present in history before this feature landed, **When** CI
   runs on an unrelated pull request, **Then** that pre-existing finding does not fail
   the unrelated pull request.

---

### User Story 4 - Dependency updates arrive as reviewable pull requests (Priority: P2)

Instead of a manual sweep, updates to Go modules, workspace JavaScript packages, GitHub
Actions versions, and Docker base images arrive continuously as small pull requests, each
carrying the full existing check suite plus the three new gates.

**Why this priority**: P2 because it is preventive rather than detective. The other three
stories stop a bad change; this one reduces how often a bad change becomes possible. It
also depends on the other gates being in place to be safe — automated bumps landing
without a vulnerability or image-build gate would be *less* safe than manual ones.

**Independent Test**: after merge, confirm the dependency bot has opened at least one
update pull request per configured ecosystem within its first scheduled run, and that
each such pull request runs the full check suite.

**Acceptance Scenarios**:

1. **Given** an outdated Go module requirement, **When** the bot's schedule fires,
   **Then** an update pull request is opened against `master` on its own branch.
2. **Given** several outdated packages in the same ecosystem, **When** the bot runs,
   **Then** they arrive grouped per the configured grouping rather than as one pull
   request per package.
3. **Given** an update pull request, **When** CI runs on it, **Then** the same required
   checks apply as to a human-authored pull request — no bypass.
4. **Given** the maintainer has capacity limits, **When** the bot has reached the
   configured open-pull-request cap for an ecosystem, **Then** it opens no further ones
   until some are closed.

---

### Edge Cases

- **A docs-only pull request.** All four new gates must report *skipped*, not run and not
  vanish. A gate that vanishes leaves a required check permanently "Expected" once the
  branch-protection ruleset is applied, which blocks the merge forever.
- **A pull request that touches only `.github/workflows/**` or the dependency-bot
  config.** Per the existing design these paths appear in no filter and run nothing on
  the pull request; the full set runs on the merge commit on `master`. The new gates
  follow the same rule rather than inventing an exception.
- **A pull request from the dependency bot.** Runs the full suite. It must not need
  repository secrets to do so, because pull requests from bot-authored branches in a
  private repository still run with the repository's token but should not require any
  additional credential.
- **The advisory database changes with no code change.** A pull request that was green
  yesterday can fail today because a new advisory was published. This is correct
  behaviour, not a flake; the fix is a dependency bump, not a retry.
- **A vulnerability with no fixed version available.** The gate fails and there is no
  bump that clears it. There must be a documented, reviewable way to record a temporary,
  justified exception with an expiry rather than disabling the whole gate.
- **Image build cache.** A cold build of both images on every pull request is slow. The
  gate must stay well inside the runner-minute budget the existing workflow was tuned
  for, without a stale cache masking a genuine build break.
- **A secret shaped like a placeholder.** `.env.example` intentionally contains
  key-shaped placeholder values; these must not fail the scan.
- **Pre-existing history.** The secret scan must be scoped so that findings predating
  this feature do not fail every unrelated pull request. A separate, one-time full-history
  audit is in scope for reporting, not for gating.

## Requirements *(mandatory)*

### Functional Requirements

**Change detection and reporting**

- **FR-001**: Every new gate MUST participate in the existing change-detection design:
  gated by an `if:` condition on a paths-filter output, never by a top-level path filter
  on the workflow, so that an irrelevant pull request reports the check as *skipped* and
  never as absent.
- **FR-002**: Every new gate MUST run unconditionally on pushes to `master` and on manual
  re-runs, matching the existing `github.event_name != 'pull_request' || ...` output
  pattern already used by the four existing filters.
- **FR-003**: Each new gate MUST declare a stable, human-readable check name suitable for
  listing in the branch-protection ruleset recorded in
  `specs/domains/platform-operations.md` § 2.2, and that document MUST be updated in the
  same change to list the new required checks.
- **FR-004**: Each new filter or filter reuse MUST be documented inline in the workflow
  with the same rationale-comment style the file already uses — stating what can change
  the gate's verdict and why a path is or is not listed.

**Vulnerability scanning**

- **FR-005**: The system MUST fail a pull request that introduces or retains a Go
  dependency with a published advisory that is reachable from the module's own code.
- **FR-006**: The Go vulnerability gate MUST distinguish reachable from unreachable
  findings and MUST NOT fail on unreachable ones, so the gate stays actionable rather
  than becoming noise the team learns to ignore.
- **FR-007**: The system MUST fail a pull request whose workspace JavaScript dependencies
  carry an advisory at or above a declared severity floor, and MUST record that floor
  explicitly rather than relying on a tool default.
- **FR-008**: Both vulnerability gates MUST name, in their failure output, the advisory
  identifier, the affected package, and the version that fixes it when one exists.
- **FR-009**: The system MUST provide a reviewable, in-repository mechanism to record a
  temporary exception for a specific advisory that has no available fix, including a
  written reason. An exception MUST be visible in a diff and MUST NOT be a blanket
  disabling of the gate.
- **FR-010**: The vulnerability gates MUST run against the versions actually resolved by
  the lockfiles in the tree, not against a freshly resolved dependency graph.

**Secret scanning**

- **FR-011**: The system MUST fail a pull request that adds content matching a recognised
  secret pattern to any tracked file.
- **FR-012**: The secret scan MUST cover, at minimum, the credential shapes this project
  actually handles: the configuration encryption key and the four LLM provider API keys,
  in addition to the scanner's default rule set.
- **FR-013**: The secret scan MUST be scoped to the pull request's own changes so that
  findings predating this feature do not fail unrelated pull requests.
- **FR-014**: The system MUST support an in-repository allowlist for justified false
  positives, where each entry carries a reason and is reviewable in a diff.
- **FR-015**: The secret scan MUST NOT print the matched secret value in full in its
  output, since workflow logs are themselves an exposure surface.
- **FR-016**: A one-time full-history scan MUST be run as part of delivering this feature
  and its result recorded, so the team knows whether any credential already resides in
  history and needs rotation. This scan is a reported deliverable, not a recurring gate.

**Container image builds**

- **FR-017**: The system MUST build the API container image from `apps/api/Dockerfile` and
  the dashboard container image from `apps/dashboard/Dockerfile` on any pull request that
  can affect them, and MUST fail if either build fails.
- **FR-018**: The image-build gate MUST build only — it MUST NOT push an image to any
  registry, publish an artifact, or require registry credentials.
- **FR-019**: The image-build gate MUST use layer caching such that a typical pull request
  does not pay full cold-build cost, while still failing on a genuine build break rather
  than serving a stale success from cache.
- **FR-020**: Each image MUST be a separately identifiable job or step, so a failure names
  which image broke without the maintainer reading the whole log.
- **FR-021**: The image-build filters MUST include every path the respective build context
  actually reads, including shared workspace manifests and lockfiles for the dashboard
  image and the Go module files for the API image.

**Dependency updates**

- **FR-022**: The system MUST open automated update pull requests for four ecosystems: the
  Go module under `apps/api`, the pnpm workspace at the repository root, the GitHub
  Actions used by workflows, and the base images referenced by both Dockerfiles.
- **FR-023**: Automated update pull requests MUST target `master` on their own branches
  and MUST run the same required checks as human-authored pull requests, with no bypass.
- **FR-024**: Updates MUST be grouped to limit review load rather than opening one pull
  request per package, and each ecosystem MUST declare an open-pull-request cap.
- **FR-025**: The update schedule and caps MUST be recorded in the configuration file
  itself, so the cadence is a reviewable fact rather than a service-side setting.

**Reproducible dependency resolution** *(added after planning — see plan.md § Summary and
research.md § 0; the original description did not anticipate these, and three requirements
above are unsatisfiable without them)*

- **FR-027**: The workspace dependency lockfile MUST be tracked in version control, and a
  frozen install MUST succeed and leave it unmodified. Today it is excluded from version
  control, which fails the three existing web checks outright and leaves "the dependency
  set we tested" undefined — the exact quantity FR-010 requires the audit gates to pin
  down, and the input the dependency bot needs to compute an update at all (FR-022).
- **FR-028**: The dashboard container image MUST install from that same tracked lockfile,
  so the dependency set that is audited and the dependency set that ships are the same set.
  It currently installs unfrozen, which would leave the audit gate's guarantee stopping at
  the repository boundary.

**Documentation**

- **FR-026**: The durable rules introduced here MUST be folded into
  `specs/domains/platform-operations.md` when the feature ships, and the operations
  documentation under `docs/` MUST describe how to respond to each new failing gate —
  in particular how to record a vulnerability exception and a secret-scan allowlist entry.

### Key Entities

- **Gate**: a named CI check with a pass/skip/fail verdict, a stable name, and a declared
  set of paths whose change can alter its verdict.
- **Advisory finding**: an identifier, an affected package and version range, a fixed
  version when one exists, and a reachability verdict.
- **Exception record**: an advisory identifier, a written reason, and an owner, recorded
  in-repository and reviewable in a diff.
- **Allowlist entry**: a path or pattern plus a written reason for why a secret-shaped
  match at that location is legitimate.
- **Update policy**: an ecosystem, a directory, a schedule, a grouping rule, and an
  open-pull-request cap.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A pull request that breaks either container build fails CI; verified by
  deliberately breaking each Dockerfile once during delivery and observing the failure.
- **SC-002**: A pull request that adds a recognised secret pattern fails CI; verified by
  a deliberate test during delivery, with the matched value not printed in full.
- **SC-003**: A pull request whose dependencies carry a reachable advisory at or above the
  declared severity floor fails CI; verified by a deliberate downgrade during delivery.
- **SC-004**: A documentation-only pull request reports every new gate as *skipped* and
  spends no additional runner minutes beyond the change-detection job.
- **SC-005**: Total wall-clock time for a full-suite pull request grows by no more than
  50% versus the pre-change baseline, measured on the merge commit that lands this
  feature against the merge commit before it.
- **SC-006**: Within one scheduled cycle after merge, at least one automated update pull
  request exists for each of the four configured ecosystems, or the ecosystem is
  demonstrably already current.
- **SC-007**: A full-history secret scan has been run once and its result recorded — either
  "no findings" or an explicit list with a rotation decision for each.
- **SC-008**: Every new required check name appears in the recorded branch-protection
  ruleset, so applying that ruleset needs no further edit to this feature's work.
- **SC-009**: The three currently-failing web checks report success on `master`, and a
  repeated frozen install produces no change to the tracked lockfile.
- **SC-010**: A secret present in history *before* the pull request's own commits does not
  fail that pull request, while one added *by* it does — both verified by a deliberate test
  during delivery, since a mis-scoped scan either fails everything or gates nothing.

## Assumptions

- The repository stays private on the GitHub Free tier, so server-side branch protection
  remains unavailable and the recorded ruleset stays aspirational. New checks are still
  named there so the ruleset is applied wholesale the moment the plan allows it.
- Scanning runs entirely inside GitHub Actions with the default repository token. No new
  third-party service account, no new repository secret, and no registry credential is
  introduced.
- The four LLM provider keys stay confined to the gateway container's environment
  (Constitution Principle V); this feature adds detection, it does not relocate any
  credential.
- "Reachability" for Go means call-graph reachability from the module's own packages, the
  standard behaviour of the Go ecosystem's vulnerability tooling.
- The dependency bot is GitHub's built-in one, configured by a file in the repository, so
  no external automation account is required.
- Container images are built for the runner's own architecture only. Multi-architecture
  builds are out of scope.
- No application code, migration, DTO, or generated file changes. If delivery discovers a
  vulnerability requiring a dependency bump, that bump is part of this feature; a code
  change to accommodate it would be a separate feature.

## Out of Scope

- Pushing images to a registry, image signing, provenance attestation, or SBOM generation.
- Container image *vulnerability* scanning (scanning the built image's OS packages). Only
  the *build* is gated here; image scanning is a candidate follow-up.
- License compliance scanning.
- Rewriting git history to remove any secret the one-time full-history scan finds. If a
  finding requires history surgery, that is a separate, explicitly approved operation.
- The other CI gaps identified alongside this one — end-to-end tests in CI, coverage
  measurement, backup/restore tooling, and metrics — each of which is its own feature.
