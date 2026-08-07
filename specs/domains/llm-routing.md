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
  aggregator. Exception: the `generation` chain puts OpenRouter first — resume tailoring
  is quality-critical, so it pays for the aggregator up front and keeps the free tiers as
  its fallbacks.
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

### 2.1 `gateway/config.yaml` — the routing contract

Mounted read-only into the `litellm` service. For **every** task key there must be a public
group named exactly the task key whose deployment is a free-tier provider, plus an ordered
fallback list declared under `litellm_settings.fallbacks`. Three order constraints must hold
for every task:

- All free tiers (Cerebras, Groq, Cohere) precede any OpenRouter tier (030-FR-006).
- The final tier is the Ollama deployment (030-FR-008).
- **No chain is empty and no chain terminates on a hosted provider.**

```yaml
general_settings:
  master_key: os.environ/LITELLM_MASTER_KEY

model_list:
  - model_name: match                       # public group, free tier 1
    litellm_params: {model: cerebras/<verified>, api_key: os.environ/CEREBRAS_API_KEY}
  - model_name: match-groq
    litellm_params: {model: groq/<verified>, api_key: os.environ/GROQ_API_KEY}
  - model_name: match-cohere
    litellm_params: {model: cohere_chat/<verified>, api_key: os.environ/COHERE_API_KEY}
  - model_name: match-openrouter
    litellm_params: {model: openrouter/<verified>, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: local                       # terminal tier, shared by every task
    litellm_params:
      model: ollama_chat/<LLM_MODEL>
      api_base: os.environ/OLLAMA_URL
      api_key: os.environ/OLLAMA_KEY
  # … the same five-tier pattern for generation, rephrase, ghost, default

litellm_settings:
  drop_params: true
  num_retries: 1
  request_timeout: 110          # must stay under the adapter's 120s client timeout
  allowed_fails: 3
  cooldown_time: 60
  fallbacks:
    - match: [match-groq, match-cohere, match-openrouter, local]
    # … one line per task key
```

The task keys and what they serve: `match` (job scoring), `generation` (cover letter — resume
generation split into the stage keys of §2.2), `rephrase` (keyword suggestions), `ghost`
(ghost-job detection), `default` (salary, outreach, recruiter extraction).

**Request contract.** `POST {GATEWAY_URL}/chat/completions` with
`Authorization: Bearer {LITELLM_MASTER_KEY}`. `model` is always one of the task keys (§2.2 adds
the generation stage keys) — never a provider or upstream model name. The app sends `temperature`, an optional
`max_completion_tokens`, and `response_format: {"type":"json_object"}` for structured calls;
`stream` is always false. **The proxy must fail loudly (4xx) on an unknown group, never
silently route it to a default.**

**Response contract.** Success is the OpenAI chat-completion shape with at least one choice.
Failure after the whole chain is exhausted is a non-2xx that the adapter classifies through
`infrastructure/shared`; existing worker retry/skip semantics are unchanged. The HTTP status
maps onto the sentinel taxonomy:

| Status | Sentinel |
|---|---|
| 429 | `ErrRateLimited` |
| 401 / 403 | `ErrCredentialRejected` |
| 402 | `ErrInsufficientCredits` |
| 404 / 400 / 422 | `ErrModelUnavailable` |
| 5xx | `ErrProviderUnavailable` |

**Credential handling (030-C4).** Every `api_key` is an `os.environ/…` reference; **no
literal key may appear in the file.** The `litellm` compose service passes
`CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY`, `OLLAMA_URL`,
`OLLAMA_KEY` and `LITELLM_MASTER_KEY`, each with an **empty default (`${VAR:-}`)**, so a
missing variable never aborts config load (030-FR-011). An empty key produces an auth failure
at request time, which advances the chain (030-FR-007).

> **The capability trap (030-C5).** Every model in a chain for a JSON-consuming task must
> support `response_format: {"type":"json_object"}`. Because `drop_params: true` silently
> drops an unsupported parameter, a non-JSON-capable model degrades into prose the app cannot
> parse — **and the fallback chain will not rescue that**, because the request succeeded.
> Model IDs are verified against each provider's live catalogue at implementation time and
> pinned with a dated comment.

**Change contract (030-C6).** Changing which model serves a task requires exactly: edit this
file → `docker compose restart litellm`. No application rebuild, no migration, no dashboard
action.

Config can also be hot-reloaded without a restart where the admin API is enabled:
`curl -X POST http://litellm:4000/config/reload -H "Authorization: Bearer $LITELLM_MASTER_KEY"`.

### 2.2 Generation stage keys (035-split-model-generation)

