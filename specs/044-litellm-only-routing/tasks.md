---

description: "Task list for 044-litellm-only-routing"
---

# Tasks: LiteLLM-Only Inference and Per-Scenario Model Assignment

**Input**: Design documents from `/specs/044-litellm-only-routing/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks are included and are **not optional here**. The spec requires them
(FR-017 makes the configuration guardrail build-failing; FR-002 and FR-020 are behavioural rules with
no other enforcement) and Constitution Principle IV binds every change in this repository.

**Revised 2026-08-12** after `/speckit.analyze`. Changes: the constitution amendment moved to the
front and is **done**; the eval invocation was wrong in three places and is fixed; five guardrail and
coverage gaps became tasks (T009, T010, T043, T049, plus the config split at T012). Task IDs were
renumbered; a reference to an old ID from before this revision will not resolve.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable — different files, no dependency on incomplete work
- **[Story]**: US1 (one path), US2 (per-scenario models), US3 (measured pins), US4 (embedding data)
- Paths are repo-relative. Go paths are under `apps/api/` unless stated.

## Terminology

One concept, four names in this repository: **task key** (code, `taskKey`; and
`specs/domains/llm-routing.md`), *scenario* (this feature's spec), *model group* (`gateway/config.yaml`),
*public group* (LiteLLM's own docs). **"Task key" is the canonical name** — it matches the code and the
existing domain record. T060 states the synonyms once in the domain doc; no code rename.

## A note on story independence, stated honestly

US1 and US2 are **not** cleanly separable in one place: `gateway/config.yaml`. Removing the local
tier (US1) and splitting `default` (US2) both rewrite the same chains, and FR-009 forbids leaving a
declared-but-unrequested group behind in the gap. So the config-file tasks of both stories land in
one commit (T017 with T030–T033).

Everything else in each story is genuinely independent, and US3 and US4 are fully independent of both.
Pretending otherwise would produce a task list that cannot be executed in the order it claims.

---

## Phase 1: Setup

**Purpose**: authorise the change, then capture what must not silently change.

- [x] **T001 Amend `.specify/memory/constitution.md` to 2.0.0** — Principle V rewritten to
  "Self-Hosted Control Plane, Single Inference Path", sync-impact header records the redefinition,
  its reasoning, what is knowingly given up, and the template re-check. **Done 2026-08-12.**
  *This is first, not last: it authorises every deletion below, so each intermediate commit is legal
  under the constitution in force when it is made.*
- [ ] T002 Record the SC-007 matching baseline: run the existing 50-job benchmark and save per-job similarity and match ordering to `specs/044-litellm-only-routing/baseline-matching-20260812.json`, noting the current embedding model and dimension in the file header
- [ ] T003 [P] Record the current per-scenario served models: run one job through match, generation, salary, outreach and recruiter, and save the `served_model` log lines to `specs/044-litellm-only-routing/baseline-routing-20260812.md` as the before-side of SC-004/SC-005
- [ ] T004 [P] Confirm `COHERE_API_KEY` in the working `.env` reaches an account with embed-v4.0 access, and record the confirmed model id and available output dimensions in `research.md` under R3

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: the contracts every story is checked against. All are written to **fail** first.

**⚠️ No story work begins until T005–T013 are complete.**

- [ ] T005 Rewrite the chain invariants in `internal/platform/llm/gateway_config_test.go`: delete the `terminalTier`/`local` assertion, add ≥2-tiers and ≥2-distinct-providers checks, and add a check that no group is named `default` and no tier is named `local` (contracts/gateway-config.md C6)
- [ ] T006 Extend the inline-fixture tests at `internal/platform/llm/gateway_config_test.go` (`TestGatewayInvariantsAcceptValidConfig`, `TestGatewayInvariantsRejectBrokenConfig`) to cover every new invariant — a guardrail that can only pass guards nothing, and this file already holds that line
- [ ] T007 Rename and extend `requestedGenerationGroups` in `internal/platform/llm/gateway_config_test.go` to the full task-key set of contracts/gateway-config.md C1-2, so a missing chain for any key fails the build
- [ ] T008 [P] Add the `embed`-chain invariants to `internal/platform/llm/gateway_config_test.go`: every tier declares `output_dimension`, all tiers agree, all declare `input_type: search_document`, and the width equals the `EMBED_DIMS` default read from `internal/config/defaults.go` (contracts/gateway-config.md C3-4, C5-1)
- [ ] T009 [P] **Narrow the tool-capability assertion** in `internal/platform/llm/gateway_config_test.go` from the `default` chain to `salary` (contracts/gateway-config.md C3-2). *Without this the assertion keeps passing while guarding nothing, because the chain it names is being deleted.*
- [ ] T010 [P] **Widen the reasoning-switch assertion** in `internal/platform/llm/gateway_config_test.go` from `generation-*` stage deployments to **every** `openrouter/*` tier (contracts/gateway-config.md C6). *This feature adds OpenRouter tiers to `outreach`, `salary` and `recruiter`; an unbounded thinking model there returns a 200 with empty content, which is what broke every resume run before 2026-08-07.*
- [ ] T011 [P] **Assert the `EMBED_MODEL_ID` mirror** in `internal/platform/llm/gateway_config_test.go`: the application's configured value equals the `embed` deployment's model string (contracts/gateway-config.md C6). *A drifted mirror mislabels the provenance of every stored vector while looking correct, defeating FR-021.*
- [ ] T012 Make `GATEWAY_URL` and `LITELLM_MASTER_KEY` required in `internal/config/config.go`, returning an error naming the missing key in the shape `internal/queue/policy.go:validateLiveness` uses — **and split the entry point** so non-AI binaries are exempt: `Load()` validates the AI surface, a sibling skips it, with the exempt callers named explicitly (`cmd/seed/main.go:42`, `internal/db/capacity_test.go:15`, the `live_test.go` files). *Without the split, `make seed` demands a gateway URL to load fixtures* (contracts/configuration.md K1-1, K1-4)
- [ ] T013 [P] Add `TestConfigRequiresGateway` to `internal/config/config_test.go` asserting the error names the key, that no reachability check is attempted (K1-2), and that the non-AI entry point loads cleanly with no gateway configured (K1-4)

**Checkpoint**: both guardrails fail against the current tree for the right reasons. Story work begins.

---

## Phase 3: User Story 1 — Every AI request goes through one path (Priority: P1) 🎯 MVP

**Goal**: one outbound inference path. The Ollama adapter is gone, embeddings go through the proxy,
and the application will not start without a gateway.

**Independent test**: start with `GATEWAY_URL=` → refuses to boot naming the key; start configured →
run one job through match and confirm the embedding call and the fit call both appear in the
observability record with a served model and a cost.

### Tests first

- [ ] T014 [P] [US1] Write `internal/platform/llm/infrastructure/gateway/embed_golden_test.go` asserting the exact request body key set is `{model, input}` and nothing else — a stray `dimensions` or `input_type` key must fail (contracts/embeddings.md E1-2, E6)
- [ ] T015 [P] [US1] Write `internal/platform/llm/infrastructure/gateway/embed_test.go` over `httptest`: happy path, empty `data` → `ErrInvalidResponse`, wrong-length vector → error, and each status in contracts/embeddings.md E3 → its sentinel
- [ ] T016 [P] [US1] Update `internal/platform/llm/application/router_test.go` — delete `TestRouterNilGatewayRoutesToLocalWithLocalModel`, add a test that `Embed` routes to the gateway under the `embed` key and that `ProviderClass()` is `hosted` unconditionally
- [ ] T017 [US1] Add an import-graph assertion in a new `internal/inferencepath_test.go`, in the style of `internal/toolfence_test.go`, that no package outside `internal/platform/llm/infrastructure/gateway` reaches an inference endpoint — the mechanical form of FR-001

### Routing catalogue

- [ ] T018 [US1] In `gateway/config.yaml`, declare the `embed` group (`cohere/embed-v4.0`, `output_dimension: 1024`, `input_type: search_document`, `api_key: os.environ/COHERE_API_KEY`) and its `embed-openai` fallback tier (`openai/text-embedding-3-small`, `dimensions: 1024`), and add `- embed: [embed-openai]` to `litellm_settings.fallbacks` — **land with T030–T033, see the independence note**

### Implementation

- [ ] T019 [US1] Implement `Embed` in `internal/platform/llm/infrastructure/gateway/gateway.go` as a real `POST {base}/embeddings` — request/response per contracts/embeddings.md E1/E2, error classification through `infrastructure/shared`, `ReportServedModel`/`ReportUsage` and the `logServed` line on every path, and the `EMBED_DIMS` length assertion (E2-2)
- [ ] T020 [US1] Send observability metadata on the embedding request from the existing `observabilityMetadata` helper in `internal/platform/llm/infrastructure/gateway/gateway.go` — `existing_trace_id` from context, `generation_name: "embed"`, `tags: ["embed"]` (E1-4)
- [ ] T021 [US1] Drop the `ollama domain.Provider` parameter from `gateway.New` in `internal/platform/llm/infrastructure/gateway/gateway.go` and remove the `ollama` field from `Provider`
- [ ] T022 [US1] Gut `internal/platform/llm/application/router.go`: remove `local` and `localModel`, `NewRouter(taskKey string, gateway domain.Provider)`, `resolve()` returns `(gateway, taskKey)`, `Embed` routes to the gateway under `embed`, `ProviderClass()` returns `hosted`
- [ ] T023 [US1] Delete `internal/platform/llm/infrastructure/ollama/` entirely, including `ollama_test.go` and `golden_request_test.go` — there is no Ollama wire format left to guard, so the golden is deleted rather than ported
- [ ] T024 [US1] Update the facade `internal/platform/llm/llm.go`: delete `New`, `NewOllama`, `OllamaProvider`; `NewProviders(cfg)` returns `(*GatewayProvider, error)`; keep `ProviderClass` and its constants
- [ ] T025 [US1] Remove the retired keys from `internal/config/config.go` and `internal/config/defaults.go` per contracts/configuration.md K3, delete `ModelOr` and `GenerationModelOr`, rename `EMBED_MODEL` to `EMBED_MODEL_ID`, and change the `EMBED_DIMS` default to `1024`
- [ ] T026 [US1] Rewire `composeLLM` in `cmd/server/compose.go`: no `ollamaProvider`, no `llmHandles.Ollama`, every `llm.NewRouter` call takes two arguments; `composeProfile` takes `llm.Provider` and is handed a router, with `EMBED_MODEL_ID` as its provenance value (contracts/embeddings.md E5-3)
- [ ] T027 [US1] Collapse concurrency in `internal/queue/policy.go`: `TaskPolicy.Concurrency` sourced from `AI_CONCURRENCY_CLOUD`, `PoolSize()` returns it, `validatePolicy` checks the one field; stop `internal/queue/middleware.go`'s `Gate` consulting `ClassResolver` for admission while leaving `ClassResolver` itself in place for the backlog DTO
- [ ] T028 [US1] Document `providerClass` as permanently `"hosted"` in `internal/dto/queue_backlog.go` and in `packages/shared/src/index.ts`'s `QueueBacklogDto`, without changing the shape (Principle III — see plan.md Complexity Tracking)
- [ ] T029 [US1] Rebuild `cmd/llmsmoke/main.go` on the gateway provider: `-task <key>` defaulting to `match`, plus `-embed <text>` and `-embed-check` implementing quickstart step 5's asymmetry check (research.md R11)
- [ ] T030 [US1] Update every hand-written fake that constructed an Ollama provider — `internal/profile/resume_service_test.go` (`stubEmbedder`), `internal/matching/application/*_test.go`, `internal/generation/application/summary_option_routing_test.go` and any other `_test.go` that calls `llm.NewOllama` or the 4-argument `NewRouter`

**Checkpoint**: US1 independently testable. `go test ./...` green, `GATEWAY_URL=` refuses to boot,
smoke embeds through the proxy, `make seed` still works without a gateway.

---

## Phase 4: User Story 2 — Each kind of AI work runs on a model chosen for it (Priority: P1)

**Goal**: `default` and `local` gone; `salary`, `outreach`, `recruiter` independently routed; every
chain re-cut per the spec's assignment table.

**Independent test**: run one of each kind of work and group the record by task key — each appears
under its own name; changing one key's model in the config leaves the others' served model unchanged.

### Routing catalogue (one commit with T018)

- [ ] T031 [US2] Rewrite `gateway/config.yaml`'s model list: add `salary`, `outreach`, `recruiter` groups with their tiers per the spec assignment table; carry `model_info.supports_function_calling: true` on every `salary` tier only (contracts/gateway-config.md C3-2)
- [ ] T032 [US2] Delete the `default` and `local` groups and every `-*` tier belonging to them from `gateway/config.yaml`, and remove `local` from every fallback list
- [ ] T033 [US2] Re-cut every chain in `litellm_settings.fallbacks` to contracts/gateway-config.md C2-4, including the lead-model changes: `generation` and `outreach` lead with `openrouter/anthropic/claude-sonnet-5` (quality-writing, FR-011)
- [ ] T034 [US2] Update the per-group comment blocks in `gateway/config.yaml` to record each key's **class and why** (FR-012) — including, for `generation-analyze` and `generation-select`, the 2026-08-07 measurement that justifies a mechanical stage leading with a paid tier (FR-011's second clause) — and correct the file's header comment, which currently describes a free-tier-first rule and an Ollama terminal tier that no longer exist

### Application wiring

- [ ] T035 [US2] Replace `DefaultRouter` with `SalaryRouter`, `OutreachRouter` and `RecruiterRouter` in the `llmHandles` struct and `composeLLM` in `cmd/server/compose.go`, and hand each to its own consumer at the existing wiring sites (salary, outreach, recruiter)
- [ ] T036 [US2] Change the salary policy's `LLMTaskKey` from `"default"` to `"salary"` in `internal/queue/policy.go`
- [ ] T037 [P] [US2] Remove the self-hosted branch from `summaryOptionRouters` in `cmd/server/compose.go` — every option now has a non-empty task key
- [ ] T038 [P] [US2] Delete the `local` option and the `SelfHosted()` method from `internal/generation/domain/summary_option.go`, and update the doc comment that explains the self-hosted routing
- [ ] T039 [P] [US2] Remove the self-hosted option from the dashboard summary picker in `apps/dashboard/src/features/generate/` and `apps/dashboard/src/features/tailor/`, and from any shared option type in `packages/shared/src/index.ts`

### Tests

- [ ] T040 [P] [US2] Add a test to `internal/generation/application/summary_option_routing_test.go` pinning that a persisted `"local"` option id resolves to the default through `LookupSummaryOption`'s miss path and does not fail a run (data-model.md §5)
- [ ] T041 [P] [US2] Update `internal/summarycatalogue_test.go` so every catalogue option's task key must exist and be chained — with no self-hosted exemption left
- [ ] T042 [US2] Add a routing test asserting the three former-`default` consumers request three distinct task keys, in `internal/salary/application/service_test.go`, `internal/outreach/application/service_test.go` and `internal/recruiter/application/posting_test.go`

**Checkpoint**: US2 independently testable. `default` appears nowhere; three keys route separately.

---

## Phase 5: User Story 4 — Existing job data survives the embedding change (Priority: P2)

**Goal**: the vector columns move to 1024, stale vectors are discarded rather than compared, and
re-embedding happens through the path that already exists.

**Independent test**: on a database populated before the change, migrate, run one match, and confirm
the row carries a current-model embedding; confirm no row with a stale `embedModel` is ever scored.

- [ ] T043 [US4] Write `internal/db/migrations/00044_embedding_dims_1024.sql` exactly as data-model.md §1 states, including the down migration that nulls rather than pretending to restore 768-dimension vectors
- [ ] T044 [US4] Add `"embedModel"` to `UpdateJobEmbedding` and `UpdateJobEmbeddingWithHash` in `internal/db/queries/job.sql`, add the `ClearStaleJobEmbeddings` query, and regenerate `internal/db/sqlcgen/` with sqlc — never hand-edit the generated files
- [X] T045 [US4] Write the embedding provenance on every job embedding write in `internal/matching/application/service.go`, passing the configured `EMBED_MODEL_ID` (contracts/embeddings.md E4-1)
- [X] T046 [US4] Implement the FR-020 exclusion rule as a predicate: a row whose `embedModel` differs from the configured current value is treated as unembedded in `internal/matching/application/service.go`, never compared against a current vector
- [ ] T047 [P] [US4] Add a migration test asserting both columns report 1024 dimensions after `00044` and that `Job."embeddingHash"` and `Profile."embedding"` are null, in the Docker-backed integration suite
- [ ] T048 [P] [US4] Add an integration test for the real embedding round-trip: identical text yields identical vectors, and a related pair outscores an unrelated pair (research.md R2's silent-failure check)
- [X] T049 [P] [US4] **Unit-test the FR-020 exclusion predicate** in `internal/matching/application/`: a row with a stale `embedModel` is re-embedded and never scored against a current vector. *T047 tests the migration and T048 the round-trip; neither covers the one rule standing between two vector spaces in one column, and it fails silently — scores get worse, nothing errors.*
- [ ] T050 [US4] Re-run the T002 benchmark after migration and re-embedding, and record the before/after ordering comparison in `specs/044-litellm-only-routing/baseline-matching-20260812.json` with any change explained (SC-007)

**Checkpoint**: US4 independently testable against a pre-populated database.

---

## Phase 6: User Story 3 — Model assignments are confirmed by measurement (Priority: P2)

**Goal**: no quality-writing pin ships as a guess.

**Independent test**: every quality-writing key has a dated artifact naming its candidates, cases,
scores, costs and latencies, and the config either matches the winner or records why not.

**`-eval.models` takes task keys, not model ids** (`eval_live_test.go:48`, `gateway/config.yaml:89-90`).
Comparing two candidate *models* therefore means declaring each as a temporary group first — see
research.md R10 for the shape.

- [ ] T051 [US3] Declare the candidate groups for `generation-summary`, `generation-select-premium` and `generation` in `gateway/config.yaml` as temporary `*-candidate-a`/`*-candidate-b` deployments, and `docker compose restart litellm`
- [ ] T052 [US3] Run the live comparison for `generation-summary` — `go test -tags eval_live ./internal/generation/application/ -run TestLiveComparison -eval.models generation-summary-candidate-a,generation-summary-candidate-b` — and commit the artifact under `internal/generation/application/evaldata/`
- [ ] T053 [P] [US3] Run the same comparison for `generation-select-premium` and commit its artifact
- [ ] T054 [P] [US3] Run the same comparison for `generation` (cover letter) and commit its artifact — this pin changed class in this feature, so it is the one most in need of evidence
- [ ] T055 [US3] For `outreach`, which has no corpus: generate the same three drafts under both candidates, and record a dated side-by-side note in `specs/044-litellm-only-routing/outreach-comparison-20260812.md` explicitly labelled a judgement, not a score (research.md R10)
- [ ] T056 [US3] Reconcile `gateway/config.yaml` with the results: change each pin to the winner or record the reason for keeping it in the comment beside that group, **and delete every candidate group** — left declared, they are unrequested groups and T007's guardrail fails the build, which is the mechanism that stops this scaffolding becoming permanent

**Checkpoint**: SC-009 satisfiable — every quality-writing pin has a dated artifact, and no candidate
scaffolding survives.

---

## Phase 7: Polish & cross-cutting

- [ ] T057 Add the `litellm` service to `docker-compose.prod.yml` — same image, same read-only `gateway/config.yaml` mount, same Python healthcheck, no `depends_on` (contracts/configuration.md K5-1)
- [ ] T058 Delete the `# --- local model pinning ---` block from `docker-compose.prod.yml`, and rewrite the `# --- LLM access ---` comment, which currently describes OLLAMA_KEY as "the local-first path (Principle V), which the app must be able to reach with no gateway at all" — a description of a guarantee that no longer exists (K5-2)
- [ ] T059 [P] Remove `OLLAMA_URL` and `OLLAMA_KEY` from the `litellm` service environment in `docker-compose.yml` and add `OPENAI_API_KEY: ${OPENAI_API_KEY:-}` (K4)
- [ ] T060 Rewrite the LLM block of `.env.example` around the new surface, including the FR-024 statement that prompt content is sent to third-party providers on every request with no configuration under which it is not — the file currently opens "LLM — Ollama only" (K6)
- [ ] T061 Amend `specs/domains/llm-routing.md` in place: mark superseded where it is stated — the local terminal tier (§2, 030-FR-008), local-first fallback (§2, 030-FR-009), the `default` key (§2.1), the free-tier-first blanket rule (§2, 030-FR-006), and the embeddings rows in §3 and §7.3 that read "never the gateway" and "never recorded"; record the constitution's move to 2.0.0; and state the task key / scenario / model group synonyms once (see Terminology above)
- [ ] T062 [P] Update `docs/docs/ai/llm-abstraction.md` and `docs/docs/ai/overview.md` — two providers become one, and the embedding path is no longer an exception
- [ ] T063 [P] Correct any local-first description in `AGENTS.md` and `README.md`, including the self-hosted claim if it promises inference without third-party calls
- [ ] T064 Add the 044 row to the feature registry in `specs/README.md`
- [ ] T065 Run `make test-lint` and `make test-integration`, then walk quickstart.md steps 1–10 end to end and fix what they surface

---

## Dependencies

```
Setup (T001 done, T002–T004)
  └─> Foundational (T005–T013)   ← guardrails failing, for the right reasons
        ├─> US1 (T014–T030)  ─┐
        ├─> US2 (T031–T042)  ─┤ T018 and T031–T034 edit gateway/config.yaml — one commit
        ├─> US4 (T043–T050)   │ independent except T045/T046 needing T026's wiring
        └─> US3 (T051–T056)   │ needs US2's chains live to measure the real pins
                              └─> Polish (T057–T065)
```

- **T001 is first and is done.** The constitution authorises the deletions; amending it afterwards
  would leave every intermediate commit in violation of a live MUST.
- **T005–T013 block everything.** They are the contracts the rest is checked against.
- **US1 ↔ US2**: coupled only in `gateway/config.yaml`. All Go work in each is independent.
- **US4** depends on US1's `Embed` (T019) and wiring (T026) to write provenance; its migration (T043)
  and query work (T044) can start immediately after Foundational.
- **US3** depends on US2's chains being live — measuring a pin that is not deployed measures nothing.
- **T065 is last** and is not a formality: quickstart's steps are written to fail informatively.

## Parallel opportunities

- **Setup**: T003, T004 together.
- **Foundational**: T008–T011 and T013 alongside T005–T007 and T012.
- **US1 tests**: T014, T015, T016 together, before T019–T022.
- **US2 dashboard/catalogue**: T037, T038, T039 together; then T040, T041 together.
- **US4**: T047, T048, T049 together once T043–T046 land.
- **US3**: T053 and T054 together after T052 establishes the artifact format.
- **Polish**: T059, T062, T063 together.

## Parallel example: User Story 1

```bash
Task: "Write gateway/embed_golden_test.go asserting the request key set is {model, input}"
Task: "Write gateway/embed_test.go covering empty data, wrong-length vector, each error status"
Task: "Update application/router_test.go: Embed routes under `embed`, ProviderClass is hosted"
```

## Implementation strategy

### MVP (US1 only)

Setup → Foundational → US1, with the `gateway/config.yaml` work of T018 landing together with
T031–T034 because FR-009 forbids the intermediate state. That makes the practical MVP **US1 + US2's
config tasks**; US2's application wiring can follow.

Stop and validate: `GATEWAY_URL=` refuses to boot; a match produces an embedding call and a fit call,
both in the observability record. That is SC-001 and SC-002 — the two criteria that define the
feature.

### Incremental delivery

1. Setup + Foundational → guardrails failing for the right reasons
2. US1 + config → one inference path → **validate SC-001, SC-002, SC-008**
3. US2 wiring → per-key routing → **validate SC-003, SC-004, SC-005**
4. US4 → embedding migration → **validate SC-007**
5. US3 → measured pins → **validate SC-009**
6. Polish → deployment, documentation → **validate SC-006 over a normal day**

### Notes

- SC-006 (95% served by each key's assigned lead tier over a normal day) cannot be validated before
  the feature is deployed and running for a day. It is a post-merge observation, not a merge gate —
  recorded here so nobody reports it as satisfied on the strength of a single smoke run.
- Commit after each task or logical group; `gateway/config.yaml` is the one file where that rule
  bends, for the reason stated at the top.
