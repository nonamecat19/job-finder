# Phase 0 Research: Browser-Fidelity Retrieval and Escalation Ladder

## 1. TLS/HTTP2 fingerprinting client (FR-002, FR-003)

**Decision**: Use `github.com/bogdanfinn/tls-client`, configured with its bundled Chrome-126
profile, as the transport for rung 1 (direct browser-fidelity request), replacing
`ratelimit.Transport`'s current wrap of `http.DefaultTransport` for this path.

**Rationale**: Go's standard `net/http`/`crypto/tls` stack produces a TLS ClientHello and HTTP/2
SETTINGS frame order that is trivially distinguishable from any real browser (extension order,
cipher suite list, ALPN, JA3/JA4 hash) regardless of how faithful the header set is — this is
exactly the "connection-level characteristics" FR-003 calls out as distinct from headers.
`tls-client` ships maintained per-browser-version fingerprint profiles (including header order
via its own header-ordering support) and is already a header-and-TLS-together solution, which
satisfies FR-004 (single configured identity used consistently) without stitching two libraries
together. It plugs in as a drop-in `http.Client`-compatible interface, so `ratelimit.Transport`
still wraps it as `Base` — per-host pacing is preserved unchanged.

**Alternatives considered**:
- `github.com/refraction-networking/utls` directly — lower-level (ClientHello only, no
  HTTP/2 frame/header-order matching, no maintained per-browser profile registry); would
  require hand-maintaining the fingerprint as Chrome versions roll, conflicting with FR-002's
  "MUST describe a browser release that is current."
- Do nothing (headers only, standard `net/http` transport) — rejected: acceptance scenario 4
  and FR-003 explicitly require connection-level consistency, and several already-blocked
  sources (wellfound, glassdoor, jobgether — see research from codebase survey) are exactly the
  kind of bot-management-fronted hosts that fingerprint at the TLS layer first.

## 2. Real-browser rung isolation (FR-019, SC-012)

**Decision**: A second, separate chromedp `ExecAllocator`/browser context, launched and torn
down independently from `scraping.Service.BrowserContext()` (the existing resume/PDF renderer).
Own process, own user-data-dir, no shared `browserCtx` field.

**Rationale**: `scraping.go:97-100` already documents the existing browser as trusted precisely
*because* it only renders first-party HTML. Reusing that same Chromium process for third-party
pages would silently invalidate that invariant (shared cookie jar / process / extensions
surface). A second allocator with the same `--no-sandbox` container accommodation but its own
lifecycle keeps the existing guarantee intact and gives the new rung an independent
crash/hang-cleanup boundary (edge case: "real-browser rung fails to start, hangs, or leaks a
process").

**Alternatives considered**: Reusing `BrowserContext()` with per-call `chromedp.NewContext` tabs
— rejected because tabs in the same browser process still share the underlying profile/cookie
store and process health, which is the isolation boundary FR-019 cares about, not just tab
separation.

## 3. Challenge/refusal detection (FR-012, edge case: status-code-independent detection)

**Decision**: Pattern-match known challenge-provider markup/response shape (Cloudflare
`cf-mitigated`/challenge HTML fingerprints, Akamai/PerimeterX/DataDome telltale script tags or
JSON shapes, generic "Just a moment...", "Access Denied", CAPTCHA iframe markers) against the
response body regardless of status code, plus structural signals (near-empty body where the
adapter's existing selector previously matched content, response size far below the host's
historical page size). Centralize this in `retrieval/challenge.go` as the single implementation
every rung and adapter calls, replacing the three adapters that currently do their own ad hoc
string matching (`wellfound.go`, `glassdoor.go`, `jobgether.go`).

**Rationale**: FR-020 requires one shared interface so "no source implements its own challenge
handling" — the three existing adapters already prove the per-adapter pattern doesn't scale
(duplicated, inconsistent marker lists) and is exactly what this feature must consolidate.
Structural/body-based detection (not status code) matches the edge case directly: "a host
serves a challenge under a success status code."

**Alternatives considered**: Status-code-based detection (403/503 heuristics) — rejected
outright by FR-012 and the edge case. A pluggable per-adapter detector interface — rejected
because it reintroduces the per-source coupling FR-020 exists to remove; a single shared
detector with a marker list that grows over time is simpler and centrally testable.

## 4. Per-host daily budget and cooling-off state (FR-026, FR-030, FR-031)

**Decision**: Extend the existing `ratelimit` foundation with a Postgres-backed
`HostRetrievalState` row per host (see data-model.md) read/written by the `retrieval` package,
rather than folding budget/cooling-off into the in-memory `ratelimit.Transport` limiter map.

**Rationale**: `ratelimit.Transport`'s per-host `rate.Limiter` map is in-process and unpersisted
by design (steady-state pacing only) — it already correctly handles FR-031 (pacing shared across
concurrent sources targeting one host, since it's keyed by `req.URL.Host` process-wide). Daily
budget, consecutive-block count, and cooling-off expiry, by contrast, must survive restarts
(FR-005 parity: "persist ... across runs and restarts") and be visible to the operator surface
(FR-033), which requires durable storage, not an in-memory map. The two mechanisms are
complementary: `ratelimit.Transport` still enforces the steady per-request cadence; the new
`HostRetrievalState` gate sits above it and refuses/defers a request before it reaches the
transport once the daily budget is spent or the host is cooling off.

**Alternatives considered**: Store budget/cooling-off in Redis (already used for BullMQ-class
queueing per the constitution) — rejected as unnecessary; this state changes at most a few times
per host per day, has no queueing semantics, and belongs next to the other per-host retrieval
facts (rung preference, cookies) in one row for the operator screen to read in one query.

## 5. Cookie/visitor-state storage format

**Decision**: Store serialized `http.Cookie` jar contents (a `net/http/cookiejar`-compatible
JSON encoding) plus any issued challenge-provider clearance token, in a `jsonb` column on
`HostRetrievalState`, encrypted at rest the same way `JobSource.config` secrets are handled
today (existing config encryption already lists `FLARESOLVERR_URL`-adjacent keys as
sensitive — same treatment applies here since cookies are credential-adjacent).

**Rationale**: Mirrors the existing precedent in `djinni_session.go`/`jobleads_session.go` of
persisting a session cookie through `Sources.Update(... config patch ...)`, but keyed by host
instead of by source (FR-007: MUST NOT share visitor state between hosts, and the per-source
`JobSource.config` JSONB is the wrong granularity — multiple sources can share one host).

**Alternatives considered**: A `net/http/cookiejar.Jar` kept only in memory — rejected, fails
FR-005's "persist ... across runs and restarts" requirement outright.
