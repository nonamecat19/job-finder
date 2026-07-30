#!/usr/bin/env bash
#
# Claude Code PostToolUse hook: after an Edit/Write to a DTO file under
# apps/api/internal/dto/*.go, regenerate the tygo-generated shared TS types
# so packages/shared/src/generated.ts never goes stale relative to the DTO
# that was just edited (FR-023). See
# specs/023-workflow-quality-gates/contracts/hooks.md.
#
# PostToolUse CANNOT block — this hook repairs, it does not reject.
# Regeneration lands in the working tree for review (FR-026), never staged
# or committed by this hook.
#
# Note (until feature 024 lands): this closes only HALF the drift gap.
# packages/shared/src/index.ts is a hand-maintained duplicate of the
# tygo-generated generated.ts (see AGENTS.md "Conventions") — this hook
# cannot regenerate a hand-maintained file. Feature 024 removes that
# duplicate; once it does, this hook is sufficient on its own.
#
# No recursion (FR-030): bound only to apps/api/internal/dto/*.go. Its
# output (packages/shared/src/generated.ts) does not match that filter.
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

read_hook_input

FILE_PATH="$(hook_field '.tool_input.file_path')"

if [[ -n "$FILE_PATH" ]] && [[ "$FILE_PATH" != *"apps/api/internal/dto/"*.go ]]; then
  exit 0
fi

if ! command -v tygo >/dev/null 2>&1; then
  echo "tygo is not installed; packages/shared/src/generated.ts was NOT regenerated." >&2
  VERSION_FILE="$REPO_ROOT/apps/api/.tygo-version"
  if [[ -f "$VERSION_FILE" ]]; then
    PINNED="$(tr -d '[:space:]' < "$VERSION_FILE")"
    echo "Install it with: go install github.com/gzuidhof/tygo@v${PINNED}" >&2
  fi
  exit 0
fi

ERR_LOG="$(mktemp)"
trap 'rm -f "$ERR_LOG"' EXIT

cd "$REPO_ROOT/apps/api"
BEFORE="$(git -C "$REPO_ROOT" status --porcelain -- packages/shared/src/generated.ts 2>/dev/null || true)"
if tygo generate 2>"$ERR_LOG"; then
  AFTER="$(git -C "$REPO_ROOT" status --porcelain -- packages/shared/src/generated.ts 2>/dev/null || true)"
  if [[ -n "$AFTER" ]] && [[ "$AFTER" != "$BEFORE" ]]; then
    emit_context "tygo generate regenerated packages/shared/src/generated.ts — review the diff before committing. Remember packages/shared/src/index.ts is still hand-maintained (AGENTS.md) and needs its own edit until feature 024 lands."
  fi
else
  emit_context "tygo generate failed: $(cat "$ERR_LOG")"
fi

exit 0
