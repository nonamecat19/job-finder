# Phase 0 Research: Jobgether Job Source

## R1: Access model — public vs login-gated

**Decision**: Treat Jobgether as public/anonymous, following the `GlassdoorAdapter` pattern
(plain HTTP, no session) rather than the `DjinniAdapter`/`JobLeadsAdapter` login-gated
pattern.

**Rationale**: The spec's Assumptions state "Jobgether listing and detail pages are publicly
viewable without an account for the fields this feature needs," and nothing in the
Requirements or Key Entities calls for stored credentials or a session — unlike JobLeads
(FR-018, Source Credentials entity), Jobgether has no credential-related requirement at all.
Building a session layer for a source the spec explicitly treats as anonymously accessible
would be unwarranted complexity.

**Alternatives considered**: Login-gated like JobLeads/Djinni. Rejected — no requirement or
edge case in the spec calls for credentials, and Jobgether's own value proposition (aggregating
publicly posted remote listings with a visible match score) implies a public browsing surface,
matching Glassdoor's public search-results pages more closely than JobLeads's account-only
saved searches.

## R2: Fetch mechanism — HTML scrape vs API

**Decision**: Scrape Jobgether's server-rendered search-results and listing-detail pages with
`goquery`, via the same `scraping.Service.FetchHTML` transport every other scrape adapter
uses. `Kind()` is `dto.SourceKindScrape`.

**Rationale**: No public, documented Jobgether JSON API is known to exist (unlike RemoteOK).
HTML scraping is the only viable path, consistent with Glassdoor/Indeed/DOU. This mirrors
Glassdoor's R3 decision exactly, substituting Jobgether's domain.

**Alternatives considered**: None — no API alternative is known.

## R3: Blocked/throttled response handling

**Decision**: Implement a `jobgetherIsBlockedPage(html string) bool` helper mirroring
`glassdoorIsBlockedPage`, checking for Jobgether's own rate-limit/interstitial markers (title
or body text indicating a challenge/rate-limit page) once the real response is captured as a
fixture. Until then, the helper degrades safely: any response that fails to parse into
recognizable listing cards is treated as "could not be interpreted" (FR-011) rather than
silently returning zero results, and a request-level transport error (429, 403, connection
refused) is treated as "blocked" distinctly from both "zero results" and "unparsable" outcomes
per the spec's edge cases.

**Rationale**: The spec explicitly requires distinguishing "source blocks or throttles the
request" (edge case) from "zero results" (edge case) from "response shape changed upstream so
nothing parses" (edge case) — FR-011 makes the second and third distinction a hard requirement.
Glassdoor's `glassdoorIsBlockedPage` is the established precedent for this three-way
distinction in a public-scrape adapter; Jobgether needs the same shape with its own markers.

**Alternatives considered**: Treating any non-2xx or empty-parse response as a single generic
failure. Rejected — violates FR-011 directly, and would surface "Jobgether blocked us" and
"Jobgether changed its markup" identically in the source's health state, making the failure
undiagnosable from the Sources screen.

## R4: Job field mapping

**Decision**: Map Jobgether's search-results cards and detail pages to `dto.NormalizedJob`
following the Glassdoor/Indeed convention:

| Jobgether field | NormalizedJob field | Notes |
|---|---|---|
| listing detail-page path/ID | `ExternalID` | stable per-listing identifier (FR-003), parsed from the listing URL |
| job title | `Title` | required — card skipped without it, mirrors Glassdoor |
| company name | `Company` | |
| location text | `Location` | Jobgether is remote-focused; many listings show "Remote" as location text |
| remote indicator | `Remote` | best-effort text match against title/location, same convention as `glassdoorRemoteRe` |
| salary text (when published) | `SalaryRaw` | often absent — spec's "no salary range published" edge case; ingest with the field empty rather than dropping the listing |
| listing URL | `URL` | canonical Jobgether posting page |
| summary text at list-level; full body at detail-level | `Description` | list-level summary at ingestion (FR-003), full text after enrichment (FR-009), mirrors Indeed/Glassdoor's split |
| posting date text | `PostedAt` | parsed/normalized to RFC3339 where resolvable; enrichment resolves it fully per FR-009 |
| Jobgether's own AI match-percentage score, when present | `Raw["jobgetherMatchScore"]` | descriptive metadata only — never read by matching/scoring code (FR-012, edge case) |

