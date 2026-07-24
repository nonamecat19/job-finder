# Phase 0 Research: Glassdoor Job Source

## R1: Retrieval mechanism — direct scrape vs. jobspy-sidecar

**Decision**: Dedicated direct Glassdoor adapter (`adapters.GlassdoorAdapter`), independent
of `apps/jobspy-sidecar`'s generic `site="glassdoor"` capability (the sidecar's
`SearchRequest.site: Literal["linkedin", "indeed", "glassdoor"]` already accepts it, but
nothing in `apps/api` currently sets `SearchQuery.Site` from a saved subscription — the path
is unwired and unused today).

**Rationale**: Same reasoning the Indeed feature (`002-indeed-job-source/research.md` R1)
already established and that still holds: the sidecar wraps a third-party library as a black
box with no per-listing detail-fetch endpoint (blocks FR-009/US3 enrichment) and opaque
failure modes. A direct adapter matches every existing scrape adapter (DOU, Djinni, Indeed,
WorkUa) — same `scraping.Service`, same goquery parsing, same enrichment hook, same
distinguishable failure semantics (FR-011, FR-018) the codebase already knows how to operate
and debug. Consistency with the established pattern outweighs the smaller short-term effort
of exposing the sidecar's existing (but currently dead) `site=glassdoor` path.

**Alternatives considered**:
- *Wire up the existing `jobspy` adapter's `site=glassdoor` path*: rejected — no detail
  enrichment (FR-009 unmet), and Glassdoor is the site `python-jobspy` itself documents as
  its least reliable target (heaviest anti-bot measures of the three sites it supports);
  inheriting that opacity is worse, not better, than owning the scrape directly.
- *Both, sidecar as fallback*: rejected as unnecessary scope for a first iteration, per the
  same reasoning as R1 in 002.

## R2: Search configuration entry point

**Decision**: Operator-pasted Glassdoor search-results URL via the existing `Subscription`
flow (`POST /api/subscriptions`), identical UX to Indeed/RemoteOK/DOU/Djinni. No
keyword/location structured-parameter form.

**Rationale**: Matches the established convention exactly (spec FR-014). Glassdoor's own
search UI already exposes every filter a bespoke form would reimplement; a pasted URL
captures all of it and keeps the adapter simple: fetch, paginate, parse.

**Alternatives considered**: keyword+location structured params — rejected, narrower than a
pasted URL and inconsistent with every prior source's precedent.

## R3: Glassdoor listing page structure, pagination, and anti-bot posture

**Empirical finding (implementation-phase live check)**: A plain unauthenticated HTTP GET
(no JS, no cookies — the exact posture `scraping.Service.FetchHTML` uses) against both
`glassdoor.com/` and a Glassdoor job-search URL returns HTTP 403 with a `<title>Security |
Glassdoor</title>` challenge page (`noindex, nofollow`, no job markup at all) on every
request, with no variation observed. A real browser session (JS execution, cookies) against
the same search URL renders normally and exposes stable `data-test` attributes on real
markup: card `[data-test="jobListing"]` (with a `data-jobid` attribute holding Glassdoor's
own numeric listing ID directly on the card — no HTML parsing needed to get a stable
`ExternalID`), `[data-test="job-title"]`, `[data-test="emp-location"]`,
`[data-test="detailSalary"]`, `[data-test="job-age"]`, and a `[class*="RatingText"]` element
for the employer's star rating. Detail pages resolve to a canonical URL shape
`/job-listing/<slug>-JV_<...>.htm?jl=<jobid>`. This confirms: (a) the markup schema, once
reachable, is far more stable/semantic than Indeed's (`data-test` attributes vs. hashed CSS
classes) — a genuine improvement over the Indeed precedent; but (b) **plain HTTP is blocked
100% of the time**, not merely "sometimes" as initially assumed by analogy with Indeed. This
raises the likely real-world block rate for `Search`/`HealthCheck` from "occasional" to
"the default steady state" — see decision below for how that changes the practical
implication of R1/R3's original plain-HTTP-first choice.

