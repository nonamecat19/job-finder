# Phase 0 Research: Supply-chain and build-integrity CI gates

**Feature**: `039-supply-chain-ci` | **Date**: 2026-08-08

This document resolves every open technical question the specification deliberately left
to the plan phase, and records two blocking defects discovered while surveying the tree.

---

## 0. Blocking discoveries (found during research, not in the original description)

### 0.1 `pnpm-lock.yaml` is gitignored — web CI is red on `master` right now

`.gitignore:1` lists `pnpm-lock.yaml`, so no lockfile is tracked. Every workflow job that
sets up Node does so with `actions/setup-node@v4` + `cache: pnpm`, then runs
`pnpm install --frozen-lockfile`. With no lockfile in the checkout, the cache step itself
errors before install is ever reached:

```
##[error]Dependencies lock file is not found in /home/runner/work/job-finder/job-finder.
Supported file patterns: pnpm-lock.yaml
```

Verified against run `31253913356` (push to `master`, 2026-08-08): `frontend test
(vitest)`, `frontend typecheck`, and `lint (web)` all failed for this reason while every
Go job passed. The last green run predates the `.gitignore` entry.

**Decision**: removing `pnpm-lock.yaml` from `.gitignore` and committing the lockfile is
task zero of this feature.

**Rationale**: three of this feature's requirements are unsatisfiable without it.
FR-010 requires the vulnerability gates to audit the versions the lockfile actually
resolves — with no lockfile there is no such set. FR-022 requires a dependency bot for the
pnpm workspace; the bot's npm ecosystem needs `pnpm-lock.yaml` to compute an update.
FR-023 requires bot pull requests to run the same checks as human ones; those checks are
currently failing for everyone. Beyond this feature, an untracked lockfile means every
`pnpm install` can resolve a different graph, which makes "the dependency set we tested"
an undefined quantity — the exact property a supply-chain gate exists to pin down.

**Alternatives considered**:

- *Leave it ignored and drop `--frozen-lockfile`.* Rejected: it converts a hard failure
  into silent nondeterminism, and makes both new vulnerability gates advisory at best —
  they would audit whatever the registry served that minute.
- *Fix it as a separate hotfix feature first.* Rejected as extra ceremony for a
  one-line `.gitignore` deletion plus a generated file, when this feature already owns
  "the dependency set is pinned and audited". It is sequenced as the first task so it
  lands early and unblocks the rest, and it is called out in the plan's Summary so a
  reviewer is not surprised to see a lockfile in a CI pull request.

**Consequence for sequencing**: no other gate in this feature can be validated on a
pull request until this lands, because the paths-filter `web` output is true for any
change to `package.json`/`pnpm-lock.yaml` and the web jobs would fail alongside the new
ones, muddying the signal.

### 0.2 The dashboard image build does not use the lockfile

`apps/dashboard/Dockerfile` copies `pnpm-workspace.yaml`, the root `package.json`, and two
workspace `package.json` files — but no lockfile — then runs:

```
RUN pnpm install --no-frozen-lockfile --filter @job-finder/shared --filter @job-finder/dashboard
```

That flag is currently *load-bearing*, because there is no lockfile to freeze against
(§ 0.1). Once the lockfile is tracked, the image build should copy it and install frozen,
otherwise the audited dependency set and the shipped dependency set are two different
things and FR-010's guarantee stops at the repository boundary.

**Decision**: as part of this feature, `apps/dashboard/Dockerfile` copies
`pnpm-lock.yaml` and installs with `--frozen-lockfile`.

**Rationale**: gating the image build (FR-017) is worth much less if the image resolves
dependencies the gates never saw. This is a two-line Dockerfile change with no application
code impact, and the new image-build job proves it still builds.

**Alternatives considered**: leaving `--no-frozen-lockfile` and noting the gap in the
domain doc. Rejected — it leaves the feature's central claim ("what we audit is what we
ship") false on the dashboard side, for no saving.

---

## 1. Go vulnerability scanning

