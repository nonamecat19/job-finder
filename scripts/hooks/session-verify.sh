#!/usr/bin/env bash
#
# Claude Code Stop hook: run a scoped `test-lint` and block the session
# from ending if it fails (FR-025). See
# specs/023-workflow-quality-gates/contracts/hooks.md — Stop ->
# session-verify.sh.
#
# Stop is the one event that CAN block: exit 2 (or {"decision":"block"} on
# stdout) prevents Claude from stopping, with stderr fed back as the reason
# to repair. Everything else in this hook family only repairs; this one
# also verifies and gates (research R2).
#
# Scoping (FR-029, SC-011 <=2min budget): only the applications actually
# touched during the session are checked, via `git diff --name-only`
# against the merge-base with origin/master (falling back to local master)
# plus unstaged/untracked changes. Nothing changed -> exit 0 immediately.
#
# Loop safety (research R8): a blocking Stop re-enters the agent loop,
# which could end in another Stop. This script blocks at most ONCE per
# session_id, recording a marker under the system temp dir, so a second
# consecutive failure reports without blocking and the session can always
# terminate.
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

read_hook_input

SESSION_ID="$(hook_field '.session_id')"
if [[ -z "$SESSION_ID" ]]; then
  SESSION_ID="unknown"
fi

MARKER_DIR="${TMPDIR:-/tmp}/claude-quality-gates-session-verify"
mkdir -p "$MARKER_DIR"
MARKER_FILE="$MARKER_DIR/${SESSION_ID//\//_}"

cd "$REPO_ROOT"

# --- scope: which apps were actually touched? ---
BASE_REF=""
if git rev-parse --verify --quiet origin/master >/dev/null; then
  BASE_REF="origin/master"
elif git rev-parse --verify --quiet master >/dev/null; then
  BASE_REF="master"
fi

CHANGED=""
if [[ -n "$BASE_REF" ]]; then
  MERGE_BASE="$(git merge-base HEAD "$BASE_REF" 2>/dev/null || echo "$BASE_REF")"
  CHANGED="$(git diff --name-only "$MERGE_BASE" -- . 2>/dev/null || true)"
fi
# Unstaged/staged working-tree changes and untracked files, regardless of
# whether a base ref was resolvable (e.g. a repo with no master yet).
CHANGED="$CHANGED
$(git diff --name-only HEAD 2>/dev/null || true)
$(git ls-files --others --exclude-standard 2>/dev/null || true)"

GO_TOUCHED=0
WEB_TOUCHED=0
while IFS= read -r f; do
  [[ -z "$f" ]] && continue
  case "$f" in
    apps/api/*) GO_TOUCHED=1 ;;
    apps/dashboard/*|packages/shared/*) WEB_TOUCHED=1 ;;
  esac
done <<<"$CHANGED"

if [[ "$GO_TOUCHED" -eq 0 ]] && [[ "$WEB_TOUCHED" -eq 0 ]]; then
  exit 0
fi

FAIL_LOG="$(mktemp)"
trap 'rm -f "$FAIL_LOG"' EXIT
STATUS=0

if [[ "$GO_TOUCHED" -eq 1 ]]; then
  if ! make lint-go test-go >"$FAIL_LOG" 2>&1; then
    STATUS=1
  fi
fi

if [[ "$WEB_TOUCHED" -eq 1 ]] && [[ "$STATUS" -eq 0 || -s "$FAIL_LOG" ]]; then
  if ! make lint-web test-react >>"$FAIL_LOG" 2>&1; then
    STATUS=1
  fi
fi

if [[ "$STATUS" -eq 0 ]]; then
  # A clean pass clears any previous block marker, so a later broken
  # session in the same conversation is still caught fresh.
  rm -f "$MARKER_FILE"
  exit 0
fi

if [[ -f "$MARKER_FILE" ]]; then
  # Already blocked once this session; report without blocking again so
  # the session can always terminate (research R8).
  echo "test-lint is still failing (already blocked once this session; not blocking again):" >&2
  tail -n 40 "$FAIL_LOG" >&2
  exit 0
fi

touch "$MARKER_FILE"
echo "Scoped test-lint failed — session-end verification (FR-025) blocks completion until it passes:" >&2
tail -n 80 "$FAIL_LOG" >&2
exit 2
