# Quickstart: Verifying the Gates Actually Gate

Runnable checks proving each phase works end to end. A gate that has never been observed refusing something has not been tested — every check below deliberately breaks something and confirms the refusal.

## Prerequisites

```bash
make setup-hooks          # once per clone; worktrees share the config
pnpm install
pnpm --filter @job-finder/shared build
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(tr -d '[:space:]' < apps/api/.golangci-version)
```

Confirm the pins agree with what is installed — mismatched versions make the gate flap between machines:

```bash
golangci-lint version    # must equal apps/api/.golangci-version
sqlc version             # must equal apps/api/.sqlc-version
tygo --version           # must equal apps/api/.tygo-version
```

---

## P1 — Isolation (US1)

**The trunk refuses a commit.**

```bash
git checkout master
echo "# scratch" >> AGENTS.md
git add AGENTS.md && git commit -m "test: should be rejected"
# EXPECT: non-zero exit, message naming the branch-and-PR rule
git checkout -- AGENTS.md
```

**A branch accepts the same commit.**

```bash
git checkout -b 999-gate-smoke
echo "# scratch" >> AGENTS.md
git add AGENTS.md && git commit -m "test: should succeed"
# EXPECT: commit created
git reset --hard HEAD~1 && git checkout master && git branch -D 999-gate-smoke
```

**Push to master refuses.**

```bash
git checkout master
git push origin master --dry-run
# EXPECT: pre-push aborts before contacting the remote
```

**The override works** (FR-005 — the trunk must never be unrecoverable):

```bash
git commit --no-verify -m "fix: emergency"    # EXPECT: succeeds
```

**The agent guard blocks.** With the repository on `master`, ask an agent to commit. Expect the tool call blocked and the reason surfaced, without git being invoked. Directly:

```bash
echo '{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}' \
  | ./scripts/hooks/guard-master.sh; echo "exit=$?"
# EXPECT on master: exit=2 with a stderr message
# EXPECT on a branch: exit=0, silent
```

---

## P2 — The quality command actually checks quality (US2)

**Baseline.**

```bash
time make lint          # EXPECT: green, under 60s (SC-004)
make test-lint          # EXPECT: green — lint-go, lint-web, test-go, test-react
```

**A Go violation is caught, with a location.**

```bash
cat >> apps/api/internal/config/config.go <<'EOF'

func gateSmoke() { x := 1; _ = x; var unused int }
EOF
make lint-go
# EXPECT: non-zero, file:line naming the offending rule
git checkout -- apps/api/internal/config/config.go
```

**A dashboard violation is caught.**

```bash
cat >> apps/dashboard/src/lib/api.ts <<'EOF'

const gateSmoke = () => { let unusedVar = 1; };
EOF
make lint-web
# EXPECT: non-zero, file:line:col plus the rule name
git checkout -- apps/dashboard/src/lib/api.ts
```

**Generated code is exempt** (FR-010) — a linter that flags `sqlcgen` or `generated.ts` produces permanent unfixable noise:

```bash
make lint 2>&1 | grep -E "sqlcgen|generated\.ts"
# EXPECT: no output
```

**A missing tool fails loudly, never silently passes** (FR-027):

```bash
PATH=/usr/bin:/bin make lint-go
# EXPECT: non-zero, naming golangci-lint and the install command
```

**The documentation matches reality** (SC-010): read the `make test-lint` line in `AGENTS.md` against the Makefile recipe. No Python claim; no lint claim that does not run.

---

## P3 — Real-infrastructure verification (US3)

**Locally.**

```bash
make test-integration    # EXPECT: green against real Postgres + Redis
make test-e2e            # EXPECT: 3 Playwright specs green
```

**A broken migration turns integration red** — the defect class this job exists to catch:

```bash
# Introduce a query referencing a column no migration creates, then:
make sqlc-generate       # EXPECT: fails, or produces code that fails to build
```

**In CI**: open a pull request and confirm `integration test` and `e2e (playwright)` both appear and pass. Then confirm the ordering guard holds — the migration step must run before the full suite, or dependent packages race the schema.

**The nightly runs** (FR-019):

```bash
gh workflow run "API CI"                      # manual dispatch works
gh run list --workflow="API CI" --limit 5     # scheduled runs appear
```

---

## P4 — Edit-time repair and session-end verification (US4)

**Editing a query regenerates the typed database layer, unprompted** (FR-022):

```bash
git status --short apps/api/internal/db/sqlcgen/    # baseline: clean
# Have an agent edit apps/api/internal/db/queries/job.sql
git status --short apps/api/internal/db/sqlcgen/    # EXPECT: regenerated, visible for review
```

**Editing a DTO regenerates shared types** (FR-023):

```bash
# Have an agent edit apps/api/internal/dto/jobs.go
git status --short packages/shared/src/generated.ts  # EXPECT: modified
```

Note: until feature 024 lands, `packages/shared/src/index.ts` still needs a hand edit. The hook cannot cover a hand-maintained duplicate.

**Go source is formatted and vetted on edit** (FR-024):

```bash
# Have an agent write badly-indented Go into apps/api/internal/...
gofmt -l apps/api/internal    # EXPECT: no output — already reformatted
```

**Regeneration does not recurse** (FR-030):

```bash
echo '{"tool_name":"Edit","tool_input":{"file_path":"'$PWD'/apps/api/internal/db/sqlcgen/models.go"}}' \
  | ./scripts/hooks/regen-sqlc.sh; echo "exit=$?"
# EXPECT: no-op — sqlcgen/ does not match the binding's path filter
```

**Session end blocks on a failing suite** (FR-025):

```bash
# Break a Go test, then ask the agent to finish.
# EXPECT: stop blocked, failure reported back to the agent.
```

Directly:

```bash
echo '{"session_id":"qs-1"}' | ./scripts/hooks/session-verify.sh; echo "exit=$?"
# EXPECT: exit=0 clean tree; exit=2 with a broken test
```

**It blocks at most once per session** (loop safety):

```bash
echo '{"session_id":"qs-1"}' | ./scripts/hooks/session-verify.sh; echo "exit=$?"
# EXPECT: exit=0 on the second call with the same session_id, even while still broken
```

**It stays inside the budget** (SC-011):

```bash
touch apps/dashboard/src/lib/api.ts
time (echo '{"session_id":"qs-2"}' | ./scripts/hooks/session-verify.sh)
# EXPECT: under 2 minutes; Go suites skipped entirely since no Go file changed
```

---

## Full-loop check

The feature is done when this sequence works without intervention:

1. On `master`, ask an agent to make a change. It creates a branch — the guard makes the alternative impossible.
2. It edits a `.sql` query. `sqlcgen` regenerates on its own.
3. It edits a DTO. `generated.ts` regenerates on its own.
4. It tries to finish with a failing test. The stop is blocked; it fixes and retries.
5. It commits on the branch and opens a pull request.
6. All ten checks run. A deliberate lint violation shows red before merge, not after.
