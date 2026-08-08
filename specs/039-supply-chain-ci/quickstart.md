# Quickstart: validating the supply-chain gates

**Feature**: `039-supply-chain-ci` | **Date**: 2026-08-08

How to prove each gate works — positively (it passes on a clean tree) and negatively (it
fails on a planted defect). The negative cases are what SC-001 through SC-003 measure, so
they are deliverables, not optional extras. Each one plants a defect, observes the failure,
and reverts.

## Prerequisites

```bash
# Go toolchain matching apps/api/go.mod (1.26.x)
go version

# pnpm 11 via corepack
corepack enable && pnpm --version

# Docker with buildx
docker buildx version

# The two new pinned tools
go install golang.org/x/vuln/cmd/govulncheck@v$(tr -d '[:space:]' < apps/api/.govulncheck-version)
# gitleaks: download the pinned release for your platform
tr -d '[:space:]' < .gitleaks-version
```

`gh` authenticated against the repository is needed for the CI-side checks.

---

## 0. Confirm the starting state (before any work)

The tree is currently **red on `master`** for a reason this feature fixes first
(research § 0.1). Confirm it, so the repair is observable:

```bash
gh run list --branch master --limit 1
gh run view <id> --json jobs -q '.jobs[] | "\(.conclusion)\t\(.name)"'
```

Expected before Phase A: `frontend test (vitest)`, `frontend typecheck`, and `lint (web)`
report `failure`; every Go job reports `success`.

---

## Phase A — the lockfile

```bash
# after removing the pnpm-lock.yaml line from .gitignore
pnpm install
git status --short pnpm-lock.yaml     # expect: ?? -> A after `git add`
pnpm install --frozen-lockfile        # must now succeed with no lockfile mutation
git diff --exit-code pnpm-lock.yaml   # must be clean
```

**Expected**: `--frozen-lockfile` succeeds and does not modify the lockfile. If it does
modify it, the committed lockfile is out of step with the manifests and must be
regenerated before proceeding.

Then, on the pull request: the three web jobs go from `failure` to `success`. That
transition is the whole of Phase A's evidence.

---

## 1. Secret scan

### Positive

```bash
make secrets
```

**Expected**: exit `0`, no findings. If `.env.example` or `gateway/config.yaml` produce a
finding, the allowlist is incomplete (data-model R2) — fix the config, not the fixture.

### Negative (SC-002 evidence)

```bash
# Plant a key-shaped value the rules should catch. Use a value that is
# syntactically valid but has never been issued.
printf 'CONFIG_ENCRYPTION_KEY=%s\n' "$(printf '0123456789abcdef%.0s' 1 2 3 4)" \
  > /tmp/leak-test.env
git add -f /tmp/leak-test.env 2>/dev/null || cp /tmp/leak-test.env ./leak-test.env
git add leak-test.env && git commit -m "test: planted secret (to be reverted)"
make secrets
```

**Expected**: exit `1`. Output names the rule id (`job-finder-config-encryption-key`), the
file, and the line — and the matched value appears **redacted**, not in full. A full value
in the output is a FR-015 failure and blocks the feature.

```bash
git reset --hard HEAD~1 && rm -f leak-test.env
```

Repeat once per provider-key rule, or at minimum for one provider prefix, to prove the
project rules and not only the default rule set are active.

### Full-history audit (FR-016, SC-007)

Run once, locally, and record the result:

```bash
gitleaks detect --redact --no-banner --config .gitleaks.toml \
  --report-path /tmp/history-scan.json
jq 'length' /tmp/history-scan.json
```

**Expected**: `0`. If non-zero, each finding needs a recorded rotation decision in
`specs/domains/platform-operations.md` before the feature ships. Do **not** rewrite history
as part of this feature — that is a separate, explicitly approved operation (spec, Out of
Scope).

---

## 2. Go vulnerability scan

### Positive

```bash
make vuln-go
```

**Expected**: exit `0`. Unreachable findings, if any, are printed as informational
(FR-006) and do not fail.

### Parser self-test

```bash
./scripts/govulncheck-check.sh --self-test
```

**Expected**: exit `0`, having exercised malformed-id, bad-date, empty-reason,
expired-entry, stale-entry, and duplicate-id fixtures.

### Negative (SC-003 evidence, Go side)

```bash
cd apps/api
git switch -c scratch-vuln-test
# Downgrade a dependency to a version with a known reachable advisory.
# Pick one from https://pkg.go.dev/vuln/ that this module actually calls into.
go get <module>@<vulnerable-version> && go mod tidy
cd .. && make vuln-go
```

**Expected**: exit `1`. Output names the `GO-YYYY-NNNN` id, the module, the fixed version,
and the calling trace.

Then prove the exception mechanism:

```bash
printf 'GO-YYYY-NNNN  %s  scratch test, reverted immediately\n' \
  "$(date -d '+30 days' +%F)" >> apps/api/.govulncheck-ignore
make vuln-go   # expect exit 0

# and prove expiry is enforced
sed -i "s/$(date -d '+30 days' +%F)/2020-01-01/" apps/api/.govulncheck-ignore
make vuln-go   # expect exit 2, naming the expired entry
```

