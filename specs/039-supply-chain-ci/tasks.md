# Tasks: Supply-chain and build-integrity CI gates

**Feature**: `039-supply-chain-ci` | **Date**: 2026-08-08
**Input**: [spec.md](./spec.md), [plan.md](./plan.md), [research.md](./research.md),
[data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: this feature's "tests" are the negative cases in `quickstart.md` — plant a
defect, observe the gate fail, revert. They are deliverables (SC-001..003), so they appear
as tasks rather than being generated as a separate suite. The one piece of conventional
test code is the ignore-file parser self-test (T017), which needs no network and no real
advisory.

**Organization**: phases follow the user stories in `spec.md`. US1 (image builds), US2
(vulnerabilities), and US3 (secrets) are all P1 and mutually independent — any one of them
delivers value alone. US4 (Dependabot) is P2 and depends only on Phase 2.

---

## Phase 1: Setup

**Goal**: nothing in this feature can be validated until the workflow is green, and it is
not green today.

- [ ] T001 Confirm and record the current failure: run `gh run list --branch master --limit 1` and `gh run view <id> --json jobs`, and paste the three failing web job names plus the `Dependencies lock file is not found` error into the pull-request description as the before-state (quickstart § 0)
- [ ] T002 Remove the `pnpm-lock.yaml` line from `/home/nnc/Projects/job-finder/.gitignore`
- [ ] T003 Generate the lockfile with `pnpm install` at the repository root, then verify determinism with `pnpm install --frozen-lockfile && git diff --exit-code pnpm-lock.yaml`
- [ ] T004 Commit `/home/nnc/Projects/job-finder/pnpm-lock.yaml` and push, then confirm `frontend test (vitest)`, `frontend typecheck`, and `lint (web)` all report success on the pull request

**Checkpoint**: web CI green. Every later task can now be validated by a real check run.
Do not start Phase 2 until T004 is observed green — a red web job sitting beside a new gate
makes the new gate's signal unreadable.

---

## Phase 2: Foundational (blocking prerequisites)

**Goal**: the change-detection plumbing and the documentation table that every gate below
plugs into. Nothing here adds a gate; everything here is required by all of them.

- [ ] T005 Add an `any` output to the `changes` job in `/home/nnc/Projects/job-finder/.github/workflows/api-ci.yml`, with filter `- '**'` and the same `github.event_name != 'pull_request' || ...` push-override the four existing outputs use, plus an inline comment stating why a tree-wide filter exists here when the file's whole design is about narrowing (research § 5)
- [ ] T006 Correct the job table in § 2.1 of `/home/nnc/Projects/job-finder/specs/domains/platform-operations.md`: it lists nine jobs and omits `shared types — no duplicates / no weakened fields / complete nullability`, which the workflow does define and report. Ten jobs gate today, before this feature adds any (contracts/check-names.md)
- [ ] T007 Add a "Supply-chain gates" subsection to `/home/nnc/Projects/job-finder/specs/domains/platform-operations.md` § 3 recording the `make audit` group and the explicit exemption of these gates from the `test-lint` coverage invariant, with the two reasons from plan.md Complexity Tracking (non-determinism in time; network/Docker dependency)

**Checkpoint**: `any` filter available, domain doc accurate about the pre-existing state.

---

## Phase 3: User Story 3 — a committed secret never reaches master (P1)

**Goal**: a pull request adding a recognised secret pattern fails, with the match redacted.

**Independent test**: quickstart § 1 — clean positive on the tree, planted-key negative
naming rule/file/line with the value redacted, then reverted.

**Why first among the P1s**: it is the only failure mode in the feature that is
irreversible. It is also the cheapest gate to build and the fastest to run, so it proves
the Phase 2 plumbing before the expensive gates depend on it.

- [ ] T008 [P] [US3] Create `/home/nnc/Projects/job-finder/.gitleaks-version` containing the pinned gitleaks release version, one line, no `v` prefix (contracts/file-formats.md § 5)
- [ ] T009 [US3] Create `/home/nnc/Projects/job-finder/.gitleaks.toml` with `[extend] useDefault = true`, one `[[rules]]` block for `CONFIG_ENCRYPTION_KEY` bound to its variable name (never bare 64-hex), and one per provider prefix for Cerebras, Groq, Cohere, and OpenRouter — each with `id`, `description`, `regex`, and `keywords` (contracts/file-formats.md § 2)
- [ ] T010 [US3] Add the `[allowlist]` block to `.gitleaks.toml` seeded with `.env.example`, `gateway/config.yaml`, and `specs/039-supply-chain-ci/**`, each preceded by a TOML comment giving its reason, and with no bare `.*` entry (data-model S1, S2)
- [ ] T011 [US3] Add the `secret scan` job to `/home/nnc/Projects/job-finder/.github/workflows/api-ci.yml`: `needs: changes`, `if: needs.changes.outputs.any == 'true'`, `actions/checkout@v4` with `fetch-depth: 0`, download the pinned binary, run `gitleaks detect --redact --no-banner --config .gitleaks.toml --log-opts "<base>..<head>"`, and distinguish exit `1` (findings) from exit `2` (config error) in the failure message
- [ ] T012 [P] [US3] Add the `make secrets` target to `/home/nnc/Projects/job-finder/Makefile`, running the same command over the working tree, with a missing-binary message mirroring `scripts/sqlc-check.sh` in structure
- [ ] T013 [US3] Validate positively: `make secrets` exits 0 on the clean tree with no finding from `.env.example` or `gateway/config.yaml` (data-model R2)
- [ ] T014 [US3] Validate negatively (SC-002): plant a syntactically valid but never-issued `CONFIG_ENCRYPTION_KEY` and one provider key, confirm exit 1 naming rule/file/line, confirm the matched value is **redacted** in the output, then revert (quickstart § 1)
- [ ] T014a [US3] Validate the scan's scoping (SC-010, FR-013): on a scratch branch, commit a planted secret, then commit an unrelated change on top and open the pull request from the *second* commit's range only — confirm the out-of-range secret does not fail it, while an in-range one does. A `--log-opts` mistake either fails every pull request or gates nothing, and only this test tells the two apart
- [ ] T015 [US3] Run the one-time full-history scan (FR-016) and record the result — "no findings" or a list with a rotation decision each — in `/home/nnc/Projects/job-finder/specs/domains/platform-operations.md`. Do not rewrite history (spec, Out of Scope)

**Checkpoint**: US3 delivers on its own. If the rest of the feature stopped here, the
repository would still be strictly better protected than before.

---

## Phase 4: User Story 2 — a known vulnerability blocks the merge (P1)

**Goal**: reachable Go advisories and high-or-above workspace advisories fail the merge,
with a reviewable exception path for the unfixable ones.

**Independent test**: quickstart §§ 2–3 — clean positives, parser self-test, planted
downgrades on each side, and the exception + expiry mechanism proven.

### Go side

- [ ] T016 [P] [US2] Create `/home/nnc/Projects/job-finder/apps/api/.govulncheck-version` with the pinned `golang.org/x/vuln` version, matching the format of `.golangci-version` (contracts/file-formats.md § 5)
- [ ] T017 [US2] Write `/home/nnc/Projects/job-finder/scripts/govulncheck-check.sh`: `set -euo pipefail`, resolve `REPO_ROOT` from `BASH_SOURCE`, read and enforce the version pin with a message mirroring `scripts/sqlc-check.sh`, run `govulncheck -format json ./...` in `apps/api`, filter findings by reachability (FR-006) and by the ignore file, and exit 0/1/2/3 exactly as contracts/file-formats.md § 1 specifies. Every failing finding must render its advisory id, package, and fixed version — or state that no fix exists — before the script exits (FR-008); this is an implementation requirement, not only something T028 checks afterwards
- [ ] T018 [US2] Add `--self-test` to `scripts/govulncheck-check.sh` covering malformed id, unparseable date, empty reason, expired entry, stale entry, and duplicate id — one in-script fixture per row of the parser-rules table, no network required
- [ ] T019 [P] [US2] Create `/home/nnc/Projects/job-finder/apps/api/.govulncheck-ignore` containing only the header comment block documenting the `<GO-id>  <expiry>  <reason>` grammar — no entries (data-model E4)
- [ ] T020 [US2] Add the `vulnerability scan (go)` job to the workflow: `needs: changes`, `if: needs.changes.outputs.go == 'true'`, `actions/setup-go@v5` with `go-version-file: apps/api/go.mod`, `go install` at the pinned version, then `./scripts/govulncheck-check.sh`
- [ ] T021 [P] [US2] Add the `make vuln-go` target to the Makefile calling `scripts/govulncheck-check.sh`

### Web side

- [ ] T022 [P] [US2] Add the `pnpm.auditConfig.ignoreCves` stanza to `/home/nnc/Projects/job-finder/package.json` as an empty array (data-model W2)
- [ ] T023 [US2] Add the `vulnerability scan (web)` job to the workflow: `needs: changes`, `if: needs.changes.outputs.web == 'true'`, pnpm + Node setup matching the existing web jobs, frozen install, then `pnpm audit --audit-level=high --prod=false`
- [ ] T024 [P] [US2] Add the `make vuln-web` target to the Makefile
- [ ] T025 [US2] Add an exceptions table to `/home/nnc/Projects/job-finder/specs/domains/platform-operations.md` keyed by advisory id, recording reason, fixed-version status, and review date — the reason store for `ignoreCves`, which cannot carry comments (data-model W1). Record alongside it that the severity floor is `high` and *why* `moderate` was rejected (research § 2), so the choice survives the fold into the domain doc as a decision rather than a bare number

### Validation

- [ ] T026 [US2] Run `./scripts/govulncheck-check.sh --self-test` and confirm exit 0
- [ ] T027 [US2] Validate positively: `make vuln-go` and `make vuln-web` both exit 0 against the committed tree. A real first-run finding is fixed with a bump inside this feature, or an exception plus its documented reason — not by weakening the gate
- [ ] T028 [US2] Validate negatively (SC-003): downgrade one Go dependency to a version with a known reachable advisory and one workspace package to a version with a `high` advisory; confirm each gate fails naming id, package, and fixed version (FR-008); revert both (quickstart §§ 2–3)
- [ ] T029 [US2] Prove the exception mechanism end to end: add an ignore entry for the planted Go advisory and confirm exit 0; set its expiry to a past date and confirm exit 2 naming the expired entry; remove it (data-model E2)
- [ ] T030 [US2] Prove the severity floor: install a package whose only advisory is `moderate`, confirm `make vuln-web` passes while listing it, then revert (FR-007)

**Checkpoint**: US2 delivers on its own and does not depend on US3.

---

## Phase 5: User Story 1 — a broken container build is caught before merge (P1)

**Goal**: neither Dockerfile can break and merge green, and the image ships the same
dependency set the audit gates saw.

**Independent test**: quickstart § 4 — both images build clean, then each is deliberately
broken in turn and the matching CI check fails naming that image.

- [ ] T031 [US1] Fix the dashboard image's dependency resolution in `/home/nnc/Projects/job-finder/apps/dashboard/Dockerfile`: copy `pnpm-lock.yaml` alongside the manifests and change `pnpm install --no-frozen-lockfile` to `--frozen-lockfile`, with a comment recording why (research § 0.2 — what is audited must be what ships). Depends on T003
- [ ] T032 [P] [US1] Add the `build image (api)` job to the workflow: `needs: changes`, `if: needs.changes.outputs.go == 'true'`, `docker/setup-buildx-action@v3` then `docker/build-push-action@v6` with `file: apps/api/Dockerfile`, `context: .`, `push: false`, `load: false`, `cache-from: type=gha,scope=api`, `cache-to: type=gha,mode=max,scope=api`
- [ ] T033 [P] [US1] Add the `build image (dashboard)` job with the same shape, `if: needs.changes.outputs.web == 'true'`, `file: apps/dashboard/Dockerfile`, cache scope `dashboard`
- [ ] T034 [US1] Add an inline comment above both jobs mapping every `COPY` line in each Dockerfile to the existing filter entry that covers it, and stating why no new filter is needed — including that a `gateway/**`-only change builds the API image unnecessarily, and why a false run is preferred to a false skip (research § 4.1, FR-021)
- [ ] T035 [P] [US1] Add the `make images` target to the Makefile building both images locally with plain `docker build` and distinct tags
- [ ] T036 [US1] Validate positively: `make images` builds both, with the dashboard image now installing frozen. A failure here means the committed lockfile and the manifests copied into the image disagree
- [ ] T037 [US1] Validate negatively (SC-001): break `apps/api/Dockerfile` (reference a nonexistent build stage), push as a scratch pull request, confirm `build image (api)` fails and `build image (dashboard)` is unaffected; repeat for the dashboard image; revert both (quickstart § 4). Run the API break a **second** time after a green build has populated the layer cache, to prove a warm cache still catches it — FR-019's stale-cache half is argued for in research § 4 but only this run tests it

**Checkpoint**: all three P1 stories delivered. This is the feature's MVP boundary.

---

## Phase 6: User Story 4 — dependency updates arrive as reviewable pull requests (P2)

**Goal**: continuous, grouped, capped update pull requests across four ecosystems, each
running the full gate set built above.

**Independent test**: quickstart § 7 — one update pull request per ecosystem within a
cycle, each running the full suite.

- [ ] T038 [US4] Create `/home/nnc/Projects/job-finder/.github/dependabot.yml` with the five `updates:` entries (gomod at `/apps/api`, npm at `/`, github-actions at `/`, docker at `/apps/api`, docker at `/apps/dashboard`), weekly on Monday, minor+patch grouped with majors ungrouped for gomod and npm, and an `open-pull-requests-limit` on every entry (contracts/file-formats.md § 3). Depends on T004 — the npm entry is inert without a tracked lockfile
- [ ] T039 [US4] Add an inline comment to `dependabot.yml` recording why `gomod` points at `/apps/api` and not `/`: there is no root Go module, and Dependabot reports no error when it finds no manifest, so the misconfiguration is silent (data-model U1)
- [ ] T040 [US4] Record in `/home/nnc/Projects/job-finder/specs/domains/platform-operations.md` the images Dependabot cannot cover — the compose-file pins for pgvector, redis, minio, flaresolverr, ClickHouse, and Langfuse — and that they stay manual (research § 6.1)
- [ ] T041 [US4] Verify after merge (SC-006): trigger a manual check from the repository's Dependabot page, then confirm with `gh pr list --author "app/dependabot"` that each ecosystem either opened a pull request or is demonstrably current. Rule out the silent `gomod` misconfiguration explicitly

---

## Phase 7: Polish and cross-cutting

- [ ] T042 Add the `make audit` alias to the Makefile running `vuln-go`, `vuln-web`, and `secrets`, first non-zero wins — matching how `make lint` composes `lint-go` and `lint-web`
- [ ] T043 Add all five new check names verbatim to the § 2.1 job table in `/home/nnc/Projects/job-finder/specs/domains/platform-operations.md`, with trigger, filter, and budget columns filled (contracts/check-names.md)
- [ ] T044 Add all five new check names to the § 2.2 ruleset's `required_status_checks` list and update the "nine job names" prose to fifteen. A gate absent here runs but never gates (data-model G4, FR-003)
- [ ] T045 [P] Write the gate-response runbook in the Docusaurus operations page under `/home/nnc/Projects/job-finder/docs/`: what each of the five new failures means, how to add a `.govulncheck-ignore` entry with an expiry, how to add an `ignoreCves` entry plus its domain-doc row, how to add a gitleaks allowlist entry, and the explicit instruction that a failing image build is never suppressed (FR-026)
- [ ] T046 [P] Note in `/home/nnc/Projects/job-finder/AGENTS.md` that `make audit` exists alongside `make test-lint`, and that it is deliberately not part of the merge-gate alias
- [ ] T047 Measure the wall-clock delta (SC-005): compare the merge commit landing this feature against the merge commit before it, and record both numbers in the pull-request description. If the growth exceeds 50%, apply the recorded lever — gate image builds on `master` pushes plus a manual label — rather than removing a gate
- [ ] T048 Open a docs-only scratch pull request and confirm every new check reports **skipped** except `secret scan`, which runs (SC-004). A check reported as *missing* rather than *skipped* is an FR-001 failure and blocks the feature — once the § 2.2 ruleset is applied, a missing required check sits at "Expected" forever
- [ ] T049 Walk the "Definition of done" checklist at the end of `quickstart.md` and confirm every box, then delete the scratch branches created during validation

---

## Dependencies

```text
Phase 1 (T001–T004)  ── blocks everything
   │
   ├─▶ Phase 2 (T005–T007)  ── blocks every gate phase
   │       │
   │       ├─▶ Phase 3 US3 (T008–T015)   ─┐
   │       ├─▶ Phase 4 US2 (T016–T030)   ─┼─ mutually independent, any order,
   │       └─▶ Phase 5 US1 (T031–T037)   ─┘  can run in parallel
   │
   └─▶ Phase 6 US4 (T038–T041)  ── needs T004 only, not the gate phases
                                    (but is sequenced after them so bot pull
                                     requests land into a complete gate set)

Phase 7 (T042–T049) ── after all of the above
```

**Within-phase dependencies**

- T003 → T004 → T031 (the Dockerfile fix needs the lockfile to exist)
- T009 → T010 → T011 (config before the job that runs it)
- T017 → T018 → T020 (script before self-test before the job)
- T022 → T023 (the stanza must exist before the gate reads it)
- T032, T033 → T034 (comment describes both jobs)
- T043 → T044 (the § 2.1 table is the source for the § 2.2 list)

## Parallel execution

Tasks marked `[P]` touch different files and have no incomplete dependency:

- **Phase 3**: T008 and T012 alongside T009
- **Phase 4**: T016, T019, T021, T022, T024 in any order
- **Phase 5**: T032, T033, T035 concurrently (T034 after both jobs exist)
- **Phase 7**: T045 and T046 concurrently

The three gate phases are independent slices. With more than one implementer, US3, US2,
and US1 can proceed simultaneously after T007; the only shared file is
`.github/workflows/api-ci.yml`, so merge conflicts there are the coordination cost.

## Implementation strategy

**MVP**: Phase 1 + Phase 2 + Phase 3 (US3). That lands the green build, the plumbing, and
the gate protecting against the one irreversible failure mode. Fourteen tasks.

**Incremental delivery**: each of Phases 3, 4, and 5 is independently shippable. If time
runs short, Phase 6 (Dependabot) is the correct thing to defer — it is preventive rather
than detective, and it is *safer* deferred, because automated bumps landing without the
vulnerability and image-build gates in place would be worse than manual ones.

**Ordering rationale**: the P1 stories are ordered US3 → US2 → US1 by irreversibility of
the failure they prevent (leaked key, then vulnerable dependency, then broken build) and
by ascending build cost, so the cheapest gate proves the Phase 2 plumbing first.

## Task summary

| Phase | Story | Tasks | Count |
|---|---|---|---|
| 1 Setup | — | T001–T004 | 4 |
| 2 Foundational | — | T005–T007 | 3 |
| 3 | US3 secrets (P1) | T008–T015 (incl. T014a) | 9 |
| 4 | US2 vulnerabilities (P1) | T016–T030 | 15 |
| 5 | US1 image builds (P1) | T031–T037 | 7 |
| 6 | US4 Dependabot (P2) | T038–T041 | 4 |
| 7 Polish | — | T042–T049 | 8 |
| **Total** | | | **50** |

## Requirement traceability

Added after the `/speckit-analyze` pass, which found five tasks mapping to no requirement
(the lockfile work) and two requirements with no validating task.

| Requirement | Tasks |
|---|---|
| FR-001..004 | T005, T034, T039, T043, T044, T048 |
| FR-005..010 | T016–T030 |
| FR-011..016 | T008–T015, T014a |
| FR-017..021 | T031–T037 |
| FR-022..025 | T038–T041 |
| FR-026 | T007, T040, T045, T046 |
| FR-027, FR-028 *(added post-analysis)* | T002–T004, T031 |
| SC-001..008 | T037, T014, T028, T048, T047, T041, T015, T044 |
| SC-009, SC-010 *(added post-analysis)* | T004, T014a |
