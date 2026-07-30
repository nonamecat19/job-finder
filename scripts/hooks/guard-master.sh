#!/usr/bin/env bash
#
# Claude Code PreToolUse hook: block the agent from committing or pushing
# while checked out on master. See
# specs/023-workflow-quality-gates/contracts/hooks.md — Layer 2 — PreToolUse
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

# --no-verify is the documented FR-005 override. Let it through here too —
# the git hook itself is the actual enforcement point for --no-verify, and
# refusing to let the agent even issue the override command would remove the
# only escape hatch for repairing a genuinely broken trunk. What matters is
# that using it is visible in the transcript, which it already is: this
# script does not suppress or rewrite the command.
#
# Check the branch of the project the agent is actually working in
# ($CLAUDE_PROJECT_DIR, exported to every hook subprocess), not this script's
# own location — the two differ whenever the hook is invoked from a worktree
# other than the one scripts/hooks/ happens to live under. Fall back to the
# current working directory for standalone/manual invocation.
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$PWD}"
BRANCH="$(git -C "$PROJECT_DIR" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")"

if [[ "$BRANCH" == "master" ]]; then
  echo "On master. Agent-authored changes never land directly on master." >&2
  echo "Create a branch first: git checkout -b <nnn>-<slug>" >&2
  exit 2
fi

exit 0
