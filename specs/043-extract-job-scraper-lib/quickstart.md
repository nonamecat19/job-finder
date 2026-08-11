# Quickstart: Validate the Extraction

Runnable validation scenarios that prove the feature works end-to-end. None of these require deploying the full stack; all run from a checkout.

## Prerequisites

- Go 1.26.5+, pnpm, Docker (for `make test-lint` only)
- The `jobscraper` library checked out as a sibling: `../jobscraper` (or adjust the `replace` path)
- `apps/api/go.mod` has `require github.com/job-finder/jobscraper v0.0.0` + `replace github.com/job-finder/jobscraper => ../jobscraper`
- DB/Redis are NOT required for scenarios 1–3; only scenario 4 needs the app's Postgres (for the `StateStorePort`/`RosterPort` wiring)

## Scenario 1 — Library compiles standalone (SC-002)

**Goal**: prove the library has no DB/queue/config dependency.

```sh
cd ../jobscraper
go mod tidy
go build ./...
go test ./...
```

**Expected**: `go build ./...` succeeds. `go.mod` declares `goquery`, `bogdanfinn/fhttp`, `bogdanfinn/tls-client`, `chromedp/cdproto`, `chromedp/chromedp`, `golang.org/x/time/rate` — and NO `pgx`, `asynq`, `viper`, `minio`, `pgvector`, `goose`, or `github.com/job-finder/api/...` (SC-002). Verify:

```sh
grep -E 'pgx|asynq|viper|minio|pgvector|goose|job-finder/api' go.mod && echo "FAIL: forbidden dep found" || echo "PASS: go.mod clean"
```

**Pass**: `PASS: go.mod clean` and `go test ./...` green.

## Scenario 2 — Throwaway consumer scrapes two sources (SC-001)

**Goal**: prove a brand-new Go project can use the library with zero app-side code.

```sh
mkdir /tmp/jobscraper-smoke && cd /tmp/jobscraper-smoke
go mod init smoke
go get github.com/job-finder/jobscraper
# add a replace pointing at the local checkout:
# replace github.com/job-finder/jobscraper => /path/to/jobscraper
```

`main.go`:
```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/job-finder/jobscraper/adapter"
    "github.com/job-finder/jobscraper/adapters"
    "github.com/job-finder/jobscraper/model"
)

func main() {
    ctx := context.Background()
    reg := adapter.NewRegistry(
        &adapters.AdzunaAdapter{},          // API-based
        &adapters.RemoteokAdapter{},        // HTML-based
    )
    for _, a := range reg.All() {
        jobs, err := a.Search(ctx, model.SearchQuery{Keywords: "go developer"}, nil)
        fmt.Printf("%s: %d jobs, err=%v\n", a.Key(), len(jobs), err)
    }
}
```

```sh
go run .
```

**Expected**: prints two lines (`adzuna: N jobs, err=...`, `remoteok: N jobs, err=...`). Compilation succeeds with no `pgx`/`asynq`/`viper` in the throwaway's `go.sum` (verify: `grep -E 'pgx|asynq' go.sum` returns nothing). Setup time under one hour (SC-001).

**Pass**: compiles, runs, returns jobs (or a network error from the live site — that's fine; the point is no DB is needed).

## Scenario 3 — Retrieval engine without adapters (SC-006)

**Goal**: prove `retrieval` is importable without `adapters`.

```sh
cd /tmp/jobscraper-smoke
cat > main.go <<'EOF'
package main

import (
    "context"
    "fmt"
    "time"

    "github.com/job-finder/jobscraper/retrieval"
)

type memStore struct{ states map[string]*retrieval.HostState }

func (m *memStore) Get(_ context.Context, host string) (*retrieval.HostState, error) {
    return m.states[host], nil
}
func (m *memStore) Upsert(_ context.Context, host string, s *retrieval.HostState) error {
    m.states[host] = s; return nil
}
func (m *memStore) FetchAndSetCrawlDelay(context.Context, string) error { return nil }
func (m *memStore) RecordBlock(context.Context, string, string) error  { return nil }
func (m *memStore) RecordSuccess(context.Context, string, string) error { return nil }
func (m *memStore) ClearRung(context.Context, string) error             { return nil }
func (m *memStore) ClearCookies(context.Context, string) error           { return nil }
func (m *memStore) LoadCookies(context.Context, string) ([]*http.Cookie, error) { return nil, nil }
func (m *memStore) SaveCookies(context.Context, string, []*http.Cookie) error  { return nil }

func main() {
    svc := retrieval.NewEngine(nil, &memStore{states: map[string]*retrieval.HostState{}}, retrieval.EngineOpts{})
    res, err := svc.Fetch(context.Background(), retrieval.FetchRequest{URL: "https://example.com/"})
    fmt.Printf("outcome=%s err=%v\n", res.Outcome.Status, err)
}
EOF
go mod tidy
go run .
```

**Expected**: `go.sum` does NOT contain `chromedp` or `goquery` (proving `adapters` was not pulled in — SC-006). Run prints an outcome (likely `challenged` or `read` depending on network). Verify:

```sh
grep -E 'chromedp|goquery|PuerkitoBio' go.sum && echo "FAIL: adapters pulled in" || echo "PASS: retrieval standalone"
```

**Pass**: `PASS: retrieval standalone`.

## Scenario 4 — App behavior unchanged (SC-003, SC-004, SC-005)

**Goal**: prove the extraction did not regress the app.

```sh
cd /home/nnc/Projects/job-finder
# before extraction, snapshot the tygo output:
cp packages/shared/src/generated.ts /tmp/generated.before.ts

# after extraction wiring (replace directive in apps/api/go.mod):
cd apps/api && go build ./...
go test ./internal/jobsources/... ./internal/retrieval/...

# regenerate tygo and diff:
cd /home/nnc/Projects/job-finder
make tygo-generate
diff /tmp/generated.before.ts packages/shared/src/generated.ts && echo "PASS: tygo byte-identical"

# full merge gate:
make test-lint
```

**Expected**:
- `go test ./internal/jobsources/... ./internal/retrieval/...` green (SC-003) — adapter tests pass unchanged.
- `diff` empty (SC-005) — tygo output byte-identical.
- `make test-lint` green (SC-004) — `lint-go` + `lint-web` + `test-go` + `test-react` all pass.

**Pass**: all three sub-checks green.

## Scenario 5 — Board adapters work through RosterPort (SC-007)

**Goal**: prove the 6 board adapters moved whole and call through the port, with no behavior regression.

```sh
cd apps/api
go test ./internal/jobsources/infrastructure/adapters/ -run 'Greenhouse|Lever|Ashby|Workable|SmartRecruiters|ATSBoard'
```

**Expected**: all 6 board adapter tests pass. The test fixtures (`testdata/`) moved with the adapters into the library; the app imports the library's adapter packages and the tests run against them unchanged.

**Pass**: all 6 test functions green, no fixture modifications.

## Notes

- Scenarios 1–3 do not require Postgres/Redis/Docker — they validate the library's independence (the whole point of the feature).
- Scenario 4 is the regression gate; run it before and after to produce the tygo diff.
- Scenario 5 is a subset of scenario 4's `go test` run, called out separately because the board-adapter port wiring is the highest-risk part of the extraction.
- The `live_smoke_test` (tagged) is not run here — it requires network + credentials and is a manual check, not a CI gate.