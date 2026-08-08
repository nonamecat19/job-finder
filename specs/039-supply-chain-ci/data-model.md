# Phase 1 Data Model: Supply-chain and build-integrity CI gates

**Feature**: `039-supply-chain-ci` | **Date**: 2026-08-08

This feature persists nothing in Postgres and adds no migration, DTO, or generated type.
Its "entities" are configuration records held in tracked files, each with a schema, a
validation rule, and an owner. This document defines them so the contracts in
[`contracts/`](./contracts/) and the tasks in `tasks.md` have one shared vocabulary.

---

## 1. Gate

A named CI check with a pass/skip/fail verdict.

| Field | Type | Source of truth | Notes |
|-------|------|-----------------|-------|
| `name` | string | the job's `name:` in `.github/workflows/api-ci.yml` | **This is the contract.** A ruleset lists it verbatim; renaming silently un-gates the check (`specs/domains/platform-operations.md` § 2.1). |
| `filter` | enum: `go` \| `web` \| `sqlc` \| `tygo` \| `any` | the `changes` job's outputs | Determines skip-vs-run. `any` is new in this feature. |
| `trigger` | set | `on:` plus the filter's push-override | Always `{pull_request, push:master, manual re-run}` for every gate in this feature. |
| `budget` | duration | recorded in the domain doc table | Used to hold the ≤10-minute parallel wall-clock target. |
| `verdict` | enum: `success` \| `skipped` \| `failure` | GitHub | **`skipped` counts as passing.** A gate must never be able to produce *absent*. |

**Invariants**

- **G1**: every gate has a non-empty `if:` referencing a `changes` output. A gate with no
  `if:` cannot report `skipped` and burns runner minutes on docs-only pull requests.
- **G2**: every gate's `filter` output folds in `github.event_name != 'pull_request'`, so a
  push to `master` runs the full set regardless of paths.
- **G3**: `name` is unique across the workflow and stable across the feature's lifetime.
- **G4**: every gate appears in the § 2.1 table of `specs/domains/platform-operations.md`
  and in the § 2.2 ruleset's required-checks list. A gate absent from § 2.2 runs but never
  gates.
- **G5**: `detect changed areas` is not a gate and is never listed in a ruleset.

**The six gates this feature adds**

| `name` | `filter` | Fails when |
|--------|----------|------------|
| `secret scan` | `any` | a recognised secret pattern appears in the diff's commit range |
| `vulnerability scan (go)` | `go` | a reachable, non-excepted advisory affects a Go dependency |
| `vulnerability scan (web)` | `web` | a workspace dependency has a non-ignored advisory ≥ `high` |
| `build image (api)` | `go` | `docker build -f apps/api/Dockerfile .` fails |
| `build image (dashboard)` | `web` | `docker build -f apps/dashboard/Dockerfile .` fails |

Five job names, six gate concerns — the two vulnerability gates are separate jobs because
they run on different filters and different toolchains.

---

## 2. Advisory finding

Produced by a scanner, consumed by a gate. Not persisted; modelled because both scanners
must yield the same shape for the failure output required by FR-008.

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `id` | string | yes | `GO-YYYY-NNNN` (Go) or `CVE-YYYY-NNNNN` / `GHSA-xxxx` (npm). |
| `package` | string | yes | Module path or npm package name. |
| `affected_version` | string | yes | The version currently resolved in the tree. |
| `fixed_version` | string \| null | yes | `null` when upstream has published no fix — the case FR-009's exception mechanism exists for. |
| `severity` | enum: `low` \| `moderate` \| `high` \| `critical` | npm only | `govulncheck` does not assign severity; it assigns reachability instead. |
| `reachable` | bool | Go only | FR-006: `false` findings are printed and do not fail the build. |
| `trace` | string | Go, when `reachable` | The calling path, so the reader knows which of their own functions is implicated. |

**Invariants**

- **A1**: a finding with `reachable == false` never fails the Go gate.
- **A2**: a finding with `severity < high` never fails the web gate.
- **A3**: every failing finding's rendered output names `id`, `package`, and
  `fixed_version` (or states that no fix exists). FR-008.

---

## 3. Exception record (Go)

One line of `apps/api/.govulncheck-ignore`. Suppresses exactly one advisory, temporarily.

| Field | Type | Required | Validation |
|-------|------|----------|-----------|
| `id` | string | yes | Matches `^GO-[0-9]{4}-[0-9]{4,}$`. |
| `expires` | date | yes | `YYYY-MM-DD`. Must parse; must be in the future at check time. |
| `reason` | string | yes | Free text to end of line, non-empty after trimming. |

**Grammar and lifecycle** — see [`contracts/file-formats.md`](./contracts/file-formats.md) § 1.

**Invariants**

- **E1**: a malformed line fails the gate. A silently-skipped bad line is a silently
  disabled exception, which is worse than either outcome.
- **E2**: an entry whose `expires` has passed fails the gate, naming the entry. Renewal is
  a reviewed diff; there is no way to let an exception persist unattended.
- **E3**: an entry for an advisory that no longer appears in any finding fails the gate as
  *stale*. This keeps the file from accumulating dead entries that read as live risk.
- **E4**: the file may be empty (only comments), and empty is the expected steady state.

---

## 4. Exception record (web)

