# Phase 0 Research: work.ua / robota.ua adapters

**Date**: 2026-07-16. All findings below are from live probes on that date; markup and protection posture can change.

---

## Decision 1: robota.ua is not implementable — descoped

**Decision**: Do not build a robota.ua adapter. Park it pending official/partner API access.

**Rationale**: robota.ua is behind a Cloudflare **managed** bot challenge on every path probed. Evidence:

| Probe | Result |
|---|---|
| `POST https://dracula.robota.ua/` (their SPA's GraphQL endpoint) | `HTTP 403` — Cloudflare interstitial |
| `GET https://robota.ua/zapros/golang/ukraine` (public listing) | `HTTP 403` — body contains `Just a moment...` |
| `GET https://robota.ua/robots.txt` | `HTTP 403` — challenge page, **not even robots.txt is readable** |

The 403 body carries `window._cf_chl_opt = { ..., cType: 'managed', cZone: 'robota.ua' }` and `<noscript>Enable JavaScript and cookies to continue</noscript>` — a JS/cookie challenge requiring browser execution plus fingerprint checks.

This falsifies the spec's assumption *"robota.ua exposes a usable public feed"* and its broader assumption *"listings are publicly readable"* — true for work.ua, false for robota.ua.

Two reasons this is a stop, not a hurdle:

1. **Technical**: no plain-HTTP path exists. The spec's whole premise — "similar to djinni/dou", i.e. fetch server-rendered HTML with `scraping.FetchHTML` — cannot apply.
2. **Consent**: a managed challenge is the operator explicitly asserting they do not want automated access. Defeating it means fingerprint spoofing or a challenge-solving service, i.e. circumventing an access control rather than reading a public page. That is a different act from scraping an open site, and it is not something this plan will build.

**Alternatives considered**:

- *Headless browser (chromedp — already vendored for PDF rendering)*: rejected. Vanilla headless Chromium is itself a well-known fingerprint and is routinely caught by managed challenges, so it likely fails anyway; making it succeed means adding stealth/evasion, which is the consent problem above wearing a hat. Would also drag a heavyweight browser into the ingestion hot path for one source.
- *Third-party CF-solving service (FlareSolverr, commercial solvers)*: rejected on the same consent grounds, plus it breaches Principle V (local-first, self-hosted — routing job discovery through an external paid service).
- *Official API*: **selected as the parked path.** robota.ua does operate partner/employer API programs. This is the only route that gets robota.ua data with the operator's consent. Requires the user to make contact; no code until access is granted.

**Consequence**: spec User Story 2 (P2) is deferred. Stories 1/3/4 are unaffected — they were always work.ua-specific or source-generic.

---

## Decision 2: work.ua search via `?search=` query param, not a hand-built slug

**Decision**: Request `https://www.work.ua/jobs/?search={url-encoded keywords}` and let work.ua redirect.

**Rationale**: Verified live — `?search=golang+developer` returns `HTTP 200` and the effective URL is `https://www.work.ua/jobs-golang+developer/`. The site performs its own slugification server-side and Go's `http.Client` follows the 3xx by default, so multi-word and Cyrillic keywords need no slug logic from us.

**Alternatives considered**: hand-building `/jobs-{slug}/` — rejected as a reimplementation of the site's slug rules that would silently break on spaces, Cyrillic, and punctuation.

---

## Decision 3: remote filter uses work.ua's own path segment

**Decision**: When `query.Remote` is true, request `https://www.work.ua/jobs-remote/?search={kw}`.

**Rationale**: Verified live — returns `HTTP 200` with 15 cards, and the URL is stable (no redirect). Using the board's native filter means results are authoritative rather than regex-guessed.

**Alternatives considered**: djinni's approach of matching a `remote|віддалено` regex against card text — rejected for search filtering (the board already knows), but **retained for the `Remote` bool** on each normalized job, since a card in a non-remote search may still be remote.

---

## Decision 4: pacing at 2s, from robots.txt

**Decision**: hard floor of 2s between every work.ua request. `workuaCrawlDelay = 2 * time.Second` for list pagination; `WORKUA_DETAIL_DELAY_MS` (default `2000`) for enrichment.

**Rationale**: `https://www.work.ua/robots.txt` declares `Crawl-delay: 2`. This is the board publishing its own terms for automated access; honoring it is what keeps this adapter on the right side of the line robota.ua draws with Cloudflare. It also directly serves spec SC-008 (never get rate-limited) and FR-011.

Relevant robots.txt rules confirmed:

- `Crawl-delay: 2`
- `Disallow: /jobs-*-/` — note this does **not** match our URLs: `/jobs-golang/`, `/jobs-golang+developer/`, and `/jobs-remote/` have no trailing `-/`. Search paths are permitted.
- `Disallow: /api/v*/jobs/*/phone-click/` — irrelevant; we never touch it.
- No `Disallow` covers `/jobs/{id}/` detail pages or keyword search pages.

**Alternatives considered**: reusing djinni's 1500ms default — rejected; it is below work.ua's published crawl-delay.

---

## Decision 5: work.ua markup (verified live)

Fixtures to capture from these URLs. Selectors are best-effort with fallbacks, per the djinni/dou house style.

**List page** (`/jobs-php/` → 14 cards; `/jobs-php/?page=2` → 11 cards, different first id, so pagination works):

| Field | Selector / source |
|---|---|
| Card | `div.card.job-link` (also carries `data-id="{id}"`) |
| Title + link | `h2 a[href^="/jobs/"]` |
| External ID | numeric segment of `/jobs/{id}/`, or the card's `data-id` |
| Company | `a[href^="/jobs/by-company/"]` |
| Pagination | `?page=N` on the canonical search URL |

**Detail page** (`/jobs/8047944/`, `HTTP 200`):

| Field | Selector |
|---|---|
| Title | `h1#h1-name` |
| Description | `#job-description` |
| Company | `a[href^="/jobs/by-company/{id}/"]` |
| Salary | `li` containing `span.glyphicon-hryvnia-fill` (title attr `Зарплата`) |

**Empty-results marker** (added 2026-07-16, during task generation): a genuinely-empty search renders an element matching `no-results` plus the text `За вашим запитом`. Verified by contrast: `/jobs-kyiv-php/?salaryfrom=4&experience=2` → 0 cards **with** the marker, while `/jobs-kyiv-php/` → 7 cards and `/jobs-php/?salaryfrom=4` → 13 cards. This makes FR-007's "nothing matched" vs "markup broke" distinction a deterministic check rather than a heuristic — cards absent *and* marker absent means the parse broke. Worth more than it looks: djinni/dou can only guess at this.

**Alternatives considered**: parsing embedded JSON-LD — not investigated in depth; the HTML selectors above are verified and match existing house style. Worth revisiting only if selectors prove fragile.

---

## Decision 6: no schema change, no dashboard change

**Decision**: reuse `dto.NormalizedJob`, `dto.SearchQuery`, `dto.SourceKindScrape`, and the existing `UpdateJobDetail` sqlc query as-is.

**Rationale**: `UpdateJobDetail` (`apps/api/internal/db/queries/job.sql:29`) already takes exactly `Description`, `SalaryRaw`, `Location`, `Remote`, `Raw`, `PostedAt` — the full set work.ua detail pages yield. `SourceKind` already has a `scrape` value. `SourcesPage.tsx`'s `CONFIG_FIELDS` map lists only sources with configurable secrets (`adzuna`, `djinni`, `jobspy`); work.ua needs no credentials, so omitting it is correct and renders no config inputs.

Per Principle III, nothing needs regenerating — no new cross-boundary type is introduced.

---

## Open questions for `/speckit-tasks`

1. ~~**Subscription URL shape**~~ — **RESOLVED during `/speckit-tasks`.** A work.ua "subscription URL" means any public search-results URL the user pastes; authenticated saved-search pages under `/jobseeker/my/` are robots-disallowed and stay out. The worry that this made Story 4 redundant with the keyword path was **wrong**: work.ua search URLs encode filters `dto.SearchQuery` cannot express. Verified live from the search form — `region`, `category`, `employment`, `experience`, `salaryfrom`, `salaryto`. `/jobs-kyiv-php/?salaryfrom=4` returns 13 cards and is unreachable via the keyword path. Story 4 keeps its value; scope note recorded in `tasks.md` Phase 5.
2. **Salary parsing depth**: `SalaryRaw` (a string) is all the spec requires. dou has a `parseDOUSalary` min/max extractor. Deferred unless a consumer needs the numbers.
