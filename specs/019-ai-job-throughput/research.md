# Phase 0 Research: AI Job Throughput & Stuck-Run Recovery

**Feature**: 019-ai-job-throughput | **Date**: 2026-07-28

All findings below are from reading the current code, not assumption. File:line references
are the anchors implementation will touch.

---

## R1 — Where the "throttle" actually is

**Decision**: There is no HTTP rate limiting on AI provider traffic. The bottleneck is
worker concurrency, hardcoded to 1 for every AI task type. Fix concurrency; verify (and
regression-test) that AI traffic never picks up the scraper pacer.

**Evidence**:

- `apps/api/cmd/server/servers.go:70-77` — six asynq servers; `match`, `generate`,
  `enrich`, `salary`, `ghost` all constructed with `concurrency = 1`, `ingest` with 2. The
  comment states the reason: "local LLM handles one request at a time comfortably".
- `apps/api/internal/ratelimit/transport.go` (`DefaultRPS = 0.7`) is applied only via
  `retrieval.DefaultTransport`, which is wired into exactly two clients:
  `apps/api/internal/jobsources/httpjson.go:22` and
  `apps/api/internal/scraping/scraping.go:40`.
- Every LLM provider builds its own `http.Client` with no custom Transport
  (`llm/ollama.go:46`, `llm/cerebras.go:39`, `llm/openrouter.go:47`), so it uses
  `http.DefaultTransport` — unpaced.

**Consequence for FR-003**: already satisfied in fact, not by construction. Add a guard
test asserting no LLM provider client uses `retrieval.DefaultTransport`, so a future
"wrap everything in the paced transport" change can't silently throttle AI calls.

