# Implementation Plan: Salary Inference

**Branch**: `006-salary-inference` | **Date**: 2026-07-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-salary-inference/spec.md`

## Summary

Give every job a salary band. Three independent sources — a self-hosted LLM reading the posting, a cached public compensation dataset keyed by (title, geo, company-size) bucket, and the system's own parse of compensation stated in already-ingested postings — each emit a band and a confidence. Those blend by confidence-weighted average into five new columns on `"Job"`. A user-configured USD floor then hides jobs whose band is entirely below it, on by default, revealable by a toggle.

Structurally: one migration, one new `internal/salary` package, three source implementations behind one interface, a floor predicate threaded through the three existing job-list queries, and dashboard work for the band display, the toggle, and a settings surface that does not yet exist.

## Technical Context

**Language/Version**: Go 1.23+ (`apps/api`), TypeScript/React (`apps/dashboard`)

**Primary Dependencies**: existing `internal/llm` (structured completion), `encoding/csv` (stdlib) for the reference dataset, existing `internal/scraping` for its retrieval. **No new Go module dependency.**

**Storage**: PostgreSQL via sqlc. **One migration, `00009_salary_inference.sql`** — five `ALTER TABLE` columns on `"Job"` plus the new `"SalaryCache"` table. Per the constitution's goose rule, 00009 must be unique and sequential; the tree currently holds 00001–00006 and sibling tasks are landing 00007 and 00008, so **00009's availability must be re-verified at implementation time** rather than assumed from this document.

**Testing**: `go test ./internal/salary/...` for the parser, the blender, and each source against fixtures; `vitest` for the dashboard band/toggle components. This change crosses `apps/api` and `apps/dashboard`, so **`make test-lint` is a binding gate**, not optional (Principle IV).

**Target Platform**: Linux server, Docker Compose, same containers as the rest of the stack.

**Project Type**: Web service (Go API) + React dashboard.

**Performance Goals**: The floor predicate runs on every default feed load and must not regress feed latency — hence the partial index in [data-model.md](./data-model.md). Inference itself is background work with no latency target; it is bounded by model throughput and is idempotent per job (FR-022).

**Constraints**: Inference runs on the self-hosted model only (FR-025). The reference dataset is retrieved once at startup and cached; an unreachable dataset must degrade quality, never block startup (SC-010).

**Scale/Scope**: Single-user self-hosted deployment. The dataset is tens of thousands of rows collapsing to a few thousand buckets — small enough that the cache table needs no partitioning and the load is a single upsert pass.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| **I. No Auto-Apply, Ever** | PASS | Read-only inference and a view filter. No code path touches an application, a message, or a listing action. The floor filter deliberately does **not** mutate job status (FR-017) — it hides rows from a view, and the record stays in `found` and stays retrievable. This was the user's explicit choice over auto-hiding, and it keeps the filter reversible. |
| **II. Grounded Generation** | **PASS, with the feature's central risk** | See below. |
| **III. Typed Contracts** | PASS | New DB columns go through sqlc regeneration, not hand-written structs. `JobFilters` gains one field in `apps/dashboard/src/lib/api.ts`; if the band is exposed on the job DTO it must flow through the existing tygo path, never a hand-duplicated TS interface. No new cross-language type is authored by hand. |
| **IV. Test Discipline** | PASS | `go test` for the API, `vitest` for the dashboard, and `make test-lint` as the boundary gate since the change spans two apps. SC-003 (parser accuracy across every ingested currency) and SC-004 (held-out estimate accuracy) are both measurable as tests over real ingested data, not assertions of intent. |
| **V. Local-First, Self-Hosted** | **CONDITIONAL PASS** | See below. |

### Principle II — the honest version

Principle II binds LLM-generated content to the user's profile or the source posting, and forbids fabrication. A salary estimate sits awkwardly against it: the LLM source is, by construction, producing a number the posting does not contain. That is the feature.

The reading under which this passes: Principle II's rationale is about *misrepresenting the user to an employer* — hallucinated experience on a resume. A salary estimate is never sent to anyone. It is a decision aid shown only to the user, in their own feed.

But the risk it protects against is real here in a different form: a confidently-wrong salary number that a user carries into a negotiation is a concrete harm, and it is the harm this feature can actually cause. The design's answer is that **no band is ever displayed without its provenance and its confidence** — FR-006 (low-confidence marking), FR-011 (source recorded), FR-021 (per-source breakdown), and SC-008 (a user can tell which source drove the number without reading logs). These are not polish; they are what makes the LLM source constitutionally acceptable, and they should be treated as non-negotiable in implementation rather than deferred as UI nicety.

Distinguishing `posting` from the estimate sources in `"salarySource"` ([data-model.md](./data-model.md)) is the same argument at the schema level: "this posting says it pays X" and "postings like this pay about X" must never be collapsed into one indistinguishable number.

### Principle V — the levels.fyi question

**This is the item flagged in the task description, and it does not resolve to a clean PASS. Two separate issues sit underneath it, and only one of them is really about the constitution.**

**On the constitution itself: PASS.** Principle V requires core inference to run against the local model with no third-party *paid AI API*, and permits external services for discovery. The reference dataset is a static file retrieved once, cached locally in `"SalaryCache"` (FR-012), and queried offline thereafter. No inference leaves the machine; the LLM source runs on the local runtime (FR-025); and SC-010 requires the system to start and serve normally when the dataset is unreachable. The dataset is a local-first cached asset, not a runtime dependency. Nothing here weakens the self-hosting guarantee.

**On the terms of use: unresolved, and the premise needs checking before implementation.** The task describes a "levels.fyi PUBLIC CSV (downloaded at startup)". That premise should not be carried into implementation unverified:

1. **levels.fyi does not appear to publish an official public CSV export or an open-data licence.** The widely-circulated levels.fyi salary CSVs are third-party scrapes redistributed on dataset-sharing sites. Building against one is not "using a public CSV" — it is depending on a redistribution whose provenance and licence are the redistributor's, not levels.fyi's.
2. **levels.fyi's terms prohibit automated collection.** Retrieving data at startup by scraping the site would be circumventing that, which puts it in the same category as the robota.ua decision in [spec 001](../001-workua-robotaua-adapters/research.md) — where this project chose to park a source rather than work around the operator's stated position. That precedent cuts against doing here what was declined there.

Calling this a "gray area" understates it for option 1 and misstates it for option 2. Scraping levels.fyi directly is not gray; it is against their terms. What is genuinely gray is redistributed scraped datasets of it.

**Recommendation, for the Director to decide before the implementing task starts**: use an explicitly-licensed compensation dataset instead. The bucket abstraction (`titleBucket`, `geoBucket`, `companySizeBucket`, `source`) is deliberately dataset-agnostic and the `"source"` column exists so the dataset can be swapped without a schema change — so this substitution costs nothing structurally, and is much cheaper to make now than after the loader is written. Candidates and their licence terms are Phase 0 work in [research.md](./research.md).

**Gate status**: `levels-fyi` as a source name is a **placeholder pending that decision**. The other two sources — the local LLM and the self-hosted ingested-compensation cache — carry no such question and are unblocked. If the licensing question stalls, the feature still ships on those two, at reduced accuracy against SC-004.

> **Complexity Tracking**: no principle is violated, so no deviation is recorded. The Principle V item above is a factual question about an external dataset, not a request to deviate from the constitution.

## Project Structure

### Documentation (this feature)

```text
specs/006-salary-inference/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0: dataset licensing, parser strategy, blending alternatives
├── data-model.md        # Phase 1: schema change + reused types
├── quickstart.md        # Phase 1: validation guide, Levels 1-4
└── tasks.md             # Phase 2: implementation task breakdown
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── salary/                        # NEW — the feature's own package
│   │   ├── types.go                   # SalaryBand, SourceEstimate, BlendedEstimate, Source interface
│   │   ├── parse.go                   # salaryRaw → band (multi-currency)
│   │   ├── parse_test.go              # table-driven over every currency in the corpus
│   │   ├── blend.go                   # confidence-weighted blending
│   │   ├── blend_test.go              # incl. blend-of-one is a no-op
│   │   ├── bucket.go                  # title/geo/company-size normalization
│   │   ├── source_llm.go              # CompleteStructured[SalaryBand]
│   │   ├── source_dataset.go          # SalaryCache lookup + fallback widening
│   │   ├── source_ingested.go         # observed-compensation cache
│   │   ├── loader.go                  # startup dataset retrieval → upsert
│   │   └── service.go                 # orchestration, per-job idempotence
│   ├── config/config.go               # MODIFIED — SalaryFloorUsd
│   ├── db/
│   │   ├── migrations/
│   │   │   └── 00009_salary_inference.sql   # NEW — verify number is free
│   │   └── queries/
│   │       ├── joblist.sql            # MODIFIED — floor predicate, all three queries
│   │       └── salary.sql             # NEW — SalaryCache upsert/lookup
│   └── httpapi/                       # MODIFIED — floor filter param, band on job DTO
└── cmd/server/main.go                 # MODIFIED — construct service, run loader at startup