Resume generation is not one call but four, and they do not want the same model. The task key
`generation` therefore split into one key per stage, so each stage is served by the cheapest
option that does its job. `generation` itself is **retained**, now serving only the on-demand
cover letter.

| Task key | Stage | Why this tier |
|---|---|---|
| `generation-analyze` | Vacancy analysis | Mechanical extraction. Economy model; short output. |
| `generation-select` | Skill and highlight selection | Ranking and picking, not writing. Economy model; the largest structured output of the four. |
| `generation-select-premium` | Selection escalation (035-FR-007) | Reached only when the economy model returns incomplete selection output twice. Premium model. |
| `generation-summary` | Professional summary | The one stage where writing quality decides whether the document is usable. Premium model. |

Measured 2026-08-07 on one vacancy with an identical prompt and strict schema: the economy model
does the mechanical work as well as the premium one at 1/30th the price, and fails only at the
summary (035-research.md R1).

**Every chain still terminates at `local`** — including `generation-select-premium`. An escalation
key is the easiest one to forget, because it looks like a variant of `generation-select` rather
than a group the application requests, and it is requested directly (`compose.go`
`GenerationPremiumRouter`). A key declared without a chain terminates on its own hosted provider,
so an escalation fired during an Anthropic outage would fail the run instead of falling through to
Ollama. That is 030-FR-008 and Constitution V, and it is not weakened by the split (035-FR-011).

**The reasoning switch (035-FR-014).** Every stage deployment must declare how that model's
deliberation is bounded. Reasoning tokens count against `max_completion_tokens`, so a thinking
model left to deliberate freely spends its entire output budget reasoning and returns **empty
content** — a 200 response with nothing in it, which no fallback rescues because the request
succeeded. This is the same shape of trap as the JSON-capability trap above, and it is not
hypothetical: it is what made every resume run fail before 2026-08-07. The switch differs by
provider family (035-research.md R2):

| Provider family | Switch |
|---|---|
| OpenAI, Anthropic, Google | `reasoning_effort: low` |
| z-ai, deepseek-flash | `reasoning: {enabled: false}` |
| deepseek-v4-pro | Honours neither — **not eligible** as a stage deployment |

Adding a stage candidate therefore includes declaring its switch. A deployment without one is a
configuration error, not a default.

**Guardrail.** `internal/platform/llm/gateway_config_test.go` parses `gateway/config.yaml` and
fails the build when a requested `generation-*` group has no chain, a chain does not end at
`local`, a chain names an undeclared tier, an `openrouter/*` stage deployment omits its reasoning
switch, or an `api_key` is a literal. The file is not compiled into the binary, so this test is
the only thing that makes a forgotten chain fail loudly rather than silently at request time.

**Retune procedure (035-FR-016).** Unchanged from 030-C6 and it must stay that way: edit
`gateway/config.yaml` → `docker compose restart litellm`. Repointing a stage at a different model,
or adding a candidate to a chain, is a configuration edit and a restart of the routing service —
no application rebuild, no migration, no code change, no dashboard action.

**Local pinning.** When `GATEWAY_URL` is empty the gateway is bypassed and each stage uses its
`LLM_MODEL_GENERATION_*` value: `LLM_MODEL_GENERATION_ANALYZE`, `_SELECT`, `_PREMIUM`, `_SUMMARY`.
Each resolves through `Config.GenerationModelOr` — empty falls back to `LLM_MODEL_GENERATION`, then
to `LLM_MODEL` — so an operator who pins nothing per stage keeps today's behaviour.

### 2.3 No LLM framework — the port is kept (2026-08-07 decision)

**Decision: `domain.Provider` stays hand-owned. `langchaingo` is not adopted, and no Go LangGraph
port is adopted.** This is recorded here rather than in a feature directory because it is a standing
constraint that must survive those directories being removed on ship.

The comparison was made against what is actually in the tree, not against the idea of a framework.
`langchaingo`'s `llms.Model` is a downgrade on every axis this codebase depends on:

| In `domain.Provider` today | `langchaingo` equivalent |
|---|---|
| `ResponseModeStrict` + marshalled JSON Schema per call (033) | no strict-schema abstraction; per-provider JSON-mode flags at best |
| `strictifySchema` / `makeNullable` — `additionalProperties:false`, nullable optionals | none |
| `CompleteStructured[T]` — typed generic, schema cache, parse-and-retry, `Validator` hook | `outputparser`, string-typed and weaker |
| `WithServedModelCapture` reading `x-litellm-model-name` | no hook; the fallback-tier visibility § 2.1 and § 2.2 depend on would be lost |
| `Router` with `ProviderClass`, gateway ↔ local | no routing concept |

