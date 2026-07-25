# Phase 0 Research: Wellfound Job Source

## R1: Access model — public scrape vs. login-gated

**Decision**: Treat Wellfound as publicly reachable, following the `GlassdoorAdapter`/
`IndeedAdapter`/`RemoteOKAdapter` pattern rather than the login-gated `DjinniAdapter`/
`JobLeadsAdapter`/`JobLeadsSession` pattern.

**Rationale**: The spec's Assumptions state Wellfound listing pages are publicly viewable (no
account required) for the fields this feature needs, and explicitly scope a session-gated
listing to "enrich with summary-only data and mark accordingly" rather than blocking the run.
No credential entity appears in the spec's Key Entities (unlike JobLeads's "Source
Credentials"). Building a login/session mechanism the spec doesn't ask for would be
unwarranted divergence from the simpler precedent.

**Alternatives considered**: Login-gated from the start (Djinni/JobLeads pattern). Rejected —
no credentials are named in the spec, and the spec's edge case for a company/listing needing a
signed-in session already anticipates a *degrade*, not a hard failure, which fits the
public-scrape-with-graceful-enrichment-degrade shape rather than requiring an account.

## R2: Fetch mechanism — HTML scrape vs API

**Decision**: Scrape Wellfound's public search-results and listing-detail pages with
`goquery`, via the same `scraping.Service.FetchHTML` plain-HTTP transport every other
non-login adapter uses (no cookies, no session).

**Rationale**: No public, documented Wellfound JSON API is known to exist (unlike RemoteOK).
HTML scraping is the only viable path, consistent with Glassdoor/Indeed/DOU. `Kind()` is
therefore `dto.SourceKindScrape`.

**Alternatives considered**: None — no API alternative is known.

## R3: Anti-bot posture and failure classification