apps/dashboard/src/
├── lib/api.ts                         # MODIFIED — JobFilters gains the toggle field
└── features/
    ├── feed/FeedPage.tsx              # MODIFIED — band display, below-floor chip, toggle
    ├── job-detail/JobDetailPage.tsx   # MODIFIED — Story 3 breakdown
    └── settings/                      # NEW — surface for the floor (see below)
```

**Structure Decision**: The feature gets its own `internal/salary` package rather than being spread across `enrichment` and `matching`. It has its own domain types, three interchangeable sources behind one interface, and a blending rule with real invariants — that is a package, not a handful of helpers. It slots into the existing background-work path the same way enrichment does.

**The settings surface is genuinely new.** `apps/dashboard/src/features/` today holds feed, job-detail, profile, sources, status, tailor, and tracker — there is no settings page to add a field to. `SALARY_FLOOR_USD` as an env var satisfies FR-014 on the API side by itself; the dashboard entry means building a settings feature from scratch, including a route, a persistence path for user-set config, and the API endpoints behind it. **That is plausibly larger than the rest of this feature's dashboard work combined**, and it should be split into its own task rather than absorbed here. Recorded as such in [tasks.md](./tasks.md).

## Key Design Decisions

Full reasoning in [research.md](./research.md). Summary:

1. **Band on `"Job"`, no related table** — 1:1 with a posting; a join table would cost a join on the feed's hottest query and buy nothing. Accepted consequence: no estimate history, and Story 3's breakdown must be recomputed rather than read back (see decision 6).
2. **Three sources behind one interface** — each emits `(SourceEstimate, error)` independently and never sees another's answer (FR-003). This is what makes FR-023's per-source failure isolation fall out for free, and what lets the dataset source be swapped or dropped without touching the others — directly relevant given the licensing question above.
3. **Confidence-weighted blending, sum capped at 1** — the user's specified rule. The cap is not cosmetic: uncapped, three sources at 0.5 yield a confidence of 1.5, which breaks every threshold comparison downstream.
4. **`posting` beats everything** — a parsed stated compensation replaces the estimate rather than blending into it at high confidence (FR-008). Blending ground truth with a guess produces a number that is worse than the ground truth alone.
5. **Bucket normalization is the load-bearing part of the dataset source** — matching `Sr. Backend Engineer, Kyiv, unknown-size` against a dataset built from US big-tech ladders is where accuracy is won or lost, far more than in any blending arithmetic. Fallback widens company size to `unknown`, then geo to `*`, lowering confidence at each widening.
6. **Story 3's breakdown is computed, not stored** — persisting `Components` would need either a JSON blob on `"Job"` or the related table the user ruled out. Open point: as designed, the breakdown is only available at computation time, which makes Story 3 hard to serve on demand. Options are to recompute on request (a model call per page view — likely unacceptable) or to accept one JSON column. **Needs a decision before Story 3 is built**; Stories 1 and 2 are unaffected.
7. **Parser precedes inference** — parsing `salaryRaw` across the corpus both populates the ingested-cache source and produces the labelled held-out set SC-004 is measured against. It is the first implementation task for that reason, not merely because it is the easiest.
8. **`NULL` must pass the floor predicate** — SQL three-valued logic drops `NULL` rows from `"salaryMax" >= $1`, which would silently hide every band-less job, exactly inverting FR-019. The predicate must make this explicit, and `CountJobs` must carry the identical predicate or pagination totals will disagree with page contents.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Reference dataset licensing blocks the `levels-fyi` source | **Medium-high** — see Constitution Check | Source interface makes it swappable; `"source"` column already anticipates a different dataset. Feature degrades to two sources rather than failing. Resolve before the loader is written. |
| Confidently-wrong estimate misleads a real negotiation | Medium | The Principle II mitigations — confidence marking, source labelling, per-source breakdown — are non-negotiable, not deferrable UI work. |
| Floor predicate silently hides band-less jobs | Medium | Direct consequence of decision 8 if written naively. Explicit test that a `NULL`-band job survives a non-zero floor. |
| Monthly-vs-annual mixing produces a 12× error | Medium | `Period` is on `SalaryBand` specifically so it cannot be implicit. Normalize to annual before storage; assert in parser tests. |
| Bucket matching too sparse to be useful for non-US geos | **High** for the dataset source | Fallback widening plus per-widening confidence penalty. If UA/EU buckets are empty, the source honestly reports low confidence rather than a fabricated band — which is the correct failure and is why SC-004 pairs accuracy with confidence correlation. |
| Migration number 00009 collides with a sibling task | Medium | Tree holds 00001–00006; siblings are landing 00007/00008. Re-verify before writing, per the constitution's uniqueness rule. |
| Settings-page scope swallows the feature | Medium | Split into its own task. The env var alone satisfies FR-014 for the API. |
| `CountJobs` and the list queries diverge on the predicate | Medium | Same predicate in all three, asserted by an integration test that compares the reported total against the rows actually returned. |

## Phase Status

- [x] Phase 0: research.md — parser strategy, blending alternatives, dataset licensing survey
- [x] Phase 1: data-model.md, quickstart.md
- [x] Constitution re-check post-design — Principle V conditional, see above
- [x] Phase 2: tasks.md
- [ ] **Blocking decision**: reference dataset licensing (Principle V) — Director
- [ ] **Blocking decision**: Story 3 breakdown persistence (key design decision 6) — before Story 3 only
