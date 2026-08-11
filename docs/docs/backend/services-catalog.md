---
title: Services catalog
sidebar_position: 7
description: One entry per business package — responsibility, key types, collaborators, and HTTP surface.
---

# Services catalog

Every package under `apps/api/internal` that owns a business capability, with what it
does, what it depends on, and where it is exposed.

```mermaid
flowchart LR
    subgraph Sync["Exposed synchronously"]
        profile
        applications
        keyword
        coach
        interviewprep
        companyintel
        recruiter
        referral
        outreach
        postage
        notifier
        subscriptions
        aifeature
        resumeshape
    end
    subgraph Async["Driven by a queue"]
        ingestion
        matching
        generation
        enrichment
        salary
        ghostjob
    end
    subgraph Support["Support"]
        jobsources
        retrieval
        scraping
        ratelimit
        storage
        activity
        seed
    end
```

## Discovery

### `jobsources`
Everything about reading a source. The adapter contract and the ~25 provider adapters now
live in the job-scraper library (`ports/source.go`, `adapters/`); this package holds the
source service (`application/service.go`), ingestion — scheduler
(`interfaces/worker/scheduler.go`), ingest task handler (`interfaces/worker/handler.go`),
dedupe and persistence (`application/ingest/`), and the saved-search, subscription and
reconciliation runners (`application/search_service.go`) — plus `roster/` for ATS board
discovery. Port: `Repository` (`domain/repository.go:9-16`). HTTP: `/sources`, `/searches`,
`/roster`, `/hosts`. Queue: `ingest`. Detail: [Job sources](/ingestion/job-sources).

### `retrieval`
The app-side configuration of the library's fetch engine: `NewService` assembles the
three-rung ladder with the cooling-off and retest knobs (`service_impl.go:16-24`), per-host
state with encrypted cookies (`state.go`), and the shared paced transport
(`transport.go`). The rungs, browser identity and challenge handling themselves are the
library's (`retrieval/rung*.go`, `identity.go`, `challenge.go`). HTTP: `/hosts/{host}/*`.

### `scraping`
The app's `ports.Scraper`: an ordinary HTTP client plus a lazily launched headless browser
(`scraping.New()` in `cmd/server/platform.go:88`). It lives here rather than in the library
so one browser is shared between job sources and the company-intel and PDF paths. Closed by
`main.run` on shutdown.

### `ratelimit`
Crawl-delay-aware pacing. Per-host daily budgets were removed in migration
`00029_drop_host_budget.sql`. Detail: [Rate limiting](/ingestion/rate-limiting).

### `ghostjob`
Ghost-posting detection. Queue: `ghost:score`, LLM task key `ghost`. HTTP:
`POST /jobs/{id}/ghost-score` — manual or ingestion-triggered only, never scheduled.

## Intelligence

### `llm`
`Provider` interface, the Ollama and LiteLLM-gateway adapters, the task-bound `Router`,
error taxonomy, structured-output retry loop. Detail:
[LLM abstraction](/ai/llm-abstraction).

Provider and model selection is **not** a service — it lives in `gateway/config.yaml`.
There is no `llmsettings` package and no `LlmTaskSetting` table; migration
`00033_drop_llm_task_setting.sql` removed them.

### `resumeshape`
Persists the resume generation shape config — summary length, section toggles, bullet and
page targets — validated as a whole before any field is stored. HTTP:
`/settings/resume-shape`.

### `aifeature`
Per-feature enable/disable settings backed by `AiFeatureSetting`. HTTP:
`/settings/ai-features`. Constructed inside `composeMatching`.

### `matching`
Two-phase scoring: pgvector recall, then LLM fit scoring; persists `MatchResult`. Owns
`jobs.Service` used by the jobs handler. Queue: `match`, LLM key `match`. Files include
`ports.go`, `repository.go`, `integration_test.go`.

### `enrichment`
Fetches full job detail after ingest, per-source delays (`DJINNI_DETAIL_DELAY_MS`,
`WORKUA_DETAIL_DELAY_MS`). Constructed with ten source adapters plus the asynq client
(`compose.go`). Queue: `enrich`.

### `generation`
Tailored resume and cover-letter generation grounded in the profile
(`RESUME_GROUNDING_LEVEL`), PDF rendering via `RENDERCV_BIN`, versioned
`GeneratedDocument` rows. Queue: `generate`, LLM key `generation`. HTTP: `/documents/*`,
`POST /jobs/{id}/generate`.

