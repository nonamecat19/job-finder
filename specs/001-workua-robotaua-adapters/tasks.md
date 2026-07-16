---

description: "Task list for work.ua job source adapter"
---

# Tasks: work.ua Job Source Adapter

**Input**: Design documents from `/specs/001-workua-robotaua-adapters/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/adapter-contract.md, quickstart.md

**Tests**: Included. Not because the spec asked, but because Constitution Principle IV makes them a gate — *"A change is not 'done' until its own language's test suite passes locally."* Unit tests run against saved fixtures (no network); the live smoke test is opt-in behind the `live` build tag, matching existing convention.

**Organization**: Grouped by user story so each ships independently.

**Revision 2026-07-16** (post-`/speckit-analyze`): renumbered. T009 and T036 are new, addressing findings C1 and H1 — see the callouts on those tasks. Prior T009/T033 (a `WORKUA_DETAIL_DELAY_MS` config field with no consumer) were incoherent and have been replaced.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US3, US4)
- Exact file paths included per task

## Path Conventions

All paths are repo-relative from `/home/nnc/WebstormProjects/job-finder`. This feature lives entirely in `apps/api` (Go). No dashboard, no migration, no `packages/shared` change.

## Scope note: User Story 2 (robota.ua) is NOT in this task list

Deferred pending official API access — robota.ua is behind a Cloudflare managed challenge with no plain-HTTP path (see [research.md](./research.md)). No tasks exist for it by design. If access is granted later, it gets its own spec/plan cycle.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Capture the HTML fixtures every unit test parses.

- [X] T001 Create fixture directory `apps/api/internal/jobsources/adapters/testdata/`
- [X] T002 Capture list-page fixture to `apps/api/internal/jobsources/adapters/testdata/workua_list.html` via `curl -s -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36' 'https://www.work.ua/jobs-php/'` (expect ~14 `.job-link` cards)
- [X] T003 Capture detail-page fixture to `apps/api/internal/jobsources/adapters/testdata/workua_detail.html` from any `/jobs/{id}/` URL found in T002's fixture — wait 2s after T002 (work.ua `Crawl-delay: 2`). Verify the saved file contains a `<time datetime="...">` element; T027 depends on it
- [X] T004 [P] Capture empty-results fixture to `apps/api/internal/jobsources/adapters/testdata/workua_empty.html` via `'https://www.work.ua/jobs-kyiv-php/?salaryfrom=4&experience=2'` — a genuinely-empty search that renders work.ua's `no-results` marker (see T013 rationale)
- [X] T005 Record all three fixtures' capture date and source URLs in a comment block at the top of `apps/api/internal/jobsources/adapters/workua_test.go`

**Checkpoint**: Fixtures on disk. Tests can be written without network access.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: The adapter type, its identity, its pacing infrastructure, and its registration.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T006 Create `apps/api/internal/jobsources/adapters/workua.go` with package decl, `WorkUaAdapter` struct holding `Scraping *scraping.Service`, and constants `workuaBaseURL = "https://www.work.ua"`, `workuaMaxSubscriptionPages = 50`, plus the **exported** floor `WorkUaMinDelay = 2 * time.Second` with a comment stating it is work.ua's published `Crawl-delay: 2` — a floor config may raise but never lower
- [X] T007 Implement `Key() string` returning `"workua"` and `Kind() dto.SourceKind` returning `dto.SourceKindScrape` in `apps/api/internal/jobsources/adapters/workua.go` — per contracts/adapter-contract.md, `Key()` is permanent (it is the `job_source.key` and the enrichment `switch` discriminator)
- [X] T008 Implement `HealthCheck(ctx, config)` in `apps/api/internal/jobsources/adapters/workua.go` — fetch `https://www.work.ua/jobs/`, return `(strings.Contains(html, "work.ua"), nil)`; a fetch failure returns `(false, nil)` not an error, mirroring `djinni.go:230` (FR-009)

### ⚠️ Finding C1 — per-source enrichment delay (blocks T036)

> **Why this exists**: `apps/api/cmd/server/main.go:120` currently does `enrichDelay := time.Duration(cfg.DjinniDetailDelayMs) * time.Millisecond` and passes it to `NewHandler` as a **single shared** `delay` field (`handler.go:30`) used by every source. There is no per-source delay. Without T009, work.ua enrichment would inherit djinni's 1500ms default — **below work.ua's mandated 2s crawl-delay**, violating FR-011 and SC-008 and undermining the pacing argument the whole plan rests on.

