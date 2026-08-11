# Contract: RosterPort

The interface the library's 6 board adapters call through for employer-board and board-candidate persistence. The app implements it against `roster.Repository` (sqlcgen-typed); the library never sees `pgx`/`pgtype`/`sqlcgen`.

## Interface

```go
// Package rosterport
package rosterport

import "context"

type EmployerBoard struct {
    ID                   string
    Vendor               string
    EmployerIdentifier   string
    DisplayName          string
    AddedVia             string
    Enabled              bool
    LastSuccessAt        *time.Time
    LastPostingCount      int
    ConsecutiveEmptyRuns int
}

type BoardCandidate struct {
    ID                 string
    Vendor             string
    EmployerIdentifier string
    InferredFromJobID  *string
    State              string
}

type EmployerHealthChecker func(ctx context.Context, employerIdentifier string) (postingCount int, err error)

type RosterPort interface {
    ListForRun(ctx context.Context, vendor string) ([]EmployerBoard, error)
    RecordRunOutcome(ctx context.Context, employerID string, postingCount int) error
    GetByVendorAndEmployer(ctx context.Context, vendor, employerIdentifier string) (EmployerBoard, error)
    InsertEmployerBoard(ctx context.Context, vendor, employerIdentifier, displayName, addedVia string) (EmployerBoard, error)
    DeleteEmployerBoard(ctx context.Context, id string) error

    ListBoardCandidates(ctx context.Context) ([]BoardCandidate, error)
    GetBoardCandidate(ctx context.Context, vendor, employerIdentifier string) (BoardCandidate, error)
    GetBoardCandidateByID(ctx context.Context, id string) (BoardCandidate, error)
    InsertBoardCandidate(ctx context.Context, vendor, employerIdentifier string) (BoardCandidate, error)
    DecideBoardCandidate(ctx context.Context, id, state string) error

    ListApplyURLsForDiscovery(ctx context.Context, limit int32) ([]string, error)
}
```

## Callers (library side)

| Caller | Methods used | When |
|---|---|---|
| `adapters.runBoardVendor` | `ListForRun`, `RecordRunOutcome` | every board-adapter `Search` call |
| `adapters.vendorHealthCheck` | `ListForRun` | every board-adapter `HealthCheck` |
| `adapters.healthCheckEmployer` | (returns `EmployerHealthChecker`, calls the fetch fn only) | `NewBoardAdapters` returns the checker map |
| `adapters.NewBoardAdapters` | returns `map[string]EmployerHealthChecker` | app wires into `roster.NewService` |

The discovery/candidate flow (`Discover`, `Accept`, `Reject`, `ListCandidates`) is **not** called by the library's adapters. It is called by the app's `roster.Service`/`roster.View` (HTTP + scheduler). Those methods stay app-side but the port includes them so `roster.Service` is the single port implementer (avoids a second adapter struct).

## Implementer (app side)

`apps/api/internal/jobsources/roster.Service` (existing struct, now satisfying the port):

- `ListForRun` — calls `q.ListEmployerBoardsByVendor`, caps at `MaxEmployersPerRun`, field-copies `sqlcgen.EmployerBoard` → `rosterport.EmployerBoard` (using `dbutil.UUIDString` for the ID).
- `RecordRunOutcome` — `dbutil.ParseUUID(id)`, calls `q.RecordEmployerBoardRunOutcome`.
- `GetByVendorAndEmployer` — calls `q.GetEmployerBoard`, returns zero-value `EmployerBoard` on `pgx.ErrNoRows` (matches current `getByVendorAndEmployer`).
- `InsertEmployerBoard` — calls `q.InsertEmployerBoard` with params.
- `DeleteEmployerBoard` — `dbutil.ParseUUID`, `q.DeleteEmployerBoard`.
- `ListBoardCandidates`/`GetBoardCandidate`/`GetBoardCandidateByID`/`InsertBoardCandidate`/`DecideBoardCandidate` — thin wrappers with UUID conversion.
- `ListApplyURLsForDiscovery` — `q.ListApplyURLsForDiscovery`.

The app's `roster.View` (DTO → dashboard) and `roster.candidates` (discovery orchestration) stay in the app and call the port methods on `roster.Service` — they are not part of the library surface.

## Guarantees

- The library NEVER imports `pgx`, `pgtype`, or `sqlcgen` (FR-002, FR-010).
- `EmployerBoard.ID` is a `string` (UUID form) at the port boundary; the app converts to/from `pgtype.UUID` via `dbutil` (FR-010).
- A consumer can implement `RosterPort` with an in-memory store to run the board adapters without a DB.

## Compatibility

The 11 methods cover the exact surface `roster.Service` + the board-adapter helpers use today (verified against `roster/service.go`, `roster/candidates.go`, `atsboard.go`). The port is a rename + struct swap; no behavior changes.