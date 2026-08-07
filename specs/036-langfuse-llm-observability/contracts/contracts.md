# Phase 1 Contracts: Self-Hosted LLM Observability

**Feature**: `036-langfuse-llm-observability` | **Date**: 2026-08-07
**Revised**: 2026-08-07 after audit. §2–§5 previously specified fields, variables and services that do
not exist. See research.md's corrections log.

Seven contracts. No HTTP API is added to the platform — the only new network surface is the
collector's own, which is an operator tool.

---

## 1. Routing-service callback contract — `gateway/config.yaml`

`litellm_settings` gains a callback declaration. This is the whole of US1.

```yaml
litellm_settings:
  # ... existing drop_params / num_retries / request_timeout / fallbacks ...

  # 036-langfuse-llm-observability. Both lists, not just success: a call that
  # exhausts every tier is exactly the call worth having a record of (FR-002).
  success_callback: ["langfuse"]
  failure_callback: ["langfuse"]
```

**Binding rules**

- **C1-1**: Both `success_callback` and `failure_callback` MUST list the collector.
- **C1-2**: The callback MUST NOT be added to any per-deployment `litellm_params`. It is global;
  per-deployment callbacks produce coverage that depends on which tier answered.
- **C1-3**: No credential may appear literally in this file.
- **C1-4**: Adding the callbacks MUST NOT change `request_timeout`, `num_retries`, `allowed_fails`,
  `cooldown_time` or any `fallbacks` entry. The worst-case timing arithmetic documented in that file
  (`tiers × (1 + num_retries) × request_timeout` under the Go adapter's 15-minute safety net) must
  remain exactly as it is.
- **C1-5**: With the collector unreachable, this configuration MUST still serve requests normally
  (FR-004). Verified in quickstart, not assumed.
- **C1-6**: The pinned image already ships the Langfuse SDK, verified against
  `ghcr.io/berriai/litellm:main-stable`. No custom image build is required, and none may be
  introduced — a custom image would put SC-002's "config-only" claim out of reach.

**Retune procedure** (FR-006): edit this file or the collector's environment, then
`docker compose restart litellm`. No application rebuild, no migration.

---

## 2. Environment contract

**Corrected**: the previous version specified `LANGFUSE_RETENTION_DAYS`, which does not exist —
automated retention is an enterprise feature (research R7). It also omitted every variable the
collector actually requires to boot.

New variables, each defaulting to empty so a missing value never aborts configuration load.

| Variable | Consumed by | Empty means |
|---|---|---|
| `LANGFUSE_PUBLIC_KEY` | litellm container | collector disabled; calls proceed |
| `LANGFUSE_SECRET_KEY` | litellm container | collector disabled; calls proceed |
| `LANGFUSE_HOST` | litellm container | collector disabled; calls proceed |
| `LANGFUSE_DB_URL` | langfuse web + worker | collector fails to start; nothing else affected |
| `LANGFUSE_CLICKHOUSE_URL` | langfuse web + worker | collector fails to start |
| `LANGFUSE_S3_*` | langfuse web + worker | collector fails to start |
| `LANGFUSE_REDIS_URL` | langfuse web + worker | collector fails to start |
| `NEXTAUTH_SECRET`, `NEXTAUTH_URL`, `SALT` | langfuse web | collector fails to start |
| `AUTH_DISABLE_SIGNUP` | langfuse web | **open signup** — see C2-5 |
| `EVAL_PRUNE_RETENTION_DAYS` | the platform's pruning job | defaults to 30 (FR-008) |

**Binding rules**

- **C2-1**: `LANGFUSE_HOST` MUST resolve to an address inside the deployment. A third-party host
  violates FR-005 and is a review-blocking defect.
- **C2-2**: The application container MUST NOT receive any `LANGFUSE_*` variable (FR-007).
  **This is not satisfiable today.** `docker-compose.prod.yml:27` gives the `api` service
  `env_file: .env`, so it already receives every provider credential — itself a standing violation of
  Constitution Principle V's credential clause. This contract therefore requires **narrowing that
  service's environment to an explicit list** as part of this feature. Adding a collector secret to
  the existing channel while describing it as isolated is forbidden.
- **C2-3**: With all `LANGFUSE_*` unset, the stack MUST come up, serve every AI task, and pass every
  test suite (FR-015, SC-007).
- **C2-4**: Every variable MUST be present in `.env.example` with an empty default and a comment
  stating what empty means.
- **C2-5**: `AUTH_DISABLE_SIGNUP` MUST be set. The collector holds the user's legal name, employers
  and contact details in plain text; an instance that accepts new signups from anyone who can reach
  it is not "inside the trust boundary" in any meaningful sense.
- **C2-6**: `LANGFUSE_HOST` is a **container-network** address (`http://langfuse-web:3000`). Any
  host-side verification MUST use the published host port instead — the same trap `.env.example`
  already documents for `GATEWAY_URL`.

---

## 3. Compose service contract

**Corrected**: the previous version specified a single v2 container and instructed edits to a
`litellm` service in `docker-compose.prod.yml` that does not exist.

Langfuse v4 is a service group (research R2):

```text
langfuse-web:      # UI + ingestion API
langfuse-worker:   # async processing
clickhouse:        # NEW stateful service — the real cost of this feature
# reuses: postgres (separate logical DB), redis, minio
```

**Binding rules**

- **C3-1**: Every image tag MUST be pinned to a specific version and recorded in a comment, the way
  `ghcr.io/berriai/litellm:main-stable` is recorded today.
- **C3-2**: `litellm` MUST NOT gain a `depends_on` for any collector service. Inference must start
  and serve whether or not the collector is up (FR-004).
- **C3-3**: The collector MUST use a **separate logical database** on the existing Postgres server for
  its metadata. It MUST NOT share a schema with the platform, and its migrations MUST NOT be run by
  goose.
- **C3-4**: The operator UI port MUST be bound to loopback in the production compose file, and
  SHOULD be in dev. It is an operator tool holding the user's full profile in plain text.
- **C3-5**: **`docker-compose.prod.yml` has no `litellm` service.** Prod therefore has no gateway and
  cannot run this feature at all. Either the gateway is added to prod, or a comment in that file MUST
  record that prod deliberately has no gateway and that observability is dev-only. Silently
  instructing edits to a service that is not there — as the previous version of this contract did —
  is not an option.
- **C3-6**: ClickHouse MUST have its own volume with bounded growth. Co-locating unbounded prompt-body
  storage with the platform's own datastores means a healthy collector filling a shared disk can stop
  inference — a failure mode the FR-004 gate does not test.

---

## 4. Application contract — outbound metadata (US2)

**Corrected twice**: the previous version specified `metadata.trace_id`, which *overwrites* the trace
on every call, and assumed per-task grouping came free from the `model` field, which it does not.

### 4.1 Go seam

```go
// apps/api/internal/platform/llm/domain
type CompleteOptions struct {
    // ... System, Temperature, MaxTokens, Model, ResponseMode, JSONSchema ...

    // TraceID groups this call with the other calls of the same logical run.
    // Empty means uncorrelated — the pre-036 behaviour, and valid.
    TraceID string

    // TaskKey is the requested routing key, sent so the collector can group by
    // task rather than by serving deployment. Empty means ungrouped.
    TaskKey string
}

func (o *CompleteOptions) Trace() string    // "" when o is nil
func (o *CompleteOptions) Task() string     // "" when o is nil
```

- **C4-1**: Both fields MUST be optional. Their zero values MUST leave every existing call site
  behaviourally and byte-for-byte unchanged on the wire.
- **C4-2**: Both MUST be opaque to the LLM layer — never parsed, validated, or interpreted.
- **C4-3**: Neither may be interpolated into any prompt. They are transport metadata; a run id
  reaching the model is a grounding-surface change this feature does not make.

### 4.2 Wire format — gateway adapter

```json
{
  "model": "generation-summary",
  "messages": [ ... ],
  "metadata": {
    "existing_trace_id": "<activity-run id>",
    "generation_name":   "generation-summary",
    "tags":              ["generation-summary"]
  }
}
```

- **C4-4**: The correlation key MUST be **`existing_trace_id`**, never `trace_id`. `trace_id` rewrites
  the trace's `name`, `input`, `output` and `tags` on every call, so trace-level fields would describe
  whichever call finished last. The integration's own source carries this warning.
- **C4-5**: When both fields are empty, the `metadata` key MUST be **absent** — not `null`, not `{}`.
- **C4-6**: The metadata object MUST carry only the correlation id, the generation name and the tag.
  No profile field, no job identifier, no user identifier.
- **C4-7**: `generation_name` MUST be set to the requested task key. Without it the record's `name`
  defaults to `litellm-{call_type}` and every task looks identical.

### 4.3 Value contract

- **C4-8**: The trace value MUST be the platform's existing activity-run identifier, so FR-010's
  cross-reference holds in both directions with no lookup table.
- **C4-9**: Every call of one run — including retries and escalations — MUST carry the same trace
  value (FR-009). Concurrent runs MUST carry different ones (FR-011).

### 4.4 Call sites

**Corrected**: these are not in `service.go`. Six are free functions in `rendercv_llm.go` taking
`(ctx, lc, model, ...)` with no `*activity.Recorder` in scope, so each needs a signature change.

| Site | File:line |
|---|---|
| `analyzeVacancy` | `generation/application/rendercv_llm.go:68` |
| `selectContent` | `rendercv_llm.go:299` |
| `writeSummary` | `rendercv_llm.go:352` |
| structure re-tailor | `rendercv_llm.go:372` |
| `expandContent` | `rendercv_llm.go:386` |
| `condenseContent` | `rendercv_llm.go:476` |
| cover letter | `generation/application/service.go` |
| match | `matching/application/service.go` |

- **C4-10**: All eight MUST be covered. A partially-correlated run produces a trace that looks
  complete and is not, which is worse than no trace.

---

## 5. Coverage statement (FR-013)

Binding, and to be reproduced in `specs/domains/llm-routing.md`.

| Path | Recorded? |
|---|---|
| Any chat completion through the gateway provider | **Yes**, including the `local` tier reached through the proxy |
| Chat completion when `GATEWAY_URL` is unset (local-first path) | **No** — no proxy in the path. Principle V working as designed |
| **Embeddings — always** | **No.** `Router.Embed` delegates straight to the local provider (`application/router.go:71-73`), and the gateway provider's `Embed` forwards to Ollama. Already binding as 030-FR-014 |
| ↳ affected call sites | `matching/application/service.go`, `profile/application/service.go` |

- **C5-1**: This table MUST be updated in the same change as any new AI call path. A call path added
  without a coverage decision is a defect against FR-013.
- **C5-2**: The embeddings row MUST NOT be written as conditional. The previous version said "when
  `EMBED_URL` points directly at Ollama", implying a configuration under which embeddings *are*
  covered. There is none.

---

## 6. Cost contract (FR-014)

- **C6-1**: The integration writes the proxy's computed response cost. A deployment absent from the
  proxy's cost map yields **no** cost rather than zero, so a genuinely free call and an unpriced one
  are indistinguishable in the record.
- **C6-2**: Either the `local` deployment is given an explicit zero cost in `gateway/config.yaml`, or
  FR-014 MUST be weakened to "a record exists" and its zero-cost claim dropped. Asserting "0, not
  null" without one of these is asserting something the data cannot support.

---

## 7. Retention contract (FR-008, FR-008a)

**New.** The previous version had no retention contract because it believed retention was a
configuration setting.

- **C7-1**: The platform MUST own a scheduled pruning job deleting collector records older than
  `EVAL_PRUNE_RETENTION_DAYS`, defaulting to 30. It runs on the existing `robfig/cron` scheduler.
- **C7-2**: Each run MUST record what it deleted; a failure MUST be visible, not silent (FR-008a).
- **C7-3**: `specs/domains/platform-operations.md` MUST NOT state a retention guarantee until this
  job exists. A binding rule with no mechanism is exactly the drift the specs lifecycle exists to
  prevent.
- **C7-4**: Retention MUST be proven by deletion — write a backdated record, run the job, confirm it
  is gone (SC-008) — not by reading configuration.
- **C7-5**: Deleting a job or resetting a profile in the platform does **not** propagate to the
  collector; those prompt bodies survive until the window expires. This MUST be documented rather
  than left for someone to discover.
