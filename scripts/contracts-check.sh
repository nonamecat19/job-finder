#!/usr/bin/env bash
#
# Fail if the committed JSON Schema in apps/api/internal/events/schema is
# stale relative to the Go event structs in apps/api/internal/events.
#
# Runs `go run ./cmd/contractsgen` and then checks whether the generated
# tree changed. A non-empty diff means someone edited an event or payload
# struct without regenerating, so the checked-in schema (and, downstream,
# the generated Pydantic models in apps/ai) no longer match the Go types.
#
# Fix a failure with:
#   make contracts-generate && git add apps/api/internal/events/schema
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$REPO_ROOT/apps/api"
GEN_DIR="apps/api/internal/events/schema"
AI_GEN_DIR="apps/ai/src/jobfinder_ai/contracts"

echo "contractsgen: regenerating $GEN_DIR ..."
(cd "$API_DIR" && go run ./cmd/contractsgen)

echo "contracts-generate-ai: regenerating $AI_GEN_DIR ..."
"$REPO_ROOT/scripts/contracts-generate-ai.sh"

STATUS=0
git -C "$REPO_ROOT" diff --exit-code -- "$GEN_DIR" "$AI_GEN_DIR" || STATUS=1

UNTRACKED="$(git -C "$REPO_ROOT" ls-files --others --exclude-standard -- "$GEN_DIR" "$AI_GEN_DIR")"
if [[ -n "$UNTRACKED" ]]; then
  echo "untracked generated files:" >&2
  echo "$UNTRACKED" >&2
  STATUS=1
fi

if [[ "$STATUS" -ne 0 ]]; then
  cat >&2 <<EOF

------------------------------------------------------------------
contracts output is stale: $GEN_DIR and/or $AI_GEN_DIR do not
match the event structs in apps/api/internal/events.

Regenerate and commit the result:
  make contracts-generate
  git add $GEN_DIR $AI_GEN_DIR
------------------------------------------------------------------
EOF
  exit 1
fi

echo "contracts output is up to date."
