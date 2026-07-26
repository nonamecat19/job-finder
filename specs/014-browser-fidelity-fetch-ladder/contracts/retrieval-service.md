# Contract: `retrieval.Service` (Go internal interface)

Shared interface every scraped adapter uses instead of `scraping.Service.FetchHTML`/
`HTTPClient()` directly (FR-020). Lives in `apps/api/internal/retrieval`.

```go
package retrieval

type Service interface {
    // Fetch retrieves one page for the given host, walking the escalation
    // ladder from HostRetrievalState.currentRung upward until it reads
    // successfully or every rung has been tried. It never returns a bare
    // error for "blocked" — a blocked page is a PageOutcome, not a Go error.
    // A Go error return means something operational failed (bad URL,
    // context cancelled), not "the host said no."
    Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)

    // HostStatus reads the current HostRetrievalState for the operator
    // surface (FR-033) without making a request.
    HostStatus(ctx context.Context, host string) (HostStatus, error)

    // ClearRungPreference resets currentRung to the cheapest rung for host,
    // for the next run to re-test (FR-015, FR-014's manual counterpart).
    ClearRungPreference(ctx context.Context, host string) error

    // ClearCookies discards stored visitor state for host (FR-015).
    ClearCookies(ctx context.Context, host string) error

    // OverrideCoolingOff permits one on-demand run to bypass an active
    // cooling-off window without resetting its expiry (FR-027). Returns the
    // remaining cooling-off duration so the caller can surface the risk.
    OverrideCoolingOff(ctx context.Context, host string) (remaining time.Duration, err error)
}

type FetchRequest struct {
    URL              string
    Headers          map[string]string // adapter-specific additions, merged under BrowserIdentity's set
    UsesUserAccount  bool              // true ⇒ never escalate past `direct`; a Challenged/Refused result reports for manual resolution (FR-018)
    RefererPage      string            // optional: page this request should appear to have navigated from (FR-008)
}

type FetchResult struct {
    Outcome PageOutcome // Status, Method, Reason — see data-model.md
    Body    string      // populated only when Outcome.Status == Read
}
```

## Behavioral contract (verified by tests, not just types)

1. Given `HostRetrievalState.currentRung = "browser"` for `host`, `Fetch` MUST attempt `browser`
   first, not `direct` (FR-013).
2. Given a `direct` attempt returns `Challenged`, `Fetch` MUST retry the same `URL` at the next
   rung in the same call, not require the caller to re-invoke (spec User Story 2, scenario 2).
3. Given every rung from the starting point through `flaresolverr` returns `Challenged` or
   `Refused`, `Fetch` MUST return `FetchResult{Outcome: {Status: Challenged|Refused, ...}}` with
   a non-nil `error == nil` — i.e., "blocked" is a normal return value, never a Go `error`
   (so a naive `if err != nil` caller can't accidentally treat a block as a crash and vice versa;
   callers MUST branch on `Outcome.Status`).
4. Given `flaresolverr` is unconfigured or fails its health check, and the ladder would escalate
   to it, `Fetch` MUST return `Outcome.Status = Deferred` (or `Challenged`, per data-model —
   resolved in FR-017 as "blocked with that stated reason"), and MUST NOT return a Go `error`
   that would fail the surrounding run (FR-017).
5. Given `FetchRequest.UsesUserAccount = true` and rung `direct` returns `Challenged` or
   `Refused`, `Fetch` MUST return that outcome immediately without trying `browser` or
   `flaresolverr` (FR-018).
6. Given a host is cooling off and `OverrideCoolingOff` was not called this request, `Fetch`
   MUST return `Outcome.Status = Deferred` without making any network request (FR-026).
7. Given a successful `Read` at any rung, `Fetch` MUST persist that rung as `currentRung` and
   reset `consecutiveBlocks` to 0 before returning (FR-013).
