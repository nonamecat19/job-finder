# Phase 0 Research: Indeed Job Source

## R1: Retrieval mechanism — direct scrape vs. sidecar

**Decision**: Dedicated direct Indeed adapter (`adapters.IndeedAdapter`), independent of
`apps/jobspy-sidecar`'s existing `site="indeed"` capability. (User decision, spec FR-017.)

**Rationale**: The sidecar's Indeed support is a black box wrapping the third-party
`python-jobspy` library — it has no per-listing detail-fetch endpoint (needed for FR-009/US3
enrichment), and its failures are opaque (whatever `python-jobspy` does upstream). A direct
adapter matches the existing DOU/Djinni/WorkUa pattern exactly: same `scraping.Service`,
same goquery parsing, same enrichment hook, same failure semantics the rest of the codebase
already knows how to operate and debug.

**Alternatives considered**:
- *Surface sidecar's Indeed as its own source key*: rejected — smallest short-term effort,
  but no detail-enrichment path (FR-009 unmet) and doubles down on an already best-effort
  third-party library's fragility rather than the codebase's own defensive-selector pattern.
- *Both, with sidecar as fallback*: rejected as unnecessary scope for a first iteration; the
  sidecar keeps working unchanged and independently, so nothing is lost by not wiring a
  fallback path now.

## R2: Search configuration entry point

**Decision**: Operator-pasted Indeed search URL via the existing `Subscription` flow
(`POST /api/subscriptions`), identical UX to DOU/Djinni. No keyword/location/country
parameter form. (User decision, spec FR-015.)

**Rationale**: DOU already works this way (subscription-URL only, no keyword search — see
`DouAdapter.Search`'s `fmt.Errorf("dou keyword search not implemented...")`). Indeed's own
search UI already exposes every filter (keywords, location, remote, date posted, salary,
job type) that a bespoke parameter form would have to reimplement; a pasted URL captures
all of it for free and keeps the adapter's job simple: fetch, paginate, parse.

**Alternatives considered**: keyword+location structured params (rejected — narrower than
what Indeed's own search supports, and inconsistent with DOU's precedent); supporting both
(rejected as unnecessary added surface for v1).

## R3: Indeed listing page structure & pagination

**Decision**: Parse job cards defensively with a multi-selector fallback chain
(title anchor first, several company/location/salary selector candidates), matching the
belt-and-suspenders style already used in `djinni.go`/`dou.go`/`workua.go`. Paginate by
incrementing the `start` query parameter on the pasted search URL in steps of 10 (Indeed's
own per-page count), stopping when a page returns zero new cards, a hard page cap is hit, or
the first card on a page repeats the previous page's first card (loop guard, mirroring
`djinniMaxSubscriptionPages`/`seenFirstHref`).

**Rationale**: A one-shot fetch of `indeed.com/jobs?q=...&l=...` confirms: job cards are
titled via an `<h3>` wrapping an `<a href>`; company name and "Remote"/location text follow
as plain text near the title; salary appears as free text (e.g. "$80 - $90 an hour -
Contract"); pagination links use `?start=0`, `?start=10`, `?start=20`, ... as an explicit
result-offset, not a page-number. No CAPTCHA was encountered on this fetch, but Indeed is
widely known to serve Cloudflare/bot-challenge pages intermittently and by IP reputation —
this is treated as an operational risk (see R4), not a design blocker, since every existing
scrape adapter in this codebase already treats markup drift and blocks as expected,
best-effort failure modes.

**Alternatives considered**: relying on a single strict CSS class selector — rejected,
Indeed's class names are non-semantic/hashed and known to churn; a strict selector would be
the first thing to break. Headless-browser rendering (via the existing `scraping.Service`
Chromium context used for PDF rendering) — deferred out of scope; plain HTTP+goquery is
consistent with every other adapter and is tried first, matching the constitution's
best-effort framing for external discovery sources.

## R4: Anti-bot / rate-limiting risk

**Decision**: Accept the risk as an operational characteristic, not a blocking design
problem. Mitigate via: (a) conservative request pacing — no request faster than every
500ms, matching FR-010 and the existing `douDetailDefaultDelay`/`WorkUaMinDelay` convention;
(b) a bounded page cap so a block can't spiral into a hung run; (c) `HealthCheck` reports
unhealthy with a clear reason when Indeed blocks or the response can't be parsed, exactly as
DOU/Djinni already do, so operators see it on the Sources screen without digging into logs;
(d) a run that gets blocked mid-pagination keeps whatever it already collected and reports
partial success (FR-008), never discarding progress.

**Rationale**: This mirrors the constitution's explicit framing of external discovery
sources (Adzuna, LinkedIn/Indeed/Glassdoor via JobSpy) as "best-effort... not a hard
dependency for core functionality" — Indeed as a direct source inherits the same posture.
No anti-bot evasion (proxy rotation, CAPTCHA solving, browser fingerprint spoofing) is
attempted; that would cross into the explicitly out-of-scope territory the spec's
Assumptions section already rules out ("aggressive crawling and anti-bot evasion are out of
scope").

**Alternatives considered**: headless-browser fetch to better blend in as a real browser —
deferred; plain HTTP is tried first since it's what every existing adapter does and keeps
the implementation consistent; if empirical live testing during implementation shows plain
HTTP is reliably blocked, escalating to `scraping.Service.BrowserContext` is a contained,
same-adapter follow-up, not a redesign.

## R5: Detail-page enrichment fields

**Decision**: `IndeedAdapter.FetchDetail(ctx, jobURL, config) (IndeedDetailPatch, error)`
returning description, remote flag, posted-at, and raw HTML/text — same shape as
`DjinniDetailPatch`/`DouDetailPatch` — called from a new `enrichIndeed` method in
`enrichment.Handler`, dispatched from the existing `switch job.SourceKey` in `ProcessTask`.

**Rationale**: Matches the established enrichment contract exactly; `enrichment.Handler`
already takes one adapter per enrichable source as a constructor field and dispatches by
source key string — adding Indeed is additive, not a redesign of that dispatch.

**Alternatives considered**: none — this is a direct application of the existing pattern,
no meaningful alternative structure exists within the codebase's conventions.

## Remaining NEEDS CLARIFICATION

None. All Technical Context unknowns resolved above.