- [X] T009 Restructure `Handler` for per-source pacing in `apps/api/internal/enrichment/handler.go`:
  - Replace the `delay time.Duration` field with `defaultDelay time.Duration` and `delays map[string]time.Duration`
  - Add `func (h *Handler) delayFor(sourceKey string) time.Duration` returning `h.delays[sourceKey]` when present, else `h.defaultDelay`
  - Extend `NewHandler` to take `workua adapters.WorkUaAdapter`, `defaultDelay time.Duration`, and `delays map[string]time.Duration` (replacing the single `delay` param)
  - Change `enrichDjinni` and `enrichDOU` to call `h.delayFor("djinni")` / `h.delayFor("dou")` instead of `h.delay`
  - **Zero behaviour change for existing sources**: with `defaultDelay = DjinniDetailDelayMs` and no map entries for djinni/dou, both keep today's exact timing (SC-009)
- [X] T010 [P] Add `WorkUaDetailDelayMs int \`env:"WORKUA_DETAIL_DELAY_MS" envDefault:"2000"\`` to `apps/api/internal/config/config.go` — comment that 2000 matches work.ua's published `Crawl-delay: 2`, and that `adapters.WorkUaMinDelay` clamps it so a misconfigured env var cannot go below the floor
- [X] T011 Register the adapter in `apps/api/cmd/server/main.go`: construct `workuaAdapter := adapters.WorkUaAdapter{Scraping: scrapingSvc}` alongside the existing djinni/dou construction (~line 69) and add it to the `jobsources.NewRegistry(...)` list
- [X] T012 Build the per-source delay map at the `NewHandler` call site in `apps/api/cmd/server/main.go` (~line 120-121), passing `workuaAdapter` through:
  ```go
  enrichDelay := time.Duration(cfg.DjinniDetailDelayMs) * time.Millisecond
  enrichDelays := map[string]time.Duration{
      "workua": time.Duration(cfg.WorkUaDetailDelayMs) * time.Millisecond,
  }
  enrichHandler := enrichment.NewHandler(database.Queries, sourcesSvc, djinniAdapter, douAdapter, workuaAdapter, asynqClient, enrichDelay, enrichDelays)
  ```
- [X] T013 Register `adapters.WorkUaAdapter{}` in the seed registry in `apps/api/cmd/seed/main.go` (~line 64) — omitting this breaks `make seed` for any fixture FK-ing the `workua` source row

**Checkpoint**: `go build ./...` passes; `go test ./internal/enrichment/...` still passes (existing djinni/dou timing unchanged); work.ua appears on the Sources page with a health dot and no config inputs. Commit here.

---

## Phase 3: User Story 1 - Discover work.ua jobs from a keyword search (Priority: P1) 🎯 MVP

**Goal**: Enable work.ua, run a keyword search, see jobs in the job list.

**Independent Test**: Enable only work.ua, run a keyword search, confirm jobs appear with title, company, and a working link. No other source needed.

### Tests for User Story 1

> Write these first; they must fail before implementation.

- [X] T014 [P] [US1] Write `TestWorkUaParseCards_EmptyVsBroken` in `apps/api/internal/jobsources/adapters/workua_test.go` — assert `workua_empty.html` and garbage HTML are distinguishable. **This is the FR-007 core**: work.ua emits a `no-results` element plus the text `За вашим запитом` on genuinely-empty searches, so "nothing matched" vs "markup broke" is a deterministic check. **Assert on the marker's presence, never on card count being zero** — the fixture URL is empty today but could return cards on a future re-capture
- [X] T015 [P] [US1] Write `TestWorkUaKey` and `TestWorkUaKind` in `apps/api/internal/jobsources/adapters/workua_test.go` asserting `"workua"` / `dto.SourceKindScrape`
- [X] T016 [P] [US1] Write `TestWorkUaParseCards` in `apps/api/internal/jobsources/adapters/workua_test.go` over `workua_list.html` — assert ≥10 jobs, every job has non-empty `Title`, non-empty `Company`, absolute `URL` prefixed `https://www.work.ua`, non-nil `ExternalID`, and `SourceKey == "workua"` (SC-003)
- [X] T017 [P] [US1] Write `TestWorkUaCyrillicRoundTrip` in `apps/api/internal/jobsources/adapters/workua_test.go` — assert a Cyrillic title survives parsing byte-identical and that truncated `Raw` HTML contains no `utf8.RuneError` (FR-005)
- [X] T018 [P] [US1] Write `TestWorkUaSearchURL` in `apps/api/internal/jobsources/adapters/workua_test.go` — table test asserting URL construction: plain keywords → `/jobs/?search=...`; `Remote: true` → `/jobs-remote/?search=...`; multi-word keywords URL-encoded (FR-003, research Decisions 2-3)

