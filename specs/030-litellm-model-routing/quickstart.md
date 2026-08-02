# Quickstart: Validating Gateway-Owned Model Routing

Validation guide only — implementation lives in `tasks.md`. Details of the config and the deletion checklist are in [contracts/](./contracts/).

## Prerequisites

- `.env` with `GATEWAY_URL=http://litellm:4000`, `LITELLM_MASTER_KEY` set, and the provider keys `CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY` (any may be blank for the failover scenarios).
- Docker Compose stack: `make up` (Postgres, Redis, Ollama, litellm, api, dashboard).
- Migrations applied through `00033_drop_llm_task_setting.sql`.

## Scenario 1 — The settings surface is gone (US1, FR-001/FR-002)

```bash
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/v1/settings/llm         # expect 404
curl -s -o /dev/null -w '%{http_code}\n' localhost:8080/api/v1/settings/llm/models  # expect 404
```

Open the dashboard Settings page: no provider/model selects, no Cerebras credential banner. "AI features" and "Danger zone" tiles render and still work.

```bash
psql "$DATABASE_URL" -c '\d "LlmTaskSetting"'   # expect: did not find any relation
```

## Scenario 2 — Free tier serves the happy path (US2, FR-006)

Trigger one job match and one generation from the dashboard (or enqueue via the API), then:

```bash
docker compose logs api | rg 'served_model'
```

Expect `served_model` naming the first free-tier model in the chain (Cerebras by default), and `task=match` / `task=generation` for the respective calls. Both jobs complete normally.

## Scenario 3 — Forced failover (US2, FR-007)

Blank the first tier's key and restart only the proxy:

```bash
CEREBRAS_API_KEY= docker compose up -d --force-recreate litellm
```

Re-run a match. Expect: the task still succeeds, and `served_model` now names the next tier (Groq). No user-visible error, no manual intervention. Repeat by blanking each free-tier key in turn — the final free-tier removal must land on the OpenRouter model.

## Scenario 4 — All hosted tiers down (FR-008, SC-005)

Blank every hosted key (`CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY`) and recreate the proxy. Re-run a match: it must complete, with `served_model` naming the Ollama deployment.

## Scenario 5 — Gateway absent entirely (FR-009)

```bash
GATEWAY_URL= docker compose up -d --force-recreate api
```

Re-run a match and a generation: both succeed against Ollama directly. `GET /api/v1/activity/queues` still reports an admission-gate class for every LLM queue (`local` in this configuration, `hosted` when the gateway is wired).

## Scenario 6 — Routing change with no redeploy (US3, SC-003)

Edit one task's primary model in `gateway/config.yaml`, then:

```bash
docker compose restart litellm
```

Re-run that task and confirm `served_model` reflects the new model, that other tasks are unchanged, and that no application container was rebuilt or restarted.

## Scenario 7 — Embeddings untouched (FR-014)

Re-import or re-embed a profile and confirm embedding calls still go to Ollama (`EMBED_URL`), with no gateway request logged for them.

## Test suites

```bash
make test          # go test + vitest
make test-lint     # cross-app gate, required before done
```

Expect the deleted suites (`llmsettings` service/http tests, `LlmSettingsCard.test.tsx`, Cerebras adapter tests) to be gone, and new coverage per [contracts/task-router.md](./contracts/task-router.md) C5 to pass.
