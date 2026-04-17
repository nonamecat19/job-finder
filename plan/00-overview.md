# 00 — Overview

## Vision

A self-hosted, modular AI job-search platform that:

1. **Discovers** jobs automatically across multiple job websites (APIs and scrapers).
2. **Scores** each job against your master profile (fit score, matched/missing skills).
3. **Generates** a tailored resume and a short cover letter per vacancy, grounded strictly in your real experience.
4. **Tracks** applications through a simple pipeline (kanban).

The human stays in the loop: the system prepares everything, **you** click "Apply" on the job site.

## Confirmed decisions

| Decision | Choice |
|---|---|
| Hosting | Fully self-hosted, docker compose |
| Backend | NestJS (TypeScript) |
| Frontend | React dashboard (Vite + TS) |
| LLM | Ollama, fully local (provider abstraction keeps cloud APIs possible later) |
| Job sources (initial) | Adzuna / Remotive / Arbeitnow (APIs), Djinni + DOU (scrape), LinkedIn / Indeed / Glassdoor (via JobSpy sidecar) |
| Auto-apply | **No** — out of scope permanently for v1; human applies |
| Repo structure | pnpm-workspaces monorepo: `apps/api`, `apps/dashboard`, `apps/jobspy-sidecar`, `packages/shared` |

## Goals

- **Modular**: each NestJS module has exactly one responsibility (see `02-architecture.md`).
- **Extendible**: adding a new job website = writing one adapter class implementing `JobSourceAdapter` and registering it. No changes to ingestion, matching, or generation.
- **Grounded generation**: the LLM may reorder, rephrase, and emphasize — it must never invent experience, employers, or dates.
- **Resilient ingestion**: a broken scraper (LinkedIn markup change, Cloudflare block) degrades that one source, never the whole pipeline.

## Non-goals (v1)

- Auto-submitting applications (ToS/ban risk, ethically murky).
- Multi-user / SaaS features. Single user, LAN/localhost deployment.
- Mobile app.
- Interview prep / salary negotiation features (possible later).

## Glossary

- **Master profile** — the single source of truth about you: all experience, skills, projects, achievements. Superset of any single resume. Stored in JSON Resume-compatible schema.
- **NormalizedJob** — the canonical job shape every adapter must produce (title, company, location, remote flag, salary, description, URL, source, posted date).
- **Saved search** — a query (keywords, location, remote, sources) that ingestion runs on a schedule.
- **Fit score** — 0–100 rating of job ↔ profile match, produced by embeddings pre-filter + LLM analysis.
- **Tailored resume** — a per-vacancy resume derived from the master profile, emphasizing what the JD asks for.

## Document map

| File | Content |
|---|---|
| `01-existing-tools.md` | Domain research: existing tools, reuse verdicts, job-API comparison |
| `02-architecture.md` | Monorepo layout, NestJS module map, adapter pattern, data flow, risks |
| `03-data-model.md` | Postgres entities, ORM choice |
| `04-dashboard.md` | React dashboard pages and API contract |
| `05-infra.md` | docker-compose services, Ollama models, env layout |
| `06-roadmap.md` | Implementation phases P1–P6 |
