# Phase 0 Research: LiteLLM-Only Inference and Per-Scenario Model Assignment

**Feature**: 044-litellm-only-routing · **Date**: 2026-08-12

Every unknown in the plan's Technical Context is resolved below. Findings that came from reading the
tree are cited by `file:line`; findings that came from vendor documentation are cited by URL and
dated, because a vendor claim recorded without a date rots into a lie.

---

## R1 — Embeddings through the proxy: the request shape

**Decision.** `gateway.Provider.Embed` issues `POST {GATEWAY_URL}/embeddings` with
`{"model": "embed", "input": [text]}` and reads `data[0].embedding`. The proxy exposes an
OpenAI-compatible `/v1/embeddings` route, and the scenario name goes in `model` exactly as it does
for chat.

**Rationale.** It is the same contract the chat path already uses — a public group name resolved by
the proxy — so the "application names a scenario, the gateway picks the model" rule (FR-008) needs
no exception for embeddings. It also means the embedding call passes through the same success and
failure callbacks, which is what makes FR-004 true without any Go-side instrumentation.

**Alternatives considered.**
- *Keep a dedicated embedding client pointed at the proxy's Cohere passthrough.* Rejected: a second
  client is the thing this feature removes, and passthrough routes bypass the group indirection.
- *Send `input` as a bare string rather than a single-element array.* Both are accepted; the array
  form is chosen because it is the shape a future batch call grows into without changing the parse.

