# Phase 0 Research: Djinni Preset-Search Rewrite

**Date**: 2026-07-28
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)

This research resolves every unknown needed for Phase 1 design. Every
decision is grounded in the codebase as cited. Open citations use
`file:line` form.

---

## R0. Summary of findings

The existing adapter already supports basic-search as one of two modes
(spec `015-djinni-basic-search-mode`). The rewrite is therefore mostly
**deletion**, not new logic:

- Drop the dashboard (`subs/{id}/`) search path and the session/login
  machinery, keep the basic-search path verbatim.
- Collapse the URL validator to preset-only and a goose migration that
  deletes pre-existing `subs/{id}/` subscriptions, per the user's
  choice of option B at specify-time.
- Anonymous fetch flips `UsesUserAccount()` from `true` to false,
  which *improves* retrieval resilience by letting the public page use
  the full browser-identity ladder instead of being pinned to the
  direct rung.
- No DTO/type changes → no `tygo`/`sqlc`/`packages/shared` regeneration.
- The dashboard display summary logic is behavior-preserving; only the
  dashboard-mode UI marker collapses.

The 8 decisions below are the result of investigating each unknown.

---

## R1. What the "preset-search URL" is, exactly

**Decision**: A preset-search (a.k.a. basic-search) URL is any URL whose
host is `djinni.co` or `www.djinni.co`, whose path is `/jobs` or
`/jobs/`, and whose query string contains `search_type=basic-search`.
Optional filters: `primary_keyword`, `salary` (integer string),
`exp_level` (repeatable, `Ny` tokens), `employment` (`remote` today).
Any other query params are tolerated and re-issued verbatim but not
interpreted.

**Rationale**: This shape is what `djinni_searchmode.go:55` and
`djinniSearchSummary.ts:18-27` already gate on, what
`validateDjinniSubscriptionURL` already accepts at
`subscriptions/service.go:273-276`, and what the spec FR-001/FR-002
codifies. The example URL in the spec (`?search_type=basic-search
&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y
&exp_level=3y&employment=remote`) is the canonical case.

**Alternatives considered**:
- *Accept any `djinni.co/jobs*` URL.* Rejected — would silently re-accept
  single-posting URLs (`/jobs/{id}`) the validator already rejects today
  (`subscriptions/service.go:277-280`) and would lose the "preset"
  distinction the operator relies on.
