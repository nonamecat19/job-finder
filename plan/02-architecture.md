# 02 — Architecture

## Monorepo layout (pnpm workspaces)

```
job-finder/
├── apps/
│   ├── api/                  # NestJS backend
│   ├── dashboard/            # React (Vite + TS)
│   └── jobspy-sidecar/       # Python FastAPI wrapping JobSpy (LinkedIn/Indeed/Glassdoor)
├── packages/
│   └── shared/               # TS types shared api↔dashboard: DTOs, NormalizedJob,
│                             # JSON Resume schema types, enums (ApplicationStatus, SourceKind)
├── plan/                     # these documents
├── docker-compose.yml
├── pnpm-workspace.yaml
└── .env.example
```

`packages/shared` is plain TS compiled with tsup; both apps import it as `@job-finder/shared`. The Python sidecar mirrors `NormalizedJob` in a Pydantic model (kept in sync manually — it has ~12 fields).

## NestJS module map (`apps/api/src/modules/`)

One module = one responsibility. Modules communicate through injected services and BullMQ queues, never by reaching into each other's repositories.

### `LlmModule`
- `LlmProvider` interface: `complete(prompt, opts)`, `completeStructured<T>(prompt, zodSchema)`, `embed(text): number[]`.
- `OllamaProvider` (v1, default): chat via `/api/chat` with `format: json` for structured calls; embeddings via `nomic-embed-text`.
- `completeStructured` validates output against a zod schema and retries with the validation error appended (max 2 retries) — essential with local models.
- Provider selected by `LLM_PROVIDER` env; adding Claude/OpenAI later = one new class.

### `ProfileModule`
- Master profile CRUD. Stored as JSON Resume-compatible document + free-form "extra knowledge" notes (things that don't fit the schema but the LLM may use: preferences, narrative, tech war stories).
- Resume import: upload PDF → text extraction (`pdf-parse`) → `LlmModule.completeStructured` → draft profile the user reviews in the dashboard.

### `JobSourcesModule` — the extensibility point
```ts
interface JobSourceAdapter {
  readonly key: string;                    // 'adzuna' | 'djinni' | ...
  readonly kind: 'api' | 'scrape' | 'sidecar';
  search(query: SearchQuery): Promise<NormalizedJob[]>;
  healthCheck?(): Promise<boolean>;
}
```
- Adapters register via a DI multi-provider token (`JOB_SOURCE_ADAPTERS`); `JobSourceRegistry` collects them. **Adding a job site = one class + one line in the module's providers array.** Nothing downstream changes.
- v1 adapters:
  - `AdzunaAdapter`, `RemotiveAdapter`, `ArbeitnowAdapter` — plain HTTP (axios), `kind: 'api'`.
  - `DjinniAdapter`, `DouAdapter` — cheerio over fetched HTML (both boards are server-rendered); session cookie via env for Djinni; fall back to `ScrapingModule` Playwright if markup demands it. `kind: 'scrape'`.
  - `JobSpyAdapter` — HTTP call to `jobspy-sidecar` `/search?site=linkedin|indeed|glassdoor`. One adapter, `site` selected per saved search. `kind: 'sidecar'`.
- Per-source enable/disable + credentials stored in `job_sources` table, editable from the dashboard.

### `ScrapingModule`
- Shared Playwright chromium pool (max 2 pages), per-domain rate limiter (bottleneck), retry with backoff, realistic UA.
- Optional FlareSolverr passthrough for Cloudflare-protected pages (env-gated).
- Only scrape-kind adapters depend on it.

### `IngestionModule`
- `@nestjs/schedule` cron reads enabled `saved_searches`, enqueues one BullMQ job per (search × source).
- Worker calls the adapter, then per job: **dedup** by `dedupe_key = sha256(lower(company) + lower(title) + canonical_url)`; new jobs are persisted and an `job.ingested` event enqueues matching.
- Adapter failure = that queue job fails and is logged to `source_runs`; other sources unaffected. 3 consecutive failures flag the source unhealthy in the dashboard.

### `MatchingModule`
Two-stage, cheap-first:
1. **Embedding pre-filter**: job description embedding (pgvector) vs. profile embedding → cosine similarity. Below threshold (configurable, default 0.35) → mark `low_fit`, skip LLM.
2. **LLM fit analysis** (`completeStructured`): returns `{ score: 0-100, matchedSkills[], missingSkills[], summary, redFlags[] }` stored in `match_results`.

### `GenerationModule`
- **Tailored resume**: input = master profile + JD + match result. LLM selects/reorders/rephrases profile content into a tailored JSON Resume. **Grounding guardrails**: prompt forbids inventing facts; post-check verifies every employer/date/degree in output exists in the master profile (string match on a whitelist), rejects and regenerates otherwise.
- **PDF render**: `PdfRenderer` interface; v1 `HtmlPdfRenderer` = Handlebars HTML template → Puppeteer `page.pdf()` (reuses Playwright chromium). RenderCV container can slot in later behind the same interface.
- **Cover letter**: short (≤150 words), 3-paragraph structure (hook referencing the company/role, 2–3 concrete matching experiences, close). Same grounding rules.
- All outputs versioned in `generated_documents`; regeneration keeps history.

### `ApplicationsModule`
- Tracker: statuses `found → shortlisted → docs_generated → applied → interview → offer | rejected`, timestamps, free-form notes per application.
- Pure CRUD + status transitions; feeds the dashboard kanban.

## Data flow

```
cron ──▶ IngestionModule ──▶ JobSourceRegistry ──▶ adapter.search()
                │                                   (api / scrape / sidecar)
                ▼
        dedup + persist job ──▶ queue: match
                                    │
                     MatchingModule: embed → prefilter → LLM score
                                    │
                                    ▼
                   Dashboard: feed sorted by fit score
                                    │  (user shortlists)
                                    ▼
                 GenerationModule: tailored resume + cover letter (PDF)
                                    │  (user downloads, applies manually)
                                    ▼
                 ApplicationsModule: kanban tracking
```

## Risk notes

- **LinkedIn/Indeed/Glassdoor**: reverse-engineered endpoints via JobSpy break periodically (LinkedIn changed structure 3× in 2025). Treat as best-effort; the pipeline must run fine on API sources alone. Never attach your real LinkedIn session to scraping.
- **Djinni/DOU**: low volume, polite rate limits (1 req/2s), cache pages; ToS-gray like all scraping — personal use, single user, low frequency.
- **Local LLM quality**: structured-output retries + grounding post-check compensate; matching/generation prompts must be tested against `qwen2.5:14b` and `llama3.1:8b` (see `05-infra.md`).
- **Schema drift** between TS `NormalizedJob` and sidecar Pydantic model — integration test in P5 covers it.
