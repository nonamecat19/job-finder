# Quickstart: Validating Throttle-Only Rate Control

**Feature**: `017-throttle-only-rate-control` | **Date**: 2026-07-28

How to prove the feature works end to end. Scenarios map to the spec's success criteria.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build   # tygo-generated types depend on this first
make up                                  # Postgres + Redis via Docker Compose
```

Migration `00029` applies on API start. Confirm it landed:

```bash
docker compose exec postgres psql -U postgres -d jobfinder \
  -c "\d host_retrieval_state"
```

Expect **no** `budget_period_start`, `budget_used`, or `budget_limit`. Expect
`crawl_delay_seconds` still present.

---

## Scenario 1 — No daily cap remains (SC-001, SC-002, US1)

Push a single host past the old 200-request cap in one session and confirm nothing refuses.

```bash
make test-integration
```

The integration suite must contain a case that issues more than 200 retrievals against one
stubbed host and asserts:

- every fetch is attempted; none returns `PageDeferred`
- no outcome reason contains `budget`
- the run verdict is `success`

Manual equivalent: enable several sources on one board, trigger repeated runs, then check the
run history for any blocked verdict citing a budget. There must be none.

**Fails if**: any request is refused for volume, or a reason string mentions an allowance.

---

## Scenario 2 — Pacing still holds the line (SC-005)

Removing the cap must not let request rate climb.

```bash
make test-go
```

`apps/api/internal/ratelimit` tests must assert:

- a burst of N requests to one host takes at least the pacing floor for N tokens
- two concurrent callers to the same host share one limiter and do not double the rate
- loopback destinations are exempt and stay fast

**Fails if**: observed rate to a host exceeds the configured pace, or concurrent sources each
get their own budget of tokens.

---

## Scenario 3 — Crawl delay is honoured (FR-009)

This is new behaviour; the column existed but nothing read it. See research.md Finding 1.

Unit level (`make test-go`), against a stubbed `robots.txt`:

| Advertised | Expected rate | Expected `source` |
|---|---|---|
| `Crawl-delay: 5` | 0.2 rps | `site-requested` |
| `Crawl-delay: 0` | `DefaultRPS` | `default` |
| absent (`NULL`) | `DefaultRPS` | `default` |
| `Crawl-delay: 1` (faster than default) | `DefaultRPS` | `default` |

The `0` row is the important one: it means "asked, nothing advertised," never "no delay."

**Fails if**: a `0` or `NULL` crawl delay produces an unbounded rate, or a delay faster than
the default speeds the system up.

---

## Scenario 4 — Status reads as normal, not as an error (SC-003, SC-004, US2)

```bash
curl -s localhost:8080/hosts/djinni.co/retrieval-status | jq
```

Expect a `pacing` object with `requestsPerSecond`, `intervalSeconds`, `source`. Expect no
`budgetUsed`, `budgetLimit`, or `budgetResetsAt`. Full shape in
[`contracts/host-retrieval-status.md`](contracts/host-retrieval-status.md).

Then in the dashboard — Sources → Host retrieval status → select a host:

- pacing line present, phrased like `Pace: ~1 request every 5s (site-requested)`
- rendered in the same neutral muted style as `Rung:`, in the same row
- no alert icon, no warning or danger colour on the pacing line
- no budget counter or reset time anywhere in the panel

Then check a host with a recorded block: warning styling still applies to the block line, and
danger styling to cooling-off. The visual distinction between routine and problem must survive.

```bash
make test-react
```

Component tests assert the pacing line renders without warning/danger classes, and that block
and cooling-off lines still carry theirs.

**Fails if**: pacing shows in warning or danger styling, any budget field renders, or blocks
lose their distinct treatment.

---

## Scenario 5 — Cooling-off survives (US1 scenario 4, FR-011)

Cooling-off is deliberately retained; it reacts to observed blocks, not volume.

Drive a host to the consecutive-block threshold, then:

- further fetches return `PageDeferred` with a cooling-off reason
- the run is reported blocked — this is a genuine block, correctly surfaced
- the dashboard shows cooling-off in danger styling
- operator override clears it

**Fails if**: cooling-off was removed alongside the budget, or its deferral is reported as
success.

---

## Scenario 6 — No leftover surfaces (SC-004, US3)

```bash
grep -rn "PER_HOST_DAILY_BUDGET\|budgetUsed\|budgetLimit\|budgetResetsAt\|BudgetLimit\|CheckBudget\|DeductBudget\|IncrementBudget" \
  apps/ packages/shared/src/ README.md --include="*.go" --include="*.ts" --include="*.tsx" --include="*.sql" --include="*.md"
```

Expect zero hits outside `specs/` and `packages/shared/dist/` (rebuild the shared package if
`dist` still shows them).

Also confirm `apps/api/internal/retrieval/budget_test.go` was **renamed**, not deleted — it
contains only crawl-delay tests, never budget tests (research.md Finding 4).

---

## Generated artifacts must be regenerated, never hand-edited

Constitution Principle III. Both checks must pass:

```bash
make sqlc-check    # fails if apps/api/internal/db/sqlcgen is stale
make tygo-check    # fails if packages/shared/src/generated.ts is stale
```

Regenerate with `make sqlc-generate` and `make tygo-generate`.

The hand-maintained duplicate at `packages/shared/src/index.ts` must be edited by hand in the
same change — see plan.md Complexity Tracking for why that duplicate exists and is not being
fixed here.

---

## Full gate

This change spans `apps/api`, `apps/dashboard`, and `packages/shared`, so the constitution
requires the cross-app gate:

```bash
make test-lint
```

Not done until that passes.