**Decision**: `govulncheck` (`golang.org/x/vuln/cmd/govulncheck`), version-pinned in
`apps/api/.govulncheck-version`, installed with `go install` in the workflow and invoked
through a new `scripts/govulncheck-check.sh` that consumes its JSON output.

**Rationale**:

- It is the Go ecosystem's first-party tool and the only one that does *call-graph
  reachability* analysis, which FR-006 requires directly: a vulnerable module in the
  dependency graph fails the build only if the vulnerable symbol is actually reachable
  from this module's packages. Without that, a Go dependency tree of 82 direct
  requirements produces a steady stream of unactionable findings and the gate gets muted.
- Version-pinning in a `.<tool>-version` file is this repository's established convention
  — `.golangci-version` (2.12.2), `.sqlc-version` (1.31.1), `.tygo-version` (0.2.21) — each
  read by a `scripts/*-check.sh` that fails loudly if the tool is missing. A fourth pin
  follows the pattern exactly rather than inventing a new one.
- Installing via `go install` rather than an off-the-shelf action keeps the tool version
  under the same pin locally and in CI, so `make lint-vuln` and the workflow cannot drift.

**Alternatives considered**:

- *`golang/govulncheck-action`.* Convenient, but it pins the tool version inside the
  action rather than in the repository, so a local run and a CI run can disagree — and it
  offers no hook for the exception mechanism FR-009 needs.
- *Trivy / osv-scanner in `go.mod` mode.* Both report on the dependency graph without
  reachability, producing exactly the noise FR-006 forbids. osv-scanner remains a good
  candidate for a future *image* scan, which is explicitly out of scope here.
- *`nancy`, `snyk`.* Third-party service or account dependency, ruled out by the
  specification's assumption that no new service account is introduced.

### 1.1 The exception mechanism (FR-009)

`govulncheck` has no native ignore file — a deliberate upstream choice. FR-009 still
requires a reviewable way to record a temporary exception for an advisory with no fix.

**Decision**: `govulncheck -format json ./...` piped into `scripts/govulncheck-check.sh`,
which drops findings whose OSV identifier appears in `apps/api/.govulncheck-ignore`. Each
line of that file is `GO-YYYY-NNNN  <expiry YYYY-MM-DD>  <reason>`; the script fails if an
entry is malformed, and fails if an entry's expiry date has passed — an expired exception
becomes a build failure rather than permanent silence.

**Rationale**: it satisfies "reviewable in a diff, with a reason, not a blanket disable"
while keeping the mechanism in the same shell-script idiom as `sqlc-check.sh` and
`tygo-check.sh`. The expiry field is what stops an exception file from becoming a
graveyard: the only way to keep an exception is to renew it in a reviewed diff.

**Alternatives considered**: allowing the exception in a workflow-level `continue-on-error`
or a `|| true`. Rejected — it disables the whole gate, not one finding, and leaves no
record of why.

---

## 2. JavaScript dependency auditing

**Decision**: `pnpm audit --audit-level=high --prod=false` at the workspace root, run
after a frozen install, with exceptions recorded under `pnpm.auditConfig.ignoreCves` in
the root `package.json`.

**Rationale**:

- `pnpm audit` reads the workspace lockfile, so FR-010 holds by construction once § 0.1
  lands. It needs no extra tooling beyond the pnpm already installed for every web job.
- `--audit-level=high` makes the severity floor explicit as FR-007 demands. `high` rather
  than `moderate` is the right floor for this repository's shape: the dashboard is a
  self-hosted single-user front end with no untrusted multi-tenant input, and the
  overwhelming majority of `moderate` findings in a Vite/React devDependency tree are
  build-time-only regular-expression denial-of-service advisories that cannot be reached
  by a deployed static bundle. Starting at `high` keeps the gate credible; it can be
  tightened later without a design change.
- `pnpm.auditConfig.ignoreCves` is pnpm's native, in-`package.json` allowlist, so an
  exception is reviewable in a diff (FR-009). Reasons go in a sibling comment block in the
  domain doc, since JSON carries no comments.

**Alternatives considered**:

- *`osv-scanner` over `pnpm-lock.yaml`.* Would unify the Go and JS tooling, but loses
  pnpm's own allowlist and adds a second advisory source with different severity
  semantics.
- *`--audit-level=moderate`.* Rejected for now on the noise argument above; recorded here
  so the choice is a decision rather than a default.
- *Failing on `--prod` only.* Rejected: the build toolchain is part of the supply chain,
  and a compromised build-time package is the more realistic attack on a project like
  this one.

---

## 3. Secret scanning

**Decision**: `gitleaks` as a pinned release binary downloaded in a workflow step (not the
marketplace action), configured by a repository-root `.gitleaks.toml` that extends the
default rule set, invoked with `--redact` over the pull request's own commit range.

**Rationale**:

- **Not the action.** `gitleaks/gitleaks-action@v2` requires a `GITLEAKS_LICENSE` secret
  for organization-owned repositories, and its licensing terms are a moving target. A
  pinned binary from the upstream release page is the same scanner with no account, no
  secret, and no licence question — which the specification's assumptions require.
- **Scope to the pull request range** (`gitleaks detect --log-opts="<base>..<head>"`,
  with `fetch-depth: 0` on checkout) satisfies FR-013: a finding that predates this
  feature does not fail an unrelated pull request. On pushes to `master` the range is the
  previous commit to the new one, matching how `dorny/paths-filter` already degrades on
  push.
- **`--redact`** satisfies FR-015 — gitleaks prints the rule, file, and line but replaces
  the matched value, so the workflow log does not become the second copy of the leak.
- **`.gitleaks.toml` extending the defaults** covers FR-012 (project-specific shapes:
  the 64-hex-character `CONFIG_ENCRYPTION_KEY`, and the four provider key prefixes) and
  FR-014 (an `[allowlist]` block with paths and a `regexes` list, each with a comment
  giving the reason). `.env.example` and the `gateway/config.yaml` placeholder values are
  the first allowlist entries.

**Alternatives considered**:

- *`trufflehog`.* Strong verification feature (it can call the provider to confirm a key
  is live), but that means outbound calls from CI to third-party APIs on every pull
  request — at odds with the project's local-first posture, and a way to *use* a leaked
  key from CI.
- *GitHub's native secret scanning + push protection.* The right long-term answer and
  free for public repositories, but for a private repository it is a GitHub Advanced
  Security feature, unavailable on the Free tier this repository is on — the same
  constraint that already blocks branch protection.
- *A hand-rolled `grep` gate.* Rejected: rule maintenance is the whole cost of secret
  scanning, and gitleaks' default rule set is the artefact worth borrowing.

### 3.1 The one-time full-history scan (FR-016)

Run locally, once, during delivery: `gitleaks detect --redact --report-path` over the full
history, with the result recorded in the domain doc as either "no findings" or a list with
a rotation decision each. It is not wired into the workflow, because a full-history scan
grows without bound and its findings are, by definition, not this pull request's fault.

---

## 4. Container image build gate

**Decision**: two separate jobs — `build-api-image` and `build-dashboard-image` — each
using `docker/setup-buildx-action@v3` plus `docker/build-push-action@v6` with
`push: false`, `load: false`, and GitHub Actions layer caching
(`cache-from: type=gha`, `cache-to: type=gha,mode=max`, scoped per image).

**Rationale**:

- **Two jobs, not one** (FR-020): the check name itself says which image broke, and the
  two run in parallel, so the gate's wall-clock cost is the slower image, not the sum.
- **`push: false`, `load: false`** (FR-018): builds inside buildkit and discards the
  result. No registry, no credential, no artefact upload — and skipping `load` avoids
  paying to export the image to the runner's docker daemon, which is pure cost when
  nothing consumes it.
- **`type=gha` cache** (FR-019): buildkit's GitHub Actions cache backend is
  content-addressed by layer, so a genuine change to a `COPY`ed file invalidates from that
  layer onward and cannot serve a stale success — the concern FR-019 raises. Scoping the
  cache per image (`scope: api` / `scope: dashboard`) keeps the two from evicting each
  other within the 10 GB per-repository budget.
