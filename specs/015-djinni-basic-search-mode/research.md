# Phase 0 Research: Djinni Basic-Search Mode

## R1: Does the existing `DjinniAdapter.scrapeSubscription` already paginate a
basic-search URL correctly, including the single-page case?

**Decision**: Yes — the existing pagination guards already end a basic-search run cleanly
after a single page. **No pagination change is required to satisfy FR-004 / SC-002.** The
new work is (a) recognizing the URL shape at run time and (b) save-time validation, not a
new pagination implementation.

**Rationale** (read from `apps/api/internal/jobsources/adapters/djinni.go`):
- `Search()` calls `scrapeSubscription(ctx, query.SubscriptionURL, headers)` whenever
  `SubscriptionURL != ""`, regardless of shape. The ingestion handler
  (`apps/api/internal/ingestion/handler.go:139`) sets `query.SubscriptionURL = sub.Url`
  unconditionally when a subscription triggered the run. So a saved basic-search URL is
  already routed to `scrapeSubscription` today.
- `scrapeSubscription` loops `page = 1..djinniMaxSubscriptionPages` (50), appends
  `?page=N` to the saved URL, and breaks when:
  1. page 1 fetch fails → fatal error returned, **no** infinite loop,
  2. `len(cards) == 0` → break (empty page), **this is the single-page stop**,
  3. `cards[0].URL == seenFirstHref` → break (redirect-to-page-1 loop guard),
  4. `page > djinniMaxSubscriptionPages` (50) → for-loop exits.
- For a single-page basic-search (e.g. the `salary=1500&exp_level=1y..3y` Golang example),
  page 1 returns the cards, page 2 returns either an empty result list (Djinni shows a
  "no more results" page) OR redirects back to page 1 (same first card) — both terminate
  the run via guard #2 or #3. The run finishes with the page-1 cards and the existing
  `rec.Ok(ctx, ...)` bookkeeping marks it `succeeded` with the actual count.
- Therefore: SC-002 ("100% of single-page basic-search runs end cleanly without a loop")
  is achieved by the **existing** code path once a basic-search URL is allowed through. The
  implementation risk is around *URL validation/shape recognition*, not pagination.

**Alternatives considered**:
- *Add a separate `scrapeBasicSearch` with its own pagination loop*: rejected on
  preliminary analysis — it would duplicate a working guard set for no behavioral gain.
  Reconsidered below in R2; thesplit is adopted at the *function* level for readability and
  log distinction, but both functions call the same shared loop body, so the airbag is the
  same.
- *Lower `djinniMaxSubscriptionPages` for the basic-search mode*: rejected — the cap is a
  safety rail, not a behavioral knob; the same 50-page bound is fine for either mode
  (FR-015, spec assumptions).

## R2: Where should the URL-shape discriminator live — in `djinni.go` or a new file?

**Decision**: New, pure-function file `apps/api/internal/jobsources/adapters/djinni_searchmode.go`
containing `DjinniSearchMode` (a small enum: `Dashboard | BasicSearch | Unknown`), a
`Detect(rawURL string) DjinniSearchMode` function, and a `BasicSearchFilters` struct
with a `ParseBasicSearch(rawURL string) (BasicSearchFilters, bool)` parser. The existing
`djinni.go` keeps the HTTP/scraping side and grows a thin `scrapeDashboard` vs
`scrapeBasicSearch` split that both call a shared pagination helper so the two paths are
distinguishable in logs and code but share the same guards (R1).

**Rationale**:
- Some of this logic — `search_type == "basic-search"`, the host check, the `primary_keyword`
  / `salary` / `exp_level[]` parse — is conceptually pure and trivially table-testable with
  no HTTP. Hoisting it into its own file lets the test file mirror `djinni_searchmode_test.go`
  and run without spinning up the scraping service. It also makes the same pure logic
  available to the `subscriptions` validator (`validateDjinniSubscriptionURL`) without
  packaging gymnastics — both files are already in `package adapters`, which the
  subscriptions package already imports elsewhere; if that coupling is undesirable, the
  helpers can be exported (`DjinniDetect`, `DjinniParseBasicSearch`) and called from
  `subscriptions/service.go`, matching how `validateWellfoundSubscriptionURL` reuses a
  small `regexp` defined in the same file.
- Keeping the discriminator as a pure function (no `*goquery.Document`, no `*scraping.Service`)
  makes the dashboard's TS port of the same logic a faithful translation rather than a
  reimplementation that has to peek past HTTP noise.

**Alternatives considered**:
- *Inline the discriminator inside `subscriptions/service.go`'s new `case "djinni":`*:
  rejected — the run-time adapter (FR-002) also needs to discriminate the mode, so the same
  shape check would live in two places. Putting the canonical one in the adapters package
  and reusing it from both call sites matches how `wellfoundJobDetailPathRe` is shared.
- *Add a `Mode` column to the `subscriptions` table*: rejected — the URL itself already
  encodes the mode unambiguously, and a derived field would be a denormalized copy that
  could drift. Constitution III discourages hand-maintained duplicates across boundaries;
  the single source of truth is the URL string the operator pastes.

