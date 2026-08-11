# Domain: Job Sources

Consolidates **002** Indeed, **003** RemoteOK, **004** Glassdoor, **005** JobLeads,
**010** Wellfound, **011** Himalayas, **012** Jobgether, **013** ATS boards,
**015**/**016** Djinni search modes, **022** Djinni scraping enhancement, and **043**'s move
of every adapter into the scraper library.

Implementation: **the adapters live in `github.com/nonamecat19/jobscraper`**
(`adapter/` for the framework, `adapters/` for the 25 site adapters) since 043 — see
[`codebase-structure.md`](codebase-structure.md) § 5. `apps/api/internal/jobsources/` keeps
the application services, the sqlcgen repositories, the roster, the HTTP handlers and the
worker/scheduler. How it works: [`docs/ingestion/job-sources.md`](../../docs/docs/ingestion/job-sources.md).

**Every rule below binds regardless of which repository the code sits in.** A source's
obligations did not change when the adapters moved; only their import path did.

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
by `adapter.Adapter` in the library's `adapter/adapter.go` and the variadic
`adapter.NewRegistry(...)` call in `apps/api/cmd/server/compose.go`. Since 043 the adapter
file lands in the **library** and only the registry entry is an app-side edit.

### 1.1 The Go adapter surface

Every per-source spec (002, 003, 004, 005, 010, 011, 012) shipped an identical adapter
contract. Stated once, it binds every adapter:

```go
// package adapter — github.com/nonamecat19/jobscraper/adapter
type Adapter interface {
    Key() string
    Kind() model.SourceKind
    Search(ctx context.Context, query model.SearchQuery, config map[string]any) ([]model.NormalizedJob, error)
    HealthCheck(ctx context.Context, config map[string]any) (bool, error)
}
```

The optional interfaces an adapter may also satisfy, and the framework's helpers, come from
the same package (043-FR-004):

```go
type DetailNeeder interface   { NeedsDetail() bool }
type Credentialed interface   { UsesUserAccount() bool }
type EmployerReporter interface { LastRunDetail() []EmployerRunOutcome }

type PostingReader interface {
    MatchesPostingURL(rawURL string) bool                                                   // no I/O, never panics
    ReadPosting(ctx context.Context, rawURL string, config map[string]any) (model.NormalizedJob, error)
}

func NewRegistry(adapters ...Adapter) *Registry
func AsPostingReader(a Adapter) (PostingReader, bool)
func NeedsDetail(a Adapter) bool
func IsCredentialed(a Adapter) bool

type SourceNotFoundError struct{ Key string }
type AdapterNotRegisteredError struct{ Key string }
```

Six rules bind a `PostingReader`, and they moved with the interface because the app's
enrichment path depends on all six: `MatchesPostingURL` does no I/O and never panics on
malformed input; it returns false for search URLs on its own host; `ReadPosting` returns
partial results rather than erroring when the page loads but some fields are missing, and
errors only when the page could not be read at all; it honours the context deadline and
returns `context.DeadlineExceeded` wrapped no deeper than `errors.Is` can see; it sets
`SourceKey` to its own `Key()` and resolves the URL to absolute canonical form; and it uses
the same retrieval path as the adapter's other methods, so pacing and the ladder apply.

**`dto.NormalizedJob` and friends still exist app-side as aliases** of the library's `model`
types (`internal/dto/scraper_aliases.go`, 043-FR-003), so every app importer and the tygo
chain to `packages/shared` read exactly as before — the JSON shapes the dashboard sees did
not change (043-SC-005).

| Member | Contract |
|---|---|
| `Key()` | The source key, constant, no receiver state. It is the value used in `SourceKey` on every job the adapter returns, in `/api/sources/{key}` paths, and in `Registry.Get`. |
| `Kind()` | `model.SourceKindAPI` for JSON feeds (`remoteok`, `himalayas`), `model.SourceKindScrape` for HTML (`indeed`, `glassdoor`, `jobleads`, `jobgether`, `djinni`, `dou`, `workua`). |
| `Search` — precondition | `query.SubscriptionURL != ""`. **Keyword search is out of scope for every scraped source.** An empty subscription URL returns `fmt.Errorf("<key> keyword search not implemented — use subscription URL instead")` — the same message shape across all of them. |
| `Search` — success | `[]dto.NormalizedJob` with `SourceKey` set on every element. **An empty slice with a nil error is the valid "zero matching listings" result** and MUST stay distinguishable from a non-nil error ("could not be interpreted"). This is JS-11, mechanically. |
| `Search` — partial failure | If pages 1..N succeed and page N+1 fails, return the N pages' jobs with a **nil** error. Never `nil, err` after partial progress unless page 1 itself failed. This is JS-08, mechanically. |
| `Search` — pacing | At least 500 ms between two HTTP requests inside one call, and a fixed page cap per call (`<source>MaxSubscriptionPages`, 50 for the scraped sources). |
| `HealthCheck` | Lightweight reachability probe. Returns `(false, nil)` for a normal unreachable/blocked/unauthorized case — **never a non-nil error for that case.** The bool alone does not distinguish unreachable from unauthorized; a caller needing that distinction reads `Search`'s error message. |

**Detail fetch is not part of `Adapter`.** Each enrich-capable adapter carries its own
`FetchDetail(ctx, jobURL string, config map[string]any) (<Source>DetailPatch, error)`,
called directly by `enrichment.Handler`. The patch struct is per-source but converges on
`Description`, `SalaryRaw`, `PostedAt`, `Available`, `Raw`. Two disposals of a vanished
listing exist, and the difference is deliberate:

- **Indeed** returns a non-nil error when the detail page 404s; the caller logs a warning
  and returns `nil` to the queue rather than retrying.
- **RemoteOK, JobLeads, Jobgether** return `DetailPatch{Available: false}, nil` — not an
  error — and the caller marks the listing unavailable.

In both dispositions the summary-level fields captured at ingestion are left untouched.

**Wiring a source touches exactly these call sites**, none of which change signature:
`adapter.NewRegistry(...)` in `compose.go`; the enrich-eligibility `SourceKey` check in
`ingestion.Handler.persistIfNew`; the `switch job.SourceKey` in
`enrichment.Handler.ProcessTask`; the `enrichment.NewHandler(...)` parameter list; the
per-source arm of `subscriptions.Service.validateSubscriptionURL` (JS-15); and, only for
credentialed sources, `config.Config` plus its secret list.

### 1.2 The reused REST surface

**No job source has ever added an HTTP endpoint.** Every one is consumed through the same
routes with its key substituted:

| Method | Path | Behaviour |
|---|---|---|
| `GET` | `/api/sources` | The source appears once its `JobSource` row exists (lazily created) or with registry defaults. |
| `PUT` | `/api/sources/{key}` | Enable/disable and config patch. A stored `sessionCookie` is system-managed, never operator-editable. |
| `POST` | `/api/sources/{key}/test` | Invokes `HealthCheck`. |
| `POST` | `/api/sources/{key}/run` | Invokes `Search` with no subscription — the keyword path, which errors for every scraped source by design. Operators use the subscription run instead. |
| `POST` | `/api/sources/{key}/enrich` | Backfill sweep, for enrich-capable sources only. |
| `POST` | `/api/subscriptions` `{sourceKey, url}` | Create; validates `url` per JS-15. |
| `POST` | `/api/subscriptions/{id}/run` | Runs `Search` with that subscription's URL. |

### 1.3 Subscription URL validation

`subscriptions.Service.validateSubscriptionURL` holds one arm per source. The accepted host
set is the contract:

| Source | Accepted hosts | Extra predicate |
|---|---|---|
| `djinni` | `djinni.co`, `www.djinni.co` | path `/jobs` or `/jobs/`; query `search_type=basic-search` — see § 4 |
| `remoteok` | `remoteok.com`, `remoteok.io` | — |
| `jobleads` | `jobleads.com`, `*.jobleads.com` | — |
| `jobgether` | `jobgether.com`, `*.jobgether.com` | — |
| `himalayas` | `himalayas.app`, `*.himalayas.app` | path `/jobs…` **and** a non-empty `categories` query parameter |
| `indeed`, `glassdoor` | the source's own host | — |

## 2. Source register

Keys are the `Adapter.Key()` values. "Ingest" = registered in `adapter.NewRegistry` and
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
> only inside `composeEnrichment` (compose.go:402-407) and are **absent from the
> `adapter.NewRegistry(...)` call** in `composeJobSources` (compose.go:135-151). Since ingest
> resolves its adapter through `registry.Get(payload.SourceKey)`
> (`jobsources/interfaces/worker/handler.go:158`), a run for any of these keys fails with
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

**Since 043 the board adapters live in the library and persist through a port** (043-FR-010).
They moved **whole** — roster concerns included, no adapter was split — and every DB call
they used to make now goes through `rosterport.RosterPort`, an 11-method interface in
library-owned types. `roster.Service` implements it and owns the whole `pgtype` translation:
board IDs cross the boundary as `string` UUIDs, timestamps as `*time.Time`. The library
declares no `pgx`, `pgtype` or `sqlcgen` dependency (043-FR-002), so a consumer can run the
board adapters against an in-memory roster and no database.

The port deliberately includes the **candidate/discovery methods that no library adapter
calls** (`ListBoardCandidates`, `GetBoardCandidate`, `GetBoardCandidateByID`,
`InsertBoardCandidate`, `DecideBoardCandidate`, `ListApplyURLsForDiscovery`). That keeps
`roster.Service` the single implementer instead of forcing a second struct for the app-only
half of the surface. `roster/view.go` (dashboard DTOs) and `roster/candidates.go` (discovery
orchestration) stay app-side and call those methods on the same struct.

The board-run helpers moved with the adapters: `runBoardVendor`, `vendorHealthCheck`,
`healthCheckEmployer`, `classifyOutcome` and `NewBoardAdapters` are library-internal to
`jobscraper/adapters`. `NewBoardAdapters()` still returns the five adapters plus the
`map[string]EmployerHealthChecker`, and the app still passes that map into
`roster.NewService(q, checkers)` — the wiring shape 013 established did not change.

A boundary this mechanical is where a silent translation bug lives, so it carries its own
integration test app-side (`internal/jobsources/roster`, real database): the library's own
board tests run against an in-memory fake and cannot catch a mistake in the UUID/timestamp
conversion the app owns.

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
- 013-FR-003: a board posting normalises into the same job shape as every other source.
- 013-FR-005: a new vendor is one reader, with no change to shared ingestion, scoring or
  storage.
- 013-FR-020 names the five per-employer outcomes that must stay distinguishable: **read
  successfully, board not found, board unreadable or shape changed, refused by host, read
  but no open postings.** 013-SC-010 is the user-facing form of the same rule — an operator
  tells those four failure kinds apart from the Sources screen alone, without reading logs.
- 013-FR-016: on merge, every source the job was seen on is recorded, not just the winner.

**Roster HTTP surface** — the one place a source family did add endpoints. Implemented in
`internal/jobsources/interfaces/http/roster.go`; the five vendors themselves reuse the
`/api/sources` routes unchanged (013-FR-022).

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/roster` | List registered employer boards (013-FR-024). Each entry: `id`, `vendor`, `employerIdentifier`, `displayName`, `addedVia` (`proposed` \| `pasted`), `enabled`, `lastSuccessAt`, `lastPostingCount`, `stale`. |
| `POST` | `/api/roster` | Register by pasting a URL (013-FR-011). `201` returns the entry with `addedVia: "pasted"`. |
| `DELETE` | `/api/roster/{id}` | Remove an employer (013-FR-012). `204`. Ingested `Job` rows survive. |
| `GET` | `/api/roster/candidates` | Undecided candidates (013-FR-009). Each: `id`, `vendor`, `employerIdentifier`, `displayName`, `inferredFromJobId`, `state`. |
| `POST` | `/api/roster/candidates/{id}/accept` | Creates/enables the matching board row. `200`. |
| `POST` | `/api/roster/candidates/{id}/reject` | Marks it `rejected` — **terminal, never re-proposed.** `204`. |
| `POST` | `/api/roster/discover` | Runs candidate discovery over stored `Job` rows. Returns `{newCandidates, skippedAlreadyKnown}`. **Idempotent**: a second run neither duplicates `proposed` rows nor resurrects `rejected` ones. |

`POST /api/roster` rejects with `422` in two distinguishable shapes:
`{"error": "unsupported_vendor", …, "supportedVendors": [...]}` when the URL matches no
vendor, and `{"error": "unreadable", "message": "board did not respond with a valid posting
list"}` when the live read health-check fails. Registration validates readability **before**
adding — a board is never registered on faith.

Discovery reads only listings already stored. It never crawls the web looking for employers,
so it costs nothing and touches no third-party host.

## 4. Djinni (015, 016, 022)

Djinni went through two rounds. **015** added a public basic-search mode alongside the
existing logged-in `subs/{id}/` dashboard mode. **016** then deleted the dashboard mode
outright: after the rewrite, a preset search (`?search_type=basic-search`) is the *only*
supported Djinni subscription shape, and it needs no login. Where 015 and 016 conflict, 016
wins — 015's dual-mode requirements (015-FR-002, 015-FR-011, 015-FR-013) are void.

**The one supported shape (016-FR-001)**

```
https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote
```

Save-time validation accepts iff **all** hold: the URL parses; the host (case-insensitive,
trimmed) is `djinni.co` or `www.djinni.co`; the path is `/jobs` or `/jobs/` (trailing slash
tolerant); and the query parameter `search_type` equals exactly `basic-search`
(case-sensitive). Anything else is rejected at save time with a human-readable reason
(016-FR-008, 016-SC-007) — never deferred to fail at run time.

| Input | Result |
|---|---|
| `djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y…` | Accept |
| `www.djinni.co/jobs?search_type=basic-search&primary_keyword=Node.js` (no trailing slash) | Accept |
| `djinni.co/jobs/?primary_keyword=Golang` (no `search_type`) | Reject |
| `djinni.co/jobs/123-senior-go` (a single posting, not a search) | Reject |
| `djinni.co/my/dashboard/subs/42/` | Reject — **specific reason required**: only preset-search URLs are supported, dashboard URLs no longer are |
| `djinni.io/jobs?…`, `example.com/…` | Reject (wrong host) |

**Query-param preservation (016-FR-002).** The run issues the saved URL *exactly*. The
adapter sets `page=1` — overriding any `page=N` the operator saved, so a deep link is never
mistaken for a starting page — and leaves every other parameter byte-for-byte identical:
duplicate `exp_level` values, unrecognised keys, and their ordering. The adapter MUST NOT
interpret, normalise, drop or re-order any non-`page` parameter. An unrecognised extra
parameter is re-issued verbatim and is neither displayed nor a reason to reject the save.

**The run loop.** Parse the URL, set `page=1`, then loop up to `djinniMaxSubscriptionPages`
(50):

1. Fetch anonymously — no session cookie. The retrieval ladder may escalate to the browser
   or FlareSolverr rungs (see [`retrieval-and-ingestion.md`](retrieval-and-ingestion.md)).
2. Parse job cards into `dto.NormalizedJob` (`SourceKey="djinni"`, `ExternalID` = the path
   tail, `URL` absolute-resolved).
3. An empty card list **stops the loop as a success** — this is how the single-page case
   terminates (016-FR-003).
4. If this page's first card href equals the previous page's first card href, stop —
   the redirect-loop guard.
5. Otherwise append and continue.

Requests are sequential within a run; there is no parallel page fetch. Detail fetches are
paced by `DJINNI_DETAIL_DELAY_MS` (1500).

| Event | Verdict | Health |
|---|---|---|
| Block or challenge response | `blocked`, with a human-readable reason; listings already collected are retained | 3 consecutive failures → source flagged unhealthy |
| Every card fails to parse (upstream shape change) | "results returned but none interpretable" — **distinguishable from both "blocked" and "no matching jobs"** | same threshold |
| Legitimately zero results | `ok`, count 0 | healthy |
| Single page (page 2 empty) | `ok`, actual count, no loop, no error | healthy |
| Run interrupted (cancel, shutdown, timeout) | partial; listings gathered before the interruption are retained | — |

**The 016 migration (016-FR-009, 016-SC-009).** Goose migration
`00027_drop_djinni_dashboard_subs.sql` created an audit table `DjinniLegacySubAudit`, then
deleted every `Subscription` row with `sourceKey = 'djinni' AND url LIKE
'%/my/dashboard/subs/%'`, copying each deleted row into the audit table with `deletedAt`.
Down is a no-op — the deletion is irreversible and the audit rows *are* the record, so Down
does not drop the audit table. The migration touches no `JobSource` row, alters no column,
and cannot match a preset-search subscription. It is idempotent by construction. A dashboard
URL cannot be losslessly converted to a preset URL, so the operator recreates those searches
manually from the audit list.

> ### ⚠ Known drift — the 016 login teardown never happened
>
> 016-FR-006/007 required deleting the Djinni session-login path and its configuration.
> The code still carries `DjinniSession` — now in the library
> (`jobscraper/adapters/djinni_session.go`, constructed app-side in
> `cmd/server/platform.go`) — the `DJINNI_EMAIL` / `DJINNI_PASSWORD` config fields
> (`config/config.go:83`, listed as secrets in `config/defaults.go:61`), the `sessionCookie`
> config blob, and login-required errors in `jobscraper/adapters/djinni.go:59,63,72`. The § 2
> register reflects the code, not the spec. **043 relocated this drift without resolving it**;
> closing it is now a library change plus a config removal app-side.
>
> 016-SC-008 ("a maintainer search surfaces zero references to session login") is therefore
> **not met.** Either finish the teardown or revoke 016-FR-006/007 explicitly. Do not
> reintroduce a second Djinni search mode either way.

**Subscription display (016-FR-010..013).** `summarizeDjinniBasicSearch(url): string | null`
in `apps/dashboard/src/features/sources/djinniSearchSummary.ts` is the contract for the
Subscriptions row label. It gates on the same host/path/`search_type` predicates as the
server validator and returns `null` for anything the server would have rejected — so the
client never crashes on a shape the server did not accept. `SourcesPage.tsx` falls back to
the raw URL on `null`.

Output format: `"<keyword> · $<salary> · <expSummary> · <employment>"`, segments joined by
`" · "` in that **fixed order** — not URL order — with any absent segment omitted entirely
rather than rendered blank or as `"null"`.

`expSummary` (the range-vs-list rule, 016-FR-011/012) operates on the `exp_level` values as
a **set**, independent of the order they appear in the URL:

1. Deduplicate the tokens.
2. Each must end in `y` with a cleanly parseable integer prefix. **If any token does not,
   return the sorted unique raw tokens joined by `", "`** — never mis-collapse an
   unparseable set into a misleading range.
3. Sort the years ascending.
4. One value → `"<n> years"`. Consecutive run → `"<min>–<max> years"` with an **en-dash**
   (U+2013), not a hyphen. Non-consecutive → `"<a>, <b>, … years"`. Empty → `""`.

| Query | Summary |
|---|---|
| `…primary_keyword=Node.js&salary=3000&exp_level=2y&3y&4y&5y&employment=remote` | `Node.js · $3000 · 2–5 years · remote` |
| `…primary_keyword=Golang&salary=1500&exp_level=1y&2y&3y&employment=remote` | `Golang · $1500 · 1–3 years · remote` |
| `…primary_keyword=Golang` | `Golang` |
| `…primary_keyword=Go&exp_level=1y&exp_level=3y` (gap) | `Go · 1, 3 years` |
| `djinni.co/jobs/123-senior-go`, `djinni.co/my/dashboard/subs/42/` | `null` |

Salary is Djinni's single `salary` parameter — minimum monthly net in USD — displayed as
given, with no currency conversion or normalisation.

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

**The fields 022 added** (022-FR-005/006). They land on `dto.NormalizedJob` as optional
`omitempty` fields and surface on `GET /api/jobs/{id}` only — the list endpoint is
unchanged, because these are populated during detail enrichment. Jobs from non-Djinni
sources leave them null.

| Field | Type | Derivation |
|---|---|---|
| `experienceLevel` | string? | Regex over the description, e.g. `"2+ years"` |
| `experienceMinYears` | int? | Parsed minimum from `experienceLevel` |
| `englishLevel` | string? | Regex over the description, e.g. `"B1+"` |
| `salaryEstimateRaw` | string? | `div.salaries-info-link strong#salary-suggestion` on the detail page, e.g. `"$1500-3000"` |
| `salaryEstimateMin` / `Max` / `Currency` | int? / int? / string? | `ParseSalaryRaw()` over `salaryEstimateRaw` |
| `salaryIsEstimated` | bool (never null, default `false`) | Computed: `salaryEstimateMin != null` |

Three rules keep the estimate from contaminating the disclosed salary:

- `salaryIsEstimated` is `true` whenever the estimate fields are populated, **regardless of
  whether an employer-disclosed salary is also present.** Both coexist; the UI labels the
  estimate "Estimated" so the two are never confused.
- The `salaryEstimate*` fields are **not** fed into the salary inference pipeline. They are a
  separate signal.
- `salaryRaw` / `salaryMin` / `salaryMax` are untouched by 022.

**Extraction edge cases that shaped the regexes**

- The company name appears variously in `<title>`, an `<a>` tag, or a sidebar widget, and
  the three can disagree. The detail-page header wins over the list-page link text; when
  nothing is found anywhere, `company` falls back to `"Unknown"` and the raw HTML is kept
  for debugging.
- Experience: `"experience with React"` MUST NOT parse as a years requirement. Only
  quantified forms match — `"N+ years"`, `"N years of … experience"`, `"досвід від N
  років"`. When text extraction yields nothing, the `?exp=N` parameter on the
  salary-analytics URL is the structured fallback.
- English level covers both `"English level — B1+"` / `"English: Advanced"` and Ukrainian
  `"Англійська — B1"` / `"Рівень англійської: Intermediate"`.
- The salary-analytics widget is absent on many pages; its absence is not an error.

## 5. Source-specific deltas

Everything not stated here is the shared contract in § 1. Each source's FR-001..FR-017 are
the skeleton; only the requirements listed below are that source's own.

| Source | Delta |
|---|---|
| Indeed (002) | The pasted search URL's country/region is honoured rather than a global default (002-FR-014). Retrieval is a direct Indeed integration owned by this source, not a shared scraper (002-FR-017). A vanished detail page returns a **non-nil error** (the one source that does), which the enrichment caller logs and drops rather than retries. |
| RemoteOK (003) | `Kind()` is `dto.SourceKindAPI` — a JSON feed, not HTML. Every job carries `Remote: true`. One `Search` call issues **one** request: there is no pagination loop to pace, and no retry beyond that single fetch. `FetchDetail` re-reads the feed and matches by ID; a listing rotated out of the current payload returns `{Available: false}, nil`. |
| Glassdoor (004) | A block/challenge response is a distinct, reported outcome — not folded into a generic failure (004-FR-018). Page 1 blocked is fatal for the run; a later page blocked ends pagination with what was already collected, logged as a warning. |
| JobLeads (005) | The only source that **refuses to degrade to anonymous access.** With neither `JOBLEADS_EMAIL` nor `JOBLEADS_PASSWORD` configured, `Search` returns "jobleads requires login but no credentials configured" *without issuing a request*. Credentials are stored encrypted at rest (005-FR-018) and an invalid-credentials outcome is reported distinctly (005-SC-005). `JobLeadsSession` mirrors `DjinniSession`: `Ensure` returns the stored `sessionCookie` from the source config, refreshing via a login POST when absent and credentials exist; `Refresh` persists the new cookie and is serialised by a mutex so concurrent workers cannot stampede login. A login-page redirect surviving one `Refresh` retry becomes an error distinguishable as **authentication failure**, not as "could not be interpreted". |
| Wellfound (010), Jobgether (012) | Use the retrieval escalation ladder; otherwise the plain contract. Jobgether follows Glassdoor's blocked/unparsable posture exactly — page 1 fatal, later pages truncate — and may carry `jobgetherMatchScore` in the detail patch's `Raw`. |
| Himalayas (011) | `Kind()` is `dto.SourceKindAPI`. Pages `https://himalayas.app/jobs/api?limit=20&offset=N` at 500 ms intervals, stopping early once `offset >= totalCount`, then filters **locally**: keep jobs whose `categories` or `parentCategories` contain the subscription's category slug, and — when `timezones` was given — whose `timezoneRestrictions` overlap or are empty. Every job carries `Remote: true`. The subscription URL must carry a non-empty `categories`; a stale URL reaching `Search` without one errors rather than silently fetching unfiltered results. `HealthCheck` fetches `?limit=1&offset=0` and calls it healthy when the response decodes with a non-nil `jobs` field — **`totalCount: 0` is still healthy**, because health means the endpoint is reachable and correctly shaped, not that any subscription has matches. Uniquely, Himalayas has **no credentials, no `FetchDetail`, and no enrichment case at all** — the `default: return nil` arm of the enrichment switch covers it. |

## 6. Measurable bars

The numeric targets the source specs set. They are product bars, not test assertions — most
are measured by observation over a period of use rather than by a check in CI.

**Employer boards (013)**

- 013-SC-001: a run reads ≥95% of registered employers successfully, with a **specific reason
  for every one it does not.**
- 013-SC-003: ≥100 postings discoverable through employer boards within the first week, from a
  roster built entirely from candidates the system proposed.
- 013-SC-004: proposed candidates identify the correct employer and vendor ≥95% of the time,
  measured against the user's own accept/reject decisions.
- 013-SC-005: registering a board takes under a minute from pasting a URL to seeing it listed.
- 013-SC-006: the same opening arriving from an aggregator and a board shows as one job ≥90%
  of the time — and **genuinely distinct openings show as distinct 100% of the time.** The
  asymmetry is deliberate: a missed merge is untidy, a wrong merge loses data.
- 013-SC-008: ≥25% of the jobs the user reaches the application stage on originate from an
  employer board rather than an aggregator, within a month of use. This is the bar that says
  whether the source family was worth building.
- 013-SC-009: verified by a run in which at least one employer fails, confirming the failure
  reduced no other employer's results.

**Djinni extraction (022)**

- 022-SC-001: company name correct for ≥95% of listings that display one on the detail page.
- 022-SC-002/003: experience level and English level each extracted for ≥80% of listings that
  mention them.
- 022-SC-005/006: the job detail page loads in under 2 s with the new components rendered, and
  a user can scan company, experience, English and salary in under 5 s without reading the
  description.
