# Domain: Baseline (unspecified capabilities)

Capabilities that exist in the code and have **no spec**. They were built before speckit was
adopted in this repository (the first numbered feature is dated 2026-07-23) and were never
retro-specified.

This document is an **inventory, not a requirements record.** It tells you what exists and
where, so an agent planning a change knows the surface it is touching. It deliberately does
not invent FR/SC numbers for behaviour nobody ever specified — where a rule matters, the code
and `docs/` are the authority.

Verified against `apps/api/cmd/server/servers.go` and `compose.go`.

---

## 1. Backend feature modules

Every module below mounts HTTP routes through `httpapi.NewRouter(...)` unless noted.

| Module | What it does | Docs |
|---|---|---|
| `matching` | Fit scoring. Embedding prefilter, then structured LLM fit analysis (`FitResult`). Runs unconditionally on every ingested job — it is the one AI feature with no enable threshold. Worker on the `match` queue. | [`docs/ai/matching.md`](../../docs/docs/ai/matching.md) |
| `enrichment` | Detail-page fetch for stub listings. Worker on the `enrich` queue; per-source delays (`DJINNI_DETAIL_DELAY_MS`, `WORKUA_DETAIL_DELAY_MS`). | [`docs/ai/enrichment.md`](../../docs/docs/ai/enrichment.md) |
| `jobs` | Job list/filter/get, shortlist, hide; enqueues async document generation. | [`docs/backend/http-api.md`](../../docs/docs/backend/http-api.md) |
| `applications` | Application records and the kanban tracker state machine — shortlisted → applied → interview → offer. | [`docs/backend/domain-model.md`](../../docs/docs/backend/domain-model.md) |
| `profile` | Master profile storage and its embedding. Saving refreshes the embedding. Consumed by matching, generation, keyword, coach. | [`docs/backend/domain-model.md`](../../docs/docs/backend/domain-model.md) |
| `subscriptions` | Saved-search / subscription CRUD and enable toggle. | [`docs/ingestion/scheduler.md`](../../docs/docs/ingestion/scheduler.md) |
| `activity` | The activity trail — per-run and per-task records that every other domain writes its outcomes and failure reasons to. Load-bearing for the auditability requirements in `resume-generation.md` and `llm-routing.md`. | [`docs/async/activity-tracking.md`](../../docs/docs/async/activity-tracking.md) |
| `notifier` | Fresh-match notifications: a freshness rule in `domain/`, a write path (`MaybeNotify`) and a read path (`NotificationService`). | [`docs/async/notifications.md`](../../docs/docs/async/notifications.md) |
| `ghostjob` | Ghost-job scoring — signals suggesting a posting is not a real opening. Worker on the `ghost:score` queue. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `salary` | Salary band inference. A `salaryRaw` text parser (pure, no I/O) plus a levels.fyi loader. Worker on the `salary:infer` queue. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `postage` | Posting-age aggregation — how long a listing has been live. | — |
| `companyintel` | Company signals scraped from Crunchbase, layoffs trackers and Glassdoor, behind a registry of adapters. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `recruiter` | Recruiter identification across the posting, the company page and LinkedIn. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `outreach` | LLM-drafted outreach messages, fed by contacts and company intel. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `referral` | Referral-path discovery via the GitHub REST API (`infrastructure/github/`). | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `coach` | Fit-gap assessment between the profile and a posting; diff-driven. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `interviewprep` | Derives interview questions from posting bullets, matches them against the user's STAR stories, adds a keyword-gap summary and a company-news briefing. **No LLM call** — derivation and matching are pure/template-based. | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `keyword` | Keyword gap analysis and cached LLM rephrase suggestions (`rephrase` task key). | [`docs/ai/assistants.md`](../../docs/docs/ai/assistants.md) |
| `tailoring` | The **review-gated state machine** between generation and PDF export. It does not call the LLM (that is `internal/generation`) or render PDFs (that is `internal/generation/singlepage`). This is where `resume-generation.md` § 4 accept/reject lives. | [`docs/ai/generation.md`](../../docs/docs/ai/generation.md) |
| `aifeature` | Per-feature enable flag + score threshold, consulted after a job is scored to decide whether to auto-enqueue that feature's work. Below threshold or disabled, the feature runs on-demand only. Feature keys: `resume`, `cover_letter`, `salary_infer`. | — |
| `resumeshape` | Storage and validation for the resume shape config. Spec'd — see [`resume-generation.md`](resume-generation.md) § 5. | **undocumented in `docs/`** |
| `health` | Liveness/readiness. Spec'd — see [`platform-operations.md`](platform-operations.md) § 5. | [`docs/operations/observability.md`](../../docs/docs/operations/observability.md) |

Supporting packages with no feature surface: `apperr`, `config`, `crypto`, `db`, `dbtest`,
`dbutil`, `dto`, `httpapi`, `httpx`, `platform/{llm,scraping,storage}`, `queue`, `ratelimit`,
`retrieval`, `seed`, `strutil`, `testutil`.

## 2. Dashboard feature modules

`apps/dashboard/src/features/`: `contacts`, `feed`, `interview-prep`, `job-detail`,
`notifications`, `profile`, `settings`, `sources`, `status`, `tailor`, `tailoring`,
`tracker`.

Routes (`src/app/routes.tsx`): `/`, `/jobs/:id`, `/profile`, `/contacts`, `/tailor`,
`/sources`, `/status`, `/settings`, `/tracker`.

> Note the pair `tailor` (the page) and `tailoring` (the review flow). They are separate
> directories with adjacent names — check which one you mean before editing.

## 3. Cross-cutting behaviour with no spec

- **Credential encryption at rest.** Source credentials are encrypted with aes-256-gcm when
  `CONFIG_ENCRYPTION_KEY` is set (`internal/crypto`). Referenced by 005-FR-018 for JobLeads,
  but the mechanism itself was never spec'd.
- **No auth.** Single user, LAN/localhost only. There is no authentication layer in the API
  at all. This is a deliberate v1 scope decision recorded in the README, not an oversight —
  but it means **any exposure beyond localhost requires a reverse-proxy auth layer first.**
- **Object storage.** MinIO via `platform/storage/infrastructure/minio` for generated
  documents.
- **Scheduler.** Cron-driven source runs, in addition to on-demand runs.

## 4. Known gaps

| Gap | Detail |
|---|---|
| `resumeshape` absent from `docs/` | Feature 031/032 shipped after the last `docs/` refresh at commit `e1c1c3e`. The domain doc [`resume-generation.md`](resume-generation.md) § 5 is currently the only description. |
| Seven job-source adapters unreachable | See [`job-sources.md`](job-sources.md) § 2 — `indeed`, `remoteok`, `glassdoor`, `jobleads`, `wellfound`, `jobgether` are enrichment-only; `himalayas` is dead code. |
| 026-FR-008 deferred | Pool waiter metrics were explicitly descoped. See [`platform-operations.md`](platform-operations.md) § 7. |
