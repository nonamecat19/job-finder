# Contract: Git Hooks and Agent Hooks

Two independent enforcement layers. Every hook body is a script under `scripts/hooks/`, runnable standalone — `.claude/settings.json` only wires events to scripts.

## Shared rules

- Every script is `#!/usr/bin/env bash` with `set -euo pipefail`, mirroring `scripts/sqlc-check.sh`.
- Every script checks for its tool first and exits non-zero with an install line if absent (FR-027). A missing tool must never read as a pass.
- Every script is idempotent and safe to run by hand.
- No script writes outside the repository, and none rewrites files at commit time (FR-026).

---

## Layer 1 — Git hooks

Activated by `make setup-hooks` → `git config core.hooksPath .githooks`. Repository-level config, so one call covers the main working tree and all 12 worktrees.

### `.githooks/pre-commit`

```
Input:   none (reads current branch via git rev-parse --abbrev-ref HEAD)
Exit 0:  branch is not master
Exit 1:  branch is master
Message: names the branch-and-PR rule and prints the branch-creation command
Bypass:  git commit --no-verify   (this IS the FR-005 override)
```

### `.githooks/pre-push`

```
Input:   stdin lines "<local ref> <local sha> <remote ref> <remote sha>"
Exit 0:  no pushed ref is refs/heads/master
Exit 1:  any pushed ref is refs/heads/master
Bypass:  git push --no-verify
```

Both hooks must be no-ops on any other branch, and must not inspect content — they gate destination only.

---

## Layer 2 — Agent hooks (`.claude/settings.json`, committed)

Key facts that shaped this design (verified against the hooks reference — see research R2):

- Matchers filter on **tool name only**; path filtering uses the per-entry `if` field.
- `PostToolUse` **cannot block** — exit 2 is explicitly non-blocking for that event.
- `Stop` **can** block — exit 2, or `{"decision":"block","reason":"…"}` on stdout.
- `$CLAUDE_PROJECT_DIR` is exported to the hook subprocess.
- Hooks read a JSON object on stdin; `tool_input.file_path` carries the edited file, `session_id` identifies the session.

### `PreToolUse` → `guard-master.sh`

```
Bound to: Bash, if: Bash(git commit*) and Bash(git push*)
Reads:    stdin JSON → tool_input.command
Exit 0:   not on master, or the command is not a commit/push
Exit 2:   on master — BLOCKS the tool call, stderr fed back to the agent
Stderr:   "On master. Create a branch first: git checkout -b <nnn>-<slug>"
```

This is the layer that matters for agents: it stops the mistake before git is reached, and the agent cannot route around it with `--no-verify` without that appearing in the transcript.

### `PostToolUse` → `go-postedit.sh`

```
Bound to: Edit|Write, if: Edit(apps/api/**/*.go)
Reads:    stdin JSON → tool_input.file_path
Action:   gofmt -w <file>; go vet ./<package of file>
Exit:     always 0 — this event cannot block
Reports:  hookSpecificOutput.additionalContext when it reformats or vet complains
Scope:    the edited file and its package only, never the repository (FR-029)
```

### `PostToolUse` → `regen-sqlc.sh`

```
Bound to: Edit|Write, if: Edit(apps/api/internal/db/queries/*.sql)
Action:   make sqlc-generate
Effect:   apps/api/internal/db/sqlcgen/ refreshed in the working tree for review
Exit:     always 0
Reports:  additionalContext listing regenerated files, or the failure if sqlc is absent
```

### `PostToolUse` → `regen-tygo.sh`

```
Bound to: Edit|Write, if: Edit(apps/api/internal/dto/*.go)
Action:   make tygo-generate
Effect:   packages/shared/src/generated.ts refreshed in the working tree
Exit:     always 0
```

**No recursion (FR-030)**: regeneration writes to `internal/db/sqlcgen/` and `packages/shared/src/generated.ts`. Neither path matches any `if` filter above, so a regeneration cannot trigger another regeneration. This must be re-verified whenever a filter is widened.

**Note on `regen-tygo.sh` and feature 024**: today, editing a DTO also requires a hand edit to `packages/shared/src/index.ts`. This hook cannot automate that, because the duplicate is hand-maintained. Feature 024 removes the duplicate; after it lands, this hook is sufficient on its own. Until then it closes half the gap.

### `Stop` → `session-verify.sh`

```
Bound to: Stop (no matcher support — fires every time)
Reads:    stdin JSON → session_id
Scoping:  git diff --name-only (merge base + unstaged)
            Go paths touched        -> make lint-go test-go
            dashboard/shared paths  -> make lint-web test-react
            neither                 -> exit 0 immediately
Exit 0:   everything scoped passed, or nothing relevant changed
Exit 2:   a scoped check failed — BLOCKS the stop, stderr fed back to the agent
Budget:   ≤2 minutes (SC-011); timeout set accordingly in settings.json
```

**Loop safety**: a blocking `Stop` re-enters the agent loop, which can end in another `Stop`. The script blocks at most **once per `session_id`**, recording a marker under the system temporary directory. A second consecutive failure reports without blocking, so a session can always terminate.

---

## Local vs committed settings

| File | Contents | Committed |
|---|---|---|
| `.claude/settings.json` | hook registry only | **yes** — every clone and worktree inherits (FR-028) |
| `.claude/settings.local.json` | the maintainer's 140-entry permission allowlist | no, unchanged by this feature |

Permission-list hygiene is out of scope here and deferred.

---

## Verification

Each hook has a matching check in `quickstart.md`. Because every hook is a standalone script, breakage is detectable by running the script directly — no need to reason about Claude Code internals if the schema shifts in a future version.
