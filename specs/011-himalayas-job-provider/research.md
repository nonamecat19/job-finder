# Phase 0 Research: Himalayas Job Source

## R1: Access model — public vs login-gated

**Decision**: Treat Himalayas as fully public/anonymous, following the `RemoteOKAdapter` pattern
(public JSON feed, no session) rather than the `DjinniAdapter`/`JobLeadsAdapter` login-gated
pattern.

**Rationale**: Live probing during this research (`GET https://himalayas.app/jobs/api`) confirms
listings are served without authentication, matching the spec's assumption ("Himalayas listing
and detail pages are publicly viewable without an account"). No credentials, no session cookie,
no `job_source.config` secret to manage.

**Alternatives considered**: None — the login-gated shape only applies when anonymous access is
insufficient, which is not the case here.

## R2: Fetch mechanism — undocumented public JSON feed, not HTML scrape

**Decision**: Fetch `https://himalayas.app/jobs/api` (an undocumented but functional, unauthenticated
JSON endpoint backing Himalayas's own `/jobs` search page) via `scraping.Service.FetchHTML` with a
descriptive `User-Agent` header (FR-017) — mirroring exactly how `RemoteOKAdapter` calls RemoteOK's
API through `FetchHTML` rather than `jobsources.GetJSON`, then unmarshals the body manually. `Kind()`
is `dto.SourceKindAPI`.

**Rationale**: Live requests during this research confirmed the shape:

```json
{
  "comments": "...",
  "updatedAt": "...",
  "offset": 0,
  "limit": 20,
  "totalCount": 96869,
  "jobs": [
    {
      "title": "...", "excerpt": "...", "companyName": "...", "companySlug": "...",
      "companyLogo": "...", "employmentType": "...", "minSalary": 0, "maxSalary": 0,
      "salaryPeriod": "...", "seniority": "...", "currency": "...",
      "locationRestrictions": ["United Kingdom"], "timezoneRestrictions": [0],
      "categories": ["Engineering", "Backend-Engineering", ...],
      "parentCategories": ["Developer"],
      "description": "<h3>Overview</h3><p>...full HTML body...</p>",
      "pubDate": 1784964128, "expiryDate": 0,
      "applicationLink": "https://himalayas.app/companies/<slug>/jobs/<slug>",
      "guid": "https://himalayas.app/companies/<slug>/jobs/<slug>"
    }
  ]
}
```

`limit` is server-capped at 20 regardless of the requested value (confirmed: `limit=100` echoed
back as `limit: 20`); pagination is via `offset` only. Jobs are ordered newest-first by `pubDate`
(a Unix-seconds timestamp, not ISO text — distinct from every other adapter's date convention).
There is no per-listing detail endpoint: neither a `guid` nor an `id`-style query parameter narrows
the result set — every combination tried during this research returned the same generic
newest-first page (see R6).

Using `scraping.Service.FetchHTML` (not raw `jobsources.GetJSON`) is a deliberate choice: it is the
transport every other adapter already uses for outbound pacing/host-identification (including the
per-host pacing added in the "pace outbound requests per host" change), so Himalayas's request
volume is governed by the same mechanism as every other source instead of a second, bespoke HTTP
client.

**Alternatives considered**: Scraping the rendered `/jobs` HTML page directly (like
Djinni/DOU/Indeed/Glassdoor). Rejected — the JSON feed already exists, is far cheaper to parse
reliably, and is what the `/jobs` page itself calls client-side; scraping rendered HTML would be
strictly worse for the same data.

## R3: Server-side filtering does not work — category/timezone filtering must happen client-side

**Decision**: Treat `https://himalayas.app/jobs/api` as an unfiltered, paginated firehose. Apply
category and timezone filtering **locally**, against each page's `categories` / `parentCategories`
/ `timezoneRestrictions` fields, mirroring `ArbeitnowAdapter`'s "fetch bounded pages, filter
locally" shape rather than RemoteOK's "pass the filter through as a query param" shape.

**Rationale**: Live testing during this research shows the API silently ignores filter attempts —
`?categories=Engineering`, `?keywords=golang`, and `?guid=<a-known-guid>` all returned the same
`totalCount` and the same generic newest-first jobs as a bare request, with no error and no
narrowing. With `totalCount` around 96k and no working upstream filter, a subscription must instead
sweep a bounded number of newest-first pages and keep only jobs whose `categories` (or
`parentCategories`) array contains the operator's chosen category slug, matching Arbeitnow's
established precedent for a source with no working keyword param.

**Alternatives considered**: Relying on an assumed-but-unverified filter query param and shipping
without verifying it — rejected, since this research disproved that assumption directly; shipping
it anyway would silently return unfiltered "top of the firehose" results and would not satisfy
FR-002 ("retrieve Himalayas listings for a user-defined search configuration").

## R4: Subscription URL shape

**Decision**: The operator-pasted subscription value is Himalayas's own public `/jobs` search-page
URL, e.g. `https://himalayas.app/jobs?categories=Backend-Engineering&timezones=UTC-5,UTC-8` —
mirroring the Indeed/Glassdoor/RemoteOK/DOU precedent of "paste the URL you'd browse yourself."
`validateSubscriptionURL` gains a `case "himalayas":` that requires host `himalayas.app`, path
`/jobs` (or `/jobs/...`), and a non-empty `categories` query parameter — rejecting anything else
with a human-readable reason (FR-015). The adapter parses `categories` (comma-separated slugs) and
optional `timezones` out of that URL at `Search` time; it never calls the pasted URL itself (that
URL renders an HTML page, not JSON) — only `https://himalayas.app/jobs/api` is fetched, per R2/R3.

**Rationale**: Live testing confirmed `/jobs?categories=<slug>` and `&timezones=<a,b>` are real,
documented-by-example query parameters on Himalayas's own search page (seen in that page's own
pagination links), so this is a shape operators can actually produce themselves by using
Himalayas's UI and copying the resulting URL — consistent with every other pasted-URL source in
this codebase.

**Alternatives considered**: A structured category/role/timezone form instead of a pasted URL.
Rejected for the same reason JobLeads/RemoteOK/Glassdoor rejected it — no such per-source form UI
exists in `SourcesPage.tsx`, and introducing one here alone would be a one-off inconsistency (FR-014
"through the same subscription flow already used for existing sources").

## R5: Job field mapping

**Decision**: Map Himalayas's `/jobs/api` job objects to `dto.NormalizedJob`:

| Himalayas field | NormalizedJob field | Notes |
|---|---|---|
| `guid` (a canonical `himalayas.app/companies/.../jobs/...` URL) | `ExternalID` and `URL` | stable per-listing identifier (FR-003, FR-004); `applicationLink` is observed identical to `guid` in every sampled record, so `guid` alone is used for both |
| `title` | `Title` | |
| `companyName` | `Company` | |
| `locationRestrictions` (string array, e.g. `["United Kingdom"]`) | `Location` | joined into descriptive text when non-empty; empty array → `nil` (spec's "no explicit location" edge case) |
| — (Himalayas is remote-only, per spec's Assumptions) | `Remote` | always `true`, no derivation needed |
| `minSalary`/`maxSalary`/`currency`/`salaryPeriod` | `SalaryRaw` | formatted display string when either bound is non-zero; `nil` when both are absent/zero (spec's "no salary range published" edge case) |
| `description` (already full HTML body, not a teaser) | `Description` | converted via `jobsources.HTMLToText`, same as Remotive/Arbeitnow; see R6 — this is already complete at ingestion, not a summary |
| `pubDate` (Unix seconds) | `PostedAt` | `time.Unix(pubDate, 0).UTC().Format(time.RFC3339)`; `0`/absent → `nil` |
| `timezoneRestrictions` (array of UTC-offset ints, or empty = unrestricted) | folded into `Raw` as descriptive text | spec's "listing restricts applicants to a specific timezone band" edge case: captured as descriptive text, not used to exclude the listing (FR-003 lists it as part of "location/timezone text") |
| `categories`, `parentCategories`, `seniority`, `employmentType` | `Raw` | source-specific extras, same convention as every other adapter's `Raw` field |

**Rationale**: Follows the same mapping discipline as every existing adapter; `NormalizedJob`'s
shape is fixed by the shared DTO (constitution III) and this feature does not extend it.

**Alternatives considered**: None — field names are not provisional here (unlike JobLeads's plan,
written before any real markup was captured); this mapping is based on live-fetched, real Himalayas
API responses captured during this research.

## R6: No FetchDetail / enrichment step is needed

**Decision**: Himalayas does **not** get a `FetchDetail` method and is **not** added to
`enrichment.Handler`'s dispatch switch or `ingestion.Handler.persistIfNew`'s enrich-eligible
source-key list — the same posture as Adzuna/Jooble/Arbeitnow/Remotive/Robota/WorkUa, none of which
are enrichment-eligible today.

**Rationale**: Two independent live findings during this research rule out building an enrichment
path:

1. `/jobs/api`'s `description` field is already the full HTML posting body (verified by fetching a
   single record and reading its complete `description` value — several paragraphs of overview,
   responsibilities, qualifications, and company boilerplate), not a truncated teaser. The
   short-form field is separately named `excerpt`. So the "initial fetch" already includes
   everything US3/FR-009 asks enrichment to backfill (description, qualifications via the
   description body, posting date).
2. There is no working per-listing detail endpoint to enrich *from* even if it were needed:
   `/jobs/api?guid=<a-known-guid>` (and an `id`-shaped equivalent) returns the same generic
   newest-first page as an unfiltered request, not the matching record. Unlike RemoteOK (whose
   entire feed is small enough to re-fetch and grep by URL as its `FetchDetail` does), Himalayas's
   feed is ~96k listings deep with only offset-based pagination, so "re-scan until we find this
   URL again" is not a bounded operation and would conflict with FR-010's per-run request cap.

FR-009's obligation is worded conditionally — "when the initial fetch did not already include
them" — and that precondition never holds for Himalayas, so the requirement is satisfied by
`Search()` alone. US3's acceptance scenario ("stored posting contains the full description text and
a resolved posting date") is met at ingestion time, before any enrichment task would even run.

**Alternatives considered**: Implementing a `FetchDetail` that re-fetches page 0 and does a
best-effort URL match (mirroring `RemoteOKAdapter.FetchDetail`'s shape) purely for interface
consistency. Rejected — page 0 only reflects the *newest* listings; a job ingested even a few pages
back would almost certainly not be found there, making the method actively misleading (it would
look like it does something but silently no-op for most jobs) rather than simply absent. Omitting
it entirely, matching Adzuna/Jooble/Arbeitnow, is the honest representation of "this source is
fully populated at ingestion."

## R7: Rate limiting / request pacing

**Decision**: Page through `/jobs/api` with a fixed `himalayasRequestDelay = 500 * time.Millisecond`
between requests and a fixed `himalayasMaxSubscriptionPages` cap (mirroring
`glassdoorRequestDelay`/`glassdoorMaxSubscriptionPages` and `indeedMaxSubscriptionPages`), stopping
early once a page's `offset >= totalCount` (no more results) or the max page count is reached,
whichever comes first (FR-010).

**Rationale**: Same bounded-loop-with-pacing convention as every other paginating adapter
(Indeed/Glassdoor/JobLeads); necessary here specifically because `totalCount` is very large and
category filtering is client-side (R3) — without a hard page cap, a rare/narrow category could in
principle sweep a very large number of pages hunting for matches.

**Alternatives considered**: An unbounded sweep that stops only when the requested category has
produced enough matches. Rejected — FR-010 requires a bound on requests per run independent of
match yield; SC-002's "at least 20 distinct listings... or all available if fewer than 20 exist" is
phrased to tolerate a bounded sweep coming up short for a very narrow category.

## R8: Client identification

**Decision**: Send a descriptive `User-Agent` header on every request to `/jobs/api`
(`himalayasUserAgent`, following `remoteokUserAgent`'s exact format/tone), satisfying FR-017.

**Rationale**: Matches the established convention for every unauthenticated public-API adapter in
this codebase (RemoteOK, and the login-gated adapters' session requests).

**Alternatives considered**: None — this is a one-line convention, not a design decision with real
alternatives.