Adopting it would mean discarding features 033 and 035 to gain a message slice, and pulling a wide
transitive dependency tree into a deliberately tight `go.mod`.

The Go LangGraph ports are smaller again — a message-state graph with conditional edges, without the
durable checkpointer, interrupt/resume or time-travel that make the Python original worth having.
The durability they lack, this platform already has and better: asynq on Redis, Postgres, and the
per-task deadline and heartbeat middleware in § 4.

**What is built instead**, when the need is real rather than anticipated:

- Multi-turn and tool calling: extend this port (`CompleteChat`, a bounded typed tool loop). Owned
  here, no new module requirement.
- Provider abstraction, failover, retry, model swap without rebuild: already the LiteLLM proxy's job
  (§ 2.1). A framework would duplicate it, worse.
- Durable multi-step orchestration: already asynq (§ 4).
- Observability of cost, latency and served tier: proxy callbacks, not client instrumentation.

**The one exception worth revisiting**: `langchaingo`'s `textsplitter` is a genuinely useful leaf and
could be vendored on its own merits for retrieval work. Vendoring a leaf is not adopting a framework.

**Guardrail.** Any change adding an agent or LLM-orchestration framework to `apps/api/go.mod`
contradicts this section and requires amending it in the same change, with the reasoning above
rebutted rather than ignored.

### 2.4 `Router` — the internal Go seam

`internal/platform/llm/application`, re-exported by `internal/platform/llm`. It replaced the
`SnapshotHolder` / `RouterSnapshot` / `TaskSetting` machinery that 001 introduced.

```go
func NewRouter(taskKey string, gateway, local domain.Provider, localModel string) *Router
```

`gateway` is nil when `GATEWAY_URL` is empty; `local` (Ollama) is never nil. `localModel` is
the task's `LLM_MODEL_*` value, empty meaning "fall back to `LLM_MODEL`".

**A `Router` is immutable after construction.** There is no setter, no holder, no atomic
swap, and no code path that changes routing at runtime — that immutability is the whole point
of 030.

| Method | Contract |
|---|---|
| `Complete` / `CompleteJSON` | Route to `gateway` when non-nil with `CompleteOptions.Model = taskKey`; otherwise to `local` with `Model = localModel`. An explicit caller-supplied `Model` still wins. |
| `Embed` | **Always** `local`, regardless of chat routing (030-FR-014). |
| `ModelName` | `taskKey` when gatewayed — that is what the proxy resolves — else the effective local model. |
| `ProviderClass` | `hosted` when `gateway != nil`, else `local.IsHosted()` (030-FR-013). Satisfies `queue.ClassResolver` unchanged. |

`Router` still satisfies `domain.Provider`, so the `matching`, `generation`, `ghostjob`,
`salary`, `keyword`, `recruiter` and `outreach` call sites were untouched apart from
`composeLLM`'s construction arguments. `llm.NewProviders` returns
`(*OllamaProvider, *GatewayProvider, error)` — the Cerebras leg is gone.

The error sentinels `ErrRateLimited`, `ErrCredentialRejected`, `ErrInsufficientCredits`,
`ErrModelUnavailable`, `ErrProviderUnavailable`, `ErrInvalidResponse`, `Terminal` and
`Retryable` are **kept**, now aliasing `infrastructure/shared` directly rather than
`infrastructure/cerebras`, so worker handlers branching on them kept compiling.

**Served-model logging (030-FR-012).** Read the served model from, in order: the
`x-litellm-model-name` response header, then the body's `model` field, then `"unknown"`. Log
one structured line per request — `task`, `requested_group`, `served_model`, `duration_ms`,
`outcome` — plus `x-litellm-model-id` when present (an opaque correlation hash, not a model
name).

> **Verified 2026-07-31: the body's `model` field alone is unreliable.** On a primary-tier
> hit with no fallback, the pinned image echoes back the *requested group name*; it only
> becomes the resolved model once a fallback fires. `x-litellm-model-name` is authoritative on
> both paths. Logging must never change error classification or introduce a failure mode — an
> absent or unparsable field logs `served_model=unknown`, never an error.

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

### 4.1 The configuration surface

Registered in `internal/config/defaults.go` and read through `config.Config` struct tags.
**All have defaults; none are required** (019-FR-005). Documented in `.env.example` beside
the existing LLM block.