- **Cost estimate**: the API image's expensive layers are `apt-get` (chromium, poppler)
  and the `rendercv[full]` pip install; both sit above the `COPY apps/api .` line, so a
  typical Go change reuses them and rebuilds only `go build`. The dashboard image's
  expensive layer is `pnpm install`, above the source copies, so a typical dashboard change
  rebuilds only the Vite build. Warm-cache runs are expected in the 1–3 minute range
  against a cold cost of roughly 6–8 minutes for the API image.

**Alternatives considered**:

- *`docker compose build`.* One command for both images, but a single check for two
  images (violating FR-020), no buildkit cache export, and it drags in compose file
  parsing and its `${VAR:?}` required-variable guards, which would need a dummy `.env`.
- *`type=registry` cache.* Faster and more durable, but needs registry credentials, which
  FR-018 rules out.
- *Building only the changed image.* Already achieved through the paths filters (FR-021),
  which is where that logic belongs.

### 4.1 Paths filters for the image jobs (FR-021)

The two build contexts read different things, and getting this wrong is how an image break
merges green:

- **API image**: context is the repository root but it only `COPY`s `apps/api/**`. Filter:
  `apps/api/**` — which is exactly the existing `go` filter's first entry. Reuse the `go`
  anchor rather than duplicating it, and accept that a `gateway/**`-only change (also in
  the `go` anchor) will build the API image unnecessarily; a false *run* is cheap, a false
  *skip* is not.
- **Dashboard image**: `COPY`s `pnpm-workspace.yaml`, root `package.json`,
  `packages/shared/**`, `apps/dashboard/**`, `tsconfig.base.json`, and — after § 0.2 —
  `pnpm-lock.yaml`. The existing `web` filter already lists every one of those. Reuse it.

**Decision**: no new paths filters. The image jobs gate on the existing `go` and `web`
outputs, with an inline comment recording the mapping from each `COPY` line to the filter
entry that covers it. The vulnerability jobs gate on `go` and `web` respectively; the
secret-scan job needs a new filter, discussed next.

---

## 5. Change detection for the secret-scan job

The secret scan is the one gate whose verdict *any* file can change — a leaked key in a
`specs/` markdown file is still a leaked key. But the existing design deliberately makes
`specs/**`, `docs/**`, `AGENTS.md`, `README.md`, `Makefile`, and `.github/workflows/**`
appear in no filter, so that a docs-only pull request runs nothing.

**Decision**: add a new filter output, `any`, matching `'**'`, and gate the secret-scan job
on it. In practice that means the secret scan runs on every pull request that changes any
tracked file.

**Rationale**: FR-011 says "any tracked file", and the cost is a ~20-second job — far
below the threshold that motivated the change-detection design in the first place (the
comment in the workflow cites ~20 runner-minutes for a full Go and Node rebuild). Keeping
it inside the filter mechanism rather than dropping the `if:` entirely means it still
reports uniformly with every other check, and the `any` output stays available if a later
gate needs the same semantics.

**Alternatives considered**: omitting the `if:` and letting the job always run. Functionally
identical today, but it makes the secret-scan job the one job in the file that does not
follow the established shape, and there is no benefit to the exception.

---

## 6. Dependency bot configuration

**Decision**: `.github/dependabot.yml`, version 2, four ecosystem entries:

| Ecosystem | Directory | Grouping | Open PR cap |
|-----------|-----------|----------|-------------|
| `gomod` | `/apps/api` | one group for all minor+patch; majors individual | 5 |
| `npm` | `/` | one group for all minor+patch; majors individual | 5 |
| `github-actions` | `/` | single group, all updates | 3 |
| `docker` | `/apps/api`, `/apps/dashboard` | single group per directory | 3 |

Schedule: weekly, Monday, so a review batch lands at a predictable point in the week
rather than trickling in.

**Rationale**:

- **`gomod` at `/apps/api`**, not `/`: the Go module lives at `apps/api/go.mod`; there is
  no root module. Pointing the ecosystem at `/` finds nothing and fails silently, which is
  the most common way this file is misconfigured.
