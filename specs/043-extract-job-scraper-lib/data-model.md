# Phase 1: Data Model

This feature is a refactor/extraction, not a schema change. The "data model" here is the set of library-owned types that cross the library↔app boundary. The app's Postgres schema (sqlc-generated types, migrations) is **unchanged** — no new migration, no goose version bump.

## Library-owned model types

### `jobscraper/model/job.go`

Canonical home for the 4 types moved out of `apps/api/internal/dto`.

#### `NormalizedJob`

```go
type NormalizedJob struct {
    SourceKey   string  `json:"sourceKey"`
    ExternalID  *string `json:"externalId,omitempty"`
    Title       string  `json:"title"`
    Company     string  `json:"company"`
    Location    *string `json:"location,omitempty"`
    Remote      bool    `json:"remote"`
    SalaryRaw   *string `json:"salaryRaw,omitempty"`
    URL         string  `json:"url"`
    Description string  `json:"description"`
    PostedAt    *string `json:"postedAt,omitempty"`
    Raw         any     `json:"raw"`

    ExperienceLevel        *string `json:"experienceLevel,omitempty"`
    ExperienceMinYears     *int    `json:"experienceMinYears,omitempty"`
    EnglishLevel           *string `json:"englishLevel,omitempty"`
    SalaryEstimateRaw      *string `json:"salaryEstimateRaw,omitempty"`
    SalaryEstimateMin      *int    `json:"salaryEstimateMin,omitempty"`
    SalaryEstimateMax      *int    `json:"salaryEstimateMax,omitempty"`
    SalaryEstimateCurrency *string `json:"salaryEstimateCurrency,omitempty"`
}
```

