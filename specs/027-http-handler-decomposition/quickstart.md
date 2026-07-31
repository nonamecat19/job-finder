# Quickstart: Verifying HTTP Handler Decomposition

This feature's success is "nothing changed, except where the code lives". Verification is therefore almost entirely comparison against a captured baseline.

## Prerequisites

```bash
cd apps/api && go build ./... && cd ../..   # must succeed — see tasks.md Phase 0
```

---

## 0. Capture the baseline — do this FIRST, before any move

Route parity cannot be verified retroactively. Capture on the pre-change build:

```bash
# Full route inventory, from chi's own walker
cat > /tmp/routes_dump_test.go <<'EOF'
// drop into internal/httpapi, run once, delete
EOF
```

Simpler and sufficient — extend the existing `router_test.go` with a walker that prints every method+path, then:

```bash
go test ./internal/httpapi -run TestRouteInventory -v > /tmp/routes-before.txt
sort /tmp/routes-before.txt -o /tmp/routes-before.txt
wc -l /tmp/routes-before.txt     # record this number
```

Also capture the dependency count that SC-001 measures:

```bash
go list -deps ./internal/httpapi | grep 'job-finder/api/internal' | sort > /tmp/deps-before.txt
wc -l /tmp/deps-before.txt       # expect 24
```

**Both files are the evidence for SC-001 and SC-003. Without them neither criterion is checkable.**

---

## 1. Route parity — after every wave, not just at the end (US3, SC-003)

```bash
go test ./internal/httpapi -run TestRouteInventory -v | sort > /tmp/routes-after.txt
diff /tmp/routes-before.txt /tmp/routes-after.txt
```

**Pass**: empty diff.

**Fails if**: any line differs. A route lost in wave 1 and found in wave 3 is a bisection across 20 commits — this is why the check runs between waves.

Confirm both mounts still come from one registration (FR-007):

```bash
grep -c '/api/v1/jobs' /tmp/routes-after.txt   # every route appears under both prefixes
```

---

## 2. Response parity (US3 §2, §3)

With the backend running, before and after:

```bash
for p in /api/jobs?limit=5 /api/sources /api/health/ready /api/applications; do
  curl -s -o /tmp/body.json -w "$p %{http_code}\n" localhost:3000$p
  jq -S . /tmp/body.json > /tmp/resp-$(echo $p | tr '/?=' '___').json
done
```

Diff the captured bodies across the change.

**Pass**: identical status codes and identical (key-sorted) bodies.

Error parity too:

```bash
curl -s -w ' %{http_code}\n' localhost:3000/api/jobs/not-a-real-id
curl -s -w ' %{http_code}\n' localhost:3000/api/definitely-not-a-route
```

**Pass**: unchanged status and error body shape, including the 404 text `not found: <path>`.

---

## 3. The shared package is narrow (US2, SC-001)

```bash
go list -deps ./internal/httpapi | grep 'job-finder/api/internal' | grep -v '/httpx$' | wc -l
```

**Pass**: `0` (was 24).

**Fails if**: non-zero. Inspect what is left — most likely a handler not yet moved. `health` is not a candidate: it moves to `internal/health` in wave 3 (T039) precisely so 026's `PoolStatter` cannot drag `internal/db` into the router package.

---

## 4. Adding an endpoint touches one directory (US1, SC-002)

Concrete rehearsal — add a throwaway endpoint to `jobs`:

```bash
# edit internal/jobs/interfaces/http/handler.go only
git status --short
```

**Pass**: exactly one feature directory modified, plus at most one line in `cmd/server/servers.go` if a new handler type was introduced (not needed for an added route on an existing handler).

**Fails if**: `internal/httpapi` appears in the status output.

Revert the rehearsal.

---

## 5. Every feature has the layer (SC-006)

```bash
# Handlers left behind — expect only health, router, middleware
ls internal/httpapi/*.go | grep -v _test

# Every feature adapter present
find internal -type d -path '*/interfaces/http' | sort | wc -l   # expect 19
```

**Pass**: `httpapi` contains only `router.go`, `middleware.go`, `health.go` (plus tests); 19 adapter packages exist (the four `jobsources` handlers share one).

---

## 6. The guard actually rejects (US4, SC-005)

Run both deliberate-violation checks from `contracts/depguard.md`:

```bash
cat > internal/jobs/interfaces/http/violation.go <<'EOF'
package http
import _ "github.com/job-finder/api/internal/httpapi"
EOF
make lint-go    # MUST fail, naming internal/httpapi and the depguard rule
rm internal/jobs/interfaces/http/violation.go
```

**Pass**: lint fails with file, line and rule.

**Fails if**: lint passes. A `depguard` rule whose file glob matches nothing silently passes — indistinguishable from a clean build. This check is the only thing that proves the rule is live.

Then the placement test, which covers what `depguard` structurally cannot — a handler inside its feature but outside the adapter layer:

```bash
cat > internal/jobs/violation.go <<'EOF'
package jobs
import _ "github.com/go-chi/chi/v5"
EOF
go test ./internal/ -run TestHandlersLiveInInterfaces   # MUST fail, naming the file
rm internal/jobs/violation.go
```

**Pass**: the test fails and names `internal/jobs/violation.go` plus the required destination.

Then confirm the clean tree still passes both:

```bash
make lint-go
go test ./internal/ -run TestHandlersLiveInInterfaces
```

---

## 7. Full suites (SC-004)

```bash
make test-lint          # merge gate: lint-go + lint-web + test-go + test-react
make test-e2e           # the cross-boundary guard — must pass UNMODIFIED
```

**Pass**: both green, with **no edits** to the Playwright specs. An e2e suite that needed changing means a route or response changed, which is a defect in a feature whose premise is that nothing changed.

---

## 8. Increment audit (SC-007)

```bash
git log --oneline master..HEAD | wc -l    # expect >= 20
git rebase --exec 'cd apps/api && go build ./... && go test ./...' master
```

**Pass**: every commit builds and tests green independently.

**Fails if**: any commit is broken — incremental delivery that only works at the tip is not incremental delivery.
