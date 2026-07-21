#!/usr/bin/env bash
#
# Fail if the committed sqlc output in apps/api/internal/db/sqlcgen is stale.
#
# Runs `sqlc generate` and then checks whether the generated tree changed. A
# non-empty diff means someone edited a migration or a query file without
# regenerating, so the checked-in Go code no longer matches the schema.
#
# Fix a failure with:
#   make sqlc-generate && git add apps/api/internal/db/sqlcgen
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$REPO_ROOT/apps/api"
GEN_DIR="apps/api/internal/db/sqlcgen"
VERSION_FILE="$API_DIR/.sqlc-version"

if [[ ! -f "$VERSION_FILE" ]]; then
  echo "error: missing version pin at $VERSION_FILE" >&2
  exit 1
fi

PINNED_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

if ! command -v sqlc >/dev/null 2>&1; then
  cat >&2 <<EOF
error: sqlc is not installed.

Install the pinned version:
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@v${PINNED_VERSION}
EOF
  exit 1
fi

# `sqlc version` prints e.g. "v1.31.1"; normalise to "1.31.1".
LOCAL_VERSION="$(sqlc version | tr -d '[:space:]' | sed 's/^v//')"

if [[ "$LOCAL_VERSION" != "$PINNED_VERSION" ]]; then
  cat >&2 <<EOF
error: sqlc version mismatch.
  pinned (apps/api/.sqlc-version): $PINNED_VERSION
  installed:                       $LOCAL_VERSION

Different sqlc versions emit different code, which would make this check
flap between machines. Install the pinned version:
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@v${PINNED_VERSION}
EOF
  exit 1
fi

echo "sqlc $PINNED_VERSION: regenerating $GEN_DIR ..."
(cd "$API_DIR" && sqlc generate)

# Catches modified files. --exit-code alone ignores untracked files, so also
# check for new generated files that were never committed (e.g. a brand-new
# query file whose .sql.go was forgotten).
STATUS=0
git -C "$REPO_ROOT" diff --exit-code -- "$GEN_DIR" || STATUS=1

UNTRACKED="$(git -C "$REPO_ROOT" ls-files --others --exclude-standard -- "$GEN_DIR")"
if [[ -n "$UNTRACKED" ]]; then
  echo "untracked generated files:" >&2
  echo "$UNTRACKED" >&2
  STATUS=1
fi

if [[ "$STATUS" -ne 0 ]]; then
  cat >&2 <<EOF

------------------------------------------------------------------
sqlc output is stale: $GEN_DIR does not match the migrations and
queries in apps/api/internal/db.

Regenerate and commit the result:
  make sqlc-generate
  git add $GEN_DIR
------------------------------------------------------------------
EOF
  exit 1
fi

echo "sqlc output is up to date."