- *Strictly enumerate allowed query params.* Rejected — the existing
  convention (`djinni_searchmode_test.go:145-150`, spec edge "unrecognized
  extra query parameter") preserves unknown params verbatim because
  Djinni may add filters later; enumerating would over-fit.

---

## R2. Which existing files/symbols are deleted vs. kept

**Decision** (concrete inventory):

### Deleted
- `apps/api/internal/jobsources/adapters/djinni_session.go` (whole file):
  `DjinniSessionProvider`, `DjinniConfigStore`, `DjinniSession`,
  `djinniLogin`, `djinniCSRFToken`, `djinniBaseURL`, `djinniUserAgent`.
- `apps/api/cmd/server/platform.go:45,110`: `Platform.DjinniSession`
  field and its construction from `cfg.DjinniEmail`/`cfg.DjinniPassword`.
- `apps/api/cmd/server/compose_sources.go:12,46`: the `Session:` arg to
  `DjinniAdapter{…}` and the line `p.DjinniSession.Sources = sourcesSvc`
  (keep the `JobLeadsSession` half at `compose_sources.go:47`).
- `apps/api/internal/config/config.go:97-98`: `DjinniEmail`,
  `DjinniPassword`.
- `apps/api/internal/config/defaults.go:35`: the `"DJINNI_EMAIL",
  "DJINNI_PASSWORD"` entries in the optional-keys list (keep
  `JOBLEADS_EMAIL`, `JOBLEADS_PASSWORD` — same line).
- `.env.example:70-71`: the `DJINNI_EMAIL=` / `DJINNI_PASSWORD=` lines.
- In `djinni.go`: `DjinniAdapter.Session` field (`:31`), `authHeaders`
  (`:37-48`), `setDjinniCookie` (`:50-56`), the `fetchDoc` relogin branch
  (`:62-87`), `fetchParse` auth bits (`:89-95`), `UsesUserAccount`
  (`:107`), the `DjinniModeDashboard` switch arm (`:121-122`), and
  `scrapeDashboard` (`:194-201`), `djinniIsLoginPage` (`:231`).
- In `djinni_searchmode.go`: `DjinniModeDashboard` (`:24`), the dashboard
  branch of `djinniDetectShape` (`:50-53`).
- In `subscriptions/service.go:269-272`: the dashboard accept-branch of
  `validateDjinniSubscriptionURL` (replace with a reject that explains
  only preset URLs are supported).
- Testcases that exercise dashboard/session/login:
  `djinni_test.go`'s `djinniLoginServer`, `fakeConfigStore`,
  `TestDjinniSession*`, `TestDjinniLogin*`, `TestDjinniIsLoginPage`,
  `TestDjinniSearchDashboardModeUnchanged`; `djinni_searchmode_test.go`
  dashboard-detect cases (e.g. the `subs/{id}/` cases of `TestDjinniDetect`).

### Kept (verbatim or trimmed-to-anonymous)
- `djinni.go`: `DjinniAdapter{Scraping}`, `Key()="djinni"`,
  `Kind()=SourceKindScrape`, `NeedsDetail()=true`, `HealthCheck`,
  `firstNonEmpty`, `djinniMaxSubscriptionPages=50`, `djinniRemoteRe`,
  `paginateDjinni`, `scrapeBasicSearch`, `parseDjinniCards`,
  `DjinniDetailPatch`, an anonymized `FetchDetail` (calls
  `d.Scraping.FetchHTML` directly, no relogin), `Search` (basic-search
  branch + the keyword fallback).
- `djinni_searchmode.go`: `DjinniModeUnknown`/`DjinniModeBasicSearch`,
  `DjinniDetect`, `djinniDetectShape` (basic-search-only),
  `BasicSearchFilters`, `ParseBasicSearch`, `summarizeExpLevels`.
- `djinniSearchSummary.ts`/`djinniSearchSummary.test.ts`: verbatim. The
  dashboard branch already returns `null` today; "is null" is still a
  useful contract ("this URL is not a preset URL"), so the helper is a
  stable contract, not a thing to be rewritten.
- `compose_tasks.go:23-27`: `DjinniDetailDelayMs` stays (still paces the
  enrich queue default delay). Only the comment at `config.go:104-106`
  is reworded to drop "authenticated djinni account."
- `enrichment/handler.go:144-179`: the `enrichDjinni` branch stays
  (preset listings still `NeedsDetail=true` per FR-014); only the
  `sources.GetByKey`/`DecryptConfig` call at `:145-149` is removed
  (config no longer carries a session).
- `httpapi/sources.go:96-103`: the `/enrich` guard for `djinni` + `dou`
  stays verbatim (both still `NeedsDetail`).
- Seed fixtures `seed/savedsearch.go:34` (search targets — not a sub
  URL), `seed/sourceruns.go:19-20` (run fixtures), `seed/testdata.go`
  (static job rows) — kept; they don't exercise the new path.
- Seed *subscription* `seed/subscriptions.go:18-23`: replaced with a
  valid preset URL (the spec's Golang example is the canonical
  replacement), because the current seeded URL
  (`https://djinni.io/jobs?technology=golang&remote=true`) has the wrong
  host and `seedSubscriptions` bypasses validation at
  `subscriptions.go:79-94`.

**Rationale**: This is the smallest surface that satisfies FR-006/FR-007
(delete the legacy paths and login config) without touching shared
infrastructure that FR-014/FR-016/FR-017 require the preset path to keep
reusing. JobLeads mirrors Djinni's session model (`jobleads_session.go`
header literally says "mirrors `djinni.go`/`djinni_session.go`"), so
JobLeads must **not** be deleted alongside — only the Djinni half of
each shared line is removed (`compose_sources.go:46-47`).

**Alternatives considered**:
- *Delete `djinniSearchSummary.ts` entirely.* Rejected — it is the
  deterministic contract for the basic-search display the operator reads
  (spec FR-010..FR-013); removing it would force every display decision
  into the React render path, which is harder to test.
- *Also drop `DjinniDetailDelayMs`.* Rejected — it is the default enrich
  queue delay and FR-016 requires pacing parity; the comment alone
  changes.
- *Keep `djinni_session.go` for JobLeads reuse.* Rejected — JobLeads has
  its own `jobleads_session.go`; the Djinni file is Djinni-only.

---

## R3. How the preset path reuses shared infrastructure after the rewrite

