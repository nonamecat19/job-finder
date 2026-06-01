# 01 — Backend migration: NestJS → Go

## Context

`apps/api` is a ~3k-LOC NestJS backend for a single-user job hunting pipeline:
ingest jobs from adapters → dedupe/persist → embed + LLM fit-score → generate tailored
resume/cover-letter PDFs → track applications on a kanban. It runs HTTP + BullMQ workers
+ cron in one process, backed by Postgres/pgvector and Redis, calling two Python sidecars
(`jobspy-sidecar`, `rendercv` CLI) and local/hosted LLMs (Ollama, Cerebras).

Goal: move the backend to the Go ecosystem for a single static binary, lower memory, no
Node/TS runtime in prod. **The DB schema, React dashboard, and Python sidecars stay.**
Because it is single-user on a shared DB with no schema change, we do a **big-bang rewrite**
on a branch and one docker-compose swap — no dual-run.

Locked decisions: **asynq** (Redis stays) · **pgx + sqlc** · **big-bang cutover** ·
**keep React dashboard, generate its TS DTOs from Go**.

## What stays untouched
- `apps/dashboard` (React/Vite) — only its DTO types get regenerated from Go.
- `apps/jobspy-sidecar` (Python FastAPI over python-jobspy) — no Go port of python-jobspy; Go calls it over HTTP exactly as today.
- `rendercv` CLI (Python/Typst) — Go shells out to it, same as `RenderCvRenderer` does now (`os/exec`).
- Postgres schema (PascalCase tables, `jsonb`, `vector(768)`), Redis, FlareSolverr, `.env` keys.
- `pgvector/pgvector:pg16` and `redis:7-alpine` compose services.

## Dependency / concept mapping

| NestJS / TS | Go replacement |
|---|---|
| Nest + Express, `setGlobalPrefix('api')`, CORS | `chi` router, `/api` subrouter, `cors` middleware |
| Nest DI, modules | manual constructor wiring in `cmd/server/main.go` (no framework at this size) |
| BullMQ (`@nestjs/bullmq`) 3 queues + retries/backoff | `hibiken/asynq` — task types `ingest`/`match`/`generate`, `MaxRetry` + exponential retry |
| `@nestjs/schedule` `@Cron` + `cron-parser` | asynq periodic (`PeriodicTaskManager`) or `robfig/cron/v3`; keep the "due since lastRunAt" check |
| Drizzle ORM + `drizzle-kit` migrations | `pgx/v5` pool + `sqlc` generated queries; `pgvector-go` for vector cols; `jsonb` via `[]byte`/`json.RawMessage` |
| Drizzle migrations in `drizzle/` | reuse existing SQL as `goose` migrations; sqlc reads the same schema |
| `axios` | `net/http` (or `resty`) |
| `cheerio` (Djinni/DOU adapters) | `goquery` (same CSS-selector API) |
| Playwright chromium `page.pdf()` + `getBrowser()` | `chromedp` (CDP), `page.PrintToPDF`; keep chromium in the image |
| Handlebars `.hbs` templates | `html/template` (port `resume.hbs`, `cover-letter.hbs`) |
| `zod` + `zod-to-json-schema` for structured LLM output | Go structs + `invopop/jsonschema` to emit schema; `json.Unmarshal` + validate; same retry-with-error loop |
| `pdf-parse` (resume import) | `ledongthuc/pdf`, fallback shell `pdftotext` (poppler) |
| `crypto` AES-256-GCM | `crypto/aes` + `crypto/cipher` — **must reproduce `iv(12)‖tag(16)‖ct` base64 layout** so existing `JobSource.config` decrypts |
| `packages/shared` TS types | Go `internal/dto` structs → `tygo` generates dashboard `.d.ts` |
| `reflect-metadata`, class DTOs | plain structs + `chi` binding/`encoding/json` |

## Target Go layout (single module, single binary)

```
apps/api-go/
├── cmd/server/main.go        # wire deps; run http + asynq worker + scheduler (goroutines, mirrors current 1-process model)
├── internal/
│   ├── config/               # env load (os.Getenv/caarlos0-env), same .env keys
│   ├── db/                    # pgx pool, sqlc output (queries/, models.go), goose migrations
│   ├── dto/                   # DTO structs = source of truth for tygo → dashboard TS
│   ├── crypto/               # aes-256-gcm, byte-compatible with common/crypto.ts
│   ├── queue/                # asynq client + task payloads (ingest/match/generate)
│   ├── llm/                  # Provider iface: Complete, CompleteStructured[T], Embed; ollama.go, cerebras.go
│   ├── scraping/             # http fetchHtml (goquery), chromedp browser pool, FlareSolverr passthrough
│   ├── jobsources/           # Adapter iface + registry; adapters/{adzuna,remotive,arbeitnow,djinni,dou,jobspy}.go
│   ├── ingestion/            # scheduler + asynq handler: adapter.Search → dedupe → persist → enqueue match
│   ├── matching/             # embed prefilter (cosine via pgvector) → LLM fit score
│   ├── generation/           # tailorResume/writeCoverLetter + grounding + html/template→chromedp PDF + rendercv exec
│   ├── profile/              # CRUD, PDF import → LLM structured draft
│   ├── applications/         # status transitions, kanban feed
│   └── http/                 # chi handlers mirroring current controllers; /api/health, /stats
└── Dockerfile                # multi-stage: build static binary, runtime = chromium + rendercv + pdftotext
```

