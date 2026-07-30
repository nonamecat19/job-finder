#!/usr/bin/env bash
#
# Claude Code PostToolUse hook: after an Edit/Write to a Go file under
# apps/api, run gofmt on that file and go vet on its package (FR-024).
# See specs/023-workflow-quality-gates/contracts/hooks.md.
#
# Scope is the edited file and its own package ONLY — never
# repository-wide (FR-029). PostToolUse cannot block; this hook repairs
# formatting and reports vet complaints, it does not reject the edit.
#
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/common.sh"

read_hook_input

FILE_PATH="$(hook_field '.tool_input.file_path')"

if [[ -z "$FILE_PATH" ]] || [[ "$FILE_PATH" != *"apps/api/"*.go ]]; then
  exit 0
fi

if [[ ! -f "$FILE_PATH" ]]; then
  # File was deleted, or this is some other non-file edit; nothing to format.
  exit 0
fi

MESSAGES=()

if ! command -v gofmt >/dev/null 2>&1; then
  echo "gofmt is not installed (part of the Go toolchain: https://go.dev/dl/); $FILE_PATH was NOT reformatted." >&2
else
  BEFORE_HASH="$(cksum < "$FILE_PATH" 2>/dev/null || true)"
  gofmt -w "$FILE_PATH"
  AFTER_HASH="$(cksum < "$FILE_PATH" 2>/dev/null || true)"
  if [[ "$BEFORE_HASH" != "$AFTER_HASH" ]]; then
    MESSAGES+=("gofmt reformatted $FILE_PATH")
  fi
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go is not installed; skipped 'go vet' on the edited package." >&2
else
  PKG_DIR="$(dirname "$FILE_PATH")"
  VET_LOG="$(mktemp)"
  trap 'rm -f "$VET_LOG"' EXIT
  if ! (cd "$PKG_DIR" && go vet . >"$VET_LOG" 2>&1); then
    MESSAGES+=("go vet ./$(realpath --relative-to="$REPO_ROOT/apps/api" "$PKG_DIR" 2>/dev/null || echo "$PKG_DIR") reported: $(cat "$VET_LOG")")
  fi
fi

if [[ ${#MESSAGES[@]} -gt 0 ]]; then
  emit_context "$(printf '%s; ' "${MESSAGES[@]}")"
fi

exit 0