| Key | Default | Effect |
|---|---|---|
| `AI_CONCURRENCY_CLOUD` | `3` | Simultaneous AI items per task type when that task resolves to a hosted provider |
| `AI_CONCURRENCY_LOCAL` | `1` | Same, for a local Ollama — preserves the pre-019 behaviour |
| `INGEST_CONCURRENCY` | `2` | Unchanged behaviour, promoted from a hardcoded literal |
| `ENRICH_CONCURRENCY` | `1` | Same. Deliberately low — these are authenticated per-job page fetches |
| `AI_TASK_TIMEOUT_MATCH` | `5m` | Deadline for `match` |
| `AI_TASK_TIMEOUT_GENERATE` | `15m` | Deadline for `generate` |
| `AI_TASK_TIMEOUT_SALARY` | `5m` | Deadline for `salary:infer` |
| `AI_TASK_TIMEOUT_GHOST` | `5m` | Deadline for `ghost:score` |
| `AI_TASK_TIMEOUT_ENRICH` | `10m` | Deadline for `enrich` |
| `AI_TASK_TIMEOUT_INGEST` | `30m` | Deadline for `ingest` |
| `ACTIVITY_HEARTBEAT_INTERVAL` | `30s` | How often a running worker refreshes `ActivityRun.heartbeatAt` |
| `ACTIVITY_STALE_AFTER` | `2m` | A `running` row silent this long is `interrupted` |
| `ACTIVITY_SWEEP_INTERVAL` | `1m` | Sweeper period; a sweep also runs once at startup |
| `ACTIVITY_QUEUED_GRACE` | `30m` | A `queued` row older than this with no live asynq task is `interrupted` |
| `OLLAMA_KEEP_ALIVE` | `30m` | Sent as `keep_alive` so a local model stays resident across a queue drain. Ignored by Ollama Cloud; an empty string omits the field |
| `LLM_MAX_IDLE_CONNS_PER_HOST` | `4` | Go's default of 2 forces a fresh TLS handshake on the third concurrent hosted request |

Startup validation, not runtime surprises: concurrency values `< 1` are rejected with a
config error naming the key; durations are parsed with `time.ParseDuration` and `<= 0` is
rejected; `ACTIVITY_STALE_AFTER` must be **at least twice** the heartbeat interval; and
`ACTIVITY_STALE_AFTER + ACTIVITY_SWEEP_INTERVAL` must stay under five minutes to satisfy
019-FR-009 / 019-SC-005.

The asynq pool for an LLM task type is sized `max(AI_CONCURRENCY_CLOUD,
AI_CONCURRENCY_LOCAL)`; the admission gate then enforces whichever applies at run time.

**Non-goal, guarded by test:** no pacing key (`ratelimit` / `retrieval`) changes. AI provider
traffic does not pass through the paced transport and must not start doing so (019-FR-003).

### 4.2 Activity states and the backlog endpoint

019 widened `ACTIVITY_STATES` in `packages/shared/src/index.ts` with two additions:

- `timed_out` — the run exceeded its task-type deadline. Rendered as a **danger** variant.
- `interrupted` — the worker vanished (crash, shutdown, power loss). Rendered as a
  **warning** variant.

Both surface `error` as the reason (019-FR-011, 019-SC-007). `ActivityRunDto` gained
`heartbeatAt: string | null` and `timeoutMs: number | null`; everything else is unchanged, so
the change is additive apart from clients that switch on state needing two more branches.
`POST /activity/retry` accepts `timed_out` and `interrupted` exactly as it accepts `failed`
and `cancelled` — `ListFailedActivityRuns` widens its `state IN (...)` list to all four
(019-FR-012).

`GET /api/activity/queues` is the read-only backlog snapshot (019-FR-016):

```json
{"queues": [{
  "queue": "match", "providerClass": "hosted", "concurrency": 3,
  "pending": 684, "active": 3, "scheduled": 0, "retry": 2, "archived": 11,
  "processedPerMinute": 18.4, "etaSeconds": 2230
}]}
```

- One entry per queue, in the fixed order `ingest`, `match`, `generate`, `enrich`, `salary`,
  `ghost`.
- `providerClass` is `null` for queues with no LLM task key (`ingest`, `enrich`).
- `concurrency` is the effective admission capacity for the **currently resolved** provider
  class — not the pool size.
- `etaSeconds` is `null` when `processedPerMinute` or `pending` is 0.
- **An Inspector failure for one queue must not fail the whole response**: that entry omits
  its counters and carries an `"error"` string instead. `503` is returned only when the
  Inspector is entirely unavailable (Redis down), matching the health-check convention.

The TS type is `QueueBacklogDto` in `packages/shared/src/index.ts`.

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

### 5.1 What 030 deleted

Kept as a record of what "gone" meant, so a future change does not resurrect part of it by
accident. Each line is independently greppable.