### Implementation for User Story 1

- [X] T019 [US1] Implement `parseWorkUaCards(doc *goquery.Document) []dto.NormalizedJob` in `apps/api/internal/jobsources/adapters/workua.go` — select `div.card.job-link`; title+href from `h2 a[href^="/jobs/"]`; company from `a[href^="/jobs/by-company/"]` falling back to `"Unknown"` (FR-006); skip any card with empty title or href; resolve href absolute against `workuaBaseURL`
- [X] T020 [US1] Add external-ID extraction to `parseWorkUaCards` in `apps/api/internal/jobsources/adapters/workua.go` — numeric segment of `/jobs/{id}/`, falling back to the card's `data-id` attribute; `nil` if neither parses (FR-004, drives dedup)
- [X] T021 [US1] Add `Remote`, `SalaryRaw`, `Location` extraction to `parseWorkUaCards` in `apps/api/internal/jobsources/adapters/workua.go` — `Remote` via `workuaRemoteRe = regexp.MustCompile(`(?i)remote|віддалено|дистанційно`)` over card text; absent salary/location → `nil` (FR-006)
- [X] T022 [US1] Retain truncated card HTML into `Raw` in `apps/api/internal/jobsources/adapters/workua.go` using `strutil.Truncate(itemHTML, 4000)` — rune-safe, never a byte slice; add the same warning comment `djinni.go:141` carries about Cyrillic and multi-byte splits (FR-005)
- [X] T023 [US1] Implement `Search(ctx, query, config)` in `apps/api/internal/jobsources/adapters/workua.go` — build `https://www.work.ua/jobs/?search={url.QueryEscape(keywords)}`, or `/jobs-remote/?search=...` when `query.Remote` is true; delegate to `d.Scraping.FetchHTML` (follows work.ua's redirect to the canonical slug automatically)
- [X] T024 [US1] Implement the zero-results branch in `Search` in `apps/api/internal/jobsources/adapters/workua.go` — return `(nil, nil)` plus a `slog.Warn` whose message distinguishes "no matches" (page has the `no-results` marker) from "markup may have changed" (no cards, no marker), per T014 (FR-007). Never return an error for zero results
- [X] T025 [US1] Add keyword-search pagination to `Search` in `apps/api/internal/jobsources/adapters/workua.go` — follow `?page=N` up to `workuaMaxSubscriptionPages`, sleeping `WorkUaMinDelay` between pages; stop on empty page, on a repeated first-card URL, or on `ctx.Done()` (FR-011, mirrors `djinni.go:91`)

**Checkpoint**: `go test ./internal/jobsources/adapters/ -run WorkUa` passes. US1 is fully functional and demoable — this is the MVP. Commit here.

---

## Phase 4: User Story 3 - Full job descriptions filled in after discovery (Priority: P3)

**Goal**: Discovered work.ua jobs get their full description, salary, location, and posting date shortly after discovery.

**Independent Test**: Discover a work.ua job, wait for background enrichment, confirm the description grows from teaser to full text and the posting date is populated.

### Tests for User Story 3

- [X] T026 [P] [US3] Write `TestWorkUaFetchDetailParse` in `apps/api/internal/jobsources/adapters/workua_test.go` over `workua_detail.html` — assert `Description` is non-empty and substantially longer than any card teaser from `workua_list.html`
- [X] T027 [P] [US3] Write `TestWorkUaPostedAt` in `apps/api/internal/jobsources/adapters/workua_test.go` — **finding H1's regression guard**. Assert `parseWorkUaPostedAt("2026-07-16 02:29:02")` returns an RFC3339 string, and add an explicit assertion that `dbutil.TimestampFromPtr` returns `Valid: true` for that output. Include a negative case proving the **raw** attribute value yields `Valid: false`, documenting why normalisation is required
- [X] T028 [P] [US3] Write `TestWorkUaDetailMissingFields` in `apps/api/internal/jobsources/adapters/workua_test.go` — assert HTML lacking salary/location/date yields `nil` for those fields and no error (FR-006)

### Implementation for User Story 3

- [X] T029 [US3] Define `WorkUaDetailPatch` struct in `apps/api/internal/jobsources/adapters/workua.go` with fields `Description string`, `SalaryRaw *string`, `Location *string`, `Remote bool`, `PostedAt *string`, `Raw map[string]string` — shape per data-model.md, mirroring `DjinniDetailPatch`

### ⚠️ Finding H1 — posting date needs normalisation, not pass-through

> **Why T030 exists**: work.ua renders the posting date as `<time datetime="2026-07-16 02:29:02">` — space separator, no timezone. `dbutil.TimestampFromPtr` (`apps/api/internal/dbutil/uuid.go:70`) tries only RFC3339 and date-only `2006-01-02`, and on failure **returns a zero timestamp silently, with no error**. Passing the raw attribute through would make `PostedAt` quietly NULL forever: FR-010 would look implemented, tests not specifically asserting the date would pass, and the failure would be invisible. The adapter must normalise.

- [X] T030 [US3] Implement `parseWorkUaPostedAt(datetimeAttr string) *string` in `apps/api/internal/jobsources/adapters/workua.go` — parse with layout `"2006-01-02 15:04:05"`, return `time.Format(time.RFC3339)`; return `nil` on empty input or parse failure (never an error, per FR-006). Add a comment naming `dbutil.TimestampFromPtr`'s accepted layouts as the reason this conversion exists
- [X] T031 [US3] Implement `FetchDetail(ctx, jobURL, config)` in `apps/api/internal/jobsources/adapters/workua.go` — parse title from `h1#h1-name`, description from `#job-description`, company from `a[href^="/jobs/by-company/"]`, salary from the `li` containing `span.glyphicon-hryvnia-fill`, and **posted date from `time[datetime]` piped through `parseWorkUaPostedAt` (T030)**; each selector defensive with fallbacks; a missing field yields a zero value, never an error (FR-010)
- [X] T032 [US3] Retain truncated body HTML into `WorkUaDetailPatch.Raw` in `apps/api/internal/jobsources/adapters/workua.go` via `strutil.Truncate(bodyHTML, 8000)` (FR-005)
- [X] T033 [US3] Add the `workua adapters.WorkUaAdapter` field to `Handler` in `apps/api/internal/enrichment/handler.go` (~line 26) — the `NewHandler` param was already added in T009
- [X] T034 [US3] Add `case "workua": return h.enrichWorkUa(ctx, payload, uid, job)` to the `switch job.SourceKey` dispatch in `apps/api/internal/enrichment/handler.go` (~line 81)
- [X] T035 [US3] Implement `enrichWorkUa` in `apps/api/internal/enrichment/handler.go` — call `FetchDetail`, marshal `Raw`, call the existing `h.q.UpdateJobDetail` with `Description`/`SalaryRaw`/`Location`/`Remote`/`Raw`/`PostedAt` (via `dbutil.TimestampFromPtr`), then `h.enqueueMatch`. Model on `enrichDjinni` (~line 92). **No sqlc query change** — `UpdateJobDetail` already takes exactly these fields
- [X] T036 [US3] Apply the crawl-delay floor at the top of `enrichWorkUa` in `apps/api/internal/enrichment/handler.go` — **finding C1's enforcement point**, depends on T009:
  ```go
  delay := h.delayFor("workua")
  if delay < adapters.WorkUaMinDelay {
      delay = adapters.WorkUaMinDelay
  }
  time.Sleep(delay)
  ```
  Clamping here rather than at construction means no future config path, env typo, or new call site can drop work.ua below its published crawl-delay (FR-011, SC-008)
- [X] T037 [US3] Make `enrichWorkUa` fail soft in `apps/api/internal/enrichment/handler.go` — a `FetchDetail` error logs `slog.Warn` and returns `nil`, preserving the teaser description and never blocking other jobs' enrichment (Story 3 scenario 2)

**Checkpoint**: US1 + US3 both work. Descriptions **and posting dates** land per SC-005's revised timing. Commit here.

---

## Phase 5: User Story 4 - Reuse a saved filter from the board itself (Priority: P4)

**Goal**: Paste a public work.ua search URL into a subscription; ingest everything it returns across pages.

**Independent Test**: Paste a work.ua search URL into a subscription, run it, confirm listings from every page appear.

**Scope clarification**: A work.ua "subscription URL" means **any public search-results URL the user pastes** — not an authenticated saved-search page (`/jobseeker/my/` is `Disallow`ed by robots.txt, and we honor that). This story earns its place because work.ua search URLs encode filters `dto.SearchQuery` cannot express — verified live: `region`, `category`, `employment`, `experience`, `salaryfrom`, `salaryto`. `/jobs-kyiv-php/?salaryfrom=4` is a real query the keyword path cannot reach.

### Tests for User Story 4

- [X] T038 [P] [US4] Write `TestWorkUaSubscriptionMalformedURL` in `apps/api/internal/jobsources/adapters/workua_test.go` — assert `Search` with `SubscriptionURL: "://not a url"` errors and the message contains the offending URL (FR-013)
- [X] T039 [P] [US4] Write `TestWorkUaSubscriptionPaginationStops` in `apps/api/internal/jobsources/adapters/workua_test.go` — assert pagination halts when a page's first card repeats the previous page's first card (FR-012 loop guard)

### Implementation for User Story 4

- [X] T040 [US4] Implement `scrapeWorkUaSubscription(ctx, subURL)` in `apps/api/internal/jobsources/adapters/workua.go` — `url.Parse` the input, returning `fmt.Errorf("workua: invalid subscription url %q: %w", subURL, err)` on failure (FR-013, mirrors `djinni.go:86`)
- [X] T041 [US4] Add paged ingestion to `scrapeWorkUaSubscription` in `apps/api/internal/jobsources/adapters/workua.go` — set `?page=N` on the parsed URL, reuse `parseWorkUaCards`, stop on empty page / repeated first-card URL / `workuaMaxSubscriptionPages` / `ctx.Done()`, sleeping `WorkUaMinDelay` between pages
- [X] T042 [US4] Branch `Search` to `scrapeWorkUaSubscription` when `query.SubscriptionURL != ""` in `apps/api/internal/jobsources/adapters/workua.go` — ignoring `Keywords`/`Remote` in that path, per contracts/adapter-contract.md

**Checkpoint**: All in-scope stories functional. Commit here.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [X] T043 [P] Add `TestLive_WorkUa` to `apps/api/internal/jobsources/adapters/live_smoke_test.go` under the existing `//go:build live` tag — real search for `"php"`, assert >0 jobs, `t.Logf` the count. This is the only test that catches work.ua markup drift
- [X] T044 [P] Add a package doc comment to `apps/api/internal/jobsources/adapters/workua.go` in the house style of `djinni.go:27` — note work.ua is a general-purpose (not dev-only) Ukrainian board, needs no credentials, and that the 2s pacing is mandated by its published `Crawl-delay`
- [X] T045 Verify Constitution Principle I compliance — grep `apps/api/internal/jobsources/adapters/workua.go` for any non-GET request; there must be none (FR-015, read-only discovery)
- [X] T046 Run `cd apps/api && go test ./...` — the binding gate per Principle IV; the change is confined to `apps/api`, so `make test-lint` is optional
- [X] T047 Walk `quickstart.md` Levels 3-4 end-to-end against the running stack (`make seed && make dev`) — including the failure-mode table: blocked host, dead posting, malformed subscription URL

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately. T003 must follow T002 (needs a job URL from that fixture, plus the 2s delay).
- **Foundational (Phase 2)**: Depends on Setup. **BLOCKS all user stories.**
- **US1 (Phase 3)**: Depends on Foundational. No dependency on other stories.
- **US3 (Phase 4)**: Depends on Foundational — **specifically on T009**, without which T036 cannot be written and work.ua would silently enrich below its crawl-delay. Independently unit-testable against fixtures.
- **US4 (Phase 5)**: Depends on Foundational **and on T019-T022** (reuses `parseWorkUaCards`). The one real cross-story coupling.
- **Polish (Phase 6)**: Depends on all desired stories.

### Critical path for finding C1

`T006` (exports `WorkUaMinDelay`) → `T009` (per-source delay plumbing) → `T010` (config field) → `T012` (map wiring) → `T036` (floor clamp). Skipping any link leaves work.ua enriching at djinni's 1500ms, under the board's published crawl-delay.

### Within Each User Story

- Tests before implementation (they must fail first)
- Parsing helpers before `Search`
- `Search` before pagination
- Adapter before enrichment wiring

### Parallel Opportunities

- T004 runs parallel with T002/T003 (different fixture, independent URL)
- T010 runs parallel with T006-T009 (different file: `config.go`)
- All US1 tests (T014-T018) are parallel — all assert against fixtures, all fail until implementation lands
- T026/T027/T028 (US3 tests) parallel with each other
- T038/T039 (US4 tests) parallel with each other
- T043/T044 parallel (different files)
- **Caveat**: T019-T025 all touch `workua.go` and are strictly sequential despite sharing a phase. Same for T029-T032, and T033-T037 in `handler.go`. Do not parallelize same-file tasks.

### Story Independence

US1 alone is a shippable MVP: work.ua jobs discoverable via keyword search. US3 and US4 each add value without breaking it.

---

## Parallel Example: User Story 1 Tests

```bash
# All five US1 tests assert against fixtures — write them together, watch them all fail:
Task: "TestWorkUaParseCards_EmptyVsBroken in apps/api/internal/jobsources/adapters/workua_test.go"
Task: "TestWorkUaKey / TestWorkUaKind in apps/api/internal/jobsources/adapters/workua_test.go"
Task: "TestWorkUaParseCards in apps/api/internal/jobsources/adapters/workua_test.go"
Task: "TestWorkUaCyrillicRoundTrip in apps/api/internal/jobsources/adapters/workua_test.go"
Task: "TestWorkUaSearchURL in apps/api/internal/jobsources/adapters/workua_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 only)

1. Phase 1: Setup — fixtures captured
2. Phase 2: Foundational — per-source pacing in place, adapter registered, health dot green
3. Phase 3: US1 — keyword search works
4. **STOP and VALIDATE**: quickstart.md Levels 1-3 for US1
5. Ship it

### Incremental Delivery

1. Setup + Foundational → source appears in dashboard
2. + US1 → **MVP**: work.ua jobs discoverable
3. + US3 → full descriptions and posting dates
4. + US4 → filtered URL subscriptions
5. Polish → live smoke test, principle checks

### Commit Boundaries

Per your standing preference for small single-feature commits split by feature rather than bundled, commit at each phase checkpoint — five commits, not one:

1. `test(jobsources): add work.ua html fixtures` (Phase 1)
2. `refactor(enrichment): per-source detail delay` + `feat(jobsources): register work.ua adapter` (Phase 2 — **two commits**; the handler refactor is its own change and touches existing sources, so it stays separate from the new adapter)
3. `feat(jobsources): add work.ua keyword search` (Phase 3 — MVP)
4. `feat(enrichment): add work.ua detail enrichment` (Phase 4)
5. `feat(jobsources): add work.ua subscription urls` (Phase 5)

Phase 6's tasks fold into whichever commit they touch, except T043 (live smoke) which stands alone.

---

## Notes

- `[P]` = different files, no dependencies. Same-file tasks are never `[P]`, even in one phase.
- Every work.ua HTTP request obeys the 2s `Crawl-delay`. It is a published constraint, not a tuning knob — it's what keeps this adapter on the right side of the line robota.ua draws with Cloudflare. T036 clamps it so it cannot be lost to a config mistake.
- Zero results must never be an error (FR-007). work.ua's `no-results` marker makes the empty-vs-broken distinction exact — use it rather than inferring.
- Posting dates must be normalised, never passed through (T030). `dbutil.TimestampFromPtr` fails **silently** on work.ua's format.
- No migration, no `packages/shared` change, no tygo/sqlc regeneration. If you find yourself writing one, stop and re-read data-model.md.
