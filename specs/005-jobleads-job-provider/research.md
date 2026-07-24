# Phase 0 Research: JobLeads Job Source

## R1: Access model — login-gated vs public

**Decision**: Treat JobLeads as login-gated from the start, following the `DjinniAdapter` +
`DjinniSession` pattern rather than the stateless `RemoteOKAdapter`/`IndeedAdapter` pattern.

**Rationale**: JobLeads is a subscription/account-based job platform; its saved-search and
listing-detail pages are only meaningfully populated for an authenticated account (unlike
RemoteOK's public API or Indeed/DOU's public HTML). This is already reflected in the spec's
FR-018 and Source Credentials entity. Djinni is the only existing adapter that already solves
"log in, persist a session cookie, retry once on session expiry" — reusing that shape avoids
inventing a second auth pattern in the codebase.

**Alternatives considered**: Degrade-to-anonymous like Djinni does when no credentials are
configured. Rejected for JobLeads specifically — Djinni's public `/jobs` search still returns
useful anonymous results, but JobLeads has no meaningful anonymous listing view, so an
unconfigured JobLeads source should fail clearly ("credentials not configured") rather than
silently returning near-empty results that look like a healthy zero-match run.

## R2: Credential storage

**Decision**: Reuse the exact Djinni mechanism unchanged: raw email/password come from env
vars (`JOBLEADS_EMAIL`, `JOBLEADS_PASSWORD`, added to `config.go` and its secret-list — never
written to the DB), a `JobLeadsSession` (mirroring `DjinniSession`) logs in on demand and
persists only the resulting session cookie into the `job_source.config` JSON column through
`jobsources.Service.Update`, which already encrypts that column via `internal/crypto`
(`EncryptJSON`/`DecryptJSON`, AES-256-GCM, keyed by `CONFIG_ENCRYPTION_KEY`) before it hits
Postgres. `Config`/`Update` transparently decrypt/encrypt, so the adapter code never handles
raw ciphertext.

**Rationale**: This is the codebase's only existing pattern for a login-gated source and it
already satisfies FR-018 (secure storage, not exposed in logs/UI) end to end — no new crypto
code, no new schema, no new dashboard credential-entry UI (mirrors "operator sets env vars,
restarts the service" for Djinni today). Building a second, different credential mechanism for
one more source would be unwarranted divergence.

**Alternatives considered**: A per-source credentials field in the Sources screen UI (dashboard
form for email/password, sent to a new API endpoint). Rejected — no such UI exists for Djinni
either; env-var configuration is the established operational convention for this class of
secret in the current deployment model, and adding a UI credential form is out of scope for
matching Djinni's already-accepted pattern.

## R3: Fetch mechanism — HTML scrape vs API

**Decision**: Scrape JobLeads's server-rendered saved-search results and listing-detail pages
with `goquery`, via the same `scraping.Service.FetchHTML` transport every other adapter uses,
with the session cookie attached as a `Cookie` header (mirroring `setDjinniCookie`).

**Rationale**: No public, documented JobLeads JSON API is known to exist (unlike RemoteOK).
HTML scraping behind an authenticated session is the only viable path, consistent with
Djinni/DOU/Indeed. `Kind()` is therefore `dto.SourceKindScrape`.

**Alternatives considered**: None — no API alternative is known.

## R4: Login-page / session-expiry detection

**Decision**: Implement `jobLeadsIsLoginPage(doc *goquery.Document) bool` mirroring
`djinniIsLoginPage`: after any fetch, check whether the returned document is JobLeads's login
form (e.g., presence of a login-form selector or a redirect landing on `/login`) rather than
the expected results/detail page. On detection, trigger exactly one `Session.Refresh` + retry
(mirrors `DjinniAdapter.fetchDoc`); if still on the login page after retry, fail with a
distinguishable "authentication required" error (FR-011) rather than a generic parse failure.

**Rationale**: Session cookies expire; distinguishing "logged out" from "zero results" or
"markup changed" is required by FR-011 and the spec's "stored account credentials expire or
are revoked" edge case, and is exactly the failure mode Djinni's `fetchDoc` already handles.

**Alternatives considered**: Treating an expired session as a generic scrape failure with no
distinguishing reason. Rejected — violates FR-011 and would surface as a confusing "found 0 /
unparseable" error instead of an actionable "re-authenticate" signal in the source's health
state.

## R5: Job field mapping

**Decision**: Map JobLeads's saved-search result cards and detail pages to
`dto.NormalizedJob` following the Djinni/Indeed convention:

| JobLeads field | NormalizedJob field | Notes |
|---|---|---|
| listing detail-page path/ID | `ExternalID` | stable per-listing identifier (FR-003), parsed from the listing URL |
| job title | `Title` | |
| company name | `Company` | |
| location text | `Location` | may be empty for some postings |
| remote/hybrid/onsite indicator (when present) | `Remote` | best-effort text match, same convention as Djinni's `djinniRemoteRe` |
| salary text (when published) | `SalaryRaw` | often absent — JobLeads listings frequently omit salary, per the spec's "no salary or no explicit location" edge case |
| listing URL | `URL` | canonical JobLeads posting page |
| summary text at list-level; full body at detail-level | `Description` | list-level summary at ingestion (FR-003), full text after enrichment (FR-009) |
| posting date text | `PostedAt` | parsed/normalized to RFC3339, same convention as DOU's `dateText` handling |

**Rationale**: Field names are provisional pending the real markup (not yet captured as a
fixture); the mapping follows the same conventions as the closest existing scrape adapters so
implementation only needs to fill in real selectors, not invent new `NormalizedJob` usage.

**Alternatives considered**: None — `NormalizedJob`'s shape is fixed by the shared DTO
(constitution III); this feature does not extend it.

## R6: FetchDetail / enrichment shape

**Decision**: Implement `FetchDetail(ctx, jobURL, config) (JobLeadsDetailPatch, error)` that
fetches the individual listing's detail page (authenticated, via the same session/retry logic
as `Search`) and returns the full description and resolved posting date. If the detail page
returns a "listing no longer available" state (404 or an empty/removed-listing page), the
patch reports the listing as unavailable while preserving already-captured summary data,
matching the spec's corresponding edge case and FR-009.