## R3: Should the dashboard display summary take a new server-provided field, or parse
the URL again on the client?

**Decision**: Parse on the client. **No new `SubscriptionDto` field, no schema change, no
`packages/shared` rebuild, no `dto.go` change.**

**Rationale**:
- `SubscriptionDto.url` (`apps/api/internal/dto/jobs.go:99`,
  `packages/shared/src/index.ts:351`) already carries the URL string the operator saved.
  The dashboard already has the raw material; a server-side parse would add a parallel
  derived field that the server must keep in sync with the URL.
- The URL-shape contract is small and stable (R2's pure parser is ~30 lines of Go; the TS
  port is ~25 lines). Duplicating that small pure logic on the client side is materially
  cheaper than adding a new DTO field + tygo regeneration + `packages/shared` rebuild +
  `index.ts` hand-maintenance (constitution III) — and the duplication is *of pure URL
  parsing*, not of a job or subscription *type*.
- The Go side's parse is used for **validation** (save-time reject); the TS side's parse is
  used for **display**. They don't share data, only a URL shape; the contract that matters
  is the URL shape itself, documented in `contracts/djinni-url-shapes.md`.
- The `AGENTS.md` typed-contracts guideline specifically calls out
  `apps/api/internal/dto/dto.go` ↔ `packages/shared/src/index.ts` field-for-field parity for
  *DTO fields*. We add no DTO field, so no parity maintenance is created.

**Alternatives considered**:
- *Add `SubscriptionDto.Filters map[string]string` (or a `DjinniBasicSearchFilters` struct)
  populated server-side*: rejected — it would duplicate the URL's information into a
  derived DTO field that has to stay in sync, and the dashboard would still have to render
  different summaries for dashboard vs basic-search subscriptions (so it would need a
  discriminator anyway). The URL is the discriminant.
- *Server-side templated "summary string" field*: rejected for the same reason — a
  pre-rendered label removes client-side range-collapse intelligence and forces any future
  display tweak to also touch the Go API. The display is a presentation concern; keep it
  in the presentation layer.

## R4: How should consecutive `exp_level` values be detected so the range-vs-list
rule (FR-009, SC-004) is deterministic?