**Sources.** [Embeddings — /embeddings, LiteLLM proxy docs](https://docs.litellm.ai/docs/proxy/embedding),
[/embeddings supported providers](https://docs.litellm.ai/docs/embedding/supported_embedding) —
retrieved 2026-08-12.

## R2 — `input_type`, and the asymmetry trap

**Decision.** Declare `input_type: search_document` on the `embed` deployment in
`gateway/config.yaml`. Do **not** send it per request, and do not use `search_query` anywhere.

**Rationale.** Cohere's v3+ embedding models require `input_type`, and it changes the vector: the
same text embedded as `search_document` and as `search_query` produces different vectors by design,
because the pair is tuned for asymmetric retrieval. This platform's comparison is **symmetric** —
a profile vector against a job vector, both of which are documents (`profile.sql:43`,
`matching/application/service.go:104`). Embedding one side as a query would degrade similarity in a
way that looks like a threshold-tuning problem and is not. Pinning it in the deployment rather than
at the call site means the application cannot get it wrong per call, and a change to it is a
routing-config change like every other model decision.

**Known hazard.** LiteLLM has open reports of `input_type` not being forwarded on the **`azure_ai`**
Cohere route ([#11126](https://github.com/BerriAI/litellm/issues/11126),
[#11434](https://github.com/BerriAI/litellm/issues/11434)). This feature uses the direct `cohere/`
route, which is not the affected path — but the failure mode if it were is silent (vectors that are
merely worse), so the quickstart includes an explicit check: the same text embedded twice must
produce an identical vector, and a known-similar pair must score above a known-dissimilar pair.

**Sources.** [Cohere provider, LiteLLM](https://docs.litellm.ai/docs/providers/cohere),
[Cohere Embed v4 parameters](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-embed-v4.html)
— retrieved 2026-08-12.

## R3 — Dimensions: why 1024, fixed at the deployment

**Decision.** `cohere/embed-v4.0` with output dimensions **1024**, declared on the deployment.
`EMBED_DIMS` stays in application configuration as the width the application asserts, defaulting to
1024.

**Rationale.** embed-v4 supports 256/512/1024/1536. 1024 is the middle of that range: measurably
better than 768 (today's width) at a vector size Postgres handles without an index change, and it
avoids the 1536 storage cost for a corpus that has no vector index to speed up anyway. Fixing it on
the deployment rather than per request keeps FR-018's "fixed width" true by construction — a
per-request dimension is a way for one caller to write an incompatible vector into a shared column.

`EMBED_DIMS` is kept and becomes load-bearing rather than decorative: today nothing reads it
(`grep EmbedDims` → `config.go:38` only). After this change the embedding path asserts the returned
vector's length against it and fails the call on a mismatch. That is the guard that catches a
deployment retuned to 512 without a migration — which would otherwise surface as a Postgres error
per job, or worse, not at all.

**Alternatives considered.** *Keep 768 to avoid the migration.* Rejected: the migration is cheap here
(R5) and 768 is a Matryoshka truncation of a model that was not trained at that width by default,
so it buys a worse vector to avoid an `ALTER TABLE`.

**Confirmed 2026-08-12 (T004).** `COHERE_API_KEY` from the working `.env` reaches an account with
`embed-v4.0` access: a direct `POST https://api.cohere.com/v2/embed` call with
`{"model":"embed-v4.0","output_dimension":1024,"input_type":"search_document"}` returned HTTP 200
with a 1024-length float vector. Available `output_dimension` values per Cohere's docs: 256, 512,
1024, 1536 — 1024 is available and is the value declared on the `embed` deployment.

## R4 — Fallback chains apply to embeddings

**Decision.** The `embed` scenario gets a chain like every other scenario, satisfying FR-010 (≥2
tiers, ≥2 providers) with a second tier at the **same declared width**.

**Rationale.** LiteLLM's `litellm_settings.fallbacks` is one unified setting that applies to
embedding requests, not only chat completions — the documentation states the per-model fallback
parameters work "for embedding/image generation/etc. as well". So the embedding scenario is not a
special case of the chain invariant and should not be exempted from it.

**Constraint that is not negotiable.** Every tier of the `embed` chain MUST emit the declared width.
A fallback tier at a different dimension does not degrade the result, it corrupts the column: the
insert fails, or — if the widths happen to match across models — succeeds with a vector from a
different space, which is worse. The second tier is `openai/text-embedding-3-small` with
`dimensions: 1024`, which OpenAI supports natively, and the guardrail test asserts that every tier
of the `embed` chain declares the same width.

**Cost of that choice.** It introduces `OPENAI_API_KEY` as an optional gateway credential. If the
operator sets no OpenAI key, the tier fails auth at request time and the chain has one live tier —
the same degradation every other chain has with a missing key, and FR-017's ≥2-tier check is about
what is *declared*, not what is credentialed (030-FR-011 is unchanged).

**Sources.** [LiteLLM proxy reliability / fallbacks](https://docs.litellm.ai/docs/proxy/reliability)
— retrieved 2026-08-12.

## R5 — The migration is smaller than a dimension change usually is

**Decision.** One migration: retype both columns to `vector(1024)` discarding contents, null
`Job."embeddingHash"`, null `Profile."embedding"` and `"embedModel"`, add `Job."embedModel" text`.
No backfill worker.

**Rationale — three facts from the tree, in order of how much they save:**

1. **There is no vector index and no vector search over `Job`.** The only `<=>` operator in the
   schema is `internal/db/queries/profile.sql:43`, which scores the profile row against a vector
   passed in by the caller. `grep '<=>' internal/db/queries/` returns exactly that one line. So
   discarding stored job vectors costs no query plan and breaks no search.
2. **The caller passes a fresh vector, not a stored one.** `matching/application/service.go:89`
   embeds the job text this run and passes that value to `Similarity` at `:104`. The stored column
   (`UpdateJobEmbedding`, `job.sql:19`) is written, not read back for scoring.
3. **Lazy re-embedding already exists and is already the fast path.** `Job."embeddingHash"` was
   added by 019 precisely so unchanged content skips re-embedding (`job.sql:21-24`). Nulling the
   hash makes every job re-embed once, on its next match, through code that is already written and
   already tested. Likewise `Profile."embedding" IS NULL` makes `ProfileHasEmbedding` false, and
   `service.go:96-98` already repairs that inline.

**Alternatives considered.**
- *Dual-write 768 and 1024 during a transition.* Rejected in Complexity Tracking: two columns and a
  cross-space comparison rule, to protect vectors nothing reads.
- *A backfill worker over every job.* Rejected: it would re-embed jobs that will never be matched
  again, at per-call cost, to arrive at the state the lazy path reaches for free on demand. If a
  bulk warm-up is ever wanted, it is `make`-level tooling, not a migration.

**Down migration.** Retypes to `vector(768)` and nulls again. It cannot restore the old vectors and
must not pretend to.

## R6 — Provenance: which model produced a stored vector

**Decision.** `Profile."embedModel"` already exists (`profile.sql:31` writes it). Add the matching
`Job."embedModel" text`, written on every embedding update. FR-020's exclusion rule reads it: a row
whose `embedModel` differs from the configured current model is treated as unembedded.

**Rationale.** Without it, the next embedding change repeats this migration blind — "which rows are
stale" would be answerable only by "all of them". With it, a future change can scope its
invalidation. It also makes FR-020 implementable as a predicate rather than as a convention.

**Note on what the value is.** The application does not know which upstream model the proxy used; it
knows the scenario name. Storing `"embed"` would be useless. The value stored is the configured
current embedding identity — the deployment's declared model string, mirrored into application
config as `EMBED_MODEL_ID` (documentation of what the gateway is pinned to, asserted by the
guardrail test against `gateway/config.yaml` so the mirror cannot drift silently).

## R7 — Where startup validation goes

**Decision.** In `internal/config` alongside the existing validators, not in `main.go`. `Load`
returns an error naming the missing key; `cmd/server` already surfaces config errors fatally.

**Rationale.** The precedent is `queue.PoliciesFromConfig` / `validateLiveness`
(`internal/queue/policy.go:99-116`), which rejects bad durations and concurrency at startup with a
message naming the key. Startup validation of AI configuration belongs in the same place and the
same shape, so SC-002's "within 5 seconds, naming the setting" is satisfied by the pattern already
in use rather than a new one.

**Explicitly not validated: reachability.** Startup requires the gateway to be *configured*, never to
be *up*. The compose file deliberately has no `depends_on` for `litellm` (036-C3-2, asserted in
`internal/config/config_test.go`), and making startup wait on it would reintroduce exactly that
coupling. An unreachable gateway fails individual AI tasks, which is the edge case the spec states.

## R8 — Tool capability after the split

**Decision.** `salary` is the only tool-using scenario. Every tier of its chain carries
`model_info.supports_function_calling: true`; `outreach` and `recruiter` carry no such declaration
and are free to use tiers that lack it.

**Rationale.** This is the entire reason the split earns its keep. Today `default` is constrained to
tool-capable tiers because one of its three consumers runs a tool loop
(`salary/application/service.go:163`), so outreach and recruiter are paying a constraint they do not
use. After the split each chain is constrained by its own needs.

**Unchanged, and worth restating because it reads like a control and is not.** The declaration is
documentation a test reads. `drop_params: true` will still silently drop a `tools` array an upstream
refuses, and the only runtime detector remains the loop's required first round returning
`not_tool_capable` (`toolloop/loop.go:77-79`). The split does not fix that; it just stops it being
everybody's problem.

## R9 — What retiring the local provider class actually touches

**Decision.** `TaskPolicy.LocalConcurrency`/`HostedConcurrency` collapse to one `Concurrency`
(`AI_CONCURRENCY_CLOUD`, key name kept); `queue.Gate` stops consulting `ClassResolver` for
admission; `llm.ProviderClass` and `Router.ProviderClass()` are **kept**, returning `hosted`
unconditionally, because `QueueBacklogDto.providerClass` is a shared type the dashboard reads.

**Rationale.** `AI_CONCURRENCY_LOCAL` exists to keep a single-GPU Ollama from being swamped
(019-FR-004). With no local runtime it has nothing to protect, and `PoolSize()`'s `max(local,
hosted)` becomes `hosted`. Keeping the DTO field costs a documented constant; removing it costs a
breaking change to `packages/shared` and a dashboard edit, for no behaviour. Principle III favours
the constant.

**Renaming `AI_CONCURRENCY_CLOUD`** to something without a "cloud vs local" framing was considered
and rejected: it is a deployed environment variable, and renaming it breaks every existing `.env`
for a wording improvement.

## R10 — The pins are provisional; here is what confirms them

**Decision.** The spec's assignment table ships as written. FR-026 is satisfied by running the
existing live comparison for each quality-writing scenario and recording the artifact next to the
assignment.

**`-eval.models` takes task keys, not model ids.** The flag is declared as "comma-separated task
keys" (`internal/generation/application/eval_live_test.go:48`) and documented that way in
`gateway/config.yaml:89-90`:

```sh
go test -tags eval_live ./internal/generation/application/ \
  -run TestLiveComparison -eval.models generation-select,generation-summary
```

Passing an upstream model id instead — `anthropic/claude-sonnet-5` — sends a name the proxy has never
declared, which C1-3 requires it to reject with a 4xx. So comparing two *candidate models* means
declaring each as its own group in `gateway/config.yaml` first, then comparing the **groups**:

```yaml
  - model_name: generation-summary-candidate-a
    litellm_params: {model: openrouter/anthropic/claude-sonnet-5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
  - model_name: generation-summary-candidate-b
    litellm_params: {model: openrouter/anthropic/claude-haiku-4.5, reasoning_effort: low, api_key: os.environ/OPENROUTER_API_KEY}
```

```sh
-eval.models generation-summary-candidate-a,generation-summary-candidate-b
```

Candidate groups are temporary: they are removed once the winner is pinned, in the same change that
records the artifact. A candidate group left declared is an unrequested group, which FR-009's
guardrail rejects — the mechanism that stops this scaffolding becoming permanent.

**Rationale.** 038 already built exactly this: six deterministic corpus cases scored by the
platform's own grounding, structure, drift and completeness checks, with cost, latency and served
model per candidate, written to `internal/generation/application/evaldata/`. There is a precedent
artifact in the tree (`comparison-20260807-221152.json`) and a precedent for the result overturning
the guess — the `fast` summary option was named before it was measured, and the measurement found it
identical in quality at a seventh of the cost and *slower*, which is why its label reads "Economy"
today (`generation/domain/summary_option.go`).

**What this does not cover.** The harness scores resume generation. `outreach` has no corpus, so its
pin is confirmed by a smaller check: the same three drafts generated under both candidates and read
side by side, recorded as a dated note rather than a score. Claiming a numeric score where none was
computed would be worse than admitting the method.

## R11 — The smoke tool

**Decision.** `cmd/llmsmoke` is rebuilt against the gateway provider and takes a scenario name,
defaulting to `default`… which no longer exists — so it defaults to `match` and additionally gains
an `-embed` mode that exercises the new embedding path.

**Rationale.** It is the fastest way to answer "is the routing live and which tier answered", and
FR-005 forbids it constructing a provider of its own. Its embedding mode is also the natural home
for R2's asymmetry check.

---

## Resolved unknowns summary

| Unknown | Resolution |
|---|---|
| Embedding request shape through the proxy | `POST /embeddings`, `model: "embed"`, array input (R1) |
| Cohere `input_type` handling | Pinned `search_document` on the deployment; symmetric comparison (R2) |
| Vector width | 1024, fixed on the deployment, asserted by the app against `EMBED_DIMS` (R3) |
| Do chains cover embeddings | Yes, one unified `fallbacks` setting; second tier must match width (R4) |
| Migration strategy | Retype + null + lazy re-embed via the existing hash path; no backfill worker (R5) |
| Embedding provenance | `Job."embedModel"` added, mirroring `Profile."embedModel"` (R6) |
| Startup validation location | `internal/config`, config-error shape, no reachability check (R7) |
| Tool-capability constraints after the split | `salary` only (R8) |
| Local concurrency retirement | Collapse to one concurrency; keep the DTO field as a constant (R9) |
| How pins get confirmed | 038 live comparison; outreach by dated side-by-side note (R10) |
| Smoke tool | Rebuilt on the gateway path, gains `-embed` (R11) |

## Sources

- [Embeddings — /embeddings | LiteLLM proxy](https://docs.litellm.ai/docs/proxy/embedding)
- [/embeddings supported providers | LiteLLM](https://docs.litellm.ai/docs/embedding/supported_embedding)
- [Cohere | LiteLLM](https://docs.litellm.ai/docs/providers/cohere)
- [Proxy reliability & fallbacks | LiteLLM](https://docs.litellm.ai/docs/proxy/reliability)
- [BerriAI/litellm#11126 — `input_type` on Azure AI Cohere Embed v4](https://github.com/BerriAI/litellm/issues/11126)
- [BerriAI/litellm#11434 — `input_type` not supported on proxy for `azure_ai` cohere-embed-v-4](https://github.com/BerriAI/litellm/issues/11434)
- [Cohere Embed v4 model parameters | AWS Bedrock docs](https://docs.aws.amazon.com/bedrock/latest/userguide/model-parameters-embed-v4.html)
