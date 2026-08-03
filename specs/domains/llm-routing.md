# Domain: LLM Routing & AI Throughput

Consolidates **029** LiteLLM proxy gateway, **030** gateway-owned model routing,
**019** AI throughput & stuck-run recovery. **001-cerebras-model-toggle is fully
superseded** — see § 5.

Implementation: `apps/api/internal/platform/llm/`, `gateway/config.yaml`,
`internal/queue/policy.go`. How it works:
[`docs/ai/llm-abstraction.md`](../../docs/docs/ai/llm-abstraction.md),
[`docs/ai/overview.md`](../../docs/docs/ai/overview.md).

---

## 1. The routing model

**The application asks for a task. The gateway decides the model.** That is the whole
design, and 030 exists to make it the *only* design.

- 030-FR-004: the application requests AI work by task name only — `match`, `generation`,
  `rephrase`, `ghost`, `default` — carrying no provider or model identity.
- 030-FR-005: provider and model selection lives entirely in `gateway/config.yaml`. Changing
  which model serves a task is a YAML edit plus `docker compose restart litellm`. No
  dashboard, no rebuild, no application restart (030-SC-003: under 5 minutes, one file, one
  service).
- 029-FR-001: the gateway is a LiteLLM container in the same compose stack, presenting an
  OpenAI-compatible chat-completions endpoint (029-FR-011).
- 029-FR-003: the application speaks OpenAI-compatible protocol to it via a gateway provider
  implementation (`internal/platform/llm/infrastructure/gateway/`).

## 2. Failover chains

- 029-FR-004/005 + 030-FR-006: each task key resolves to an **ordered** chain. Free-tier
  hosted providers (Cerebras → Groq → Cohere) are attempted before the OpenRouter
  aggregator.
- 030-FR-008: the **final** entry of every chain is the locally hosted Ollama model. Ollama
  always terminates the chain — this is Constitution V made mechanical.
- 030-FR-007: the chain advances automatically on a missing credential, an authentication
  failure, or an unavailable entry.
- 030-FR-011: absent optional provider credentials never prevent startup and never cause a
  request-time error; those entries are skipped.
- 030-FR-009: when the gateway is unconfigured or unreachable, the application serves AI
  tasks with the local model rather than failing them (030-SC-005: with no external provider
  reachable at all, matching and generation still complete).
- 029-SC-006: the gateway adds no more than 200 ms median latency over calling the provider
  directly.

**Current chains** — `gateway/config.yaml`, one `<task>` / `<task>-groq` / `<task>-cohere` /
`<task>-openrouter` quartet per task key, each falling through to Ollama.

## 3. Boundaries

| # | Rule |
|---|---|
| 029-FR-006, 030-FR-014 | **Embeddings never touch the gateway.** They go directly to local Ollama. No remote provider in the chain offers an embeddings API, and 029-SC-004 requires no change in embedding latency or behaviour. |
| 030-FR-010 | Provider credentials come from environment configuration only. They are never stored in the application database and never readable through any API. They live in the `litellm` compose service's environment; the Go backend never reads them. |
| 029-FR-012 | The gateway retains and logs **no** prompt or response data. |
| 030-FR-012 | Per AI request, the provider and model that served it is recorded, so effective routing is determinable from logs alone (030-SC-006: within 2 minutes, no DB query). The application learns nothing about the upstream beyond a `served_model` log line. |
| 030-FR-015 | Environment documentation lists every provider key the gateway consumes — `CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY` — and no longer describes per-task provider settings. |

**Error classification.** Terminal provider problems — rejected key, out of credits, unknown
model — fail the task immediately with the reason on its activity record. Transient
5xx/network failures stay retryable. When the chain is exhausted, the failure surfaces the
same way a direct-Ollama failure would (029-FR-010).

## 4. Throughput and stuck runs (019)

**Concurrency**

| # | Requirement |
|---|---|
| 019-FR-001/002 | AI work items are processed concurrently, with at least 3 items of a given type in flight against a hosted provider (019-SC-002). |
| 019-FR-004 | Local-model work stays at a concurrency level that does not overwhelm the local runtime — in practice Ollama runs at one. |
| 019-FR-003 | Outbound pacing intended for scraped job boards is **never** applied to AI provider calls. Different problem, different limits. |
| 019-FR-005 | Concurrency levels are operator-configurable through existing configuration. |
| 019-FR-017 | Raising concurrency never produces conflicting or duplicated stored results. |
| 030-FR-013 | The per-task admission control that distinguishes local from hosted execution survives 030 unchanged. |

Each task type gets its own asynq **queue and server** (`internal/queue/queue.go`) — a
single server's `Queues` map only weights priority within one shared pool, it is not a
per-queue concurrency ceiling. Queues: `ingest`, `match`, `generate`, `enrich`,
`salary:infer`, `ghost:score`.

**Timeouts and recovery**

- 019-FR-006/007: a maximum execution duration per AI work item, configurable per task type
  with defaults.
- 019-FR-008: exceeding it records a **terminal** state, not an indefinite hang
  (019-SC-004).
- 019-FR-009/010: on startup, runs left non-terminal by a previous process are detected and
  closed out; so are non-terminal runs exceeding the maximum while running (019-SC-005: after
  an abrupt shutdown, 100% of in-flight runs reach a terminal state).
- 019-FR-011: terminal states are distinguishable by the user — succeeded, failed, timed out,
  aborted (019-SC-007: for any auto-closed run the user can tell why it ended).
- 019-FR-012: aborted work is either retried under the task type's policy or left terminal —
  never silently dropped.
- 019-FR-015: a hosted provider signalling quota exhaustion or rate rejection is handled as a
  routing signal, not a crash.
- 019-FR-016: pending and in-flight counts are exposed per task type so backlog is visible.

**Performance**

- 019-FR-013/014, 019-SC-003: median local-model matching time per job drops ≥30%, partly by
  not repeating per-batch work that does not vary between jobs.
- 019-FR-018: every change preserves full operation against the local model alone.

## 5. Superseded: 001-cerebras-model-toggle

001 added a dashboard Settings surface for choosing an AI provider and model **per chat
task**, with a curated hosted-model list and stored per-task assignments. 030 removed all of
it. Every one of 001's FR-001..FR-015 is void.

030's replacements, stated as removals:

- 030-FR-001: the dashboard presents **no** control for choosing a provider or model for any
  task, and no provider-credential status tied to one.
- 030-FR-002: the read/update interfaces exposing per-task provider/model assignments are
  gone, including the curated hosted-model list.
- 030-FR-003: the stored per-task assignments are gone; no runtime behaviour depends on them.
- 029-FR-007 — which required Cerebras and Ollama to stay *selectable in dashboard Settings*
  alongside the gateway — is likewise revoked by 030-FR-001. 029 was an additive step; 030
  finished the migration.
- 030-SC-001/SC-007: zero AI provider or model choices anywhere in the dashboard; the
  AI-model settings screen, its data store and its interfaces are absent, with no regression
  in the remaining settings.

What survives from 001 is the *idea*: free-tier hosted inference is worth using when
available. It is now expressed as chain order in `gateway/config.yaml` (030-FR-006), not as
a user-facing toggle. 030-SC-002 sets the bar: with free-tier keys healthy, ≥95% of AI
requests over a normal day are served by a free-tier provider.

**Do not reintroduce provider/model selection into the dashboard.** If it needs to come
back, it is a new feature that must argue against 030 explicitly.