Process model matches today: one process runs HTTP server + asynq worker + scheduler as
goroutines. `-mode=api|worker|all` flag leaves the door open to split later.

## Port order (build to parity on a branch, then one cutover)

1. **Foundation** — module init, `config`, `db` (pgx + sqlc against existing schema via goose), `crypto` (with a byte-compat round-trip test vs a value encrypted by Node), `dto`, `chi` server + `/api/health`.
2. **LLM** — `Provider` iface, `ollama.go`, `cerebras.go`, `CompleteStructured` (schema-in-prompt + parse + validate + 2 retries), `Embed` (Ollama). Port `scripts/llm-smoke.ts` as a Go smoke test.
3. **Job sources + scraping** — adapter iface/registry, `scraping` (goquery + chromedp), all 6 adapters. API adapters first (adzuna/remotive/arbeitnow), then scrape (djinni/dou), then jobspy sidecar HTTP client.
4. **Ingestion** — scheduler (due-since-lastRunAt) + asynq `ingest` handler. **Keep dedupe identical**: `sha256(lower(company)|lower(title)|canonicalUrl)`, `canonicalUrl = url.split('?')[0]` trailing-slash-trimmed. Enqueue `match`. `SourceRun` logging + unhealthy-after-3-failures.
5. **Matching** — asynq `match` handler: embed job, cosine `1 - (embedding <=> $1)` via pgvector, threshold prefilter, LLM `fitSchema` structured call, upsert `MatchResult`.
6. **Generation** — `tailorResume` + `writeCoverLetter` with grounding verify + retry (port `grounding.ts`, `rendercv-grounding.ts`, `rendercv-tailor.ts`); `html/template`→chromedp PDF; `RenderCvRenderer` = `os/exec rendercv`; versioned `GeneratedDocument`; asynq `generate` handler + `documents.controller` routes (PDF download, cover-letter edit).
7. **Profile / Jobs / Applications / Stats** — remaining CRUD controllers: profiles (+ PDF import), jobs list/filter/get/shortlist/hide, applications PATCH, `/stats`.
8. **DTO codegen + dashboard** — `tygo` config emits `internal/dto` → replace `packages/shared` DTO types (or a generated file the dashboard imports). Verify dashboard builds and talks to Go unchanged.
9. **Infra swap** — `apps/api-go/Dockerfile` (chromium + rendercv + poppler in runtime stage); point `docker-compose*.yml` `api` service at it; delete `apps/api`. `ollama`/`ollama-init` services from `05-infra.md` unchanged.

## Critical parity details (must match exactly)
- **Crypto layout** — Node `Buffer.concat([iv, tag, enc])`; Go `gcm.Seal` puts tag *after* ciphertext, so slice it off and re-concat as `iv‖tag‖ct`. Requires `CONFIG_ENCRYPTION_KEY` 32-byte hex. Round-trip test against a Node-produced value.
- **Dedupe key** — byte-identical hashing or duplicate jobs flood in. Lowercasing + `|` separators + URL canonicalization must match `ingestion.processor.ts:74`.
- **Cosine similarity** — Drizzle `cosineDistance` → SQL `<=>`; keep `1 - distance` and default threshold `0.35` (`MATCH_SIMILARITY_THRESHOLD`).
- **Structured-output retry** — same shape as `cerebras.provider.ts`: strip ```` ```json ```` fences, parse, validate, on failure append the validation error and retry (max 2 extra attempts).
- **Cron due-check** — replicate `ingestion.scheduler.ts`: every 5 min, for each enabled `SavedSearch`, `due = !lastRunAt || lastRunAt < prevCronSlot`.
- **Envelope shapes** — DTOs in `packages/shared/src/index.ts` (`JobListResponse`, `MatchResultDto`, `ApplicationDto` with `events[]`, `StatsDto.pipeline`, etc.) must serialize field-for-field so the dashboard needs no changes.

## Sub-decisions taken as defaults (flag if you disagree)
- Router **chi**; HTML parse **goquery**; headless **chromedp**; migrations **goose**; env **caarlos0/env**; JSON schema **invopop/jsonschema**; TS gen **tygo**. No DI framework — manual wiring.
- Redis-queued in-flight BullMQ jobs are **not** migrated; drain the queue before cutover (single user, low volume — acceptable).

## Verification (end-to-end, per phase and at cutover)
- **Unit/parity tests**: crypto round-trip vs a Node-encrypted fixture; dedupe hash equals a known Node output; structured-output retry on a malformed-then-valid stub; grounding rejects a fabricated employer.
- **DB**: run against the *existing* dev Postgres (sqlc/goose read current schema) — a fresh `docker compose up postgres` + goose up must produce the same tables Drizzle did.
- **Pipeline smoke**: `docker compose up` the Go api + sidecars; POST `/api/searches/:id/run`, confirm `SourceRun` rows, jobs persisted, `match` results scored, then `/api/jobs/:id/generate` produces a PDF in `/data/documents`.
- **LLM**: Go port of `llm:smoke` against Ollama and Cerebras.
- **Contract**: replay `postman/job-finder.postman_collection.json` against the Go server — every endpoint returns the same shape the dashboard expects.
- **Dashboard**: `pnpm --filter dashboard build` against tygo-generated types, then click through jobs feed → shortlist → generate → applications kanban against the Go backend.
