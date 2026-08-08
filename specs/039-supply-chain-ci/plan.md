# Implementation Plan: Supply-chain and build-integrity CI gates

**Branch**: `039-supply-chain-ci` | **Date**: 2026-08-08 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/039-supply-chain-ci/spec.md`

## Summary

Add four merge gates the current workflow lacks — a Go vulnerability scan, a JavaScript
dependency audit, a secret scan, and a container image build for each of the two images —
plus a Dependabot configuration covering the Go module, the pnpm workspace, the GitHub
Actions in use, and the Docker base images.

Research (§ 0.1) turned up a blocking defect that this feature must fix first:
**`pnpm-lock.yaml` is gitignored, so the three web jobs are failing on `master` right
now**, and three of this feature's requirements (audit the resolved lockfile, run a
dependency bot for the workspace, have bot pull requests pass the same checks) are
unsatisfiable until the lockfile is tracked. Un-ignoring and committing it is task zero.
A second, smaller finding (§ 0.2) follows from it: `apps/dashboard/Dockerfile` installs
with `--no-frozen-lockfile`, so the shipped dependency set and the audited one are not the
same set; it copies the lockfile and installs frozen instead.

The technical approach keeps every new gate inside the workflow's existing
`dorny/paths-filter` design — gated by an `if:` on a filter output so an irrelevant pull
request reports *skipped*, never absent — and pins every new tool in the repository, using
the `.<tool>-version` + `scripts/<tool>-check.sh` convention the repo already uses three
times over.

## Technical Context

**Language/Version**: YAML (GitHub Actions workflow syntax, Dependabot config v2), Bash
(the new check script, `set -euo pipefail`, matching `scripts/tygo-check.sh`), TOML
(gitleaks config). No Go, TypeScript, or SQL changes.

**Primary Dependencies**:

- `govulncheck` (`golang.org/x/vuln/cmd/govulncheck`) — pinned in `apps/api/.govulncheck-version`
- `pnpm audit` — already present via the pnpm the web jobs install
- `gitleaks` — pinned release binary, version in `.gitleaks-version`
- `docker/setup-buildx-action@v3`, `docker/build-push-action@v6`
- `dorny/paths-filter@v3`, `actions/checkout@v4`, `actions/setup-go@v5`,
  `actions/setup-node@v4`, `pnpm/action-setup@v4` — all already in the workflow

**Storage**: N/A. This feature adds no database object, no migration, and no persisted
application state. Its only durable artefacts are files in the repository and the GitHub
Actions layer cache (ephemeral, 10 GB repository budget).

**Testing**: The gates test themselves. Each is validated by a deliberate negative case
during delivery (break a Dockerfile, add a fake key, downgrade a dependency) and then
reverted — this is what SC-001 through SC-003 measure. `scripts/govulncheck-check.sh` gets
unit coverage of its ignore-file parser via a small bats-free bash test invoked from the
script's own `--self-test` flag, so the parser's failure modes (malformed line, expired
entry) are proven without needing a real advisory.

**Target Platform**: `ubuntu-latest` GitHub-hosted runners; local parity on Linux via new
`make` targets.

**Project Type**: Repository tooling / CI configuration. No application tier is touched.

**Performance Goals**: A full-suite pull request grows by no more than 50% in wall-clock
time (SC-005). Baseline from recent runs is ~2–3 minutes; the new gates are expected to add
~1.5–3 minutes, dominated by the two image builds running in parallel with everything else.
A docs-only pull request must stay at its current cost — the change-detection job alone
plus the ~20-second secret scan.

**Constraints**:

- No new repository secret, service account, registry credential, or third-party app.
  Every gate runs with the default `GITHUB_TOKEN`.
- No new required check may be introduced without adding it to the ruleset recorded in
  `specs/domains/platform-operations.md` § 2.2 in the same change (FR-003).
- Every gate must report *skipped* rather than vanish on an irrelevant pull request
  (FR-001) — a vanished check sits at "Expected" forever once the ruleset is applied.
- The four provider credentials must stay confined to the `litellm` container's
  environment. This feature adds detection only; it relocates nothing.

**Scale/Scope**: One workflow file (+~150 lines), one new Dependabot config, one new
gitleaks config, one new check script, two new version-pin files, two new ignore/allowlist
files, a two-line Dockerfile change, a one-line `.gitignore` deletion, one generated
lockfile, four new `make` targets, and updates to one domain doc and one docs page.
Fourteen new files, six edited.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Verdict | Reasoning |
|-----------|---------|-----------|
| **I. No Auto-Apply, Ever** | ✅ Not engaged | No code path touching applications, employers, or job listings is added or modified. |
| **II. Grounded Generation** | ✅ Not engaged | No LLM prompt, output path, or generation surface is touched. |
| **III. Typed Contracts Across Service Boundaries** | ✅ Upheld | No DTO, sqlc query, or tygo-generated file changes. The existing `sqlc-drift`, `tygo-drift`, and `shared-types` gates are untouched and keep guarding the boundary. Committing `pnpm-lock.yaml` strengthens the boundary by making `packages/shared`'s own dependency set reproducible. |
| **IV. Test Discipline Per Language, Enforced at the Boundary** | ✅ Upheld, and repaired | The existing per-language suites are untouched. This feature *restores* the web third of that discipline, which is currently failing on `master` for a configuration reason (research § 0.1). New gates run alongside, not instead of, the existing ones. |
| **V. Local-First, Self-Hosted by Default** | ✅ Upheld | No third-party service, account, or outbound API call is introduced. `trufflehog` was rejected in research § 3 specifically because its verification feature calls provider APIs from CI. Advisory data reaches CI through tooling already trusted in the Go and pnpm toolchains. |

**Technology & Architecture Constraints**: no migration is added, so the goose
sequential-version rule is not engaged. `packages/shared` gains a tracked lockfile, which is
a strengthening of the "single source of truth" posture, not a deviation.

**Development Workflow & Quality Gates**: the constitution requires `make` targets as the
canonical entry points. Four new targets provide local parity (research § 7).
`make test-lint` deliberately does **not** grow — see Complexity Tracking.

**Post-design re-check (after Phase 1)**: ✅ unchanged. The design added no application
dependency, no data store, and no external service. The one design decision with any
constitutional weight — keeping container builds out of `make test-lint` — is recorded in
Complexity Tracking below.

## Project Structure

### Documentation (this feature)

```text
specs/039-supply-chain-ci/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output — tool selection and the two blocking findings
├── data-model.md        # Phase 1 output — the configuration artefacts and their schemas
├── quickstart.md        # Phase 1 output — how to validate each gate, positive and negative
├── contracts/
│   ├── check-names.md   # The stable check names the branch-protection ruleset will list
│   └── file-formats.md  # Grammars for .govulncheck-ignore, .gitleaks.toml, dependabot.yml
├── checklists/
│   └── requirements.md  # Spec quality checklist (already passing)
└── tasks.md             # Phase 2 output (/speckit-tasks — not created by /speckit-plan)
```

### Source Code (repository root)

```text
.github/
├── workflows/
│   └── api-ci.yml               # EDITED: +1 filter output (`any`), +6 jobs
└── dependabot.yml               # NEW: 4 ecosystems, weekly, grouped, capped