**Alternatives considered**: raising `DefaultRPS` — rejected, it would loosen scraper
pacing (feature 017's whole point) to fix an unrelated problem.

---

## R2 — "Ollama Cloud" is not a separate provider in this codebase

**Decision**: Concurrency must be chosen by *provider class* (local vs hosted) resolved at
runtime, not by provider name.

**Evidence**:

- `llm.TaskProvider` has three values: `ollama`, `cerebras`, `openrouter`
  (`llm/router.go:11-14`). Ollama Cloud is the *same* `OllamaProvider` pointed at
  `https://ollama.com` with `OLLAMA_KEY` set (`llm/ollama.go:16-26`, `config.go:25-30`).
- The repo's own `.env.example:19-32` is exactly this shape: `OLLAMA_URL=https://ollama.com`,
  `LLM_MODEL_MATCH=gpt-oss:20b-cloud`, with `EMBED_URL=http://localhost:11434` because
  Ollama Cloud serves no embedding models.
- So the user's "local ollama match is slow" and "Ollama Cloud shouldn't throttle" describe
  one pipeline: **cloud chat + local embeddings**, serialized behind concurrency 1.

**Rule adopted**: a provider is *hosted* when it is Cerebras, OpenRouter, or an Ollama
provider whose base URL host is not loopback/private **or** whose API key is set.
Otherwise *local*.

**Alternatives considered**: a new `TaskProviderOllamaCloud` enum value — rejected, it
would change the persisted `LlmTaskSetting` contract and the Settings UI for no gain.

---

## R3 — Provider routing is dynamic; asynq concurrency is static

**Decision**: Set each AI worker's asynq `Concurrency` to the **maximum** of the two
configured levels, and gate actual execution with a runtime admission semaphore whose
capacity is re-resolved per task from the live router snapshot.

**Evidence**: `llm.SnapshotHolder` (`llm/router.go:38-63`) is swapped atomically whenever
Settings change, so a task can flip local↔hosted with no restart. `asynq.Config.Concurrency`
is fixed at `asynq.NewServer` time (`servers.go:40`). Without the gate, a user switching
`match` back to a local model would immediately run 3 concurrent local generations and
thrash the local runtime.

**Semaphore semantics**: one weighted semaphore per task type, capacity =
`max(localN, hostedN)` (matching the pool size); a task resolving to *local* acquires
`max/localN` weight… rejected as too clever. Adopted instead: **two semaphores per task
type** — a `hosted` semaphore of size N_hosted and a `local` semaphore of size N_local; a
task acquires from the one matching its resolved class. In-flight tasks keep the class they
started with (spec edge case).

**Alternatives considered**:

- Restart-on-settings-change: rejected, drops in-flight work and is user-hostile.
- Separate queues per provider class: rejected, doubles the queue topology and the routing
  decision would still be made at enqueue time, before the user may have flipped it.

---

## R4 — Why runs hang in `running` for 10+ hours

**Decision**: The stuck state is a *database* artifact, not a stuck goroutine. Fix by
heartbeating the run row and sweeping stale rows, plus a hard per-task deadline.

**Evidence**:

- `ActivityRun` (`db/migrations/00004_activity_run.sql`) has `state` defaulting to
  `queued`, moved to `running` by `StartActivityRun`, and only ever leaves via
  `FinishActivityRunOk/Error/Cancelled` — all called from a live handler
  (`activity/recorder.go:17-23`, `db/queries/activityrun.sql`).
- No sweeper, no startup reconciliation, no heartbeat column exists anywhere.
- When the process dies mid-task, asynq recovers the lease and retries — but once the
  retry budget is spent the task is archived and **no handler ever runs again**, so the row
  stays `running` forever. Several AI enqueue sites use `asynq.MaxRetry(0)`
  (`jobs/service.go:344`, `httpapi/activity.go:261,281,323`), so this is the common case,
  not the rare one.
- No `asynq.Timeout` or `asynq.Deadline` is set anywhere in the repo (grep: zero hits), so
  every task inherits asynq's 30-minute default — nothing enforces a per-type limit, and a
  wedged HTTP call can sit under `llm/ollama.go:46`'s 300s client timeout repeatedly.

**Adopted mechanism** (one detector, not two special cases):

1. `heartbeatAt` column, written by `StartActivityRun`, by every `SetActivityStep`, and by
   a ticker in the worker middleware while the task runs.
2. `activity.Sweeper` runs every 60s (and once at startup): rows in `running` with
   `heartbeatAt` older than 2 minutes → `interrupted`. Rows in `queued` older than the
   queued-grace window with no live asynq task (Inspector lookup by `queueTaskId`) →
   `interrupted`.
3. Per-task-type `context.WithTimeout` in the worker middleware → on deadline the handler
   returns and the run is finalized `timed_out` in-process, with elapsed recorded.

This satisfies FR-009's 5-minute bound (60s sweep + 120s stale threshold) and works for
crash, power loss, and multi-instance alike — no dependence on process start time.

**Alternatives considered**:

- Startup-only sweep keyed on process start time: rejected, wrong under more than one
  instance and blind to a wedge that happens while the process stays up.
- Relying on asynq archived-task inspection alone: rejected, tasks enqueued with
  `MaxRetry(0)` leave archive entries the Inspector can't always correlate back to a run
  once retention expires.

---

## R5 — Local matching cost breakdown

**Decision**: Four cuts, in expected-payoff order. Target is FR-013's 30% median cut.

Per-job work today (`matching/service.go:39-76`, `matching/scoring.go`):

| Step | Cost | Repeated per job? | Lever |
|---|---|---|---|
| `profiles.GetDefault` | 1 DB read | yes | cache per process, invalidate on profile write |
| `MasterFromConfig` + `RendercvToText` + truncate | YAML/JSON parse + string build of ~6 KB | yes, identical output every time | cache the derived profile text alongside the profile |
| `llmc.Embed(jobText)` | 1 local embedding call, `POST /api/embeddings` (legacy single-input endpoint), 8 KB text | yes, even when the job already has an unchanged embedding | skip when a stored content hash matches; batch via `/api/embed` when a batch is available |
| `HasEmbedding` + `RefreshEmbedding` + `Similarity` | 2–3 DB reads | yes | fold `HasEmbedding` into the cached profile snapshot |
| LLM fit analysis | the dominant cost when similarity ≥ threshold | only above threshold (`MATCH_SIMILARITY_THRESHOLD=0.35`) | `keep_alive` so a local model isn't reloaded between calls; prompt already puts the invariant profile first, which is prefix-cache friendly |

**Notable**: `OllamaProvider` sends no `keep_alive` (`llm/ollama.go:128-155`) — a local
Ollama unloads the model after its default idle window, so a sparse queue pays a full model
load per job. Adding `keep_alive` is the single cheapest local win, and it is a no-op
against Ollama Cloud.

**Also**: `http.DefaultTransport`'s `MaxIdleConnsPerHost` is 2. At hosted concurrency 3 the
third request opens a fresh TLS connection every time; give the LLM clients a transport with
`MaxIdleConnsPerHost` ≥ the hosted concurrency.

**Alternatives considered**: raising local concurrency above 1 — rejected as the primary
lever (spec assumption: local runtime is the bottleneck), but kept as an operator knob.

---

## R6 — Concurrency safety at N>1

**Decision**: Existing writes are already idempotent per job; no new locking needed, one
guard test added.

**Evidence**: the terminal write for matching is `UpsertMatchResult`
(`db/queries/matchresult.sql`, called at `matching/service.go:102`) keyed by `jobId` — a
duplicate concurrent match converges on last-writer-wins with identical inputs. Job
embedding writes are a plain `UpdateJobEmbedding` by id. FR-017 is therefore a test
obligation, not a design change.

---

## R7 — Backlog visibility

**Decision**: New `GET /api/activity/queues`, backed by the already-constructed
`asynq.Inspector` (`platform.go`, `Platform.AsynqInspector`), exposing per-queue
`pending`/`active`/`scheduled`/`retry`/`archived` plus a throughput-derived ETA.

**Evidence**: `httpapi/activity.go:41-71` already depends on an `ActivityInspector`
interface for cancel; extending that interface with `Queues`/`GetQueueInfo` follows the
existing pattern. Feature 018 shipped asynqmon for dev, but it is dev-only and outside the
dashboard — SC/FR-016 wants this in the product UI.

---

## R8 — New terminal states and the typed contract

**Decision**: Add `timed_out` and `interrupted` to the activity state set.

**Evidence**: `state` is a plain `text` column with no CHECK constraint
(`00004_activity_run.sql:5`), so no data migration is needed for the values themselves —
but `ACTIVITY_STATES` in `packages/shared/src/index.ts:567` is hand-maintained and
`StatusPage.tsx:229-239` switches on the literals. Per Constitution III, `packages/shared`
is the single source of truth; the Go DTO and dashboard rendering must be updated in the
same change. `ListFailedActivityRuns` (`activityrun.sql`) must widen to include the two new
states so the existing retry flow keeps working on them.

---

## R9 — Default values

| Knob | Default | Basis |
|---|---|---|
| `AI_CONCURRENCY_CLOUD` | 3 | user's stated requirement (spec assumption) |
| `AI_CONCURRENCY_LOCAL` | 1 | current behaviour, preserved |
| `AI_TASK_TIMEOUT_MATCH` | 5m | ~1 embedding + 1 chat; 300s client timeout is the existing ceiling for one call |
| `AI_TASK_TIMEOUT_SALARY` / `_GHOST` | 5m | same shape as match |
| `AI_TASK_TIMEOUT_GENERATE` | 15m | document generation is multi-call and includes rendercv |
| `AI_TASK_TIMEOUT_ENRICH` | 10m | network-bound page fetches, deliberately paced |
| `AI_TASK_TIMEOUT_INGEST` | 30m | multi-page scrape under 0.7 rps pacing |
| `ACTIVITY_HEARTBEAT_INTERVAL` | 30s | 4× margin under the stale threshold |
| `ACTIVITY_STALE_AFTER` | 2m | with a 60s sweep gives the FR-009 5-minute bound with room |
| `ACTIVITY_SWEEP_INTERVAL` | 1m | as above |
| `OLLAMA_KEEP_ALIVE` | 30m | keeps a local model resident across a queue drain; ignored by Ollama Cloud |

All exposed as env keys through the existing `defaults`/`optionalKeys` maps
(`config/defaults.go`), per FR-005.
