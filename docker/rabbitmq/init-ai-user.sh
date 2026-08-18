#!/bin/sh
# One-shot broker user setup (047-langchain-ai-service, contracts/messaging.md
# M7-3). The AI orchestration service gets its own RabbitMQ account, separate
# from the backend's RABBITMQ_DEFAULT_USER, with permissions restricted to
# exactly what it needs: consume the AI work queues (match/generate/salary/
# ghost — never ingest/enrich) and publish results. No configure permission
# at all, so it can never declare, alter or delete topology (M1-1 reserves
# that to the publisher).
#
# Run by the `rabbitmq-init` service after RabbitMQ reports healthy, via the
# management HTTP API (loopback-published in dev, in-network-only in prod —
# either way this container reaches it as `rabbitmq:15672`).
set -eu

curl -sf -u "${RABBITMQ_DEFAULT_USER}:${RABBITMQ_DEFAULT_PASS}" \
  -X PUT "http://rabbitmq:15672/api/users/${RABBITMQ_AI_USER}" \
  -H 'content-type: application/json' \
  -d "{\"password\":\"${RABBITMQ_AI_PASS}\",\"tags\":\"\"}"

curl -sf -u "${RABBITMQ_DEFAULT_USER}:${RABBITMQ_DEFAULT_PASS}" \
  -X PUT "http://rabbitmq:15672/api/permissions/%2F/${RABBITMQ_AI_USER}" \
  -H 'content-type: application/json' \
  -d '{"configure":"^$","write":"^jobfinder\\.results$","read":"^work\\.(match|generate|salary|ghost)$"}'

echo "rabbitmq-init: ai service account ready"
