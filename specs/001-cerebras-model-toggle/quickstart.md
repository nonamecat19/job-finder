# Quickstart / Validation: Cerebras Free-Tier Model Toggle

Validates the feature end-to-end. See [data-model.md](./data-model.md) and
[contracts/llm-settings.md](./contracts/llm-settings.md) for shapes.

## Prerequisites

- Stack up via Docker Compose (`make up`) with Postgres, Redis, Ollama reachable.
- Migration `00018_llm_task_setting` applied (goose runs on API start / `make migrate`).
- A Cerebras free-tier API key for the live paths. Provide it via env:
  `CEREBRAS_API_KEY=<key>` (optionally `CEREBRAS_BASE_URL`). Leave unset to validate the
  local-first / missing-credential behavior.

## Build & regen gates

```bash
pnpm --filter @job-finder/shared build      # shared types available first
make generate        # or: cd apps/api && sqlc generate && tygo generate
make test-lint       # go test + vitest + lint across apps
```
Expected: sqlc/tygo produce no drift (CI gate), all suites pass.

## Scenario A — Local-first default (no key) — Story 1 / Principle V

1. Start with `CEREBRAS_API_KEY` unset.
2. `GET /v1/settings/llm` → `credentialConfigured:false`, every task `provider:"ollama"`.
3. Run any AI task (e.g. score a job). Expected: runs on Ollama; no external calls.

## Scenario B — Switch all to Cerebras — Story 1 (P1, MVP)

1. Set `CEREBRAS_API_KEY`, restart API.
2. Dashboard → Settings → AI models → "Switch all to Cerebras", save.
   (or `PUT /v1/settings/llm` with all five tasks `provider:"cerebras"`).
3. `GET /v1/settings/llm` → all tasks `cerebras`, `credentialConfigured:true`.
4. Trigger matching + generation. Expected: both complete via Cerebras; task status/logs show
   a Cerebras model; no restart was needed (FR-005). **SC-001** (< 1 min), **SC-002**.
5. Reload dashboard and `make down && make up`. Expected: selection persists (**SC-003**).

## Scenario C — Per-task mix — Story 2 (P2)

1. Set `generation → cerebras (gpt-oss-120b)`, `match → ollama`; save.
2. Run generation and matching. Expected: generation uses Cerebras, matching uses Ollama;
   each reports its own provider/model.
3. Reload/restart → mix persists.

## Scenario D — Embeddings unaffected — FR-006 / SC-004

1. With any task on Cerebras, run an embedding-dependent flow (similarity/vector search).
2. Expected: embeddings still served by Ollama endpoint; no regression vs Ollama-only.

## Scenario E — Missing credential — Story 3 / FR-008

1. Unset `CEREBRAS_API_KEY`, restart. Set a task to `cerebras` in Settings.
2. Expected: Settings shows a "Cerebras credential not configured" state; the task keeps
   running on Ollama; no silent failure.

## Scenario F — Cerebras runtime error — Story 3 / FR-009 / SC-005

1. With a task on Cerebras, force a failure (invalid key, or exhaust free-tier quota).
2. Run the task. Expected: an actionable error surfaces on the task's status; app stays up;
   **SC-006** — the API key never appears in the response body or logs.

## Automated coverage expected (implementation, not here)

- `go test`: Router resolution table (per task, per provider, missing-credential fallback);
  `llmsettings` service load/upsert + snapshot reload; handler validation (bad key/provider/
  model); Cerebras provider parse/error mapping.
- Env-gated live smoke: real Cerebras call when `CEREBRAS_API_KEY` set, else skipped.
- `vitest`: LlmSettingsCard renders per-task matrix, switch-all, credential-missing banner;
  hooks call the right endpoints and invalidate the query on update.
