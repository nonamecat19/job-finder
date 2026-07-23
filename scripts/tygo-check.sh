#!/usr/bin/env bash
#
# Fail if the committed tygo output in packages/shared/src/generated.ts is stale.
#
# Runs `tygo generate` and then checks whether the generated file changed. A
# non-empty diff means someone edited a struct in apps/api/internal/dto without
# regenerating, so the checked-in TypeScript no longer matches the Go DTOs.
#
# Fix a failure with:
#   make tygo-generate && git add packages/shared/src/generated.ts
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$REPO_ROOT/apps/api"
GEN_FILE="packages/shared/src/generated.ts"
VERSION_FILE="$API_DIR/.tygo-version"

if [[ ! -f "$VERSION_FILE" ]]; then
  echo "error: missing version pin at $VERSION_FILE" >&2
  exit 1
fi

PINNED_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

if ! command -v tygo >/dev/null 2>&1; then
  cat >&2 <<EOF
error: tygo is not installed.

Install the pinned version:
  go install github.com/gzuidhof/tygo@v${PINNED_VERSION}
EOF
  exit 1
fi

# tygo does not report its own version (`tygo --version` prints "<unknown>"),
# so unlike the sqlc check there is no runtime version-equality guard here. CI
# installs the pinned version explicitly via `go install ...@v${PINNED_VERSION}`,
# which is what keeps local and CI output identical.

echo "tygo $PINNED_VERSION: regenerating $GEN_FILE ..."
(cd "$API_DIR" && tygo generate)

STATUS=0
git -C "$REPO_ROOT" diff --exit-code -- "$GEN_FILE" || STATUS=1

UNTRACKED="$(git -C "$REPO_ROOT" ls-files --others --exclude-standard -- "$GEN_FILE")"
if [[ -n "$UNTRACKED" ]]; then
  echo "untracked generated file:" >&2
  echo "$UNTRACKED" >&2
  STATUS=1
fi

if [[ "$STATUS" -ne 0 ]]; then
  cat >&2 <<EOF

------------------------------------------------------------------
tygo output is stale: $GEN_FILE does not match the Go DTOs in
apps/api/internal/dto.

Regenerate and commit the result:
  make tygo-generate
  git add $GEN_FILE
------------------------------------------------------------------
EOF
  exit 1
fi

echo "tygo output is up to date."