**Rationale**: Field names and selectors are provisional pending the real markup captured as a
fixture during implementation (no fixture exists yet, matching the JobLeads precedent of
documenting the mapping shape before the selectors are known). `NormalizedJob`'s shape is
fixed by the shared DTO (constitution III); this feature does not extend it — the match score
goes into the existing free-form `Raw` field precisely because it must never become a
first-class scoring input.

**Alternatives considered**: Adding a first-class `MatchScore` field to `dto.NormalizedJob`.
Rejected — this would create a field that looks like it should feed scoring, directly
contradicting FR-012's explicit prohibition; keeping it in `Raw` (already the established
place for source-specific, non-authoritative extras, e.g. Glassdoor's `employerRating`) is the
mechanism least likely to be misused later.

## R5: FetchDetail / enrichment shape

**Decision**: Implement `FetchDetail(ctx, jobURL, config) (JobgetherDetailPatch, error)` that
fetches the individual listing's detail page (unauthenticated, same transport as `Search`) and
returns the full description and resolved posting date. If the detail page reports the
listing as no longer available (404, or a "listing not found"/expired page), the patch reports
`Available: false` with a nil error, and the caller preserves already-captured summary data
rather than overwriting it with nothing — matching the spec's US3 acceptance scenario 2 and
FR-009's "unavailable" edge case, and mirroring `GlassdoorAdapter.FetchDetail` exactly.

**Rationale**: This is the uniform enrichment shape every public-scrape adapter in this
codebase already follows (Indeed, DOU, Glassdoor); Jobgether has no requirement that would
justify a different shape.

**Alternatives considered**: None — this is the established enrichment contract for a
public-scrape source.

## R6: Subscription configuration shape

**Decision**: Accept a Jobgether search-results URL as the subscription value (the URL an
operator would copy after configuring a role/location/category search on jobgether.com).
`validateSubscriptionURL` in `subscriptions/service.go` gains a `case "jobgether":` calling
`validateJobgetherSubscriptionURL`, which checks the host is `jobgether.com` or a
`.jobgether.com` subdomain and that the path looks like a search-results page (not a single
job-detail page), rejecting anything else with a human-readable reason (FR-014, FR-015),
mirroring `validateGlassdoorSubscriptionURL`'s host + shape check.

**Rationale**: Matches every other source's precedent — operator pastes a URL through the same
subscription flow used for existing sources (FR-014), no bespoke UI, and rejects unrecognizable
input at save time rather than at run time (FR-015, SC-008).

**Alternatives considered**: A structured keyword/filter input instead of a pasted URL.
Rejected for consistency with the rest of the codebase's subscription model (Glassdoor/Indeed/
RemoteOK all use pasted URLs) and to avoid a one-off UI branch in `SourcesPage.tsx`.

## R7: Rate limiting / request pacing

**Decision**: Apply the same fixed inter-request delay and page cap Glassdoor/Indeed use
(500ms floor between paginated requests, a hardcoded page cap such as
`jobgetherMaxSubscriptionPages`) to Jobgether's search-results pagination, satisfying FR-010
directly.

**Rationale**: Jobgether search results are expected to be paginated HTML like
Glassdoor/Indeed's, so the same bounded-loop-with-pacing approach applies; a hardcoded page cap
prevents a redirect-to-page-1 loop or unbounded feed from running forever, consistent with
`glassdoorMaxSubscriptionPages`/`indeedMaxSubscriptionPages`, and directly satisfies the spec's
"upstream response exceeds the expected listing count for a single fetch" edge case (keep what
was collected, report success, stop at the cap).

**Alternatives considered**: Unbounded pagination relying only on "no more results" detection.
Rejected — the spec explicitly requires a bounded request count per run (FR-010) independent
of upstream behavior, and an unbounded loop cannot satisfy the "long-running run interrupted"
edge case's expectation that a run terminates and reports partial results in bounded time.

## R8: Client identification

**Decision**: Reuse the existing `scraping.Service.FetchHTML` transport unchanged, which
already attaches a descriptive client identifier (User-Agent) to every outbound request per
the conservative-access posture established for Glassdoor/Indeed/DOU/RemoteOK (FR-017). No new
client-identification code is needed.

**Rationale**: FR-017 asks for consistency with the existing conservative-access posture, not
a Jobgether-specific identifier; the shared transport already satisfies this for every adapter
that uses it.

**Alternatives considered**: A Jobgether-specific User-Agent string. Rejected — no requirement
calls for divergence from the shared transport's existing identifier, and introducing one would
be an unexplained special case.
