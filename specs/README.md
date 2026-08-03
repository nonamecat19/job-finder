# specs/

Requirement records for job-finder. **What must be true, and why** — not how it is built.

| Directory | Holds | Read it when |
|---|---|---|
| `domains/` | Living requirement docs, one per capability area. Consolidated from the shipped features and reconciled against the code. | You are changing behaviour and need to know the rules that already bind it. |
| `archive/<nnn>-<slug>/` | The original per-feature `spec.md` (+ `contracts/`) as written at the time, unedited. Historical record only. | You need the original framing, acceptance scenarios, or dated rationale for one feature. |

**How it works is documented elsewhere** — the Docusaurus site under `docs/` covers
architecture, data model, HTTP API, workers, and operations. Do not restate implementation
detail here; link to `docs/` instead.

**Repo-wide rules** live in `AGENTS.md` (workflow) and `.specify/memory/constitution.md`
(non-negotiable principles). Do not restate those here either.

---

## Domains

| Doc | Covers | Folded-in features |
|---|---|---|
| [`domains/job-sources.md`](domains/job-sources.md) | Every job source, the shared adapter contract, per-source deltas, employer ATS boards | 002, 003, 004, 005, 010, 011, 012, 013, 015, 016, 022 |
| [`domains/retrieval-and-ingestion.md`](domains/retrieval-and-ingestion.md) | Browser-fidelity fetch ladder, per-host pacing, batched atomic persistence | 014, 017, 025 |
| [`domains/llm-routing.md`](domains/llm-routing.md) | LiteLLM gateway, per-task failover chains, AI concurrency and stuck-run recovery | 019, 029, 030 (001-cerebras superseded) |
| [`domains/resume-generation.md`](domains/resume-generation.md) | Grounded tailoring, structural invariants, configurable resume shape | 020, 028, 031, 032 |
| [`domains/profile-and-dashboard.md`](domains/profile-and-dashboard.md) | Editable resume profile, tile grid, skeleton loading, monochrome token system | 009, 001-global-dashboard-grid, 006, 021 |
| [`domains/platform-operations.md`](domains/platform-operations.md) | CI gates, health/readiness, queue monitoring, DB pool capacity, branch protection | 007, 008, 018, 023, 026 |
| [`domains/codebase-structure.md`](domains/codebase-structure.md) | Feature-module layout, shared-type single-sourcing, doc ownership | 024, 027 |
| [`domains/baseline.md`](domains/baseline.md) | Capabilities built before speckit and never spec'd — matching, applications, tracker, enrichment, ghost-job, salary, coach, referral, outreach, recruiter, interview prep, notifications, company intel, keyword rephrase, post-age | — |

## Feature registry

Every numbered feature ever run through speckit in this repo. All are shipped; the
implementation is on `master`.