**Rationale**: Mirrors Indeed's summary→full-description enrichment split, which is the
closest precedent for a source where the list view only has a summary.

**Alternatives considered**: None — this is the uniform enrichment shape every scrape adapter
already follows.

## R7: Subscription configuration shape

**Decision**: Accept a JobLeads saved-search URL as the subscription value (the URL an
operator would copy from their logged-in JobLeads account after configuring a search).
`validateSubscriptionURL` in `subscriptions/service.go` gains a `case "jobleads":` calling
`validateJobLeadsSubscriptionURL`, which checks the host is `jobleads.com` or a `.jobleads.com`
subdomain and rejects anything else with a human-readable reason (FR-015, FR-016), mirroring
`validateRemoteOKSubscriptionURL`'s host-suffix check.

**Rationale**: Matches every other source's precedent — operator pastes a URL through the same
subscription flow, no bespoke UI.

**Alternatives considered**: A structured keyword/filter input instead of a pasted URL.
Rejected for consistency with the rest of the codebase's subscription model and to avoid a
one-off UI branch in `SourcesPage.tsx`.

## R8: Rate limiting / request pacing

**Decision**: Apply the same fixed inter-request delay and page cap Indeed uses
(`indeedMaxSubscriptionPages`-equivalent constant, 500ms pacing) to JobLeads's saved-search
pagination, satisfying FR-010 directly rather than relying on a single-request shortcut (as
RemoteOK does, since RemoteOK has no pagination).

**Rationale**: JobLeads saved searches are expected to be paginated HTML like Indeed's, so the
same bounded-loop-with-pacing approach applies; a hardcoded page cap prevents a
redirect-to-page-1 loop or unbounded feed from running forever, consistent with
`djinniMaxSubscriptionPages`/`indeedMaxSubscriptionPages`.

**Alternatives considered**: Unbounded pagination relying only on "no more results" detection.
Rejected — the spec explicitly requires a bounded request count per run (FR-010) independent
of upstream behavior.