```bash
git switch - && git branch -D scratch-vuln-test
```

---

## 3. Web dependency audit

### Positive

```bash
make vuln-web
```

**Expected**: exit `0` against the committed lockfile. A first-run failure here is a real
finding, not a setup problem — respond with a bump, or with an `ignoreCves` entry plus its
domain-doc row (data-model W1).

### Negative

```bash
pnpm add -D <package>@<version-with-high-advisory>
make vuln-web
```

**Expected**: exit `1`, naming the package, the advisory id, and the severity. Confirm the
severity floor holds by also installing a package with a `moderate`-only advisory and
observing that the gate passes while listing it.

```bash
pnpm remove <package> && git checkout pnpm-lock.yaml package.json
```

---

## 4. Container image builds

### Positive

```bash
make images
```

which is equivalent to:

```bash
docker build -f apps/api/Dockerfile       -t job-finder-api:ci-check       .
docker build -f apps/dashboard/Dockerfile -t job-finder-dashboard:ci-check .
```

**Expected**: both succeed. The dashboard build must now install with `--frozen-lockfile`
(research § 0.2) — if it fails there, the committed lockfile and the manifests copied into
the image disagree, which is exactly the drift the change exists to surface.

### Negative (SC-001 evidence)

```bash
git switch -c scratch-image-test
sed -i 's/^FROM golang:1.26-bookworm AS build$/FROM golang:1.26-bookworm AS buildx/' \
  apps/api/Dockerfile
make images     # expect failure naming the api image and the missing stage
git checkout apps/api/Dockerfile

sed -i 's|COPY apps/dashboard/nginx.conf|COPY apps/dashboard/nginx.conf.missing|' \
  apps/dashboard/Dockerfile
make images     # expect failure naming the dashboard image
git checkout apps/dashboard/Dockerfile
git switch - && git branch -D scratch-image-test
```

Push each planted break as its own scratch pull request once, to prove the *CI* job fails
and names the right image (FR-020) — a local `docker build` failure proves the Dockerfile
is broken, not that the gate catches it.

---

## 5. Change-detection behaviour (SC-004)

The single most important property, and the easiest to get wrong.

```bash
git switch -c scratch-docs-only
printf '\n<!-- scratch -->\n' >> README.md
git commit -am "docs: scratch" && git push -u origin scratch-docs-only
gh pr create --fill --draft
gh pr checks --watch
```

**Expected**, on a docs-only pull request:

| Check | Verdict |
|---|---|
| `detect changed areas` | success |
| `secret scan` | **success** (it runs — filter `any`) |
| every other check, new and existing | **skipped** |

A check reported as *missing* rather than *skipped* is a FR-001 failure: once the § 2.2
ruleset is applied, a missing required check sits at "Expected" forever and blocks the
merge permanently.

```bash
gh pr close --delete-branch
```

---

## 6. Wall-clock budget (SC-005)

```bash
# baseline: the merge commit before this feature
gh run view <baseline-run-id> --json jobs \
  -q '[.jobs[] | select(.conclusion=="success")] | max_by(.completedAt).completedAt'
# after: this feature's merge commit, same query
```

**Expected**: total wall clock grows by ≤50%. The two image builds run in parallel with
everything else, so the increase should be bounded by the slower image's warm build, not
the sum of the new jobs.

If the budget is breached, the recorded lever (plan, Risks) is to gate the image builds on
`master` pushes plus a manual label rather than every pull request — a workflow edit, not a
redesign.

---

## 7. Dependabot (SC-006)

Nothing to run; verify after merge.

```bash
gh api repos/:owner/:repo/dependabot/alerts 2>/dev/null | head   # may 404 on Free tier
gh pr list --author "app/dependabot" --limit 20
```

**Expected**: within one weekly cycle, at least one update pull request per configured
ecosystem, or the ecosystem is demonstrably already current. Each such pull request runs
the full check suite with no bypass (FR-023).

To force an immediate run instead of waiting for Monday: repository *Insights → Dependency
graph → Dependabot → Check for updates*.

**Common misconfiguration to rule out**: if the `gomod` ecosystem reports "no manifest
found", `directory` is pointing at `/` instead of `/apps/api` (data-model U1). Dependabot
does not treat this as an error, so it fails silently — checking is the only way to know.

---

## Definition of done for this quickstart

- [ ] Phase A: three web jobs green on `master`
- [ ] Secret scan: clean positive, redacted negative, one provider rule proven
- [ ] Full-history scan run once, result recorded in the domain doc
- [ ] Go scan: clean positive, self-test green, negative proven, exception + expiry proven
- [ ] Web audit: clean positive, negative proven, severity floor proven
- [ ] Both images: build clean, each planted break caught and named in CI
- [ ] Docs-only pull request: every new check skipped except `secret scan`
- [ ] Wall-clock delta measured and within budget
- [ ] All five new check names present in § 2.1 and § 2.2 of the operations domain doc
