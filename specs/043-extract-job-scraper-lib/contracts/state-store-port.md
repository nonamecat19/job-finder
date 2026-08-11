# Contract: StateStorePort

The interface the library's retrieval engine reads/writes host state through. The app implements it; the library never sees `pgx`, `pgtype`, or the encryption key.

## Interface

```go
// Package retrieval
package retrieval

import (
    "context"
    "net/http"
    "time"
)

type HostState struct {
    Host               string
    IdentityVersion    string
    CurrentRung        string
    RungLastVerifiedAt *time.Time
    Cookies            []byte // plaintext JSON (map[string]string); app encrypts at rest
    ConsecutiveBlocks  int32
    CoolingOffUntil    *time.Time
    LastBlockAt        *time.Time
    LastBlockReason    *string
    CrawlDelaySeconds  *int32
}

type StateStorePort interface {
    Get(ctx context.Context, host string) (*HostState, error)
    Upsert(ctx context.Context, host string, state *HostState) error
    FetchAndSetCrawlDelay(ctx context.Context, host string) error
    RecordBlock(ctx context.Context, host string, reason string) error
    RecordSuccess(ctx context.Context, host string, rung string) error
    ClearRung(ctx context.Context, host string) error
    ClearCookies(ctx context.Context, host string) error
    LoadCookies(ctx context.Context, host string) ([]*http.Cookie, error)
    SaveCookies(ctx context.Context, host string, cookies []*http.Cookie) error
}
```

## Callers (library side)

| Caller | Methods used | When |
|---|---|---|
| `ServiceImpl.Fetch` | `Get`, `FetchAndSetCrawlDelay` (async), `RecordSuccess`, `RecordBlock`, `Upsert` | every fetch |
| `ServiceImpl.recordBlock` | `RecordBlock`, `Get`, `Upsert` | on challenge/refuse, before cooling-off |
| `ServiceImpl.HostStatus` | `Get` | HTTP endpoint `GET /api/v1/hosts/{host}` |
| `ServiceImpl.ClearRungPreference` | `ClearRung` | HTTP endpoint |
| `ServiceImpl.ClearCookies` | `ClearCookies` | HTTP endpoint |
| `ServiceImpl.OverrideCoolingOff` | `Get`, `Upsert` | HTTP endpoint |
| `directRung.Fetch` | `LoadCookies`, `SaveCookies` | cookie jar for credentialed adapters |

## Implementer (app side)

`apps/api/internal/retrieval.StateStore` (existing struct, now satisfying the port):

- `Get` — reads `sqlcgen.HostRetrievalState`, decrypts `Cookies` if `CONFIG_ENCRYPTION_KEY` set, field-copies to `retrieval.HostState`.
- `Upsert` — copies `retrieval.HostState` to `sqlcgen.UpsertHostRetrievalStateParams`, encrypts `Cookies`, calls `sqlcgen.Queries.UpsertHostRetrievalState`.
- `FetchAndSetCrawlDelay` — unchanged (fetches `robots.txt`, calls `SetHostCrawlDelay`).
- `RecordBlock`/`RecordSuccess`/`ClearRung`/`ClearCookies` — unchanged (already thin wrappers over `sqlcgen.Queries`).
- `LoadCookies`/`SaveCookies` — unchanged (encrypt/decrypt with `crypto`).

## Guarantees

- The library NEVER imports `pgx`, `pgtype`, or `sqlcgen` (FR-002).
- The library NEVER imports the app's `crypto` package; `Cookies` crossing the port is always plaintext JSON (FR-012).
- A consumer can implement `StateStorePort` with an in-memory `map[string]*HostState` to use the engine with no DB (SC-001, User Story 2).

## Compatibility

The 9 methods are the exact set the current `StateStore` already exposes (verified against `state.go`). The port is a rename + a struct swap (`sqlcgen.HostRetrievalState` → `retrieval.HostState`); no behavior changes.