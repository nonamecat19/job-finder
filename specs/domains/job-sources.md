# Domain: Job Sources

Consolidates **002** Indeed, **003** RemoteOK, **004** Glassdoor, **005** JobLeads,
**010** Wellfound, **011** Himalayas, **012** Jobgether, **013** ATS boards,
**015**/**016** Djinni search modes, **022** Djinni scraping enhancement.

Implementation: `apps/api/internal/jobsources/`. How it works: [`docs/ingestion/job-sources.md`](../../docs/docs/ingestion/job-sources.md).

---

## 1. The shared adapter contract

Features 002, 003, 004, 005, 010, 011 and 012 were each written as a standalone spec with
an identical 17-requirement skeleton, differing only in the provider name. That skeleton is
stated **once** here. It binds every job source, present and future.

A job source MUST:

| # | Requirement |
|---|---|
| JS-01 | Be a first-class source: selectable and manageable on the Sources screen alongside every other source. |
| JS-02 | Retrieve listings for a user-defined search configuration (saved search or subscription). |
| JS-03 | Carry, on each ingested listing, whatever the source publishes of: title, company, location, remote flag, posting URL, description, salary, posted date. |
| JS-04 | Recognise previously ingested listings across runs and not duplicate them in the feed. |
| JS-05 | Be independently enable/disable-able, with the setting persisted. |
| JS-06 | Support an on-demand run and an on-demand health check. |
| JS-07 | Record each run's outcome — succeeded, failed, or partial — with found/new counts and, on failure, a reason. |
| JS-08 | Retain listings already collected when a run is interrupted or fails partway. |
| JS-09 | Support retrieving the full detail of an individual listing (enrichment), separate from the list pass. |
| JS-10 | Pace its requests per host — see [`retrieval-and-ingestion.md`](retrieval-and-ingestion.md). Pacing is owned centrally; an adapter never implements its own. |
| JS-11 | Distinguish "source returned no matching listings" from "source could not be read" in run outcomes. |
| JS-12 | Flow ingested listings through the same downstream matching, scoring and enrichment as every other source. |
| JS-13 | Never submit an application, send a message, or take any action on a listing. (Constitution I.) |
| JS-14 | Accept its search configuration as an operator-supplied URL or saved subscription. |
| JS-15 | Reject, at save time with a human-readable message, a subscription URL that is not recognisable for that source. |
| JS-16 | Expose its own identity in run records, source listings and feed attribution — never borrow another source's. |
| JS-17 | Identify its outbound requests with a descriptive client identity. See `retrieval-and-ingestion.md` § Browser fidelity. |

Quality bars that applied per-source and now apply to all:

- ≥95% of ingested listings have a non-empty title, company and URL.
- ≥90% of listings have a description longer than a stub after enrichment.
- Re-running the same configuration immediately adds **zero** new feed entries.
- 100% of unrecognisable subscription URLs are rejected at save time with a reason.
- Adding a source does not increase the median end-to-end ingestion cycle time for
  pre-existing sources.

**Adding a source is one adapter file + one registry entry.** No downstream change. Enforced
by `domain.Adapter` in `apps/api/internal/jobsources/domain/adapter.go` and the variadic
`domain.NewRegistry(...)` call in `apps/api/cmd/server/compose.go`.

## 2. Source register

Keys are the `Adapter.Key()` values. "Ingest" = registered in `domain.NewRegistry` and
therefore runnable; "Enrich" = wired into `enrichment.NewHandler` for detail fetch.

| Key | Access | Credentials | Ingest | Enrich | Spec |
|---|---|---|---|---|---|
| `adzuna` | Public API | `ADZUNA_APP_ID` / `ADZUNA_APP_KEY` / country | ✅ | — | baseline |
| `jooble` | Public API | `JOOBLE_API_KEY` | ✅ | — | baseline |
| `remotive` | Public API | none | ✅ | — | baseline |
| `arbeitnow` | Public API | none | ✅ | — | baseline |
| `robota` | Public API | none | ✅ | — | baseline |
| `jobspy` | External service | `JOBSPY_URL` | ✅ | — | baseline |
| `djinni` | Scrape + login | `DJINNI_EMAIL` / `DJINNI_PASSWORD` | ✅ | ✅ | 015, 016, 022 |
| `dou` | Scrape | none | ✅ | ✅ | baseline |
| `workua` | Scrape | none | ✅ | ✅ | baseline |
| `greenhouse` | Employer board | none | ✅ | — | 013 |
| `lever` | Employer board | none | ✅ | — | 013 |
| `ashby` | Employer board | none | ✅ | — | 013 |
| `workable` | Employer board | none | ✅ | — | 013 |
| `smartrecruiters` | Employer board | none | ✅ | — | 013 |
| `indeed` | Scrape | none | ❌ | ✅ | 002 |
| `remoteok` | Scrape | none | ❌ | ✅ | 003 |
| `glassdoor` | Scrape + escalation | none | ❌ | ✅ | 004 |
| `jobleads` | Scrape + login | `JOBLEADS_EMAIL` / `JOBLEADS_PASSWORD` | ❌ | ✅ | 005 |
| `wellfound` | Scrape + escalation | none | ❌ | ✅ | 010 |
| `jobgether` | Scrape + escalation | none | ❌ | ✅ | 012 |
| `himalayas` | Scrape | none | ❌ | ❌ | 011 |

> ### ⚠ Known drift — six sources cannot run, one is unreachable
>
> Verified against `apps/api/cmd/server/compose.go` at the time this doc was written.
>
> `indeed`, `remoteok`, `glassdoor`, `jobleads`, `wellfound` and `jobgether` are constructed
> only inside `composeEnrichment` (compose.go:362-367) and are **absent from the
> `domain.NewRegistry(...)` call** in `composeJobSources` (compose.go:136-150). Since ingest
> resolves its adapter through `registry.Get(payload.SourceKey)`
> (`jobsources/interfaces/worker/handler.go:169`), a run for any of these keys fails with
> `AdapterNotRegisteredError`. They cannot be enabled, listed, health-checked, or run — which
> contradicts **JS-01**, **JS-02**, **JS-05** and **JS-06** for each of them. Their detail-page
> enrichment does work, for jobs that reached the DB some other way.
>
> `himalayas` (011) is worse: `HimalayasAdapter` is referenced nowhere outside its own source
> and test files. It is dead code — not in the registry, not in enrichment.
>
> Seed data (`internal/seed/subscriptions.go:37-70`) still creates subscriptions for
> `indeed`, `remoteok`, `glassdoor`, `jobleads`, `wellfound` and `jobgether`, so a seeded dev
> stack presents six subscriptions that cannot execute.
>
> This is recorded, not fixed — closing it is a code change, out of scope for a docs pass.
> Either register the seven adapters, or mark those specs withdrawn and delete the seed rows.

## 3. Employer ATS boards (013)

Five vendors — Greenhouse, Lever, Ashby, Workable, SmartRecruiters — read open postings
straight from employer-hosted boards. They share one `roster.Service`
(`internal/jobsources/roster/`), health-checked through the checkers map that
`adapters.NewBoardAdapters()` returns.

**Roster**

- 013-FR-008: a roster of registered employer boards, each identified by employer + vendor.
- 013-FR-009/010: the system proposes roster candidates by recognising supported board links
  in already-ingested listings; the user accepts or rejects, and a rejected candidate is not
  proposed again.
- 013-FR-011: a user registers a board by pasting its URL.
- 013-FR-012: removal never deletes jobs already ingested from that board.
- 013-FR-013: no duplicate registration of a board already in the roster.
- 013-FR-014: an employer whose board yields no postings across consecutive runs is flagged.
- 013-FR-024: the roster view shows each employer's vendor, last successful read and status.

**Merging with aggregator listings**

- 013-FR-015: a posting read from an employer board is recognised as the same job as an
  aggregator listing for the same opening.
- 013-FR-016: on merge, the employer board's apply URL wins over the aggregator's.
- 013-FR-017: on merge, **all** user-created state on the existing job is preserved. SC-007
  states it flatly: no job's user-created state is lost to a merge.
- 013-FR-018: distinct openings are never merged, including identical titles at the same
  employer.

**Run behaviour**

- 013-FR-004: postings arriving with a full description need no enrichment pass.
- 013-FR-006/007: per-host pacing applies, and a run caps both employers read and postings
  taken.
- 013-FR-019: per-employer failures are isolated — one unreadable board never aborts a run
  or reduces another employer's results.
- 013-FR-020/021/023: per-employer outcomes are reported; a run that read zero employers
  successfully is **not** reported as successful; each run reports employers read and
  postings found.
- 013-SC-002: zero runs require credentials, a stored session, or challenge-solving. Boards
  are public by design — if a vendor starts requiring auth, that is a new decision, not a
  quiet adapter change.

## 4. Djinni (015, 016, 022)

Djinni is the only source with two distinct search-URL grammars and a logged-in session.

**Two modes, one adapter (016-SC-008)**

- Preset search and basic search are both accepted as public Djinni URLs; the adapter
  recognises which mode a saved URL is (015-FR-002) and both resolve through a single
  pagination implementation.
- Both reuse the same login-aware fetch path, job-card parsing and detail enrichment
  (015-FR-005/006, 016-FR-014).
- Both paginate correctly for single-page and multi-page result sets (015-FR-004,
  016-FR-003/004).
- A URL that is neither a valid preset nor a valid basic search is rejected at save time
  with a human-readable message (015-FR-007, 016-FR-008).

**Removed by 016** — the legacy logged-in-dashboard subscription mode, its configuration
fields and environment variables (016-FR-006/007), plus a one-time migration deleting every
pre-existing subscription of the removed shape (016-FR-009). Do not reintroduce a third
Djinni mode without revisiting 016-SC-008.

**Subscription display (015-FR-008..012, 016-FR-010..013)**

Every query parameter present in a saved URL is shown. Experience levels that form a
contiguous run are collapsed to a range, repeats are deduplicated, and absent parameters are
omitted cleanly rather than rendered as empty labels.

**Field extraction (022)**

| Field | Rule |
|---|---|
| Company name | Detail-page value takes precedence over list-page value (022-FR-001). Target ≥95% coverage. |
| Years of experience | Regex over the description, English (`N+ years`) **and** Ukrainian forms (022-FR-002). Target ≥80% of listings that state it. |
| English level | Regex over the description, English and Ukrainian forms, A1–C2 (022-FR-003). Target ≥80%. |
| Salary analytics estimate | Read from the detail page's salary-analytics widget when present (022-FR-004), stored **separately** from the employer-disclosed salary with a flag distinguishing the two (022-FR-006). |

022-FR-009 is the governing rule for all four: a per-field extraction failure MUST NOT fail
the job's ingestion. Partial results with missing fields are the expected case, and the job
detail UI omits an absent field's component rather than rendering a broken slot
(022-FR-008).

## 5. Source-specific deltas

Everything not stated here is the shared contract in § 1.

| Source | Delta |
|---|---|
| Indeed (002) | The pasted search URL's country/region is honoured rather than a global default (002-FR-014). Retrieval is a direct Indeed integration owned by this source, not a shared scraper (002-FR-017). |
| Glassdoor (004) | A block/challenge response is a distinct, reported outcome — not folded into a generic failure (004-FR-018). |
| JobLeads (005) | Account credentials, if supplied, are stored encrypted at rest (005-FR-018) and an invalid-credentials outcome is reported distinctly (005-SC-005). |
| Wellfound (010), Jobgether (012) | Use the retrieval escalation ladder; otherwise the plain contract. |
| Himalayas (011) | No delta at all — the spec is the shared contract verbatim. |
