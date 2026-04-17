# 01 — Existing Tools & Domain Research

Research snapshot: July 2026. Verdicts: **wrap** (run as-is in a container), **borrow** (steal ideas/prompts/schemas), **skip**.

## Job discovery / scraping

| Tool | What it does | Stack | Verdict |
|---|---|---|---|
| [JobSpy](https://github.com/speedyapply/JobSpy) | Scrapes LinkedIn, Indeed, Glassdoor, Google Jobs, ZipRecruiter through one API; returns normalized rows. ~10k+ stars, actively maintained. | Python lib | **Wrap** — run as a thin FastAPI sidecar container (`apps/jobspy-sidecar`); a NestJS adapter calls it over HTTP. Reimplementing these scrapers in TS is wasted effort: they rely on reverse-engineered endpoints that break several times a year, and JobSpy maintainers fix them for us. |
| [JobFunnel](https://github.com/PaulMcInnis/JobFunnel) | Multi-board scrape → deduped spreadsheet. | Python | **Skip** — archived; modern anti-bot measures killed it. Its dedup idea (content hash) is worth borrowing. |
| Djinni / DOU | Ukrainian dev job boards. No public APIs. | — | **Build** own adapters: Djinni has clean server-rendered HTML (cheerio is enough when logged in via cookie); DOU jobs pages are also server-rendered. |

## Job listing APIs (no scraping)

| API | Coverage | Auth / limits | Verdict |
|---|---|---|---|
| [Adzuna](https://developer.adzuna.com/) | Aggregated listings, many countries, salary data | Free app_id/app_key, generous free tier | **Use** — first adapter, most reliable |
| [Remotive](https://remotive.com/) | Remote-only jobs | Free, no key | **Use** — trivial adapter |
| [Arbeitnow](https://www.arbeitnow.com/blog/job-board-api) | EU-focused board | Free, no key | **Use** |
| [JSearch (RapidAPI)](https://www.openwebninja.com/api/jsearch) | Google-for-Jobs data, wide coverage | Freemium, RapidAPI key | **Optional** — good coverage, but metered; add adapter later |

## Resume generation / rendering

| Tool | What it does | Verdict |
|---|---|---|
| [Reactive Resume](https://github.com/amruthpillai/reactive-resume) | Full self-hosted resume builder (NestJS + React, ironically). | **Borrow** — its template/PDF approach (HTML templates + headless Chrome print) validates our chosen render path; don't run it as a service, our generation is API-driven. |
| [RenderCV](https://github.com/rendercv/rendercv) | YAML → professionally typeset PDF (Typst). 17k+ stars. | **Optional wrap** — containerized CLI as an alternative render engine for a more "academic" look. v1 uses HTML + Puppeteer; keep `PdfRenderer` interface so RenderCV can slot in. |
| [OpenResume](https://github.com/xitanggg/open-resume) | Resume builder **and parser**. | **Borrow** — parser ideas for importing an existing PDF resume into the master profile. In practice our LLM-based parse (PDF text → structured JSON via Ollama) is simpler and better. |
| [resume-lm](https://github.com/olyaiy/resume-lm) | AI resume builder (Next.js), per-job tailoring. | **Borrow** — prompt patterns for tailoring. |
| [JSON Resume](https://jsonresume.org/) | De-facto standard resume schema + theme ecosystem. | **Adopt schema** — master profile and tailored resumes stored in JSON Resume-compatible shape; free interop with its themes. |

## Matching / ATS scoring

| Tool | What it does | Verdict |
|---|---|---|
| [Resume Matcher](https://github.com/srbhr/Resume-Matcher) | Resume ↔ JD match scoring, keyword gap analysis. | **Borrow** — scoring dimensions (keyword overlap, skills gap) inform our `MatchingModule` prompt + embedding design. |

## Auto-apply bots (out of scope, studied for patterns)

| Tool | Notes |
|---|---|
| [AIHawk / Jobs_Applier_AI_Agent](https://github.com/feder-cr/Jobs_Applier_AI_Agent_AIHawk) | Most famous auto-applier (LinkedIn). Educational-purposes disclaimer; frequent bans reported. **Borrow** its resume-tailoring and Q&A-answering prompt structure only. |
| [ApplyPilot](https://github.com/Pickle-Pixel/ApplyPilot) | Discovery → scoring → tailoring → auto-submit; supports Ollama. **Borrow** — closest pipeline to ours minus submit step; validates local-LLM feasibility. |

## Key takeaways for our design

1. **Nobody offers the full package self-hosted on NestJS** — the pieces exist (JobSpy for scraping, JSON Resume for schema, Ollama for LLM) but the glue (normalized ingestion + fit scoring + grounded generation + tracker) is the actual product. Building it is justified.
2. **Scraping LinkedIn/Indeed directly is the highest-maintenance part** — delegate to JobSpy sidecar; treat those sources as best-effort.
3. **API sources first** (Adzuna/Remotive/Arbeitnow): stable, zero anti-bot pain, perfect for proving the pipeline before touching scrapers.
4. **JSON Resume schema** as internal format buys parser/theme interop for free.