**Decision**: Follow Glassdoor's precedent exactly: attempt plain unauthenticated HTTP first;
treat a bot-challenge/rate-limit response as a distinct, reported failure mode (FR-011)
detected by response-shape heuristics (e.g. absence of any recognizable job-card markup
combined with challenge-page markers), not by brittle single-template string matching, since
challenge pages change over time. Do not build headless-browser fetching or anti-bot evasion
into this pass (FR-013, Assumptions: "aggressive crawling and anti-bot evasion are out of
scope"). If empirical testing during implementation shows plain HTTP is reliably blocked, that
is documented as a known limitation (mirroring Glassdoor's research.md R3 finding) rather than
escalated to browser-based fetching within this feature.

**Rationale**: Matches the constitution's "best-effort, not a hard dependency" posture for
scraping-based sources and the spec's edge case "source blocks or throttles the request" —
requires the run to end with a recorded failure and a human-readable reason, not a crash or a
silent empty result.

**Alternatives considered**: `scraping.Service.BrowserContext` (headless browser) as the
primary retrieval path. Rejected as out of scope for v1, exactly as Glassdoor deferred it —
a same-adapter follow-up if plain HTTP proves unusable, not a plan-level decision.

## R4: Job field mapping

**Decision**: Map Wellfound's search-results cards and detail pages to `dto.NormalizedJob`
following the Glassdoor/Indeed convention:

| Wellfound field | NormalizedJob field | Notes |
|---|---|---|
| listing detail-page path/ID | `ExternalID` | stable per-listing identifier (FR-003), parsed from the listing URL or a data attribute on the card |
| job title | `Title` | required; card skipped without it (protects SC-004) |
| company name | `Company` | present even when the company has no public profile page (spec edge case) — uses whatever name text is on the listing itself |
| location text | `Location` | may be empty |
| remote/hybrid/onsite indicator | `Remote` | best-effort text match, same convention as `glassdoorRemoteRe` |
| salary and/or equity text (when published) | `SalaryRaw` | raw text as published; captured even when only one of salary/equity is present, empty when neither is (spec edge case) |
| listing URL | `URL` | required; canonical Wellfound posting page |
| summary text at list-level; full body + qualifications at detail-level | `Description` | list-level summary at ingestion (FR-003), full text after enrichment (FR-009) |
| posting date text | `PostedAt` | resolved during enrichment (FR-009); best-effort at ingestion if present on the card |
| equity-vs-salary flag, any Wellfound-specific extras | `Raw` | free-form map, matching how Glassdoor stashes its estimate-vs-employer-stated salary flag |

**Rationale**: Field names are provisional pending real markup capture (fixtures written
during implementation); the mapping follows the same conventions as the closest existing
scrape adapter (Glassdoor) so implementation only needs real selectors, not new
`NormalizedJob` usage.

**Alternatives considered**: Adding a typed `Equity` field to `NormalizedJob`. Rejected —
constitution III fixes the shared DTO shape; reusing `SalaryRaw`/`Raw` for equity text matches
how Glassdoor's salary-estimate-vs-employer-stated distinction was already handled without a
schema change.

## R5: FetchDetail / enrichment shape

**Decision**: Implement `FetchDetail(ctx, jobURL, config) (WellfoundDetailPatch, error)` that
fetches the individual listing's detail page and returns the full description, qualifications
text (folded into `Description`), resolved posting date, and availability — same shape and
calling convention as `GlassdoorDetailPatch`/`IndeedDetailPatch`. If the detail page returns a
"listing no longer available" state (404, removed-listing page, or a page requiring sign-in
that this feature can't read), the patch reports `Available: false` while preserving the
summary data already captured, matching FR-009's second acceptance scenario and the spec's
"posting is no longer available" / "requires a signed-in session" edge cases.

**Rationale**: Mirrors Glassdoor/Indeed's summary→full-description enrichment split, the
established precedent for a source where the list view only has a summary.

**Alternatives considered**: None — uniform enrichment shape every scrape adapter follows.

## R6: Subscription URL validation

**Decision**: `validateSubscriptionURL` in `subscriptions/service.go` gains a
`case "wellfound":` calling `validateWellfoundSubscriptionURL`, which checks the host is
`wellfound.com` (or a `.wellfound.com` subdomain) — accepting the legacy `angel.co` host as
well, since Wellfound was previously branded AngelList/angel.co and old saved-search links may
still use it — and rejects a URL path shape that looks like a single job posting rather than a
search-results page, returning a human-readable reason (FR-015).

**Rationale**: Directly mirrors `validateGlassdoorSubscriptionURL`/
`validateIndeedSubscriptionURL` — same validation shape, same place, same
failure-at-save-time guarantee (SC-008).

**Alternatives considered**: A structured role/location/remote filter input instead of a
pasted URL. Rejected for consistency with every other source's subscription model (spec
Assumptions: "operators configure Wellfound the same way they configure existing sources").

## R7: Rate limiting / request pacing

**Decision**: Apply the same fixed inter-request delay and page cap Glassdoor/Indeed use
(500ms pacing, a `wellfoundMaxSubscriptionPages` constant) to Wellfound's saved-search
pagination, satisfying FR-010 directly.

**Rationale**: Wellfound saved searches are expected to be paginated HTML like
Glassdoor/Indeed's, so the same bounded-loop-with-pacing approach applies; a hardcoded page
cap prevents a redirect-to-page-1 loop or unbounded feed from running forever.

**Alternatives considered**: Unbounded pagination relying only on "no more results" detection.
Rejected — FR-010 explicitly requires a bounded request count per run independent of upstream
behavior.

## R8: Client identification

**Decision**: Requests to Wellfound carry the same descriptive client identifier / User-Agent
convention every other adapter already sends via `scraping.Service` (FR-017), with no
Wellfound-specific override needed.

**Rationale**: `scraping.Service.FetchHTML` already centralizes this; no new code is required
beyond using the existing shared transport, matching Glassdoor/Indeed's approach.

**Alternatives considered**: None — this is a shared, existing mechanism.
