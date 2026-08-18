# Quickstart: Dedicated AI Orchestration Service

**Feature**: 047-langchain-ai-service | **Date**: 2026-08-18

How to run, exercise and verify the feature. Written for the state after phase 3 begins; the
phase-1/2 checks work as soon as the broker lands.

---

## 1. Configure

Add to `.env` (see contracts/configuration.md K1 for the full table):

```bash
RABBITMQ_URL=amqp://jobfinder:<password>@rabbitmq:5672/
RABBITMQ_DEFAULT_USER=jobfinder
RABBITMQ_DEFAULT_PASS=<password>
AI_SERVICE_URL=http://ai:8000
AI_SERVICE_TOKEN=<shared secret>
AI_CAPABILITY_ROUTING=ghost:python           # start with one capability
```

`GATEWAY_URL` and `LITELLM_MASTER_KEY` are already required and unchanged. Provider keys stay
on the `litellm` service only — if any of them appears in the `ai` service's environment,
that is a defect (K2-3).

## 2. Bring the stack up

```bash
make up          # postgres, redis, rabbitmq, litellm, langfuse group, ai
make ps
```

Expected: `rabbitmq` healthy, `ai` healthy, no `asynqmon` (removed in phase 2).

The AI service starts whether or not Langfuse is up. If it refuses to start, the message names
the missing key or the invalid capability — that is K3-1/K3-4 working, not a bug.

## 3. Verify wiring before running work

```bash
# Broker reachable, topology declared
docker compose exec rabbitmq rabbitmqctl list_queues name type durable messages

# AI service ready (no model call is made by this probe — K3-3)
curl -s localhost:8000/health/ready | jq

# The service holds no provider credentials (K2-3)
docker compose exec ai env | grep -E 'CEREBRAS|GROQ|COHERE|OPENROUTER|OPENAI|DATABASE_URL' && \
  echo "DEFECT: credential leaked into ai service" || echo "OK: no provider or DB credentials"
```

Every queue listed should be `quorum` and `durable` (M1-2).

## 4. Run one AI capability end to end

Trigger ghost scoring for a job, then follow it through all three surfaces:

```bash
# 1. Publish work (via the existing API path that enqueues ghost scoring)
curl -sX POST localhost:8080/v1/jobs/<jobId>/ghost-score -H 'Cookie: <session>'

# 2. Watch the work queue drain
docker compose exec rabbitmq rabbitmqctl list_queues name messages consumers

# 3. Read the trace
open "$LANGFUSE_HOST"        # filter by capability=ghost, user id, or work id
```

**What to check** (US1): one trace exists, with a span per step, each showing input, output,
duration, model tier, tokens and cost. The trace is findable by user, job and capability
(FR-014). The gateway's own call-level record correlates with it (FR-017).

## 5. Verify the failure paths

These are the checks worth running deliberately, because they are the ones that silently rot.

**Collector down — runs must be unaffected (SC-006, FR-016):**

```bash
docker compose stop langfuse-web langfuse-worker
# run the capability again — it must complete, at unchanged median latency
docker compose start langfuse-web langfuse-worker
```

**AI service down — work must wait, not vanish (SC-007, US4 scenario 3):**

```bash
docker compose stop ai
# trigger ghost scoring; the message should sit in work.ghost with 0 consumers
docker compose exec rabbitmq rabbitmqctl list_queues name messages consumers
docker compose start ai
# the message is consumed and processed; nothing was lost
```

**Broker down — publishes must fail loudly (US5 scenario 5, M2-3):**

```bash
docker compose stop rabbitmq
# the triggering request must return an error, NOT 202 Accepted
docker compose start rabbitmq
```

**Consumer crash mid-run — redelivery must not duplicate (US5 scenario 2, FR-030):**

```bash
# trigger work, then kill the consumer mid-flight
docker compose kill ai && docker compose start ai
# after recovery: exactly one stored result, and the idempotency ledger has one row
```

**Dead-lettering (FR-031, FR-032):**

```bash
# force a non-retryable failure (e.g. malformed input), then:
docker compose exec rabbitmq rabbitmqctl list_queues name messages | grep dlq
# the management UI shows the item with x-first-failure-reason set
```

## 6. Verify parity before cutover

Every capability needs a recorded baseline before it moves (FR-021, C8-1), and a comparison
after (SC-004: ≤5% mean deviation, ≤5% outcome changes).

```bash
# Before: capture with the capability still on the Go path
AI_CAPABILITY_ROUTING= make baseline-capture CAPABILITY=ghost

# After: same fixed input set, capability routed to Python
AI_CAPABILITY_ROUTING=ghost:python make baseline-compare CAPABILITY=ghost
```

Revert is configuration alone (FR-020) — drop the capability from `AI_CAPABILITY_ROUTING` and
restart the backend. That path stays available until the capability's Go code is deleted at
confirmed cutover (C8-4).

## 7. Change a prompt without a backend release (US2, SC-003)

```bash
$EDITOR apps/ai/src/jobfinder_ai/prompts/ghost.py
docker compose restart ai
# run the capability; the new trace records a new workflow_version
```

The backend is untouched. If a prompt change needs a backend rebuild, something has leaked
across the boundary.

## 8. Tests

```bash
make test-lint          # lint-go, lint-web, test-go, test-react, lint-py, test-py
make test-integration   # real Postgres + RabbitMQ via Compose
make audit              # vuln-go, vuln-web, vuln-py, secrets
make contracts-check    # generated Pydantic models match the Go structs (E7-2)
```

`contracts-check` failing means the Go event structs changed without regenerating. Run
`make contracts-generate` and commit the result — never hand-edit the generated Python (E7-3).

## 9. Operating the queues day to day

| Task | How |
|---|---|
| See queue depth | RabbitMQ management UI, or `rabbitmqctl list_queues` |
| See why something dead-lettered | Management UI → `dlq.<work_type>` → message headers |
| Re-dispatch a dead-lettered item | Move to `jobfinder.work` with `x-attempt` reset (M4-7) |
| Check DLQ depth without opening the UI | The backend health endpoint reports it per work type (M8-2) |
| Find the trace for a bad result | Langfuse, filtered by user id and work id (SC-002, under 2 minutes) |
