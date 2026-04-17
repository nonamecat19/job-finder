# 06 — Roadmap

Each phase ends runnable and demoable. Order: prove the pipeline on stable API sources before touching fragile scrapers.

## P1 — Skeleton (foundation)
- pnpm monorepo: `apps/api` (NestJS), `apps/dashboard` (Vite stub), `packages/shared`.
- docker-compose: postgres (pgvector), redis, ollama + model init; api runs, healthcheck endpoint.
- Prisma schema from `03-data-model.md`, first migration.
- `LlmModule`: `OllamaProvider` with `complete`, `completeStructured` (zod + retry), `embed`. Smoke-test script.
- `ProfileModule`: CRUD + PDF import (pdf-parse → LLM parse → draft).
- **Done when**: `docker compose up` works; profile created via API; LLM answers structured prompt.

## P2 — Ingestion (first jobs in DB)
- `packages/shared`: `NormalizedJob`, `SearchQuery`, DTOs.
- `JobSourcesModule`: adapter interface, registry, `AdzunaAdapter` + `RemotiveAdapter` + `ArbeitnowAdapter`.
- `IngestionModule`: BullMQ queue, cron over saved searches, dedup, `SourceRun` logging.
- Dashboard v0: job feed (unsorted), sources & searches pages, "Run now".
- **Done when**: saved search pulls real jobs from 3 API sources into the feed, deduped across sources.

## P3 — Matching (fit scores)
- Profile + job embeddings, pgvector similarity prefilter.
- LLM fit analysis → `MatchResult`; queue-driven on ingest.
- Dashboard: score-sorted feed, score badges, fit breakdown on job detail.
- Prompt tuning pass against `qwen2.5:14b` on ~20 real vacancies (manual eval of scores).
- **Done when**: feed ranks jobs sensibly by fit; obvious mismatches score low.

## P4 — Generation (the payoff)
- `GenerationModule`: tailored JSON Resume (grounding guardrails + post-check), cover letter (≤150 words).
- `HtmlPdfRenderer`: Handlebars template → Puppeteer PDF; documents volume; versioning.
- Dashboard: generate/regenerate/edit-letter/download on job detail.
- **Done when**: shortlist a job → get an honest tailored resume PDF + cover letter in under ~2 min locally.

## P5 — Scrapers (coverage)
- `ScrapingModule`: Playwright pool, rate limiter, retries.
- `DjinniAdapter`, `DouAdapter` (cheerio first, Playwright fallback).
- `apps/jobspy-sidecar` (FastAPI + python-jobspy) + `JobSpyAdapter` → LinkedIn/Indeed/Glassdoor best-effort.
- Contract test: sidecar response ↔ `NormalizedJob` schema.
- **Done when**: Djinni/DOU vacancies flow through the same pipeline; JobSpy sources work when upstream is healthy and fail gracefully when not.

## P6 — Polish
- Tracker kanban (dnd), status history.
- Source health monitoring UI (consecutive-failure flagging), run stats.
- Daily digest of new high-fit jobs (Telegram bot or email via SMTP env) — optional.
- ivfflat index when job count grows; document retention/cleanup job.
- **Done when**: daily use needs only the dashboard.

## Deliberately deferred
- Auto-apply — excluded by decision.
- Cloud LLM provider class (interface ready, add when wanted).
- RenderCV render engine (behind `PdfRenderer` interface).
- JSearch adapter (metered), multi-user/auth, SSE instead of polling.