**Decision**: Parse job cards defensively with a multi-selector fallback chain, matching
`indeed.go`/`djinni.go`/`dou.go`. Paginate via Glassdoor's own `p=<page>` query parameter on
the pasted search-results URL, stopping at a hard page cap, on zero new cards, or on a
loop-guard repeat (mirroring `indeedMaxSubscriptionPages`). Treat a bot-challenge/interstitial
response (Glassdoor is known to serve these more aggressively and more often than Indeed) as
a **distinct, reported failure mode** (FR-011, FR-018) detected by response-shape heuristics
(e.g., absence of any recognizable job-card markup combined with challenge-page markers,
matched against the real captured challenge page's `<title>Security | Glassdoor</title>` and
`noindex, nofollow` robots meta tag) rather than by brittle exact-string matching on a single
challenge template, since challenge pages are also expected to change over time. Ship the
adapter as designed (plain HTTP, matching every other adapter in the codebase) rather than
building headless-browser fetching into this pass — but flag to the feature's operator, in
plain terms, that empirical testing during implementation found plain HTTP blocked on 100%
of attempts, so this source should be expected to show unhealthy/blocked out of the box
until the BrowserContext escalation (deferred below) is done as an explicit follow-up, not
a hypothetical one.

**Rationale**: Consistent with the codebase's existing best-effort scraping posture
(constitution: external discovery sources are "best-effort... not a hard dependency").
Glassdoor is empirically harder to scrape via plain HTTP than Indeed — it more consistently
requires JavaScript execution or serves interstitials to non-browser clients — so this
feature treats "blocked" as an expected steady-state outcome for some fraction of runs, not
an edge case to eliminate. FR-018 (no aggressive retry, no bypass) keeps the response
proportionate: report and back off, don't escalate.

**Alternatives considered**: Headless-browser fetch via the existing
`scraping.Service.BrowserContext` (already used for PDF rendering) as the primary retrieval
path — deferred, not adopted for v1. Rationale for deferring: every existing scrape adapter
tries plain HTTP first and only the implementation phase's live testing can show whether
plain HTTP is viable at all for Glassdoor; if live testing during implementation shows plain
HTTP is reliably and immediately blocked (0% success), escalating `Search`/`FetchDetail` to
use `BrowserContext` is a contained, same-adapter follow-up that doesn't change the adapter's
interface, the spec, or this plan — it only changes `glassdoor.go`'s internals. This
decision is documented here so the implementation phase does not silently reach for
browser-based fetching without first confirming plain HTTP's actual failure rate.

## R4: Salary and company-rating fields

**Decision**: Capture Glassdoor's salary estimate (when shown on the listing or detail page)
into `NormalizedJob.SalaryRaw` as free text, and separately record whether it is an estimate
versus employer-stated (spec FR-003, edge case "estimated salary range") in the adapter's
`Raw` map rather than as a new typed DTO field. Do **not** attempt to capture Glassdoor's
company star-rating as part of this feature — that is already a separate, existing concern
(`internal/companyintel/scrape_glassdoor.go`, `KindGlassdoorRating`) which scrapes company
review pages independently of job listings and is out of this feature's scope.

**Rationale**: Reusing `SalaryRaw`/`Raw` avoids a schema change (Constitution III: no
hand-duplicated types) and matches how RemoteOK's salary_min/salary_max ambiguity was
already handled (`remoteokSalaryRaw`). Company rating is a job-*listing* field only insofar
as Glassdoor's search page sometimes inlines it next to a posting, but the product already
has a dedicated, working pipeline for that exact data (company intel) — duplicating it into
the job-listing path would create two sources of truth for the same fact and isn't needed
for FR-003 (which only requires the *listing's* salary range).

**Alternatives considered**: Adding a `CompanyRating` field to `NormalizedJob` — rejected,
out of scope and duplicates `companyintel`'s existing responsibility.

## R5: Detail-page enrichment fields

**Decision**: `GlassdoorAdapter.FetchDetail(ctx, jobURL, config) (GlassdoorDetailPatch, error)`
returning description, salary estimate (with estimate flag), posted-at, and availability —
same shape and calling convention as `IndeedDetailPatch`/`RemoteOKDetailPatch` — called from
a new `enrichGlassdoor` method in `enrichment.Handler`, dispatched from the existing `switch
job.SourceKey` in `ProcessTask`.

**Rationale**: Matches the established enrichment contract exactly; additive, not a redesign
of the dispatch mechanism.

## R6: Subscription URL validation

**Decision**: `validateGlassdoorSubscriptionURL` in `internal/subscriptions/service.go`
accepts hosts `glassdoor.com`/`*.glassdoor.com` (mirroring `validateIndeedSubscriptionURL`'s
host-suffix check) and rejects a URL path shape that looks like a single job posting
(Glassdoor detail pages) rather than a search-results listing, returning a human-readable
reason (spec FR-015).

**Rationale**: Directly mirrors `validateIndeedSubscriptionURL`/`validateRemoteOKSubscriptionURL`
— same validation shape, same place, same failure-at-save-time guarantee (SC-008).

**Alternatives considered**: none — this is a direct precedent match, no open question.
