# Contract: configuration file formats

**Feature**: `039-supply-chain-ci`

Grammars and required shapes for the files this feature introduces. Each is the interface
between a human writing an exception and a script or service enforcing a gate.

---

## 1. `apps/api/.govulncheck-ignore`

Line-oriented UTF-8 text. Parsed by `scripts/govulncheck-check.sh`.

```ebnf
file       = { line } ;
line       = comment | blank | entry ;
comment    = "#" , { any-char } , newline ;
blank      = { space | tab } , newline ;
entry      = id , separator , date , separator , reason , newline ;
id         = "GO-" , 4*digit , "-" , 4*digit , { digit } ;
separator  = 1*( space | tab ) ;
date       = 4*digit , "-" , 2*digit , "-" , 2*digit ;   (* YYYY-MM-DD *)
reason     = non-space-char , { any-char } ;             (* to end of line *)
```

### Example

```text
# Advisory exceptions for govulncheck. Each entry suppresses ONE advisory
# until its expiry date, after which the gate fails until the entry is
# renewed in a reviewed diff. See specs/domains/platform-operations.md.
#
# <GO-id>  <expiry YYYY-MM-DD>  <reason>

GO-2026-1234  2026-10-01  No fixed version upstream; reachable only from the
```

*(a reason wraps by continuing on the same logical line only — the parser does not join
lines, so a reason must fit on one line.)*

### Parser rules

| Rule | Behaviour on violation |
|---|---|
| Malformed `id` | **fail** the gate, print the line number and the expected shape |
| Unparseable `date` | **fail**, same |
| Empty `reason` | **fail**, same |
| `date` in the past | **fail**, naming the expired id and its date |
| `id` present but matching no current finding | **fail** as *stale*, instructing removal |
| Duplicate `id` | **fail**, naming both line numbers |
| Comment or blank line | ignore |

A malformed line is never skipped silently: a bad exception line means someone believes an
advisory is suppressed when it is not, or vice versa, and both directions are dangerous.

### Exit codes of `scripts/govulncheck-check.sh`

| Code | Meaning |
|---|---|
| `0` | no findings, or every finding is unreachable or validly excepted |
| `1` | at least one reachable, non-excepted finding |
| `2` | the ignore file is malformed, or an entry is expired or stale |
| `3` | `govulncheck` is not installed, or its version differs from the pin |

Code `3`'s message mirrors `scripts/sqlc-check.sh` verbatim in structure — pinned version,
installed version, why it matters, install line — as § 3 of the operations domain doc
requires of every version guard.

### `--self-test`

Invoking the script with `--self-test` exercises the parser against in-script fixtures
covering each row of the table above and exits non-zero on any mismatch. It runs no scan
and needs no network, so `go test`-adjacent tooling is unnecessary and the parser's failure
modes are provable without waiting for a real advisory.

---

## 2. `.gitleaks.toml`

```toml
# Extends the upstream default rule set — never replaces it.
[extend]
useDefault = true

# One [[rules]] block per project-specific credential shape.
[[rules]]
id          = "job-finder-config-encryption-key"
description = "CONFIG_ENCRYPTION_KEY (32-byte hex) bound to its variable name"
regex       = '''...'''
keywords    = ["CONFIG_ENCRYPTION_KEY"]

# ... one rule per provider key prefix ...

[allowlist]
description = "Placeholders and documentation that are legitimately key-shaped"
paths       = [ '''...''' ]
regexes     = [ '''...''' ]
```

### Required properties

| Property | Requirement |
|---|---|
| `[extend] useDefault` | must be `true` (data-model R1) |
| Every `[[rules]].id` | stable, kebab-case, prefixed `job-finder-` for project rules |
| Every `[[rules]]` | has `keywords`, so the prefilter keeps the scan fast |
| Every rule bound to a variable name | matches the *assignment*, not the bare value — a bare 64-hex regex matches commit hashes, test vectors, and half the lockfile |
| Every `[allowlist]` entry | preceded by a TOML comment giving the reason (data-model S2) |
| No allowlist entry | may be a bare `.*` (data-model S1) |
| No rule | may match any value in `.env.example` (data-model R2) |

