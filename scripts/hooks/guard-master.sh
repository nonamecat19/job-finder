#!/usr/bin/env bash
#
# Claude Code PreToolUse hook: block the agent from committing or pushing
# while checked out on master. See
# specs/domains/platform-operations.md § 4.1 — Layer 2 — PreToolUse
# -> guard-master.sh.
#
# Bound in .claude/settings.json to Bash, filtered by `if` to
# Bash(git commit*) and Bash(git push*). PreToolUse is one of the few events
# that CAN block: exit 2 stops the tool call before it runs, and stderr is
# fed back to the agent as the reason.
#
# Runnable standalone for verification (contracts/hooks.md, quickstart.md):
#   echo '{"tool_name":"Bash","tool_input":{"command":"git commit -m x"}}' \
#     | ./scripts/hooks/guard-master.sh; echo "exit=$?"
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

read_hook_input

COMMAND="$(hook_field '.tool_input.command')"

# Not a commit/push at all -> nothing to guard.
if [[ -z "$COMMAND" ]]; then
  exit 0
fi
if ! [[ "$COMMAND" =~ (^|[[:space:]\;\&\|])git[[:space:]]+(commit|push)([[:space:]]|$) ]]; then
  exit 0
fi

# --no-verify is deliberately NOT an escape hatch at this layer: the whole
# point of the PreToolUse check (contracts/hooks.md) is that the agent cannot
# route around it. The git hook is where --no-verify is honoured, by a human.
#
# Resolve the checkout the command actually writes to, which is not always
# the session's own directory: an agent working in a git worktree runs
# `cd <worktree> && git commit` (or `git -C <worktree> commit`) while
# $CLAUDE_PROJECT_DIR still points at the main checkout. Reading only the
# project dir's branch gets this wrong in both directions — it blocks commits
# that land on a worktree's feature branch, and it waves through commits that
# really do land on master whenever the session itself runs from a worktree.
#
# Both patterns use a greedy `.*` prefix so the LAST occurrence wins, which is
# the one in effect by the time git runs in a chained command.
target_dir_from_command() {
  local cmd="$1" dir=""
  if [[ "$cmd" =~ .*git[[:space:]]+-C[[:space:]]+([^[:space:]\;\&\|]+) ]]; then
    dir="${BASH_REMATCH[1]}"
  elif [[ "$cmd" =~ .*(^|[[:space:]\;\&\|])cd[[:space:]]+([^[:space:]\;\&\|]+) ]]; then
    dir="${BASH_REMATCH[2]}"
  fi
  # Strip one layer of quoting; anything more exotic falls back to the
  # session directory below, which is the safe (still-guarded) answer.
  dir="${dir#[\"\']}"
  dir="${dir%[\"\']}"
  printf '%s' "$dir"
}

# .cwd is the directory the Bash tool runs in, which tracks the session more
# closely than $CLAUDE_PROJECT_DIR does; both are absent for a standalone
# invocation, hence $PWD last.
SESSION_DIR="$(hook_field '.cwd')"
BASE_DIR="${SESSION_DIR:-${CLAUDE_PROJECT_DIR:-$PWD}}"

TARGET_DIR="$(target_dir_from_command "$COMMAND")"
if [[ -n "$TARGET_DIR" && "$TARGET_DIR" != /* ]]; then
  TARGET_DIR="$BASE_DIR/$TARGET_DIR"
fi
if [[ -z "$TARGET_DIR" || ! -d "$TARGET_DIR" ]]; then
  TARGET_DIR="$BASE_DIR"
fi

BRANCH="$(git -C "$TARGET_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")"

if [[ "$BRANCH" == "master" ]]; then
  echo "On master. Agent-authored changes never land directly on master." >&2
  echo "Create a branch first: git checkout -b <nnn>-<slug>" >&2
  exit 2
fi

exit 0