- **`npm` at `/`** covers the pnpm workspace. Dependabot understands `pnpm-lock.yaml` and
  workspace protocols; it reads the root lockfile and updates the member `package.json`
  that owns each dependency. This entry is inert until § 0.1 commits the lockfile — another
  reason that task is sequenced first.
- **`docker` at both Dockerfile directories**: Dependabot's docker ecosystem scans a
  directory for Dockerfiles, so `apps/api` (`golang:1.26-bookworm`,
  `python:3.12-slim-bookworm`) and `apps/dashboard` (`node:22-bookworm`, `nginx:alpine`)
  each need their own entry.
- **Grouping minor+patch but not majors** (FR-024): a grouped major bump hides a breaking
  change behind a diff of twenty other packages. Majors arriving individually keeps each
  reviewable, and the caps keep the total bounded.
- **Caps in the file** (FR-025): `open-pull-requests-limit` is a config key, not a
  service-side setting, so the cadence stays a reviewable fact.

**Alternatives considered**:

- *Renovate.* More expressive grouping and a genuinely better scheduler, but it is a
  GitHub App install — an external automation account, which the specification's
  assumptions exclude. Worth revisiting if Dependabot's grouping proves too coarse.
- *Daily schedule.* Rejected: for a single-maintainer repository it produces a review
  queue nobody drains, and the weekly batch loses nothing that matters at this scale.
- *`gomod` grouping by dependency family.* Premature; revisit if the flat minor+patch
  group turns out to be routinely large.

### 6.1 What Dependabot cannot cover

`docker-compose.yml` and `docker-compose.prod.yml` pin `pgvector/pgvector:pg16`,
`redis:7-alpine`, `minio/minio:latest`, `ghcr.io/flaresolverr/flaresolverr:latest`,
`clickhouse`, and the Langfuse images. Dependabot's docker ecosystem does not read compose
files. These stay manual, and that gap is recorded in the domain doc rather than left as a
silent assumption. (`:latest` tags on two of them are their own problem, out of scope here.)

---

## 7. Local parity

Every gate that can run locally gets a `make` target, per the constitution's "use `make`
targets as the canonical entry points" rule:

- `make vuln-go` → `scripts/govulncheck-check.sh`
- `make vuln-web` → `pnpm audit --audit-level=high --prod=false`
- `make secrets` → gitleaks over the working tree, redacted
- `make images` → builds both images locally with plain `docker build`

`make lint` and `make test-lint` are left alone. Adding a container build to the merge-gate
alias would make the loop every contributor runs before pushing dramatically slower, and
the CI gate already covers it. The new targets are opt-in, and the domain doc says when to
reach for each.

---

## 8. Summary of decisions

| # | Question | Decision |
|---|----------|----------|
| 0.1 | Untracked lockfile | Un-ignore and commit `pnpm-lock.yaml`; task zero |
| 0.2 | Image build ignores lockfile | Copy lockfile, install `--frozen-lockfile` |
| 1 | Go vulnerabilities | `govulncheck`, pinned in `apps/api/.govulncheck-version` |
| 1.1 | Go exceptions | `apps/api/.govulncheck-ignore`, id + expiry + reason, enforced by script |
| 2 | JS vulnerabilities | `pnpm audit --audit-level=high --prod=false`; `pnpm.auditConfig.ignoreCves` |
| 3 | Secrets | pinned gitleaks binary, `.gitleaks.toml`, `--redact`, PR commit range |
| 3.1 | History audit | one-time local full-history scan, result recorded in domain doc |
| 4 | Image builds | two jobs, buildx, `push:false`, `type=gha` cache scoped per image |
| 4.1 | Image filters | reuse existing `go` and `web` outputs; no new filters |
| 5 | Secret-scan filter | new `any: '**'` output |
| 6 | Dependency bot | `.github/dependabot.yml`, 4 ecosystems, weekly, grouped, capped |
| 7 | Local parity | four new `make` targets; `make test-lint` unchanged |

No NEEDS CLARIFICATION markers remain.
