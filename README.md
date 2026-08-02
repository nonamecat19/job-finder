# job-finder

Self-hosted, modular AI job-search platform. Discovers jobs across multiple sources, scores
them against your master profile with Ollama (local or Ollama Cloud), generates grounded tailored
resumes + cover letters as PDFs, and tracks applications on a kanban. You apply manually —
no auto-apply, ever. Design docs live in [`plan/`](plan/00-overview.md).

## Layout

```
apps/api              NestJS backend (Drizzle + pgvector, BullMQ, Ollama, Playwright)
apps/dashboard        React dashboard (Vite, Tailwind, TanStack Query, dnd-kit)
packages/shared       Shared TS types (NormalizedJob, DTOs, JSON Resume subset)
```

## Quick start (full stack)

```bash
cp .env.example .env       # set DB_PASSWORD, CONFIG_ENCRYPTION_KEY (openssl rand -hex 32),
                           # ADZUNA_APP_ID/KEY if you have them
docker compose up --build  # first run pulls Ollama models (~10 GB for qwen2.5:14b)
```

Dashboard: http://localhost:8080 · API: http://localhost:3000/api/health

Queue monitoring (dev only, not in `docker-compose.prod.yml`): asynqmon at
http://localhost:8090 — live view of the six asynq queues (ingest, match, generate,
enrich, salary, ghost), task inspection, retry/delete/archive actions, and per-queue
history.

GPU strongly recommended for local Ollama (uncomment the `deploy` block in docker-compose.yml).
To use **Ollama Cloud** instead, set `OLLAMA_URL=https://ollama.com` + `OLLAMA_KEY=<key>` and
`-cloud` model tags. Cloud has no embedding models, so point `EMBED_URL` at a local Ollama.
Chat models are per-task: `LLM_MODEL_MATCH` (fit scoring), `LLM_MODEL_GENERATION` (resume/cover),
`LLM_MODEL` as fallback; embeddings use `EMBED_MODEL`.
`docker compose --profile scraping-extras up` adds FlareSolverr for Cloudflare-protected pages.

Ollama is the default, local-first provider and always terminates every routing chain. Optionally,
set `GATEWAY_URL=http://litellm:4000` and `LITELLM_MASTER_KEY` to route chat tasks (matching,
generation, rephrase, ghost-job, salary/recruiter/outreach) through the bundled LiteLLM proxy
instead: each task key resolves to an ordered free-tier-first failover chain (Cerebras → Groq →
Cohere → OpenRouter → Ollama) declared in `gateway/config.yaml`. Provider keys
(`CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY`) live in the `litellm`
compose service's environment only — the Go backend never reads them and never learns which
upstream served a request beyond a `served_model` log line. **Changing which model serves a task
is a `gateway/config.yaml` edit followed by `docker compose restart litellm` — no dashboard, no
rebuild.** With `GATEWAY_URL` unset, every task talks to Ollama directly. Embeddings always stay on
Ollama regardless of gateway configuration (no remote provider offers an embeddings API).

Rate limits and provider errors are classified rather than blindly retried. Terminal problems
(rejected key, out of credits, unknown model) fail the task immediately with the reason on its
activity record instead of retrying forever; transient 5xx/network failures stay retryable. When
the gateway chain is exhausted the failure surfaces the same way a direct-Ollama failure would.

## Dev workflow (api/dashboard on host)

```bash
docker compose up postgres redis ollama
pnpm install
pnpm --filter @job-finder/shared build
cd apps/api && pnpm db:migrate && cd ../..
pnpm dev                   # api :3000 + dashboard :5173 (proxies /api)
make setup-hooks           # once per clone — activates the branch-protection git hooks
```

`make setup-hooks` (`git config core.hooksPath .githooks`) is a repository-level config
value, so one run covers every worktree sharing this clone — but it does not happen
automatically, and an unactivated hook is an absent gate. See
`specs/023-workflow-quality-gates/` for what it enforces.

Useful:

```bash
pnpm --filter @job-finder/api llm:smoke     # test complete/structured/embed against Ollama
pnpm -r typecheck
pnpm build
```

## Database code generation (sqlc)

The Go API's DB layer in `apps/api/internal/db/sqlcgen` is **generated** — never edit it by
hand. It is derived from the migrations in `apps/api/internal/db/migrations` and the queries in
`apps/api/internal/db/queries`.

Whenever you add or change a migration or a query file, regenerate and commit the result:

```bash
make sqlc-generate
git add apps/api/internal/db/sqlcgen
```

CI enforces this. The **API CI › sqlc generate is up to date** job
(`.github/workflows/api-ci.yml`) reruns `sqlc generate` and fails if the working tree changes,
so stale generated code cannot land on master. Reproduce the check locally with:

```bash
make sqlc-check
```

The sqlc version is pinned in `apps/api/.sqlc-version` so local runs and CI emit identical code.
Install the pinned version with `make sqlc-install`; `make sqlc-check` refuses to run on a
mismatched version rather than producing a misleading diff.

## Using it

1. **Profile** page: paste/import (PDF) your master profile — the superset of all your
   experience, JSON Resume format + free-form notes. Saving refreshes its embedding.
2. **Sources** page: enable sources, add credentials (Adzuna keys, Djinni cookie), create a
   saved search, hit **Run now** (also runs on its cron).
3. **Feed**: jobs arrive scored (embedding prefilter → LLM fit analysis). Shortlist the good ones.
4. **Job detail**: generate tailored resume + cover letter (grounding post-check rejects any
   invented employer/date/degree), edit the letter, download PDFs, mark applied.
5. **Tracker**: drag cards through shortlisted → applied → interview → offer.

### Resume shape settings

**Settings → Resume shape** controls the shape of every resume generated after you save
(a generation already running finishes with the settings it started with). `0` means
*unlimited / no limit*. **Reset to defaults** restores the table below.

| Setting | Default | Range | What it does |
|---------|---------|-------|--------------|
| `summaryLines` | 4 | 1–12 | Approximate summary length, in sentences |
| `skillsEnabled` | true | — | `false` removes the skills section entirely |
| `skillsMaxGroups` | 0 | 0–20 | Skill groups to keep; `0` keeps all |
| `experienceBulletsMin` | 8 | 1–20 | Target floor of bullets per job |
| `experienceBulletsMax` | 10 | 1–20 | Hard cap of bullets per job |
| `targetPages` | 2 | 1–3 | Page count the render loop aims for |
| `projectsEnabled` | true | — | `false` removes the projects section entirely |
| `projectsMin` | 0 | 0–20 | Target floor of projects; `0` = no minimum |
| `projectsMax` | 0 | 0–20 | Hard cap on projects; `0` includes all |
| `projectBulletsMax` | 0 | 0–10 | Hard cap of bullets per project; `0` keeps all |

Minima are targets, never padding: if your master profile has fewer bullets or projects
than the floor, the resume keeps what exists and the generation's activity trail records
the shortfall — nothing is invented. Maxima are enforced deterministically after the
model responds, so they always hold. When the page target and the section lengths
conflict, the page target wins and the run records that it did.

The defaults reproduce the pipeline's pre-settings behaviour exactly, so leaving this
card alone changes nothing.

## Adding a job source

One class implementing `JobSourceAdapter`
(`apps/api/src/modules/job-sources/adapter.interface.ts`) + one entry in the `ADAPTERS`
array in `job-sources.module.ts`. Nothing downstream changes.

## Notes

- Single user, LAN/localhost only — no auth in v1. Add basic auth at nginx if exposed wider.
- Scraped sources (Indeed, Glassdoor, dou, work.ua, ...) are best-effort: upstream markup
  breakage degrades those sources only (3 consecutive failures flag a source unhealthy).
- Source credentials are encrypted at rest (aes-256-gcm) when `CONFIG_ENCRYPTION_KEY` is set.