**Decision**: Treat the saved `exp_level` query parameters as a set of integer year counts.
A set is a "consecutive sequence" iff, after sorting the deduplicated integers ascending,
the difference between each adjacent pair is exactly 1. The range is `"min–max years"`
when consecutive (using an en-dash, `–`); otherwise a comma-separated list of the integers
in ascending order followed by `"years"` (e.g. `"1, 3 years"`). A single value
rendered alone is `"3 years"` (a "range" of one is just a number — we don't print `"3–3
years"`).

**Rationale**:
- The user description explicitly anchors the rule: `2y,3y,4y,5y → "2–5 years"` and
  `1y,2y,3y → "1–3 years"` (SC-004 lists both shapes verbatim).
- The Djinni `exp_level` notation is uniformly `Ny` (1y, 2y, 3y, 4y, 5y, ...). Parsing is a
  simple `strings.TrimSuffix(v, "y")` + `strconv.Atoi`; anything that doesn't parse cleanly
  is treated as set-element text and listed discretely rather than collapsed (a safe
  fallback — never misrepresent a non-numeric level as a false range).
- Order is explicit: sort ascending before printing and before the consecutive check, so
  out-of-order URL parameters (`3y&1y&2y`) collapse to the same `"1–3 years"` label as the
  in-order case (spec edge case: "Consecutive `exp_level` values out of order in the URL").
- Duplicates (`2y&2y`) collapse silently — the spec edge case says "duplicates are
  collapsed for both run execution and display without raising an error."
- The same rule is implemented twice (Go side for any server-logging clarity, TS side for
  display) but lives as a small pure function in both (`DjinniExpLevelsSummary([]string)
  string` on Go; `summarizeExpLevels(values: string[]): string` on TS). The two functions
  share one definition, documented in `contracts/djinni-url-shapes.md`.

**Alternatives considered**:
- *Use the raw query string order rather than a sorted set*: rejected — the user wants a
  recognizable range for `2y,3y,4y,5y`, which requires sorting, and the spec's out-of-order
  edge case demands the set semantics.
- *Show the literal `Ny` tokens ("2y–5y")*: rejected — the user said "displayed as a
  range", and the range form `"2–5 years"` is the natural-language reading; the `Ny` token
  form is Djinni's URL shorthand, not a display format.

## R5: Which URL shapes are "neither dashboard nor basic-search", to reject at save time?

**Decision** (FR-007 / SC-007): A Djinni subscription URL is acceptable when it matches one
of two shapes. Everything else is rejected at save time with a human-readable reason.

1. **Dashboard shape** — `host == "djinni.co"` (or `www.djinni.co`) **and** `path` matches
   `/my/dashboard/subs/<id>/`, where `<id>` is one or more URL-safe segments (digits or
   UUID-ish). Reuse the existing implicit precedent — there was no `case "djinni":` in
   `validateSubscriptionURL` today, so dashboard URLs were previously accepted
   permissively; the new validator formalizes the shape without narrowing it for users who
   already have working dashboard subscriptions saved.
2. **Basic-search shape** — `host == "djinni.co"` (or `www.djinni.co`) **and** `path ==
   "/jobs/"` (or `/jobs`) **and** `search_type == "basic-search"` in the query string.
   That last clause is what tells a real basic-search URL apart from a vintage primary
   search URL like `djinni.co/jobs/?primary_keyword=Golang` (which the ingestion
   scheduler's existing test fixture at `apps/api/internal/ingestion/scheduler_test.go:193`
   already exercises). For save-time validation we accept only the explicit
   `search_type=basic-search` form, so we don't accidentally take responsibility for the
   ad-hoc `/jobs/?keywords=...` URL shape the existing `Search()` still supports in code
   (when no `SubscriptionURL` is set) but that no operator is expected to save.

Rejected shapes (with a human-readable reason):
- Host is `djinni.co` but path looks like a single job posting (e.g. `/jobs/<id>` with no
  `search_type=basic-search` query): reason `"djinni subscription url looks like a single
  job posting, not a search results page"`.
- Host is `djinni.co` but path is something else (e.g. `/companies/...`, `/my/...` other
  than `subs/`): reason `"djinni subscription url must be a /jobs basic-search URL or a
  /my/dashboard/subs/<id>/ dashboard URL"`.
- Host is not `djinni.co`: reason `"djinni subscription url must be a djinni.co url"`.

**Rationale**: Mirrors the existing validators exactly — each one is a pure
`url.Parse` + host check + path-shape check; all return `fmt.Errorf` with a quoted
URL and a human-readable clause. None call out to the network. The same validator is also
the run-time mode discriminator: the dashboard mode is detected by shape #1, the
basic-search mode by shape #2, anything else returns an unknown mode both at save time
(reject) and at run time (would not currently happen because save already rejected it —
but kept defensive in case an old row predates the validator).

**Alternatives considered**:
- *Accept any `djinni.co/jobs/?...` URL as basic-search*: rejected — would also accept the
  ad-hoc `primary_keyword=Golang` URL the scheduler test uses and the pre-existing
  `Search()` fallback (which builds a `keywords=` + `employment=remote` query). Pinning
  the mode to `search_type=basic-search` keeps the new feature's surface tight (FR-001)
  without changing the pre-existing non-subscription code path.
- *Accept the dashboard shape more loosely (any `/my/dashboard/...`)*: rejected — `/my/dashboard/subs/`
  is the only dashboard path `scrapeSubscription` knows how to paginate today. Anything
  else under `/my/` would just produce zero cards from `parseDjinniCards`; rejecting at
  save time is friendlier than a silent zero-result run.

## R6: Does the basic-search mode need the logged-in session at all?

**Decision**: No, and FR-018 / SC-009 hold. The existing `DjinniSession.Ensure` already
degrades to anonymous (`""`) when no credentials are configured, and `authHeaders` then
builds an empty header set. `fetchDoc` calls `djinniIsLoginPage` on the returned
document; if Djinni ever serves the login page on a public `/jobs/?search_type=...`
URL (it does not, in the user's example), `Session == nil` short-circuits with the same
error already produced for the dashboard mode today. No login-flow change is needed.

**Rationale** (read from `apps/api/internal/jobsources/adapters/djinni_session.go:71-80` and
`djinni.go:37-48, 62-87`): the session is opportunistic by construction. The dashboard mode
needs it (paginated `subs/{id}/` requires a logged-in cookie); the public `/jobs/` page
does not. Reusing the same `authHeaders`/`fetchDoc` path for the new mode therefore
satisfies FR-018 automatically — no new "anonymous mode" toggle, no skipped login attempt,
just the existing opportunistic fallback.

**Alternatives considered**: *Force anonymous for basic-search (don't call `Ensure` at all)* —
rejected: a logged-in user gets slightly more accurate results ( Djinni personalizes the
public `/jobs/` page when a session cookie is present), so reusing the session when
present is strictly better than forcing anonymous. The spec assumption already says
"reused opportunistically when present (as today) but not required."

## R7: Is any `seed` / `demo` change required?

**Decision**: Optional — add **one** basic-search seed subscription alongside the existing
Djinni dashboard seed (if one exists in `apps/api/cmd/seed/subscriptions.go`), mirroring
the spec's two example URLs, so a fresh `make seed` shows both modes in the dashboard.
This is documented in the plan's `seed/subscriptions.go` modification entry as optional,
not blocking for SC-001–SC-009.

**Rationale**: existing sources seed one representative URL each. Two Djinni seeds (one per
mode) gives an operator the visual proof of FR-008/FR-009 without pasting URLs themselves,
which is the kind of thing the spec's "Show every filter" user story tests.

**Alternatives considered**: *Seed zero basic-search rows, let operators paste*: also
acceptable; reduces the diff to true minimal. The choice is implementation-time; not a
spec blocker.