**Decision**: The remaining basic-search path uses exactly the same
shared infrastructure it uses today, with the only difference being the
fetch is anonymous:

- `Scraping *scraping.Service` (`djinni.go:31`) — `scraping.Service.FetchHTML(ctx, url, headers)` with `headers = {}`. Wired from the shared `retrieval.Service` browser-identity ladder (`platform.go:65-77`). With `UsesUserAccount()` now false, the ladder's `Credentialed` gate (`adapter.go:41-57`) releases Djinni to escalate rungs — including browser and FlareSolverr — which is *better* than today's direct-rung pinning.
- `paginateDjinni` loop guard: `q.Set("page", strconv.Itoa(i))` for `i in 1..50`, stop on empty page (`parseDjinniCards` returns 0), stop when the first card href repeats between consecutive pages (the redirect-to-page-1 sentiment guard preserved from `djinni.go:181-184`).
- Param preservation + `page` strip: `scrapeBasicSearch` forces `q.Set("page","1")` overriding any saved `page=N` (`djinni.go:217-219`), preserves all other params verbatim. Required by FR-002 and the spec edge "`page=N` already present."
- Card parse: `parseDjinniCards` (`djinni.go:237-285`), defensive selectors, `strutil.Truncate` for Cyrillic, produces `dto.NormalizedJob` with `SourceKey:"djinni"`.
- Enrich: ingestion defers match+ghost to the enrich queue (`ingestion/handler.go:185-196,300-305`); `enrichDjinni` (`enrichment/handler.go:144-179`) calls `h.djinni.FetchDetail` + `UpdateJobDetail`. `FetchDetail` becomes anonymous (`djinni.go:303-311` minus `authHeaders`/`fetchDoc` relogin).
- Dedup: `DedupeKey(company,title,URL)` (`ingestion/handler.go:242-310`) — generic, no Djinni-specific code; SC-005 depends on it.
- Health flag: `computeVerdict`/`flagIfUnhealthy` (`ingestion/handler.go:218-233,386-405`), block → `verdict="blocked"`, source unhealthy after 3 consecutive failures (`unhealthyAfterConsecutiveFailures=3`). FR-015/SC-006 require this posture preserved — the rewrite touches none of it.

**Rationale**: This is the union of FR-003/FR-004/FR-005/FR-014/FR-015/FR-016/FR-017. Every piece is already used by basic-search today; the rewrite only removes the *other* mode, so the preset path's dependencies are unchanged. The one behavior gain (full retrieval ladder) is a deliberate consequence of `UsesUserAccount()` → false.

**Alternatives considered**:
- *Re-implement `paginateDjinni` using a generic helper.* Rejected — it's already the right shape and has the loop guard the spec requires; rewriting for "purity" risks the existing guard semantics for no value.
- *Keep `UsesUserAccount()=true` to preserve today's retrieval posture.* Rejected — today's posture is *worse* for anon pages (pinned to direct rung); the spec FR-005 says reuse the fetch path, not the (suboptimal) credentialed gate. Letting the ladder escalate is the intended fix.

---

## R4. Validator behavior after the rewrite (save-time + run-time)

**Decision**:

- `validateDjinniSubscriptionURL` (the `case "djinni"` arm of
  `subscriptions/service.go:135-156`) becomes a **single** accept branch:
  host `djinni.co`/`www.djinni.co`, path `/jobs` or `/jobs/`,
  `search_type=basic-search` query present. Every other djinni.co URL
  — including `/my/dashboard/subs/{id}/` (formerly accepted at
  `service.go:269-272`) — is rejected with a single human-readable
  reason: "Djinni subscriptions support only preset-search URLs
  (`djinni.co/jobs/?search_type=basic-search&…`); dashboard URLs are no
  longer supported."
- `DjinniDetect`/`djinniDetectShape` (`djinni_searchmode.go`) lose the
  dashboard arm. `DjinniModeUnknown` stays — the adapter's `Search`
  keeps a defensive "neither..." error for `DjinniModeUnknown` even
  though save-time rejects first (defense-in-depth).
- The seeded subscription URL `subscriptions.go:21` is replaced with
  the spec's Golang preset example (or dropped) so the seed no longer
  advertises a now-invalid URL.

**Rationale**: FR-001/FR-008 and SC-007. The dashboard URL fails the
new validator with a reason that names the *replacement* shape, not
just "invalid", so the operator has an actionable message. Keeping
`DjinniModeUnknown` as a defensive run-time error (rather than removing
the enum) is cheap and guards against a save-time-bypass (e.g.,
`seedSubscriptions`).

