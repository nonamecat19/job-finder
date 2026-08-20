#!/bin/sh
# Integration test (T132, 047-langchain-ai-service, FR-024 / contracts/
# configuration.md K4-5): the backend must start and serve every non-AI
# path with the `ai` service stopped. Runs against docker-compose.prod.yml
# — the only compose file that runs `api` as a container; `ai` is
# deliberately never brought up here.
#
# Not wired into `just test-integration` or CI: Justfile and workflow
# changes for this feature are owned by a parallel task (T009/T010/T012).
# Run directly once `.env` is populated:
#   ./scripts/test-ai-optional.sh
set -eu

cd "$(dirname "$0")/.."

COMPOSE="docker compose -f docker-compose.prod.yml"

cleanup() {
  $COMPOSE down
}
trap cleanup EXIT

$COMPOSE up -d --build postgres redis minio createbuckets litellm rabbitmq rabbitmq-init api

echo "waiting for api to report ready with ai stopped..."
tries=0
until curl -sf http://localhost:3000/api/health/ready >/dev/null; do
  tries=$((tries + 1))
  if [ "$tries" -ge 60 ]; then
    echo "FAIL: api never became ready with ai stopped" >&2
    $COMPOSE ps
    exit 1
  fi
  sleep 2
done

if $COMPOSE ps ai 2>/dev/null | grep -q "Up"; then
  echo "FAIL: ai service is running — this test requires it stopped" >&2
  exit 1
fi

echo "PASS: api served /api/health/ready with the ai service stopped"