| # | Feature | Created | Status | Requirements now live in |
|---|---|---|---|---|
| 001 | Cerebras Free-Tier Model Toggle | 2026-07-23 | **Superseded by 030** | `domains/llm-routing.md` (§ Superseded) |
| 001 | Global Dashboard Grid Layout | 2026-07-28 | **Superseded by 021** | `domains/profile-and-dashboard.md` |
| 002 | Indeed Job Source | 2026-07-24 | Shipped | `domains/job-sources.md` |
| 003 | RemoteOK Job Source | 2026-07-24 | Shipped | `domains/job-sources.md` |
| 004 | Glassdoor Job Source | 2026-07-24 | Shipped | `domains/job-sources.md` |
| 005 | JobLeads Job Source | 2026-07-24 | Shipped | `domains/job-sources.md` |
| 006 | Skeleton Loading States | 2026-07-24 | Shipped | `domains/profile-and-dashboard.md` |
| 007 | CI Test Gate | 2026-07-24 | Superseded by 023 | `domains/platform-operations.md` |
| 008 | Health/Readiness Endpoints | 2026-07-24 | Shipped | `domains/platform-operations.md` |
| 009 | Fully Editable Resume Profile Tab | 2026-07-25 | Shipped | `domains/profile-and-dashboard.md` |
| 010 | Wellfound Job Source | 2026-07-25 | Shipped | `domains/job-sources.md` |
| 011 | Himalayas Job Source | 2026-07-25 | Shipped | `domains/job-sources.md` |
| 012 | Jobgether Job Source | 2026-07-25 | Shipped | `domains/job-sources.md` |
| 013 | Employer ATS Board Sources | 2026-07-25 | Shipped | `domains/job-sources.md` |
| 014 | Browser-Fidelity Retrieval Ladder | 2026-07-25 | Shipped (FR-030 revoked by 017) | `domains/retrieval-and-ingestion.md` |
| 015 | Djinni Basic-Search Mode | 2026-07-26 | Shipped | `domains/job-sources.md` |
| 016 | Djinni Preset-Search Rewrite | 2026-07-28 | Shipped | `domains/job-sources.md` |
| 017 | Throttle-Only Rate Control | 2026-07-28 | Shipped | `domains/retrieval-and-ingestion.md` |
| 018 | Asynqmon Queue Monitoring | 2026-07-28 | Shipped | `domains/platform-operations.md` |
| 019 | AI Throughput & Stuck-Run Recovery | 2026-07-28 | Shipped | `domains/llm-routing.md` |
| 020 | Constrained AI Resume Tailoring | 2026-07-28 | Shipped | `domains/resume-generation.md` |
| 021 | HeroUI Tile-Grid Dashboard Rewrite | 2026-07-28 | Shipped | `domains/profile-and-dashboard.md` |
| 022 | Djinni Scraping Enhancement | 2026-07-28 | Shipped | `domains/job-sources.md` |
| 023 | Enforced Workflow Quality Gates | 2026-07-28 | Shipped | `domains/platform-operations.md` |
| 024 | Agent Context Consolidation | 2026-07-28 | Shipped | `domains/codebase-structure.md` |
| 025 | Batched, Atomic Ingest Persistence | 2026-07-30 | Shipped | `domains/retrieval-and-ingestion.md` |
| 026 | Explicit DB Connection Capacity | 2026-07-30 | Shipped (FR-008 deferred) | `domains/platform-operations.md` |
| 027 | HTTP Handler Decomposition | 2026-07-30 | Shipped | `domains/codebase-structure.md` |
| 028 | Resume Structure Preservation | 2026-07-31 | Shipped | `domains/resume-generation.md` |
| 029 | LiteLLM Proxy Gateway | 2026-07-31 | Shipped (FR-007 revoked by 030) | `domains/llm-routing.md` |
| 030 | Gateway-Owned Model Routing | 2026-07-31 | Shipped | `domains/llm-routing.md` |
| 031 | Configurable Resume Generation Shape | 2026-08-02 | Shipped | `domains/resume-generation.md` |
| 032 | Certifications as Configurable Category | 2026-08-03 | Shipped | `domains/resume-generation.md` |

**Number 001 was used twice** (`001-cerebras-model-toggle`, `001-global-dashboard-grid`) —
a historical collision, left as-is because the directory names match the branch names that
were actually used. **The next free number is 033.**

## Working with specs

### New feature

The speckit skills (`/speckit-specify`, `/speckit-plan`, `/speckit-tasks`,
`/speckit-implement`) still scaffold `specs/<nnn>-<slug>/` at the top level of `specs/`.
That is unchanged. Use the next free number from the registry above.

### After a feature ships

1. Fold its durable requirements into the matching `domains/*.md` — the FR/SC numbers stay,
   prefixed with the feature number (e.g. `020-FR-001`) so the rule stays traceable.
2. Mark anything it supersedes in the older domain text, in place. Do not silently drop a
   revoked rule; a revoked rule with a pointer is worth more than a missing one.
3. `git mv specs/<nnn>-<slug> specs/archive/<nnn>-<slug>` and delete `plan.md`,
   `research.md`, `quickstart.md`, `tasks.md`, `data-model.md`, `checklists/`. Keep
   `spec.md` and `contracts/`.
4. Add the row to the registry table above.

### Why the process artifacts are deleted

`plan.md`, `research.md`, `quickstart.md`, `tasks.md`, `data-model.md` and
`checklists/` describe how a feature was *going to be* built and how its rollout was
*going to be* validated. Once it is on `master`, the code is the answer to all of those,
and a stale plan is worse than no plan — it makes agents implement things that were
already revised during the build. They remain in git history; `git log --diff-filter=D
--name-only -- 'specs/*'` finds them.