apps/api/
├── .govulncheck-version         # NEW: pinned govulncheck version
├── .govulncheck-ignore          # NEW: advisory exceptions (id, expiry, reason)
└── Dockerfile                   # unchanged

apps/dashboard/
└── Dockerfile                   # EDITED: copy pnpm-lock.yaml, install --frozen-lockfile

scripts/
└── govulncheck-check.sh         # NEW: runs govulncheck, filters by ignore file, self-test

specs/domains/
└── platform-operations.md       # EDITED: new required checks in the § 2.2 ruleset,
                                 #         gate-response runbook, compose-image gap

docs/
└── (operations page)            # EDITED: how to respond to each new failing gate

.gitleaks.toml                   # NEW: extends default rules + project rules + allowlist
.gitleaks-version                # NEW: pinned gitleaks release version
.gitignore                       # EDITED: remove the `pnpm-lock.yaml` line
pnpm-lock.yaml                   # NEW (generated, committed)
package.json                     # EDITED: pnpm.auditConfig.ignoreCves (empty, with docs ref)
Makefile                         # EDITED: +vuln-go, +vuln-web, +secrets, +images, +audit
```

**Structure Decision**: everything lands in existing top-level locations — no new
directory is created. New tool pins live next to the tool's scope (`apps/api/` for the Go
tool, repository root for the tree-wide one), matching where `.golangci-version`,
`.sqlc-version`, and `.tygo-version` already sit. The new check script joins the three
existing `scripts/*-check.sh` files and copies their shape: `set -euo pipefail`, resolve
`REPO_ROOT` from `BASH_SOURCE`, read the version pin, fail loudly with an install hint if
the tool is absent.

## Implementation Phases

**Phase A — unblock (must land first).** Remove `pnpm-lock.yaml` from `.gitignore`,
generate and commit the lockfile, confirm the three web jobs go green. Nothing else in this
feature can be validated on a pull request until this is done, because a failing web job
sits next to every new gate and makes the signal unreadable.

**Phase B — the gates, independently addable.** Each of the four gate groups is its own
slice and can land and be validated on its own:

- B1 secret scan (`.gitleaks-version`, `.gitleaks.toml`, the `any` filter output, the job)
- B2 Go vulnerabilities (`.govulncheck-version`, `.govulncheck-ignore`, the script, the job)
- B3 JavaScript audit (the `pnpm.auditConfig` stanza, the job)
- B4 image builds (the two jobs, plus the Dockerfile lockfile fix from research § 0.2)

**Phase C — automation and record.** Dependabot config, the four `make` targets, the
one-time full-history secret scan and its recorded result, the domain-doc ruleset update,
and the operations runbook.

Phase C's Dependabot entry for `npm` depends on Phase A. Nothing else in B or C depends on
anything but A.

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Committing the lockfile resolves a dependency set that fails the existing web jobs for a *different* reason (a package moved on since the last successful local install). | Phase A is its own slice, landed and observed before anything else. If the resolved set fails, that failure is the whole content of the slice and is fixed there. |
| The new `pnpm audit` gate fails immediately on the freshly committed lockfile. | Expected and acceptable — that is the gate doing its job on day one. The response is a dependency bump inside this feature (assumption already recorded in the spec), or a documented `ignoreCves` entry if no fix exists. |
| `govulncheck` fails immediately on the Go module. | Same treatment. Reachability filtering (FR-006) is expected to keep the first-run finding count near zero; if it does not, the ignore file with expiry dates absorbs the remainder in a reviewable way. |
| Image builds push the pull-request wall-clock past the SC-005 budget. | The two run in parallel with each other and with every existing job, so they add the *slower* image's time, not the sum. Cache scoping keeps warm builds short. If the budget is still breached, the fallback is to gate the image builds on `master` pushes plus a manual label rather than every pull request — recorded here so it is a known lever, not a surprise. |
| GHA layer cache eviction makes builds intermittently cold. | 10 GB repository budget against two images; scoped separately so they do not evict each other. A cold build is slow, never wrong. |
| Six new checks make the pull-request check list noisy. | They are additive and mostly skipped: a docs-only pull request runs one of the six. |

## Complexity Tracking

> Filled only where a design decision needs justifying against the constitution.

| Decision | Why | Simpler alternative rejected because |
|----------|-----|--------------------------------------|
| **The four new gates are not added to `make test-lint`**, against the coverage invariant in `specs/domains/platform-operations.md` § 3 ("`test-lint` must cover the union of every required CI check that does not need infrastructure"). Instead they land as a separate `make audit` alias over `vuln-go`, `vuln-web`, and `secrets`, with `make images` standing alone. | The invariant's stated purpose is that "an author who sees local success is not surprised by CI" (023-SC-005). These four gates cannot deliver that property, and folding them in would *weaken* the invariant rather than honour it. Two independent reasons: **(a) they are not deterministic in time.** A vulnerability gate that passed this morning fails this afternoon because an advisory was published — no local run can predict the CI verdict, so local success was never a promise about CI. **(b) They need the network and, for images, Docker** — the same exemption `test-integration` and `test-e2e` already hold under § 3 ("they need containers or a browser — and are deliberately **not** part of `test-lint`"). Container builds additionally cost 6–8 minutes cold, which would make the pre-push loop slower than CI and push contributors to skip it entirely. | Adding all four to `test-lint` was rejected on both grounds above. Adding only `secrets` (fast, offline, deterministic) was genuinely tempting and is the closest call here — it is left out only so that "audit-class gate" is one coherent group with one entry point rather than one gate hiding in `test-lint` and three outside it. `make audit` is documented in the same domain-doc section as the invariant, and § 3 is amended in this feature to record the exemption explicitly rather than leaving the invariant silently violated. |
| A hand-written `scripts/govulncheck-check.sh` wrapper rather than calling `govulncheck` directly in the workflow. | FR-009 requires a reviewable per-advisory exception mechanism with a reason; `govulncheck` has no native ignore file, so a wrapper is the only place that logic can live. Putting it in a script rather than inline YAML also gives local parity (`make vuln-go`) and lets the parser be self-tested. | Inline YAML with `jq` was rejected — untestable, no local parity, and it would duplicate the ignore logic between the workflow and any local invocation. |
| A pinned gitleaks **binary** rather than the marketplace action. | The action requires a licence key for organization-owned repositories; the binary is the same scanner with no account and no licence question, consistent with the spec's no-new-service-account assumption. | The marketplace action was rejected on the licensing dependency alone. |