**Alternatives considered**:
- *Different rejection reason for `subs/{id}/` vs. other invalid.* Rejected — the user's choice of option B explicitly prunes those subs, so the message should reflect "no longer supported", not "would you like to log in"; a single message is simpler and matches FR-008's "stating that only preset-search URLs are supported."
- *Delete `DjinniModeUnknown`.* Rejected — it's the defense-in-depth value when a URL slips past validation; cheap to keep.

---

## R5. Migration strategy for pre-existing `subs/{id}/` subscriptions

**Decision**: A new goose migration
`apps/api/internal/db/migrations/00027_drop_djinni_dashboard_subs.sql`
(next sequential number after `00026_host_retrieval_state.sql`, per the
constitution's unique-sequential rule) does a **delete + audit**:

```sql
-- +goose Up
DELETE FROM "Subscription"
WHERE "sourceKey" = 'djinni'
  AND "url" LIKE '%/my/dashboard/subs/%'
RETURNING "id", "name", "url", "createdAt";
```

The `RETURNING` rows are written to an audit log. Two implementation
options for the audit log:

1. **A new `DjinniLegacySubAudit` table** the migration creates, then
   inserts the `RETURNING` rows into via a single `INSERT … SELECT`
   from a CTE. Self-contained: the audit lives in the DB and is
   operator-queryable.
2. **`psql` `\copy` to a CSV** that the migration's `up` statement
   emits. Lighter but requires shell side-effects, less portable.

**Recommended**: option 1 (new audit table) — single migration,
self-contained, queryable. Schema documented in
[data-model.md](./data-model.md).

The migration's `-- +goose Down` is a no-op (the deleted rows cannot be
recovered — explicit comment to that effect, matching the spec's
"recorded list of what was removed" semantic: the audit table is the
record, the rows are gone).

**Rationale**: User chose option B at specify-time (prune/delete via a
one-time migration). The dashboard URL cannot be auto-converted
(`subs/{id}/`'s filter set lives in Djinni's server-side DB keyed by
`<id>`, not in the URL — see R8), so destructive delete + audit is the
only faithful strategy.

**Alternatives considered**:
- **Option A from specify** (mark stale in place + surface). Rejected by
  the user at Q1; chosen path is B.
- *Delete without audit.* Rejected — SC-009 explicitly requires "a
  recorded list of the deleted subscriptions is available" so the
  operator can recreate as preset URLs.
- *Application-level migration (Go on startup).* Rejected — the repo's
  convention is goose for schema-attached data changes; running ad-hoc
  deletes on every boot is a code smell and breaks the "migrate once"
  invariant. A goose migration is canonical.

---

## R6. What happens to the orphaned `sessionCookie` in `JobSource.config`

**Decision**: **Leave it.** The `JobSource.config` JSON for key
`"djinni"` may still carry `{"sessionCookie":"…"}` (encrypted via
`crypto.EncryptJSON`, per `service.go:34-43`) after the code stops
reading it. It is inert:

- The masking path (`service.go:66-76`) still masks it on read — no
  leak.
- `jobsources.Service.Update` (`service.go:178-201`) only patches fields
  the operator submits; it never auto-clears `sessionCookie`, so the
  blob churns out naturally the next time the operator saves the source
  config.
- A raw `UPDATE "JobSource" SET config = '…'` from a migration can't
  decrypt/re-encrypt portably (per `crypto.EncryptJSON`).

**Rationale**: Avoids fragile crypto round-trips in SQL; the cookie is
already stale (Djinni sessions expire) and unread. Document the
non-action explicitly in the migration file.

**Alternatives considered**:
- *Strip `sessionCookie` via migration.* Rejected (crypto round-trip,
  abortable on key rotation).
- *Drop the entire `JobSource` row for `"djinni"`.* Rejected — that
  would remove the source entirely and break the registry; the
  `JobSource` row must stay (the adapter still runs preset searches
  against it).

---

## R7. Display behavior post-rewrite

