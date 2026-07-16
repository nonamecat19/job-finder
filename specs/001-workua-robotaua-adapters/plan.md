# Implementation Plan: work.ua Job Source Adapter

**Branch**: `001-workua-robotaua-adapters` | **Date**: 2026-07-16 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-workua-robotaua-adapters/spec.md`

## Summary

Add work.ua as a job source, reaching parity with the existing djinni adapter: keyword search, remote filter, health check, subscription-URL ingestion with pagination, and post-discovery detail enrichment. Implementation is one new adapter file plus registry/enrichment/config wiring — no new architecture, no new dependencies.

**robota.ua is descoped from this plan.** Live probing showed it sits entirely behind a Cloudflare *managed* bot challenge; no plain-HTTP path exists (details in [research.md](./research.md)). Per the decision recorded on 2026-07-16, it is parked pending official API access. Spec User Story 2 is therefore deferred, not implemented. Stories 1, 3, and 4 (work.ua search, enrichment, subscriptions) are in scope.

## Technical Context

**Language/Version**: Go 1.23+ (`apps/api`)

**Primary Dependencies**: `github.com/PuerkitoBio/goquery` (HTML parsing), existing `internal/scraping.Service` (HTTP fetch). No new dependencies.

**Storage**: PostgreSQL via sqlc — existing `job_source`, `job`, `subscription`, `source_run` tables. **No schema change, no migration.**

**Testing**: `go test ./internal/jobsources/adapters/...`; live parity smoke tests behind the existing `//go:build live` tag.

**Target Platform**: Linux server (Docker Compose), same container as the rest of `apps/api`.

**Project Type**: Web service (Go API) — adapter plugin within an existing extensibility point.

**Performance Goals**: Not throughput-bound. Constraint is politeness, not speed (see below).

**Constraints**: work.ua `robots.txt` declares `Crawl-delay: 2`. All work.ua requests — list pagination and detail enrichment alike — MUST be spaced ≥2s. This is the binding constraint on the design and is stricter than the djinni default (1500ms).

**Scale/Scope**: One new adapter (~250 LOC + tests), 5 wiring touchpoints. Single-user self-hosted deployment.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Status | Notes |
|---|---|---|
| **I. No Auto-Apply, Ever** | ✅ PASS | Adapter is strictly read-only: GET requests against public listing/detail pages. No form posts, no messaging, no application path. Enforced by spec FR-015. |
| **II. Grounded Generation** | ✅ PASS | No LLM involvement. Adapter only parses and normalizes source HTML; every field traces to the source posting. Feeds the same grounded downstream flows as djinni/dou. |
| **III. Typed Contracts** | ✅ PASS | Reuses `dto.NormalizedJob` and `dto.SearchQuery` unchanged. No new cross-boundary type. Nothing to regenerate — `SourceKind` gains no new value (`scrape` already exists). |
| **IV. Test Discipline** | ✅ PASS | `go test` for adapter unit tests against saved HTML fixtures; live smoke behind the `live` build tag, matching existing convention. Change is confined to `apps/api`, so `make test-lint` is not strictly required, though harmless. |
| **V. Local-First, Self-Hosted** | ✅ PASS | work.ua is a discovery source only, never inference. No third-party paid AI API. No external service beyond the job board itself. |

**Post-Phase-1 re-check**: ✅ Still passing. The design added no LLM path, no new cross-language type, and no external dependency. The constitution's "job discovery sidecar is best-effort/unstable upstream" stance extends naturally to a scraped board — the zero-results-plus-warning degradation path (FR-007) is the concrete expression of it.

**No violations. Complexity Tracking section omitted.**

## Project Structure

### Documentation (this feature)

```text
specs/001-workua-robotaua-adapters/
├── plan.md              # This file
├── research.md          # Phase 0: robota.ua blocker + work.ua markup findings
├── data-model.md        # Phase 1: entity/field mapping
├── quickstart.md        # Phase 1: validation guide
├── contracts/
│   └── adapter-contract.md   # Phase 1: Adapter interface obligations
├── checklists/
│   └── requirements.md  # Spec quality checklist
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
apps/api/
├── internal/
│   ├── jobsources/
│   │   ├── adapter.go                 # Adapter interface (unchanged)
│   │   └── adapters/
│   │       ├── workua.go              # NEW — WorkUaAdapter
│   │       ├── workua_test.go         # NEW — unit tests over fixtures
│   │       ├── live_smoke_test.go     # MODIFIED — add TestLive_WorkUa
│   │       └── testdata/              # NEW — saved HTML fixtures
│   │           ├── workua_list.html
│   │           └── workua_detail.html
│   ├── enrichment/
│   │   └── handler.go                 # MODIFIED — "workua" case + enrichWorkUa
│   └── config/
│       └── config.go                  # MODIFIED — WorkUaDetailDelayMs
└── cmd/
    ├── server/main.go                 # MODIFIED — construct + register adapter
    └── seed/main.go                   # MODIFIED — register in seed registry
```