**Validation**: `SourceKey` non-empty (set by adapter's `Key()`); `URL` non-empty and absolute; `Title` non-empty for stored postings (the `PostingReader.ReadPosting` contract allows partial — title/company may be empty for a fill-in draft, per `domain/adapter.go` rule 3).

**Relationships**: produced by `Adapter.Search` and `PostingReader.ReadPosting`; consumed by the app's `ingest` pipeline (dedupe → persist). The JSON shape is the cross-language contract with the dashboard (SC-005: byte-identical tygo output).

#### `SearchQuery`

```go
type SearchQuery struct {
    Keywords        string   `json:"keywords"`
    Location        *string  `json:"location,omitempty"`
    Remote          *bool    `json:"remote,omitempty"`
    SalaryMin       *float64 `json:"salaryMin,omitempty"`
    Country         *string  `json:"country,omitempty"`
    Sources         []string `json:"sources,omitempty"`
    SubscriptionURL string   `json:"subscriptionUrl,omitempty"`
}
```

**Validation**: `Keywords` required (enforced by `SearchService.CreateSearch` today; stays app-side).

#### `SourceKind`

```go
type SourceKind string

const (
    SourceKindAPI     SourceKind = "api"
    SourceKindScrape  SourceKind = "scrape"
    SourceKindSidecar SourceKind = "sidecar"
    SourceKindManual  SourceKind = "manual"
)
```

**Lifecycle**: static enum; no state transitions.

#### `JobSourceDto`

```go
type JobSourceDto struct {
    ID      string         `json:"id"`
    Key     string         `json:"key"`
    Kind    SourceKind     `json:"kind"`
    Enabled bool           `json:"enabled"`
    Healthy bool           `json:"healthy"`
    Config  map[string]any `json:"config"`
}
```

## Library-owned port types

### `jobscraper/retrieval/state_port.go`

#### `HostState` (library-owned, replaces `sqlcgen.HostRetrievalState` at the port boundary)

```go
type HostState struct {
    Host              string
    IdentityVersion   string
    CurrentRung       string
    RungLastVerifiedAt *time.Time
    Cookies           []byte // plaintext JSON; app encrypts/decrypts on its side
    ConsecutiveBlocks int32
    CoolingOffUntil   *time.Time
    LastBlockAt       *time.Time
    LastBlockReason   *string
    CrawlDelaySeconds *int32
}
```

**Validation**: `Host` non-empty. `Cookies` is plaintext JSON (`map[string]string` serialized) when it crosses the port; the app's `StateStore` adapter encrypts before persisting and decrypts after reading (FR-012: the key never enters the library).

**State transitions** (driven by the engine, persisted through the port):
- `Read` on a rung → `RecordSuccess(rung)` → `CurrentRung` set to that rung, `RungLastVerifiedAt` updated.
- `Challenged`/`Refused` → `RecordBlock(reason)` → `ConsecutiveBlocks++`, `LastBlockAt`/`LastBlockReason` set.
- `ConsecutiveBlocks >= threshold` → app's `recordBlock` sets `CoolingOffUntil` (exponential backoff) and calls `Upsert`.
- `CoolingOffUntil` in the past → engine skips cooling-off check, proceeds to fetch.
- `CrawlDelaySeconds == nil` → engine triggers `FetchAndSetCrawlDelay` (async) to read `robots.txt`.

#### `StateStorePort` interface

```go
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

9 methods — the exact surface `service_impl.go` + `direct.go` use today.

### `jobscraper/rosterport/roster_port.go`

#### `EmployerBoard` (library-owned, replaces `sqlcgen.EmployerBoard` at the port boundary)

```go
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
```

#### `BoardCandidate` (library-owned, replaces `sqlcgen.BoardCandidate` at the port boundary)

```go
type BoardCandidate struct {
    ID                 string
    Vendor             string
    EmployerIdentifier string
    InferredFromJobID  *string
    State              string
}
```

#### `EmployerHealthChecker` (function type, moves from `roster`)

```go
type EmployerHealthChecker func(ctx context.Context, employerIdentifier string) (postingCount int, err error)
```

#### `RosterPort` interface

```go
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

11 methods — covers the full roster surface the adapters + discovery flow use. The app's `roster.Service` implements this against `sqlcgen.Queries`, translating between `EmployerBoard`/`BoardCandidate` and `sqlcgen.EmployerBoard`/`sqlcgen.BoardCandidate` with `dbutil.UUIDString`/`dbutil.ParseUUID` for the `string`↔`pgtype.UUID` conversions (same as `roster/view.go` does today for the DTO path).

## Adapter framework types

### `jobscraper/adapter/adapter.go`

Moved from `jobsources/domain/adapter.go` verbatim, with `dto` imports replaced by `model`:
- `Adapter` interface (`Key()`, `Kind() model.SourceKind`, `Search(...)`, `HealthCheck(...)`)
- `PostingReader` interface (`MatchesPostingURL`, `ReadPosting`)
- `Credentialed`, `DetailNeeder`, `EmployerReporter` interfaces
- `EmployerOutcome` string type + 5 constants (`read`/`not_found`/`unreadable`/`refused`/`no_postings`)
- `EmployerRunOutcome` struct
- `Registry` (by-key map + ordered slice)
- helper functions `AsPostingReader`, `NeedsDetail`, `IsCredentialed`

### `jobscraper/adapter/errors.go`

Moved from `jobsources/domain/errors.go`:
- `SourceNotFoundError`, `AdapterNotRegisteredError`

## Retrieval engine types

### `jobscraper/retrieval/ladder.go`

- `RetrievalMethod` struct (`Key string`, `Order int`)
- 3 rungs: `RungDirect` (order 0), `RungBrowser` (order 1), `RungFlareSolverr` (order 2)
- `AllRungs`, `RungForKey`, `Next()`, `Available()`, `IsEmpty()`, `MaxRungForAccount`

### `jobscraper/retrieval/outcome.go`

- `PageStatus` string type + 5 constants (`read`/`challenged`/`refused`/`unparseable`/`deferred`)
- `PageOutcome` struct (`Status`, `Method`, `Reason`, `URL`)

### `jobscraper/retrieval/identity.go`

- `BrowserIdentity` struct (`Version`, `UserAgent`, `Platform`, `Headers`, `TLSProfileID`)

### `jobscraper/retrieval/service.go`

- `FetchRequest` struct (`URL`, `Headers`, `UsesUserAccount`, `RefererPage`)
- `FetchResult` struct (`Outcome PageOutcome`, `Body string`)
- `HostStatus` / `HostPacing` structs
- `Service` interface (`Fetch`, `HostStatus`, `ClearRungPreference`, `ClearCookies`, `OverrideCoolingOff`)

## App-side adapters (implementations, not library-owned)

- `apps/api/internal/retrieval/StateStore` → implements `jobscraper/retrieval.StateStorePort`; field-copies `HostState`↔`sqlcgen.HostRetrievalState`; encrypts/decrypts cookies with `internal/crypto`.
- `apps/api/internal/jobsources/roster.Service` → implements `jobscraper/rosterport.RosterPort`; field-copies `EmployerBoard`↔`sqlcgen.EmployerBoard`; uses `dbutil` for UUID conversion.

No new migrations, no schema changes, no new enums in the DB. The data model is a port-and-adapter reshaping of existing types.