**Decision**: The dashboard's `summarizeDjinniBasicSearch`
(`djinniSearchSummary.ts`) is **kept verbatim**. It accepts a preset
URL and returns the summary string ("Node.js · $3000 · 2–5 years ·
remote") or `null` if the URL isn't a preset URL. The four display
rules (FR-010..FR-013, SC-003/SC-004) all flow from this helper and its
Go-side mirror `summarizeExpLevels` (`djinni_searchmode.go:99-162`):

- `exp_level` set forms an unbroken integer run → "min–max years" (en-dash `\u2013`).
- Non-consecutive or any non-parseable token → "a, b, … years".
- Dedup preserves first-occurrence order.
- Empty input → `""`, and an all-empty filter set → `null`.

`SourcesPage.tsx` edits are minimal:

1. **Placeholder** (`SourcesPage.tsx:322`) changes from
   `https://djinni.co/my/dashboard/subs/{id}/` to a preset example
   (`https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote`).
2. **`djinniModeMarker`** (`SourcesPage.tsx:422-427`) simplifies: with
   no dashboard subscriptions remaining post-migration, the
   `· dashboard` branch is unreachable. It can either be removed
   (collapse to a single `· basic-search` marker when
   `basicSearchLabel !== null`) or left as a dead branch (defensive).
   **Recommended**: remove — dead branches rot, and save-time + the
   migration guarantee dashboard subs don't exist.

**Rationale**: FR-010..FR-013 / SC-003 / SC-004 are already implemented
and tested (`djinniSearchSummary.test.ts:4-84`); the rewrite must be a
behavior-preserving trim. The TS helper is pure and tested; the React
render path that calls it only needs the placeholder fix and the
dead-marker removal.

**Alternatives considered**:
- *Rewrite `summarizeDjinniBasicSearch` in pure CSS.* Rejected — would
  throw away the v015-tested contract.
- *Keep the `· dashboard` marker as a "stale URL" hint.* Rejected —
  post-migration there are no such rows, so the hint would never fire;
  if one *did* appear (bug), the row's default label still shows the raw
  URL, which is enough of a hint without a stale-mode marker.

---

## R8. Why `subs/{id}/` cannot be auto-migrated to a preset URL

**Decision**: It cannot. A `subs/{id}/` dashboard URL encodes a saved
filter set **server-side** in Djinni's own DB, keyed by `<id>`. The URL
itself carries no `primary_keyword`/`salary`/`exp_level`/`employment`
parameters — the dashboard page renders the cards the server returns
for that id. Today's `scrapeDashboard` (`djinni.go:194-201`) just
paginates that URL and parses whatever cards the server returns; it
never extracts the underlying filters.

To programmatically convert `subs/{id}/` → preset URL, you would have
to:

1. Log in with the user's Djinni credentials (no longer available —
   login config is being deleted by FR-007).
2. Scrape the dashboard page's filter widget to recover the saved
   filter set — a separate scraping target with its own fragility.
3. Build a preset URL from the recovered filters.

Step 1 is impossible post-rewrite (no credentials). Step 2 is a
one-time scrape that depends on a page shape we're *removing* support
for. The conversion is therefore not viable and explicitly out of
scope.

**Rationale**: Spec.md:351-354 already states this. The user's Q1
choice of option B (delete + audit) is the consequence.

**Alternatives considered**:
- *Pre-migration scrape pass.* Rejected (requires the soon-to-be-deleted
  session path; would have to ship *before* the deletion, then be
  removed — fragile two-phase change).

---

## R9. How the Go adapter should be structured post-rewrite

**Decision**: Keep the existing two-file split (`djinni.go` +
`djinni_searchmode.go`); delete `djinni_session.go`. Shape:

- `djinni.go` (~200 LOC): `DjinniAdapter{Scraping *scraping.Service}`,
  `Key/Kind/NeedsDetail/HealthCheck`, `Search` (basic-search branch only
  + keyword fallback), `paginateDjinni`, `scrapeBasicSearch`,
  `parseDjinniCards`, `FetchDetail` (anonymous), `DjinniDetailPatch`,
  `djinniRemoteRe`, `djinniMaxSubscriptionPages=50`, `firstNonEmpty`.
- `djinni_searchmode.go` (~110 LOC): `DjinniModeUnknown`,
  `DjinniModeBasicSearch` (drop `DjinniModeDashboard`), `DjinniDetect`,
  `djinniDetectShape` (basic-search branch only), `BasicSearchFilters`
  + `ParseBasicSearch` + `summarizeExpLevels` (verbatim).

