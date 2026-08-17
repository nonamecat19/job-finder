# job-finder

Self-hosted, modular AI job-search platform. Discovers jobs across multiple sources, scores
them against your master profile via a self-hosted LiteLLM gateway to hosted LLM providers,
generates grounded tailored resumes + cover letters as PDFs, and tracks applications on a kanban.
You apply manually — no auto-apply, ever. Every AI request leaves the deployment: prompt content
(profile data, resume content, posting text) is sent to a third-party provider on every request,
with no configuration under which it is not — see `.env.example`'s LLM block.

Requirement records live in [`specs/`](specs/README.md) (start at
[`specs/domains/`](specs/domains/)); the implementation guide is the Docusaurus site
under [`docs/`](docs/); agent workflow rules are in [`AGENTS.md`](AGENTS.md).

## Layout

```
apps/api              Go backend (chi HTTP API, sqlc + goose on Postgres/pgvector,
                      asynq workers on Redis, LiteLLM gateway, scraping/retrieval ladder)
apps/dashboard        React dashboard (Vite, Tailwind, TanStack Query, dnd-kit)
packages/shared       Shared TS types (NormalizedJob, DTOs, JSON Resume subset)
```

## Quick start (full stack)

```bash
cp .env.example .env       # set DB_PASSWORD, CONFIG_ENCRYPTION_KEY (openssl rand -hex 32),
                           # LITELLM_MASTER_KEY, provider API keys, ADZUNA_APP_ID/KEY if you have them
docker compose up --build
```

Dashboard: http://localhost:8080 · API: http://localhost:3000/api/health

Queue monitoring (dev only, not in `docker-compose.prod.yml`): asynqmon at
http://localhost:8090 — live view of the six asynq queues (ingest, match, generate,
enrich, salary, ghost), task inspection, retry/delete/archive actions, and per-queue
history.

`docker compose --profile scraping-extras up` adds FlareSolverr for Cloudflare-protected pages.

There is one inference path: every AI request goes through the self-hosted LiteLLM proxy
(`gateway/config.yaml`) to a hosted provider. `GATEWAY_URL` and `LITELLM_MASTER_KEY` are required —
the application refuses to boot without them, naming the missing key. Each scenario (match,
generation, rephrase, ghost-job, salary, outreach, recruiter, every generation sub-stage, and
embeddings) resolves to its own ordered failover chain of at least two tiers across at least two
distinct providers (Cerebras, OpenRouter, OpenAI). Provider keys
(`CEREBRAS_API_KEY`, `OPENROUTER_API_KEY`, `OPENAI_API_KEY`) live
in the `litellm` compose service's environment only — the Go backend never reads them and never
learns which upstream served a request beyond a `served_model` log line. **Changing which model
serves a scenario is a `gateway/config.yaml` edit followed by `docker compose restart litellm` — no
dashboard, no rebuild.**

Rate limits and provider errors are classified rather than blindly retried. Terminal problems
(rejected key, out of credits, unknown model) fail the task immediately with the reason on its
activity record instead of retrying forever; transient 5xx/network failures stay retryable. When a
scenario's whole chain is exhausted, the task fails with that reason recorded.

## Dev workflow (api/dashboard on host)

```bash
make up                    # postgres, redis, litellm, minio
pnpm install
pnpm --filter @job-finder/shared build
make run-backend           # api :3000 — runs embedded goose migrations on startup
make run-frontend          # dashboard :5173 (proxies /api)
```

`make run-all` starts both. Migrations are embedded in the binary and applied by
`cmd/server` at startup — there is no separate migrate command.

The Generate page's live resume preview renders in-browser via two WASM modules built from a
sibling `../rendercv-go` checkout. They're fetched assets, not committed to this repo — build them
with `pnpm --filter @job-finder/dashboard build-wasm` (needs `../rendercv-go` present next to this
repo). See `apps/dashboard/public/wasm/README.md`. The dashboard still runs without them; the
preview pane just shows its unsupported/error fallback until they exist.

The trunk is not protected: nothing rejects a commit or push to `master`. See
[`specs/domains/platform-operations.md`](specs/domains/platform-operations.md) § 1 for what
was withdrawn and why.

Useful:

```bash
pnpm --filter @job-finder/api llm:smoke     # test complete/structured/embed against the gateway
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
| `certificationsEnabled` | true | — | `false` removes the certifications section entirely |
| `certificationsMin` | 0 | 0–20 | Target floor of certifications; `0` = no minimum |
| `certificationsMax` | 0 | 0–20 | Hard cap on certifications; `0` includes all |

Minima are targets, never padding: if your master profile has fewer bullets, projects or
certifications than the floor, the resume keeps what exists and the generation's activity trail records
the shortfall — nothing is invented. Maxima are enforced deterministically after the
model responds, so they always hold. When the page target and the section lengths
conflict, the page target wins and the run records that it did.

The defaults reproduce the pipeline's pre-settings behaviour exactly, so leaving this
card alone changes nothing.

**Per-group skill density** lives in the Profile, not here: each skill group can be set to
show *All skills*, *Half — most relevant first*, or *Only the skills the job asks for* on a
generated resume (default: all). The trim is applied to the tailored resume after relevance
ranking; a group set to "only relevant" with nothing matching the job is left off that resume.

## Adding a job source

One type implementing `ports.JobSource` (`ports/source.go` in the job-scraper library) in
that library's `adapters/<key>/`, plus one entry in the `adapter.NewRegistry(...)` call in
`apps/api/cmd/server/compose.go`. Nothing downstream
changes — retrieval, pacing, challenge handling and persistence are all shared.

The requirements every source must meet are in
[`specs/domains/job-sources.md`](specs/domains/job-sources.md), which also records which
adapters are currently registered.

## Notes

- Single user, LAN/localhost only — no auth in v1. Add basic auth at nginx if exposed wider.
- Scraped sources (Indeed, Glassdoor, dou, work.ua, ...) are best-effort: upstream markup
  breakage degrades those sources only (3 consecutive failures flag a source unhealthy).
- Source credentials are encrypted at rest (aes-256-gcm) when `CONFIG_ENCRYPTION_KEY` is set.

## License

Source-available, **not open source**. Copyright (c) 2026 Oleksandr Shumskyi,
all rights reserved. The code is public so it can be read and audited; running,
copying, modifying, forking, or redistributing it is not permitted. See
[`LICENSE`](LICENSE). Commercial licensing: shumsky.alexander.work@gmail.com

External contributions are not accepted.
