# Implementation Plan: LiteLLM-Only Inference and Per-Scenario Model Assignment

**Branch**: `044-litellm-only-routing` | **Date**: 2026-08-12 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/044-litellm-only-routing/spec.md`

## Summary

Delete the application's second inference path and give every kind of AI work its own name.

Three moves, in dependency order:

1. **One path.** `internal/platform/llm/infrastructure/ollama` is removed. `Router` loses its
   `local` provider and its `localModel`. `GATEWAY_URL` and `LITELLM_MASTER_KEY` become required
   configuration, validated at startup. `Embed` stops being a special case and becomes an
   OpenAI-compatible `POST /embeddings` to the same proxy under the scenario key `embed`.
2. **One name per scenario.** `default` splits into `salary`, `outreach`, `recruiter`. `local`
   disappears as a tier. The chain invariant changes from *"terminates at `local`"* to *"≥2 tiers
   over ≥2 providers"*, and `gateway_config_test.go` changes with it.
3. **Embedding migration.** `vector(768)` → `vector(1024)` for `Job` and `Profile`, existing vectors
   dropped to NULL, `Job."embeddingHash"` cleared, `Job."embedModel"` added. Re-embedding is **lazy
   and already built**: matching embeds the job on every run and the hash column is what skips it, so
   nulling the hash re-embeds each job the next time it is matched, and nulling
   `Profile."embedding"` makes `HasEmbedding` false, which the matcher already repairs
   (`matching/application/service.go:96-98`).

The third move is smaller than it reads, and the reason is worth stating: there is no vector search
over the `Job` table. The only `<=>` in the schema is `profile.sql:43`, profile-against-a-vector
supplied by the caller, and that caller (`service.go:104`) passes the embedding it computed **this
run**, not one read back from the row. Stored job vectors are provenance, not an index.

## Technical Context

**Language/Version**: Go 1.25 (`apps/api`), TypeScript/React (`apps/dashboard`, `packages/shared`)

**Primary Dependencies**: no new Go module. LiteLLM proxy (`ghcr.io/berriai/litellm:main-stable`),
pgvector, asynq/Redis, Postgres. `pgvector-go` already in use.

**Storage**: Postgres + pgvector. Two `vector(768)` columns migrate to `vector(1024)`; one new
`text` column for embedding provenance.

**Testing**: `go test ./...`; the config-as-contract test `internal/platform/llm/gateway_config_test.go`;
golden wire-request tests in `infrastructure/gateway`; `make test-integration` for the Docker-backed
paths; the 038 eval harness (`-tags eval_live`) for FR-026.

**Target Platform**: Linux, Docker Compose (dev + prod)

**Project Type**: Go backend + React dashboard monorepo

**Performance Goals**: no regression in match throughput. Embeddings gain a network hop to a hosted
provider; the ingest/match path already tolerates hosted latency and `AI_CONCURRENCY_CLOUD=3` applies
unchanged.

**Constraints**: model changes stay a YAML edit plus a routing-service restart (FR-016); no
credential may reach the application container except `LITELLM_MASTER_KEY`; the gateway's timeout,
retry and cooldown arithmetic is untouched.

**Scale/Scope**: ~14 scenario keys, one Go platform package, one composition root, one migration,
one dashboard option removal, three documentation surfaces.

## Constitution Check

*GATE: checked before Phase 0, re-checked after Phase 1.*

| Principle | Verdict |
|---|---|
| **I. No Auto-Apply** | Unaffected. No submission path is touched. |
| **II. Grounded Generation** | Unaffected in rule, touched in fact: the models serving generation change. Grounding is enforced by prompt, schema and post-processing, all unchanged — and FR-026's eval run over the existing corpus is what proves grounding scores did not regress under the new pins. |
| **III. Typed Contracts** | Respected. `QueueBacklogDto.providerClass` becomes permanently `"hosted"`; the field is kept and its meaning documented rather than removed, because dropping it is a breaking DTO change for a value that is now constant. Any dashboard-visible change regenerates through `packages/shared`. |
| **IV. Test Discipline** | Respected and extended. The chain invariant test is rewritten, not deleted. New: startup-validation test, embedding-path golden request, a test asserting no Go file imports an inference client other than the gateway adapter. |
| **V. Self-Hosted Control Plane, Single Inference Path** | **Amended, and the amendment landed first.** The constitution went to **2.0.0** on 2026-08-12: Principle V no longer promises local-first inference, and now requires exactly what this feature builds — one path through the self-hosted gateway, credentials out of the application, availability protected by ≥2 distinct providers per chain. Under 2.0.0 this feature is compliant rather than deviating. What was given up is recorded in the constitution's own sync header and in Complexity Tracking below. |

**Post-Phase-1 re-check**: unchanged. The design introduces no new violation beyond the amended
Principle V; the embedding move is a consequence of it, not a second deviation.

## Project Structure

### Documentation (this feature)

```text
specs/044-litellm-only-routing/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── gateway-config.md    # routing contract: scenario names, chain invariants
│   ├── embeddings.md        # the /embeddings request/response contract
│   └── configuration.md     # required/removed configuration surface
├── checklists/
│   └── requirements.md
└── tasks.md             # /speckit.tasks output — not created here
```

### Source Code (repository root)

```text
apps/api/
├── cmd/
│   ├── server/
│   │   ├── compose.go              # composeLLM: drop ollama arg, add salary/outreach/recruiter routers
│   │   └── main.go                 # startup validation entry
│   └── llmsmoke/main.go            # rebuilt on the gateway path
├── internal/
│   ├── config/
│   │   ├── config.go               # required GATEWAY_URL/LITELLM_MASTER_KEY; drop OLLAMA_*/LLM_MODEL_*
│   │   └── defaults.go             # defaults + secret allowlist
│   ├── db/
│   │   ├── migrations/00044_embedding_dims_1024.sql
│   │   └── queries/job.sql         # embedModel on the embedding writes
│   ├── platform/llm/
│   │   ├── llm.go                  # facade: NewProviders → NewGateway only
│   │   ├── application/router.go   # gateway-only Router
│   │   ├── infrastructure/gateway/ # gains embeddings
│   │   ├── infrastructure/ollama/  # DELETED
│   │   └── gateway_config_test.go  # new chain invariants
│   ├── queue/policy.go             # single hosted concurrency
│   ├── generation/domain/summary_option.go  # remove the self-hosted option
│   ├── matching/application/service.go      # embed via router
│   └── profile/application/service.go       # embed via router
├── gateway/config.yaml             # scenario catalogue (repo root, not apps/api)
├── docker-compose.yml              # litellm env: drop OLLAMA_*, add embedding provider key
├── docker-compose.prod.yml         # gains the litellm service
└── .env.example                    # configuration surface + the privacy statement
```

**Structure Decision**: existing monorepo layout, unchanged. This feature deletes one infrastructure
adapter and edits its composition root; it introduces no new package and no new module. The routing
catalogue stays in `gateway/config.yaml` at the repository root, which the Go binary never reads and
`gateway_config_test.go` parses.

## Implementation phases

### Phase A — routing catalogue (no Go change)

`gateway/config.yaml` gains `salary`, `outreach`, `recruiter`, `embed`; loses `default` and `local`;
every chain is re-cut per the spec's assignment table. `gateway_config_test.go` is rewritten first —
the new invariants (≥2 tiers, ≥2 providers, no `local`, embedding group excluded from the chat
invariants) fail against the current file, then pass as it is edited.

Independently shippable: the application still routes `default` until Phase C, so Phase A must keep
`default` declared until Phase C lands, or land in the same commit. **Chosen: same commit** — a
declared-but-unused key is exactly the drift FR-009 exists to prevent.

### Phase B — one inference path

1. `gateway.Provider` gains `Embed` as a real implementation: `POST {base}/embeddings`, `model:
   "embed"`, `input: [text]`, returning `data[0].embedding`; errors classified through
   `infrastructure/shared` like every other call; served-model logging and usage capture on the same
   terms as `send()`.
2. `gateway.New` loses its `ollama domain.Provider` parameter.
3. `Router` loses `local`/`localModel`; `resolve()` returns `(gateway, taskKey)`; `Embed` routes to
   the gateway under the `embed` key; `ProviderClass()` returns `hosted` unconditionally.
4. `llm.New` is deleted; `llm.NewProviders` returns `(*GatewayProvider, error)`.
5. `infrastructure/ollama` is deleted with its tests. Its golden-request test is deleted, not
   ported — there is no Ollama wire format left to guard.
6. `config`: `GATEWAY_URL` and `LITELLM_MASTER_KEY` become required with a startup error naming the
   missing key; `OLLAMA_URL`, `OLLAMA_KEY`, `OLLAMA_KEEP_ALIVE`, `EMBED_URL`, `LLM_MODEL`,
   `LLM_MODEL_MATCH`, `LLM_MODEL_GENERATION*`, `LLM_MODEL_REPHRASE`, `LLM_MODEL_GHOST`,
   `AI_CONCURRENCY_LOCAL` are removed, along with `ModelOr` and `GenerationModelOr`. `EMBED_MODEL`
   is removed (the model is the gateway's business); `EMBED_DIMS` is **kept** and becomes load-bearing
   — see data-model.md.
7. `queue/policy.go`: `LocalConcurrency` collapses into one `Concurrency` sourced from
   `AI_CONCURRENCY_CLOUD`; `Gate` stops consulting `ClassResolver` for admission.
   `QueueBacklogDto.providerClass` stays and reports `"hosted"`.

### Phase C — scenario split

`composeLLM` replaces `DefaultRouter` with `SalaryRouter`, `OutreachRouter`, `RecruiterRouter`;
`queue/policy.go`'s salary entry changes `LLMTaskKey` from `default` to `salary`; the three consuming
services take their own router. `summaryOptionRouters` drops its self-hosted branch; the `local`
summary option is removed from the catalogue and from the dashboard, and stored `"local"` choices
resolve to the default through the existing `LookupSummaryOption` miss path — which already returns
the default rather than failing a run.

### Phase D — embedding migration

Migration `00044`: `ALTER TABLE "Job" ALTER COLUMN "embedding" TYPE vector(1024) USING NULL`, same
for `"Profile"`, `UPDATE "Job" SET "embeddingHash" = NULL`, `UPDATE "Profile" SET "embedding" =
NULL, "embedModel" = NULL`, `ALTER TABLE "Job" ADD COLUMN "embedModel" text`. Down reverses to 768
and nulls again — a down migration that pretended to restore 768-dimension vectors would be lying.

Re-embedding is lazy through the paths that already exist; no backfill worker is written. What is
added is the **explicit exclusion rule** (FR-020): a row whose `embedModel` is not the current one is
treated as unembedded rather than compared.

### Phase E — deployment, docs, constitution

`docker-compose.prod.yml` gains the `litellm` service mounting the same `gateway/config.yaml`; the
app service loses every `OLLAMA_*`/`LLM_MODEL_*` variable and gains required `GATEWAY_URL`.
`.env.example` is rewritten around the new surface and carries the plain statement required by
FR-024. `specs/domains/llm-routing.md` is amended in place — superseded rules marked where they are
stated, including the constitution's move to **2.0.0**, which already landed in Phase 1 (T001)
rather than here: the amendment authorises the deletions, so it cannot trail them.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| **Constitution Principle V — local-first fallback removed** (constitution → 2.0.0, **landed 2026-08-12, ahead of implementation**) | The feature's stated purpose is one inference path. A local-first fallback *is* a second path, with its own failover, its own cost accounting and no observability. Keeping it makes FR-001 and SC-001 unachievable by construction. | *Keep the local path as break-glass*: rejected by the user in clarification, and it preserves precisely the untested second code path this feature removes. *Route the local model through the proxy as a terminal tier*: was the previous design; it satisfies "no direct call" but not the scenario-model work, and the user chose to remove Ollama outright. |
| **Removing Ollama entirely, including embeddings** | Follows from the above: an embedding path to a self-hosted runtime is a direct AI call, and it is the one path with no observability coverage today. | *Keep Ollama for embeddings only*: rejected in clarification. It leaves a self-hosted runtime as a hard requirement while delivering none of the local-first benefit that justified it. |
| **A dimension migration that discards stored vectors** | Vectors from two models are not comparable; keeping them would be worse than deleting them. | *Dual-write both widths during a transition*: two columns, two providers and a comparison rule for the overlap — real complexity for a table with no vector index and lazy re-embedding already built in. |
| **`QueueBacklogDto.providerClass` retained as a constant** | Removing a field is a breaking change to a shared type for no behavioural gain. | *Delete the field*: forces a dashboard change and a `packages/shared` version bump to express "this is always hosted now", which a documented constant already says. |

## Risks

- **Cost.** Every embedding becomes a paid or free-tier API call, on a path that runs for every
  ingested job. Cohere's trial tier is rate-limited; a large ingest run will feel it. Mitigation:
  `Job."embeddingHash"` already skips re-embedding unchanged content, and it survives this change.
- **Privacy.** After this feature there is no configuration in which profile and posting text stays
  inside the deployment. FR-024 makes that a documented statement rather than a discovery.
- **Availability.** The platform now has a hard dependency on at least one hosted provider being
  reachable. The ≥2-provider chain invariant (FR-010) is the only thing standing between a single
  provider outage and a total AI outage — which is why it is a build-failing guardrail, not a
  convention.
- **`local` in stored data.** The `"local"` summary-option id may be persisted on existing rows and
  in the summary-model setting. The existing lookup-miss path returns the default, so this degrades
  correctly; a test pins that behaviour rather than leaving it to be rediscovered.
