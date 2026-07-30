#!/usr/bin/env bash
# Shared helpers for Claude Code hook scripts under scripts/hooks/.
#
# Every hook script (guard-master.sh, go-postedit.sh, regen-sqlc.sh,
# regen-tygo.sh, session-verify.sh) sources this file instead of duplicating
# the same stdin-parsing and tool-guard boilerplate five times over. See
# specs/023-workflow-quality-gates/contracts/hooks.md for the full contract.
#
# Conventions enforced here (contracts/hooks.md "Shared rules"):
#   - set -euo pipefail in every script that sources this file
#   - read the hook JSON from stdin exactly once
#   - never write outside the repository
#   - a missing required tool is always a loud failure, never a silent pass
#
# Usage:
#   #!/usr/bin/env bash
#   set -euo pipefail
#   source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
#   read_hook_input                       # populates $HOOK_INPUT
#   file="$(hook_field '.tool_input.file_path // empty')"
#   require_tool gofmt "part of the Go toolchain: https://go.dev/dl/"
#   emit_context "reformatted $file"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# read_hook_input reads the full stdin JSON payload exactly once into
# $HOOK_INPUT. Safe to call even when stdin is empty/not JSON (e.g. running
# the script by hand without piping anything) — HOOK_INPUT is then "{}".
read_hook_input() {
  if [[ -t 0 ]]; then
    HOOK_INPUT="{}"
  else
    HOOK_INPUT="$(cat)"
    if [[ -z "$HOOK_INPUT" ]]; then
      HOOK_INPUT="{}"
    fi
  fi
}

# hook_field <jq-filter> extracts a field from $HOOK_INPUT via jq.
# Falls back to "" if jq is absent or the field is missing/null, rather than
# aborting the whole hook over a missing optional field.
hook_field() {
  local filter="$1"
  if ! command -v jq >/dev/null 2>&1; then
    echo ""
    return 0
  fi
  printf '%s' "$HOOK_INPUT" | jq -r "${filter} // empty" 2>/dev/null || true
}

# require_tool <name> <install-line> exits 1 with an actionable message
# (FR-027) when <name> is not on PATH. Never let a missing tool read as a
# pass — every hook that needs an external binary calls this first.
require_tool() {
  local name="$1"
  local install_line="$2"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "error: '$name' is not installed." >&2
    echo "Install it with:" >&2
    echo "  $install_line" >&2
    exit 1
  fi
}

# emit_context <message> writes the Claude Code hookSpecificOutput JSON shape
# so the calling agent sees what an edit-time hook did, via
# additionalContext. Safe to call multiple times; each call is its own JSON
# object on its own stdout line, which Claude Code hook consumers expect.
emit_context() {
  local message="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -cn --arg msg "$message" \
      '{hookSpecificOutput: {additionalContext: $msg}}'
  else
    # jq absent: still report something rather than swallowing the message.
    printf '{"hookSpecificOutput":{"additionalContext":%s}}\n' \
      "$(printf '%s' "$message" | sed 's/\\/\\\\/g; s/"/\\"/g' | awk '{printf "\"%s\"", $0}')"
  fi
}
