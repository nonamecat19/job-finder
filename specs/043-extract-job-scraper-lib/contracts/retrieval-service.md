# Contract: Retrieval Service

The public surface of the library's retrieval engine — the multi-rung fetch ladder with challenge detection, per-host pacing, and cooling-off. Consumers implement `StateStorePort` (see [state-store-port.md](./state-store-port.md)) and construct the engine; the library provides the `Service` interface and the three rung implementations.

## Interface

```go
// Package retrieval
package retrieval

import (
    "context"
    "time"
)

type FetchRequest struct {
    URL             string
    Headers         map[string]string
    UsesUserAccount bool
    RefererPage     string
}

type FetchResult struct {
    Outcome PageOutcome
    Body    string
}

type HostStatus struct {
    Host              string
    IdentityVersion   string
    CurrentRung       string
    LastBlockAt       *time.Time
    LastBlockReason   string
    CoolingOffUntil   *time.Time
    CrawlDelaySeconds *int
    Pacing            HostPacing
}

type HostPacing struct {
    RequestsPerSecond float64
    IntervalSeconds   float64
    Source           string
}

type Service interface {
    Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
    HostStatus(ctx context.Context, host string) (HostStatus, error)
    ClearRungPreference(ctx context.Context, host string) error
    ClearCookies(ctx context.Context, host string) error
    OverrideCoolingOff(ctx context.Context, host string) (time.Duration, error)
}
```

## Engine implementation (library provides)

The library provides `NewEngine(identity *BrowserIdentity, store StateStorePort, opts EngineOpts) Service`:
- constructs the three rungs (`directRung` from `bogdanfinn/tls-client`, `browserRung` from `chromedp`, `flareSolverrRung` from HTTP);
- `EngineOpts` carries the rung-enablement flags and the `CheapRungRetestInterval`/`CoolingOffThreshold`/`CoolingOffBaseDuration` config values (plain values, no `viper` — FR: library does not read config);
- the returned `Service` is the library's `*engineImpl` (the renamed `ServiceImpl` minus the DB/crypto/config imports).

The app's `retrieval/service_impl.go` is replaced by a thin constructor call:
```go
svc := jobscraper.NewEngine(identity, stateStorePort, jobscraper.EngineOpts{
    BrowserEnabled:       cfg.BrowserEnabled,
    FlaresolverrURL:     cfg.FlaresolverrURL,
    CheapRungRetestInterval: cfg.CheapRungRetestInterval,
    CoolingOffThreshold:     cfg.CoolingOffThreshold,
    CoolingOffBaseDuration:  cfg.CoolingOffBaseDuration,
})
```

## Pacing (library-internal)

The library owns the rate-limiting transport (vendored from `internal/ratelimit`, ~120 lines, deps `golang.org/x/time/rate` + stdlib). `NewEngine` wires the transport to the `StateStorePort`'s `Get` for crawl-delay resolution and to the `opts.HostRPSOverrides` map. `HostStatus.Pacing` reads from the same transport. The app's `transport.go` becomes a 3-line wiring helper that calls `jobscraper.NewDefaultTransport(stateStorePort, overrides)`.

## Import graph guarantee (FR-016)

`jobscraper/retrieval` does NOT import `jobscraper/adapters`. A consumer can `import "github.com/job-finder/jobscraper/retrieval"` and get only the engine + rungs + pacing, with zero site-specific code pulled in. Verified by: `retrieval/*` files reference only stdlib, `bogdanfinn/*`, `chromedp`, and (after the move) `jobscraper/retrieval/state_port.go` — none reference `adapters`.

## Error behavior

- `Fetch` returns `(FetchResult{Outcome: PageDeferred}, nil)` when the host is in cooling-off (not an error — the caller treats it as "skip, retry later").
- `Fetch` returns `(FetchResult{Outcome: PageChallenged/PageRefused}, nil)` when all rungs are exhausted (not an error — the caller decides whether to retry).
- `Fetch` returns `(_, err)` only for invalid URLs or context cancellation.
- `HostStatus` returns `(_, err)` if the `StateStorePort.Get` fails.