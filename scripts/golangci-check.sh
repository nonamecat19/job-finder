#!/usr/bin/env bash
#
# Run the pinned golangci-lint over apps/api, failing loudly (never silently
# passing) if the binary is missing or the wrong version — mirrors
# scripts/sqlc-check.sh's version-mismatch guard (FR-015, FR-027).
#
# Usage: ./scripts/golangci-check.sh   (called by `just lint-go`)
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$REPO_ROOT/apps/api"
VERSION_FILE="$API_DIR/.golangci-version"

if [[ ! -f "$VERSION_FILE" ]]; then
  echo "error: missing version pin at $VERSION_FILE" >&2
  exit 1
fi

PINNED_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

if ! command -v golangci-lint >/dev/null 2>&1; then
  cat >&2 <<EOF
error: golangci-lint is not installed.

Install the pinned version:
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${PINNED_VERSION}
EOF
  exit 1
fi

# `golangci-lint version` prints e.g.
# "golangci-lint has version 2.12.2 built with go1.26.5 from ...". Pull out
# the version token specifically rather than assuming its position, since
# the surrounding text has changed across releases.
LOCAL_VERSION="$(golangci-lint version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)"

if [[ "$LOCAL_VERSION" != "$PINNED_VERSION" ]]; then
  cat >&2 <<EOF
error: golangci-lint version mismatch.
  pinned (apps/api/.golangci-version): $PINNED_VERSION
  installed:                           $LOCAL_VERSION

Different golangci-lint versions can report different issues, which would
make this check flap between machines and CI. Install the pinned version:
  go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${PINNED_VERSION}
EOF
  exit 1
fi

cd "$API_DIR"
exec golangci-lint run ./...
