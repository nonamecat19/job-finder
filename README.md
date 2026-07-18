# job-finder

Self-hosted, modular AI job-search platform. Discovers jobs across multiple sources, scores
them against your master profile with Ollama (local or Ollama Cloud), generates grounded tailored
resumes + cover letters as PDFs, and tracks applications on a kanban. You apply manually —
no auto-apply, ever. Design docs live in [`plan/`](plan/00-overview.md).

## Layout

```
apps/api              NestJS backend (Drizzle + pgvector, BullMQ, Ollama, Playwright)
apps/dashboard        React dashboard (Vite, Tailwind, TanStack Query, dnd-kit)
apps/jobspy-sidecar   Python FastAPI wrapping JobSpy (LinkedIn/Indeed/Glassdoor, best-effort)
packages/shared       Shared TS types (NormalizedJob, DTOs, JSON Resume subset)
```

## Quick start (full stack)

```bash
cp .env.example .env       # set DB_PASSWORD, CONFIG_ENCRYPTION_KEY (openssl rand -hex 32),
                           # ADZUNA_APP_ID/KEY if you have them
docker compose up --build  # first run pulls Ollama models (~10 GB for qwen2.5:14b)
```

Dashboard: http://localhost:8080 · API: http://localhost:3000/api/health

GPU strongly recommended for local Ollama (uncomment the `deploy` block in docker-compose.yml).
To use **Ollama Cloud** instead, set `OLLAMA_URL=https://ollama.com` + `OLLAMA_KEY=<key>` and
`-cloud` model tags. Cloud has no embedding models, so point `EMBED_URL` at a local Ollama.
Chat models are per-task: `LLM_MODEL_MATCH` (fit scoring), `LLM_MODEL_GENERATION` (resume/cover),
`LLM_MODEL` as fallback; embeddings use `EMBED_MODEL`.
`docker compose --profile scraping-extras up` adds FlareSolverr for Cloudflare-protected pages.

## Dev workflow (api/dashboard on host)

```bash
docker compose up postgres redis ollama
pnpm install
pnpm --filter @job-finder/shared build
cd apps/api && pnpm db:migrate && cd ../..
pnpm dev                   # api :3000 + dashboard :5173 (proxies /api)
```

Useful:

```bash
pnpm --filter @job-finder/api llm:smoke     # test complete/structured/embed against Ollama
pnpm -r typecheck
pnpm build
```

## Using it

1. **Profile** page: paste/import (PDF) your master profile — the superset of all your
   experience, JSON Resume format + free-form notes. Saving refreshes its embedding.
2. **Sources** page: enable sources, add credentials (Adzuna keys, Djinni cookie), create a
   saved search, hit **Run now** (also runs on its cron).
3. **Feed**: jobs arrive scored (embedding prefilter → LLM fit analysis). Shortlist the good ones.
4. **Job detail**: generate tailored resume + cover letter (grounding post-check rejects any
   invented employer/date/degree), edit the letter, download PDFs, mark applied.
5. **Tracker**: drag cards through shortlisted → applied → interview → offer.

## Adding a job source

One class implementing `JobSourceAdapter`
(`apps/api/src/modules/job-sources/adapter.interface.ts`) + one entry in the `ADAPTERS`
array in `job-sources.module.ts`. Nothing downstream changes.

## Notes

- Single user, LAN/localhost only — no auth in v1. Add basic auth at nginx if exposed wider.
- LinkedIn/Indeed/Glassdoor go through the JobSpy sidecar and are best-effort: upstream
  breakage degrades those sources only (3 consecutive failures flag a source unhealthy).
- Source credentials are encrypted at rest (aes-256-gcm) when `CONFIG_ENCRYPTION_KEY` is set.
- Sidecar↔TS contract test: `pytest apps/jobspy-sidecar/test_contract.py`.
