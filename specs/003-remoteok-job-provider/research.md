# Phase 0 Research: RemoteOK Job Source

## R1: Fetch mechanism — JSON API vs HTML scrape

**Decision**: Use RemoteOK's public JSON API at `https://remoteok.com/api` (optionally
`https://remoteok.com/api?tags=<tag>`) rather than scraping the rendered listing pages.

**Rationale**: Unlike DOU/Djinni/Indeed, RemoteOK exposes a stable, documented,
unauthenticated JSON endpoint that returns the same data the listing pages render from.
This avoids goquery selector churn entirely, needs no pagination logic, and is a single
request per run instead of N page fetches — a better fit for FR-010's request-pacing intent
and the constitution's "conservative access" posture. `Kind()` is therefore
`dto.SourceKindAPI`, not `dto.SourceKindScrape`.

**Alternatives considered**: Scraping `remoteok.com/remote-<tag>-jobs` HTML pages like the
DOU/Indeed adapters do. Rejected — strictly more fragile and more requests for the same
data the API already returns structured.

## R2: Fetching JSON through the existing scraping.Service

**Decision**: Call `scraping.Service.FetchHTML(ctx, apiURL, headers)` and
`json.Unmarshal` the returned string body, despite the method's name. Pass a custom
`User-Agent` header identifying the app (FR-017) since RemoteOK's documented etiquette asks
API consumers to set a descriptive User-Agent or they may be blocked.

**Rationale**: `FetchHTML` is content-type agnostic — it does a plain GET and returns the
raw body as a string, which is exactly what's needed to unmarshal JSON. Reusing it avoids
introducing a second HTTP client/timeout policy and keeps the adapter consistent with every
other source's transport path.

**Alternatives considered**: Adding a `FetchJSON` method to `scraping.Service`. Rejected as
unnecessary — the existing method's behavior is already correct for this use case, and
adding a parallel method for one adapter is unwarranted abstraction.

## R3: Response shape and the leading "legal notice" element

**Decision**: RemoteOK's API returns a JSON array whose first element is a metadata/legal
object (no `id` field, a `legal` key) rather than a job. The adapter MUST skip any array
element that lacks an `id` or `position` field rather than assume index 0 is always the
legal notice, since RemoteOK does not guarantee the array's exact composition changes.

**Rationale**: This is RemoteOK's well-known, long-standing API quirk. Skipping by
"does this look like a job record" (presence of `id`+`position`) is more resilient than a
hardcoded `[1:]` slice if RemoteOK ever changes what leads the array — consistent with the
edge case "response shape changes upstream so nothing parses" needing a distinguishable
failure rather than silent corruption.

**Alternatives considered**: Slicing off index 0 unconditionally. Rejected — brittle to a
format change that isn't a parse failure but a reordering.

## R4: Job field mapping

**Decision**: Map RemoteOK's JSON fields to `dto.NormalizedJob` as follows:

| RemoteOK field | NormalizedJob field | Notes |
|---|---|---|
| `id` | `ExternalID` | stable per-listing identifier (FR-003) |
| `position` | `Title` | |
| `company` | `Company` | |
| `location` | `Location` | often empty string; RemoteOK is remote-only regardless |
| *(constant)* | `Remote` | always `true` — every RemoteOK listing is remote (FR-003) |
| `salary_min`/`salary_max` (when present) | `SalaryRaw` | formatted into a raw display string; both fields are commonly absent |
| `url` or `apply_url` | `URL` | prefer `url` (RemoteOK's own listing page) so the stored posting URL is always a human-openable page, per the Indeed spec's "aggregated listings that redirect off-site" edge case |
| `description` | `Description` | already full HTML/text in the API response — no separate detail fetch is required for description itself |
| `tags` | stored on `Raw` | not a first-class `NormalizedJob` field yet; carried through as raw data for now, matching how DOU carries `dateText` in `Raw` |
| `date` | `PostedAt` | ISO 8601 already; parse and reformat to RFC3339 for consistency with other adapters |

**Rationale**: RemoteOK's JSON already contains full description text, unlike Indeed's
search-results HTML which only has a summary. This means `FetchDetail` (used by
enrichment) largely already has its data from the initial `Search` call — see R5.

**Alternatives considered**: None — field names are fixed by RemoteOK's API.

## R5: FetchDetail / enrichment shape

**Decision**: Implement `FetchDetail(ctx, jobURL, config) (RemoteOKDetailPatch, error)` for
interface parity with the other adapters' enrichment hook, but its implementation re-fetches
the same `https://remoteok.com/api` payload and locates the record whose `url` matches
`jobURL` (or whose `id` matches an ID parsed from the URL), then returns the already-present
`description`/`tags`/`date` fields. If no matching record is found (listing rotated out of
RemoteOK's current feed), the patch reports the listing as unavailable rather than erroring,
per the spec's "listing whose detail page is no longer available" edge case.

**Rationale**: RemoteOK's API is a snapshot of current listings, not an addressable
per-listing detail endpoint — there's no `/api/{id}` route. Re-fetching and matching is the
only way to get authoritative fresh data for a specific listing, and mirrors how the initial
`Search` call already has everything, so enrichment's job here is mostly "confirm still
listed, refresh field values" rather than "unlock deeper data" like Indeed's
summary→full-description flow.

**Alternatives considered**: Skipping `FetchDetail`/enrichment entirely since `Search`
already returns full data. Rejected — the enrichment dispatch pattern is uniform across all
adapters (`ingestion/handler.go`'s allowlist triggers enrichment by source key), and having
RemoteOK opt out would require a special case in ingestion; matching the existing FR-009
requirement, keeping FetchDetail is simpler and gives listings a chance to be marked
unavailable when they've rotated out.

## R6: Subscription configuration shape

**Decision**: Accept either a RemoteOK tag/category listing URL
(`https://remoteok.com/remote-<tag>-jobs`) or the bare API root
(`https://remoteok.com/api`) as the saved subscription URL. `validateSubscriptionURL` in
`subscriptions/service.go` accepts hosts `remoteok.com` and `remoteok.io` (RemoteOK's
alternate domain) and rejects anything else with a human-readable reason (FR-015, FR-016).
The adapter derives the tag from the URL path (`remote-<tag>-jobs` → `<tag>`) when present,
and calls the untagged `https://remoteok.com/api` endpoint when the saved URL is the bare
root or the tag can't be parsed.

**Rationale**: Matches the Indeed precedent (FR-015: "operator-pasted URL through the same
subscription flow") rather than inventing a different configuration UX (e.g., a
keyword/tag input field) for just this one source.

**Alternatives considered**: A dedicated "tags" text field instead of a URL. Rejected for
consistency with every other source's subscription flow and to avoid a one-off UI branch in
`SourcesPage.tsx`.

## R7: Rate limiting / request pacing

**Decision**: A single RemoteOK run makes exactly one HTTP request in the common case (the
API call). No sleep/pacing loop is needed since there's no pagination. The adapter still
sets a descriptive `User-Agent` header (FR-017) as RemoteOK's API documentation requests,
and `HealthCheck` reuses the same lightweight API call rather than issuing a second request
type.

**Rationale**: FR-010's 500ms-between-requests requirement is about protecting against
runs that hammer a source with many requests; a single-request adapter trivially satisfies
the spirit of that requirement without needing a timer.

**Alternatives considered**: Adding an artificial delay before the single request "for
consistency" with other adapters. Rejected — no requirement to pace a single request against
itself; would be a no-op delay adding nothing but latency.
