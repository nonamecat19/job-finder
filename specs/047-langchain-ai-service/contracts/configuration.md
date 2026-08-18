# Contract: Configuration

**Feature**: 047-langchain-ai-service | Binding on `.env.example`, `docker-compose.yml`,
`docker-compose.prod.yml`, `apps/api/internal/config`, `apps/ai/src/jobfinder_ai/settings.py`.

## K1. New variables

| Variable | Consumed by | Required | Notes |
|---|---|---|---|
| `RABBITMQ_URL` | backend, AI service | **yes** | `amqp://user:pass@rabbitmq:5672/`. Empty or unset is a startup error naming the key |
| `RABBITMQ_DEFAULT_USER` / `_PASS` | rabbitmq | yes | Replaces the default `guest` account (M7-1) |
| `AI_SERVICE_URL` | backend | **yes** | Container-network address, e.g. `http://ai:8000`. Same loopback trap `GATEWAY_URL` documents |
| `AI_SERVICE_TOKEN` | backend, AI service | **yes** | Shared secret for the interactive HTTP surface (H7-1) |
| `AI_CAPABILITY_ROUTING` | backend | no | Per-capability switch: `python` or `go` (FR-020). Absent = `go` until a capability is cut over |
| `LANGFUSE_PAYLOAD_RETENTION_DAYS` | retention job | no | Default 30 (FR-018) |

## K2. Variables the AI service receives

- **K2-1**: Exactly these: `GATEWAY_URL`, `LITELLM_MASTER_KEY`, `RABBITMQ_URL`,
  `AI_SERVICE_TOKEN`, `LANGFUSE_PUBLIC_KEY`, `LANGFUSE_SECRET_KEY`, `LANGFUSE_HOST`, plus its
  own log level and bounds configuration.
- **K2-2**: The AI service MUST NOT receive `DATABASE_URL` or any Postgres credential (FR-008).
- **K2-3**: The AI service MUST NOT receive `CEREBRAS_API_KEY`, `GROQ_API_KEY`,
  `COHERE_API_KEY`, `OPENROUTER_API_KEY`, `OPENAI_API_KEY` or any other provider credential
  (FR-011). These stay on the `litellm` service alone, as 044-K4 already requires.
- **K2-4**: `LITELLM_MASTER_KEY` is the one inference credential it holds — the same permission
  the backend has today.

## K3. Startup validation

- **K3-1**: The AI service MUST refuse to start when `GATEWAY_URL`, `LITELLM_MASTER_KEY`,
  `RABBITMQ_URL` or `AI_SERVICE_TOKEN` is empty, naming the key — carrying forward 044-K1.
- **K3-2**: Startup validates that dependencies are *configured*, never that they are
  *reachable* (044-K1-2). An unreachable broker or gateway fails work, not boots.
- **K3-3**: No AI request may be issued at startup or as a health probe (044-K1-3, H8-2).
- **K3-4**: The capability registry is validated at startup (C1-4); an invalid capability is a
  boot failure naming it (FR-007).
- **K3-5**: The backend MUST refuse to start when `RABBITMQ_URL` is empty, and — once any
  capability is routed to Python — when `AI_SERVICE_URL` or `AI_SERVICE_TOKEN` is empty.

## K4. Compose wiring

- **K4-1**: `rabbitmq` (`rabbitmq:4.3.4-management-alpine`) joins both compose files with a
  named volume for durability and a healthcheck via `rabbitmq-diagnostics`.
- **K4-2**: `ai` joins both files, depending on `rabbitmq` healthy and `litellm` started. It
  MUST NOT depend on `langfuse-web` — tracing is never a startup dependency (FR-016, mirroring
  the `litellm` precedent in 036-C3-2).
- **K4-3**: `asynqmon` is removed. Its role — queue inspection and retry management — passes to
  the RabbitMQ management UI (FR-032).
- **K4-4**: In dev, the management UI may be published on loopback. In prod, no broker port is
  published (M7-2, FR-038).
- **K4-5**: `api` gains a dependency on `rabbitmq` healthy. It MUST NOT depend on `ai` — non-AI
  capabilities must start and serve whether or not the AI service is up (FR-024).
- **K4-6**: `redis` stays, for caching and rate-limit state only. `REDIS_URL` keeps its meaning;
  the asynq keyspace disappears with asynq.

## K5. Retention job

- **K5-1**: A scheduled job purges Langfuse trace payloads older than
  `LANGFUSE_PAYLOAD_RETENTION_DAYS` (FR-018a).
- **K5-2**: The purge MUST remove step inputs and outputs while retaining observation metadata,
  timings, token counts and cost (FR-018, SC-015).
- **K5-3**: The purge MUST be automatic and verifiable — an operator can confirm no payload
  older than the limit remains, without trusting that the job ran.

## K6. Documentation obligations

- **K6-1**: `.env.example` documents every new key with the same candour the existing file uses
  for `GATEWAY_URL` — including that `RABBITMQ_URL` and `AI_SERVICE_URL` are container-network
  addresses and that loopback ports are host-side only.
- **K6-2**: `AGENTS.md` is corrected in the same change: it currently states "No Python is in
  this repository" and describes `test-lint` as covering Go and web only.
- **K6-3**: The constitution's technology constraints and Principle IV are amended before
  implementation (plan.md § Constitution Check).
- **K6-4**: `docs/` gains the operator-facing account of the broker topology, the DLQ workflow
  and the AI service; `specs/domains/llm-routing.md` and `platform-operations.md` absorb this
  feature's durable rules at close-out.
