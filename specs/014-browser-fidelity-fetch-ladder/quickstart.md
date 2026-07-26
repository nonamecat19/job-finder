# Quickstart: Validating the Escalation Ladder

## Prerequisites

- `make up` (Postgres, Redis running) with the `scraping-extras` Compose profile enabled so
  `flaresolverr` is up: `docker compose --profile scraping-extras up -d flaresolverr`
- `FLARESOLVERR_URL` set in `apps/api`'s env (already an existing config key)
- `apps/api` built against the new `host_retrieval_state` migration: `make migrate` (or
  whatever the repo's goose invocation is under `make`)

## Scenario 1: Direct rung succeeds, no escalation (User Story 2, acceptance 1)

1. Run any currently-healthy adapter (e.g. `remotive`) end to end.
2. Assert: `HostRetrievalState.currentRung` for its host stays `"direct"`; `SourceRun.verdict =
   "success"`; no chromedp/flaresolverr process activity in logs.

## Scenario 2: Challenge triggers escalation and is remembered (User Story 2, acceptance 2–3)

1. Point the `direct` rung's HTTP client at a local `httptest.Server` fixture that serves a
   Cloudflare-style challenge page on first request, then real content on a request carrying
   the resulting clearance cookie (test double, not a live third-party host).
2. Run the adapter once: assert the run still returns real listings and
   `HostRetrievalState.currentRung` is now `"browser"` (or whichever rung the fixture requires).
3. Run again: assert the second run's first attempt goes straight to the recorded rung — no
   repeat challenge round-trip on `direct` (SC-006).

## Scenario 3: Every rung challenged ⇒ honest block, not empty success (User Story 3, acceptance 1)

1. Point all three rungs at fixtures that always return challenge-shaped responses.
2. Run the adapter: assert `SourceRun.verdict = "blocked"` with a non-null `blockReason`, and
   that the run is NOT reported as a successful zero-listings run (SC-002).

## Scenario 4: Partial block (User Story 3, acceptance 3)

1. Configure a multi-page source where page 1 succeeds and page 2 is always challenged.
2. Run: assert `verdict = "partial"`, `blockedCount = 1`, and page 1's listings are still
   present in the result.

## Scenario 5: Cooling-off and override (User Story 3, acceptance 4–5)

1. Force `consecutiveBlocks` past the configured threshold for a test host.
2. Trigger a normal scheduled run against that host: assert it is skipped/deferred and no
   request reaches the fixture server.
3. Call the on-demand override endpoint (`POST /api/hosts/:host/override-cooling-off`) and
   re-run: assert the run proceeds, the response/UI states the remaining cooling-off duration,
   and `coolingOffUntil` is unchanged afterward.

## Scenario 6: User-account source never escalates (User Story 2, acceptance 7)

1. Use an adapter flagged `UsesUserAccount = true` (e.g. the djinni/jobleads session-based
   adapters) against a fixture that challenges the `direct` rung.
2. Assert the run reports blocked immediately, with no chromedp/flaresolverr invocation at all.

## Scenario 7: Real-browser isolation (User Story 2, acceptance 8 / SC-012)

1. During a `browser`-rung run, assert (via a code-level check or process inspection in the
   integration test) that the chromedp allocator instance used is distinct from
   `scraping.Service.BrowserContext()`'s instance — e.g. by asserting they are different
   processes/user-data-dirs, or that triggering a resume-PDF render concurrently does not block
   on or share state with an in-flight third-party page render.

## Scenario 8: Per-host budget across concurrent sources (User Story 5, acceptance 1–2)

1. Register two test adapters targeting the same host.
2. Run both concurrently with a low `budgetLimit` configured for that host.
3. Assert total requests reaching the fixture server never exceed `budgetLimit`, and requests
   beyond it come back as `Deferred` outcomes, not failures (SC-007).

## Dashboard check (User Story 4)

1. With a host in a blocked/cooling-off state from Scenario 3/5, open
   `apps/dashboard`'s Sources screen.
2. Confirm the per-host panel shows current rung, last block time/reason, and cooling-off
   status without opening any logs (SC-004).
3. Use the "clear rung preference" and "clear cookies" controls; confirm the next run starts
   fresh (User Story 4, acceptance 3–4).