This matches the **RemoteOK** (`remoteok.go`, ~312 LOC) and **Himalayas**
(`himalayas.go`, ~353 LOC) templates: a value struct holding a
`*scraping.Service`, a `Search` that errors cleanly when no subscription
URL is supplied, a paginating loop bounded by 50 pages, and an anonymous
`FetchDetail`. The `djinni_searchmode.go` file is Djinni-specific (it
parses Djinni's query shape), so it doesn't collapse into a generic
file — that's fine, it's already small and its tests guard a real
contract.

**Rationale**: Minimizes refactor surface. Files that have a reason to
stay separate stay separate; the deleted file was the only thing
tying the adapter to a user-account surface.

**Alternatives considered**:
- *Collapse `djinni.go` + `djinni_searchmode.go` into one file.*
  Rejected — the discriminator is reusable (e.g. by the dashboard via
  shared TS mirror) and its tests are independent; merging would couple
  them and bloat the adapter file to ~310 LOC for no benefit.
- *Mirror RemoteOK's single `APIURL` field on Djinni (test seam).*
  Optional — a `BaseURL string` test seam can be added if the rewrite
  needs an HTTP-server test for `Search`/`FetchDetail` (the existing
  `djinni_test.go` server-based tests already use a `httptest.Server`
  and `scraping.Service` configured to point at it, so no new seam is
  *required*; keep the tests as-is).

---

## R10. Test discipline post-rewrite

**Decision**: Prune precisely.

- Go (`apps/api/internal/jobsources/adapters/djinni_test.go`):
  - DELETE: `djinniLoginServer`, `fakeConfigStore`, `TestDjinniSession*`,
    `TestDjinniLogin*`, `TestDjinniIsLoginPage`,
    `TestDjinniSearchDashboardModeUnchanged`, and any basic-search
    test cases that exercised session-cookie plumbing (the cases that
    assert `setDjinniCookie` was called).
  - KEEP: every `TestDjinniSearchBasicSearch*` (single-page,
    multi-page, loop, param-preserve, page-strip) — these already run
    with `headers = {}` once the session fields are gone.
- Go (`djinni_searchmode_test.go`): drop the dashboard-detect cases of
  `TestDjinniDetect`; keep `TestParseBasicSearch` and
  `TestSummarizeExpLevels` (they guard the contract the dashboard
  shares).
- Dashboard (`djinniSearchSummary.test.ts`): **no edit needed**. The
  existing `summarizeDjinniBasicSearch(… "not a preset" …) returns null`
  cases become the "is this URL a preset URL" contract, which is still
  the right thing to assert.
- New Go test for the migration: a unit-style test under
  `apps/api/internal/db/migrations` (if the repo has such a convention)
  — or, more likely, an integration test under `test-integration`
  asserting (a) the migration deletes a seeded `subs/{id}/` row, (b)
  leaves a preset row intact, and (c) writes the audit row. If the repo
  has no migration-test harness, the migration is validated via the
  manual `quickstart.md` steps.

**Rationale**: Constitution Principle IV — per-language discipline, no
cross-service mocks. The whole-suite gate is `make test-lint` before
merge (FR cross-app change).

**Alternatives considered**:
- *Rewrite the v015 spec's tests from scratch.* Rejected — the v015
  tests already cover basic-search; rewriting them risks losing the
  guardrails the spec preserved.

---

## Needs-clarification ledger

All unknowns resolved:

| # | Topic | Resolution |
|---|---|---|
| R1 | Preset URL shape | Defined (host + `/jobs(\/)?` + `search_type=basic-search`) |
| R2 | Keep vs delete inventory | Listed file-by-file |
| R3 | Shared-infra reuse | Enumerated; one behavior gain (full retrieval ladder) |
| R4 | Validator post-rewrite | Single accept branch; defensive run-time `Unknown` retained |
| R5 | `subs/{id}/` migration | Goose `00027_`; delete + audit table |
| R6 | Orphaned `sessionCookie` | Left inert in encrypted blob; documented non-action |
| R7 | Display post-rewrite | Helper verbatim, placeholder + dead marker fixed in `SourcesPage.tsx` |
| R8 | Why no auto-conversion | Filter set lives server-side in Djinni's DB, not in URL; login path deleted |
| R9 | Adapter structure | Two-file (`djinni.go` + `djinni_searchmode.go`); delete session |
| R10 | Test discipline | Prune dashboard/session/login cases; keep basic-search + summary |

No open `[NEEDS CLARIFICATION]` markers remain.