An entry in `pnpm.auditConfig.ignoreCves` in the root `package.json`.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | `CVE-…` or `GHSA-…`, as pnpm expects. |

JSON carries no comments, so the reason and expiry cannot live beside the entry. **The
reason lives in `specs/domains/platform-operations.md`**, in a table keyed by the same
identifier, and an entry present in `package.json` with no matching row in that table is a
review defect. This asymmetry with the Go mechanism is a cost of using pnpm's native
allowlist rather than a bespoke one; it buys the guarantee that pnpm and the gate agree
about what is ignored.

**Invariants**

- **W1**: every id in `ignoreCves` has a documented reason in the domain doc.
- **W2**: the array starts empty. A non-empty array shipped with the feature would mean
  the gate went in already muted.

---

## 5. Allowlist entry (secrets)

A rule in the `[allowlist]` block of `.gitleaks.toml`.

| Field | Type | Notes |
|-------|------|-------|
| `paths` | list of regex | Files whose secret-shaped content is legitimate. |
| `regexes` | list of regex | Value shapes that are legitimate anywhere (placeholders). |
| `commits` | list of sha | Escape hatch for a specific historical commit. Expected to stay empty. |
| *reason* | TOML comment | Required by convention above each entry; enforced by review, not by tooling. |

**Seed entries** (research § 3): `.env.example` (deliberately key-shaped placeholders),
`gateway/config.yaml` (placeholder values, already covered by its own credential test),
and this feature's own `specs/039-supply-chain-ci/**` (which documents key *names* and
example patterns).

**Invariants**

- **S1**: no allowlist entry may be a bare `.*` path or regex.
- **S2**: an allowlist entry without a reason comment is a review defect.

---

## 6. Custom secret rule

A `[[rules]]` block in `.gitleaks.toml`, extending the default rule set rather than
replacing it.

| Field | Type | Notes |
|-------|------|-------|
| `id` | string | Stable, kebab-case; appears in failure output. |
| `description` | string | What a match means, in one line. |
| `regex` | regex | The value shape. |
| `keywords` | list of string | Prefilter, so the scan stays fast. |
| `entropy` | float \| null | Optional; used where the shape alone is too generic. |

**The rules this feature adds** (FR-012): the configuration encryption key (64 hex
characters bound to its variable name, since bare 64-hex is far too generic to match
alone), and one rule per provider key prefix for Cerebras, Groq, Cohere, and OpenRouter.

**Invariants**

- **R1**: `.gitleaks.toml` sets `[extend] useDefault = true`. Replacing the default rule
  set would trade a maintained corpus for five hand-written rules.
- **R2**: no rule's `regex` may match any value present in `.env.example`, or every pull
  request fails. Verified by running the scan against the tree during delivery.

---

## 7. Update policy

One entry in the `updates:` array of `.github/dependabot.yml`.

| Field | Type | Value for this feature |
|-------|------|------------------------|
| `package-ecosystem` | enum | `gomod` \| `npm` \| `github-actions` \| `docker` |
| `directory` | path | `/apps/api`, `/`, `/`, and `/apps/api` + `/apps/dashboard` respectively |
| `schedule.interval` | enum | `weekly` |
| `schedule.day` | enum | `monday` |
| `groups` | map | minor+patch grouped; majors ungrouped (`gomod`, `npm`). Single group (`github-actions`, `docker`). |
| `open-pull-requests-limit` | int | 5 for `gomod`/`npm`, 3 for `github-actions`/`docker` |
| `target-branch` | string | omitted — defaults to the repository default branch, `master` |

**Invariants**

- **U1**: `gomod`'s `directory` is `/apps/api`. There is no root Go module; pointing at `/`
  finds nothing and reports no error, which is the failure mode this invariant exists to
  prevent.
- **U2**: `npm`'s entry is inert until `pnpm-lock.yaml` is tracked (research § 0.1).
- **U3**: `docker` needs one entry per directory containing a Dockerfile.
- **U4**: every ecosystem declares a cap. An uncapped ecosystem can open unbounded pull
  requests against a single-maintainer repository.

---

## 8. Relationships

```text
Update policy ──opens──▶ pull request ──runs──▶ Gate (×14: 9 existing + 5 new)
                                                  │
                          ┌───────────────────────┼───────────────────────┐
                          ▼                       ▼                       ▼
                  Advisory finding          secret match            image build
                          │                       │                       │
                  suppressed by            suppressed by            (no suppression
                          │                       │                    mechanism —
              Exception record (Go)      Allowlist entry              a broken build
              Exception record (web)     + Custom secret rule          has no excuse)
```

The asymmetry at the bottom right is deliberate: a vulnerability or a secret match can be
a false positive or an unfixable upstream fact, so both need a reviewable escape hatch. A
container image that does not build is never a false positive.

---

## 9. What this feature does *not* model

- No database table, column, index, or migration.
- No Go struct, DTO, or tygo-generated TypeScript type.
- No HTTP endpoint, request shape, or response shape.
- No asynq task type or queue.
- No user-visible dashboard state.

Consequently the `sqlc-drift`, `tygo-drift`, and `shared-types` gates should all report
*skipped* on this feature's own pull request, except where the committed `pnpm-lock.yaml`
trips the `web` filter and runs the web set. That expectation is itself a check on the
work: if `sqlc generate is up to date` runs, something outside this feature's scope was
touched.