### Invocation contract

```bash
gitleaks detect --redact --no-banner \
  --config .gitleaks.toml \
  --log-opts "<base>..<head>"
```

- `--redact` is **mandatory** (FR-015). Without it the workflow log becomes a second copy
  of the leak, readable by anyone who can read the run.
- `--log-opts` scopes to the pull request's own commits (FR-013), which requires
  `fetch-depth: 0` on `actions/checkout`.
- Exit `0` = clean, `1` = findings, `2` = usage or config error. The job must distinguish
  `1` from `2` in its failure message so a broken config does not read as a leak.

---

## 3. `.github/dependabot.yml`

Dependabot config schema version 2. Four `updates:` entries.

```yaml
version: 2
updates:
  - package-ecosystem: gomod
    directory: /apps/api          # NOT / — there is no root Go module (data-model U1)
    schedule: { interval: weekly, day: monday }
    open-pull-requests-limit: 5
    groups:
      go-minor-patch:
        patterns: ["*"]
        update-types: [minor, patch]

  - package-ecosystem: npm         # reads pnpm-lock.yaml; inert until it is tracked (U2)
    directory: /
    schedule: { interval: weekly, day: monday }
    open-pull-requests-limit: 5
    groups:
      npm-minor-patch:
        patterns: ["*"]
        update-types: [minor, patch]

  - package-ecosystem: github-actions
    directory: /
    schedule: { interval: weekly, day: monday }
    open-pull-requests-limit: 3
    groups:
      actions:
        patterns: ["*"]

  - package-ecosystem: docker
    directory: /apps/api           # golang:1.26-bookworm, python:3.12-slim-bookworm
    schedule: { interval: weekly, day: monday }
    open-pull-requests-limit: 3

  - package-ecosystem: docker
    directory: /apps/dashboard     # node:22-bookworm, nginx:alpine
    schedule: { interval: weekly, day: monday }
    open-pull-requests-limit: 3
```

### Required properties

| Property | Requirement |
|---|---|
| `version` | `2` |
| Every entry | declares `open-pull-requests-limit` (data-model U4) |
| Every entry | declares `schedule.interval: weekly` and `schedule.day: monday` (FR-025) |
| `gomod.directory` | `/apps/api` (U1) |
| `docker` | one entry per Dockerfile directory (U3) |
| Major updates | never inside a `groups` entry for `gomod` / `npm` — they arrive individually so each is reviewable (FR-024) |
| `target-branch` | omitted, so updates target the default branch `master` (FR-023) |

### What this file cannot cover

`docker-compose.yml` and `docker-compose.prod.yml` pin `pgvector/pgvector:pg16`,
`redis:7-alpine`, `minio/minio:latest`, `ghcr.io/flaresolverr/flaresolverr:latest`, the
ClickHouse image, and the Langfuse images. Dependabot's docker ecosystem reads Dockerfiles,
not compose files, so these stay manual. Recorded in the operations domain doc so the gap
is a known fact rather than a silent assumption.

---

## 4. `pnpm.auditConfig.ignoreCves` (root `package.json`)

```json
{
  "pnpm": {
    "auditConfig": {
      "ignoreCves": []
    }
  }
}
```

Ships **empty** (data-model W2). JSON carries no comments, so every id added here must gain
a row in the exceptions table in `specs/domains/platform-operations.md` giving the reason,
the fixed-version status, and a review date. An id here with no row there is a review
defect.

---

## 5. Version pin files

`apps/api/.govulncheck-version` and `.gitleaks-version`. Each contains exactly one
version string and a trailing newline, no `v` prefix, matching the three existing pins
(`.golangci-version` → `2.12.2`, `.sqlc-version` → `1.31.1`, `.tygo-version` → `0.2.21`).

Read with `tr -d '[:space:]' < "$VERSION_FILE"`, the idiom every existing check script and
workflow step already uses.