**Structure Decision**: The repo is a pnpm monorepo whose Go API owns job sources. This feature lives entirely inside `apps/api` and follows the constitution's stated extensibility rule — *"Adding a job site = one adapter implementing Adapter + one entry in the registry's constructor list"* (`apps/api/internal/jobsources/adapter.go:3`). No dashboard change is required: work.ua needs no credentials, so it needs no `CONFIG_FIELDS` entry in `SourcesPage.tsx` (that map is keyed only by sources with configurable secrets; absent keys render with no config inputs, which is correct here).

## Key Design Decisions

Full reasoning in [research.md](./research.md). Summary:

1. **Search URL**: use `https://www.work.ua/jobs/?search={urlencoded}`, which work.ua canonicalizes via redirect to `/jobs-{slug}/`. Chosen over hand-building the slug — it delegates slugification to the site and survives multi-word keywords. Go's `http.Client` follows the redirect by default.
2. **Remote filter**: `https://www.work.ua/jobs-remote/?search={kw}` (verified: 15 cards). The `remote` path segment is work.ua's own filter, so we don't post-filter with a text regex the way djinni does.
3. **Pacing**: an exported `WorkUaMinDelay = 2 * time.Second` floor, honored between every list page and clamped at the top of `enrichWorkUa`. Non-negotiable — it is the board's published `Crawl-delay`.
   > **Revised 2026-07-16 (finding C1).** This originally assumed enrichment already supported a per-source delay. It does not: `apps/api/cmd/server/main.go:120` derives one `enrichDelay` from `cfg.DjinniDetailDelayMs` and `Handler` holds it as a single shared `delay` field. Adding a `WORKUA_DETAIL_DELAY_MS` config value alone would have had **no consumer**, silently leaving work.ua on djinni's 1500ms — under the crawl-delay this plan's whole argument rests on. `Handler` now gains `defaultDelay` + `delays map[string]time.Duration` + `delayFor(sourceKey)` (tasks T009/T012), with a hard floor clamp in `enrichWorkUa` (T036) so no future config path can drop below 2s. Existing djinni/dou timing is unchanged.
4. **Detail enrichment**: `FetchDetail` returning a `WorkUaDetailPatch`, mirroring `DjinniDetailPatch`. Reuses the existing `UpdateJobDetail` sqlc query unchanged — it already accepts exactly the fields we recover. **Posting dates must be normalised** to RFC3339 before handing off (finding H1): work.ua's `datetime="2026-07-16 02:29:02"` parses under neither layout `dbutil.TimestampFromPtr` accepts, and that function fails silently rather than erroring.
5. **External ID**: the numeric id from `/jobs/{id}/` (also present as the card's `data-id`). Satisfies FR-004 dedup.
6. **Cyrillic safety**: raw HTML retained via `strutil.Truncate` (rune-safe), never a byte slice — the same trap `apps/api/internal/jobsources/adapters/djinni.go:141` documents.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| work.ua changes markup, selectors break | Medium (expected maintenance per spec) | Defensive multi-selector fallbacks; zero-results path logs a distinguishing warning (FR-007), never errors. |
| work.ua blocks or rate-limits us | Low if `Crawl-delay: 2` honored | Hard 2s floor clamped in `enrichWorkUa` (T036), not merely defaulted in config — a config-only default was the C1 bug. Failure is confined to this source (FR-008). |
| Enrichment refactor (T009) regresses djinni/dou timing | Low | `defaultDelay` keeps today's exact behavior for both; no map entry means no change. `go test ./internal/enrichment/...` gates it (SC-009). |
| Fixtures drift from live markup | Medium | Live smoke test behind the `live` tag catches it on demand; fixture capture date recorded in `research.md`. |
| robota.ua official access never materializes | Unknown | Story 2 stays deferred; no code written against it. Nothing else in the plan depends on it. |

## Phase Status

- [x] Phase 0: research.md — robota.ua blocker documented, work.ua markup verified live
- [x] Phase 1: data-model.md, contracts/adapter-contract.md, quickstart.md
- [x] Constitution re-check post-design — passing
- [ ] Phase 2: tasks.md — run `/speckit-tasks`
