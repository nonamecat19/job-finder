#!/usr/bin/env bash
#
# Claude Code PostToolUse hook: after an Edit/Write to a query file under
# apps/api/internal/db/queries/*.sql, regenerate the sqlc-generated Go layer
# so it never goes stale relative to the query that was just edited
# (FR-022). See specs/archive/023-workflow-quality-gates/contracts/hooks.md.
#
# PostToolUse CANNOT block (exit 2 is non-blocking for this event) — this
# hook repairs, it does not reject. Regeneration lands in the working tree
# for the author to review before committing (FR-026); it is never staged
# or committed by this hook.
#
# No recursion (FR-030): this hook is bound only to
# apps/api/internal/db/queries/*.sql. Its own output
# (apps/api/internal/db/sqlcgen/**) does not match that filter, so
# regenerating cannot re-trigger itself.
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

read_hook_input

FILE_PATH="$(hook_field '.tool_input.file_path')"

# Defensive re-check (contracts/hooks.md): the settings.json `if` filter
# already restricts when this hook fires, but a hook should never trust
# that alone.
if [[ -n "$FILE_PATH" ]] && [[ "$FILE_PATH" != *"apps/api/internal/db/queries/"*.sql ]]; then
  exit 0
fi

if ! command -v sqlc >/dev/null 2>&1; then
  echo "sqlc is not installed; internal/db/sqlcgen was NOT regenerated." >&2
  VERSION_FILE="$REPO_ROOT/apps/api/.sqlc-version"
  if [[ -f "$VERSION_FILE" ]]; then
    PINNED="$(tr -d '[:space:]' < "$VERSION_FILE")"
    echo "Install it with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@v${PINNED}" >&2
  fi
  # PostToolUse cannot block; still exit 0 so the tool call itself (the
  # edit that already happened) is not treated as a failure. The gap is
  # caught later by `make sqlc-check` / the sqlc-drift CI job.
  exit 0
fi

ERR_LOG="$(mktemp)"
trap 'rm -f "$ERR_LOG"' EXIT

cd "$REPO_ROOT/apps/api"
BEFORE="$(git -C "$REPO_ROOT" status --porcelain -- internal/db/sqlcgen 2>/dev/null || true)"
if sqlc generate 2>"$ERR_LOG"; then
  AFTER="$(git -C "$REPO_ROOT" status --porcelain -- internal/db/sqlcgen 2>/dev/null || true)"
  if [[ -n "$AFTER" ]] && [[ "$AFTER" != "$BEFORE" ]]; then
    emit_context "sqlc generate regenerated apps/api/internal/db/sqlcgen — review the diff before committing."
  fi
else
  emit_context "sqlc generate failed: $(cat "$ERR_LOG")"
fi

exit 0