**HTTP** — `GET /v1/settings/llm`, `PUT /v1/settings/llm` and `GET /v1/settings/llm/models`
all 404. `app.LlmSettings.Mount` left the mount list in `cmd/server/servers.go` and the
`LlmSettings` field left the `App` struct.

**Go backend** — the whole `internal/llmsettings/` tree; the whole
`internal/platform/llm/infrastructure/cerebras/` package; `SnapshotHolder`, `RouterSnapshot`,
`TaskSetting`, `TaskProvider` and its constants from
`internal/platform/llm/application/router.go`; the Cerebras and snapshot re-exports from
`llm.go` (`llm.CerebrasProvider`, `llm.NewCerebras`, `llm.CerebrasModel(s)`,
`llm.IsSupportedCerebrasModel`, `llm.DefaultCerebrasModel`); `LlmTaskSettingDto`,
`LlmSettingsResponseDto`, `UpdateLlmSettingsRequestDto`, `CerebrasModelDto` and
`LlmModelsResponseDto` from `internal/dto/settings.go` (**`AiFeatureSettingDto` stays**);
`CerebrasAPIKey` / `CerebrasBaseURL` from config with their default and secret-list entries;
the `llmsettings` wiring and the Cerebras leg of `NewProviders` in `compose.go`; and
`internal/db/queries/llmsetting.sql` with its generated model.

**Database** — migration `00033_drop_llm_task_setting.sql` drops `"LlmTaskSetting"` (down
recreates and reseeds). No reference to it survives outside `db/migrations/`.

**Dashboard and shared types** — `LlmSettingsCard.tsx` and its test, deleted; the *AI models*
tile removed from `SettingsPage.tsx` (**the *AI features* and *Danger zone* tiles are
unchanged**), with `SettingsPage.test.tsx` asserting its absence; `useLlmSettings`,
`useLlmModels`, `useUpdateLlmSettings` from `features/settings/hooks.ts`; `settings.getLlm`,
`settings.putLlm`, `settings.llmModels` from `lib/api.ts`; the whole `llmSettings` group from
`lib/queryKeys.ts`; and the four DTOs from `packages/shared/src/index.ts`. `StatusPage.tsx`'s
provider-specific copy — "an upstream provider (Cerebras)" — was genericised to "an upstream
AI provider".

**Environment** — the `CEREBRAS_API_KEY` / `CEREBRAS_BASE_URL` block and all prose about
dashboard-selectable providers left `.env.example`, replaced by `GROQ_API_KEY`,
`COHERE_API_KEY` and a note that provider keys are consumed by the litellm container only
(030-FR-015).

```sh
rg -n "LlmTaskSetting|llmsettings|CerebrasModel|settings/llm" apps packages   # migrations only
rg -n "SnapshotHolder|TaskProviderCerebras|IsSupportedCerebrasModel" apps     # no hits
rg -n "CEREBRAS_API_KEY" apps .env.example                                    # env doc only
```

## 6. Measurable bars

**Throughput (019)**

- 019-SC-001: a backlog of 700 hosted-provider AI items completes in at most **40%** of the
  wall-clock time it took before.
- 019-SC-002: at least 3 hosted items observably in flight simultaneously during a drain.
- 019-SC-003: median per-job local matching time drops ≥30% against the recorded baseline on
  the same 50-job benchmark — **with match scores inside the agreed tolerance and zero
  feature-threshold flips.** A speed-up that changes which jobs cross a threshold is not a
  speed-up.
- 019-SC-004: no run stays non-terminal longer than its configured maximum plus 5 minutes,
  under any shutdown or crash scenario.
- 019-SC-006: the higher concurrency shows **no higher failure rate** than the
  single-at-a-time baseline, within normal variance.

**Gateway (029, 030)**

- 029-SC-001 / 030-SC-003: changing which model serves a task is one YAML edit plus a reload —
  no application restart, no code change, under 5 minutes.
- 029-SC-002: all five task types complete successfully through the gateway with at least one
  provider configured.
- 029-SC-003: when a task's primary provider fails with a retryable error, the chain advances
  and the task still completes, assuming any provider in it is healthy.
- 029-SC-005: per-task and per-model spending is visible in the gateway's admin interface
  within 30 seconds of a request completing.
- 030-SC-002: with free-tier keys healthy, ≥95% of AI requests over a normal day are served by
  a free-tier provider.

029-SC-007 required the Cerebras and Ollama direct providers to keep working unchanged. **That
is void** — 030 deleted the Cerebras provider outright (§ 5.1). It was an additive-migration
criterion, satisfied at the time and superseded since.