### `salary`
`salary.Service` with `LevelsFyiLoader` (CSV at `LEVELS_FYI_CSV`) and LLM inference;
caches into `SalaryCache`. Queue: `salary:infer`, LLM key `default`.

### `keyword`
JD keyword diff (`DiffService`), rephrase suggestions via `ProviderRephraseModel` behind
a `CachedRephraser` (`KEYWORD_REPHRASE_CACHE_TTL_SEC`), and STAR stories. HTTP:
`/jobs/{id}/keyword-diff`.

## Career workflow

### `profile`
Master profile, resume document, embeddings, config upload. HTTP: `/profiles/*`
including `/profiles/{id}/resume` and `/profiles/config/status`. Supplies
`ProfileEntries` to coach and STAR stories to interview prep.

### `applications`
Kanban statuses, event history (`Application.events` jsonb), stats. HTTP:
`/applications`, `/stats`.

### `coach`
`coach.Service` + `AssessmentService`, built over the keyword rephrase model and profile
entries (`compose.go`). HTTP: `POST /jobs/{id}/coach/assess`, cached read at
`/jobs/{id}/coach/assessment`.

### `interviewprep`
Combines profile STAR stories with company intel to produce prep material. HTTP:
`GET /jobs/{id}/interview-prep`.

### `companyintel`
Company profile and signals (`Company`, `CompanySignal`). HTTP:
`/companies/{jobId}/intel` and `/intel/refresh`.

### `recruiter`
Resolves hiring contacts for a job from posting text and company pages; LinkedIn
scraping is opt-in via `LINKEDIN_SCRAPE_ENABLED`. HTTP: `/jobs/{id}/contacts`,
`/contacts/refresh`.

### `referral`
Contact graph (`Contact`, `ContactConnection`), CSV import, GitHub sync, referral paths.
HTTP: `/contacts`, `/contacts/import`, `/contacts/{id}/github-sync`,
`/jobs/{id}/referral-paths`.

### `outreach`
Generates outreach messages in selectable tones, using recruiter and company-intel
services plus the default router. HTTP: `/jobs/{id}/outreach/generate`, `/outreach/tones`.

### `subscriptions`
Recurring searches with their own cron (`Subscription`, `00024_subscription_cron.sql`).
HTTP: `/subscriptions`, `/subscriptions/{id}/run`, `/subscriptions/run-all`.

### `notifier`
Fresh-match notifications gated by `MATCH_NOTIFY_SCORE_THRESHOLD` and
`MATCH_NOTIFY_RATE_LIMIT`, stored in `FreshMatchNotification`. HTTP: `/notifications`,
`/notifications/unseen-count`.

### `postage`
Post-age response-rate analytics. HTTP: `/postage-response-rate`.

## Platform

| Package | Responsibility |
| --- | --- |
| `activity` | `ActivityRun` recorder, heartbeat, `Sweeper` for vanished workers |
| `storage` | documents on MinIO or `DOCUMENTS_DIR` |
| `queue` | task types, payloads, policies, admission gate, deadline middleware |
| `config` | typed environment configuration |
| `crypto` | AES-256-GCM for secrets at rest |
| `apperr` | error kinds and HTTP mapping |
| `dto` | wire types, tygo source |
| `db`, `dbutil`, `dbtest` | pool, migrations, generated queries, test helpers |
| `seed` | development data (`cmd/seed`) |
| `strutil`, `testutil` | shared helpers |

## Collaboration graph for the busiest packages

```mermaid
flowchart TD
    HTTPJ["JobsHandler"] --> MJ["matching.jobs.Service"]
    HTTPJ --> GEN["generation.Service"]
    MJ --> MR["matching repository"]
    MJ --> RTR["llm.Router (match)"]
    GEN --> PROF["profile.Service"]
    GEN --> RTR2["llm.Router (generation)"]
    GEN --> ST["storage"]
    ING["ingestion.Handler"] --> SRC["jobsources registry"]
    SRC --> RET["retrieval ladder"]
    ING --> ENR["enrichment.Handler"]
    KW["keyword.DiffService"] --> PROF
    COACH2["coach.AssessmentService"] --> KW
    PREP["interviewprep.Service"] --> PROF
    PREP --> CI2["companyintel.Service"]
    OUT["outreach"] --> REC2["recruiter.Service"]
    OUT --> CI2
```
