# Contract: Handler Inventory

All 23 handler files in `internal/httpapi`, with measured internal dependencies, destination package, and migration wave.

**Measurement** (2026-07-30, at `ede4b90`):

```bash
for f in internal/httpapi/*.go; do
  case "$f" in *_test.go|*router.go|*helpers.go|*middleware.go) continue;; esac
  echo "$(basename $f .go) -> $(grep -h 'job-finder/api/internal' $f | sed 's|.*internal/||;s|"||' | tr '\n' ' ')"
done
```

## Wave 1 — `dto`-only (6)

Zero feature coupling. Verbatim move plus import-path and `httpx.` edits.

| Handler | Internal deps | Destination |
|---|---|---|
| `activity` | `dto` | `internal/activity/interfaces/http` |
| `contacts` | `dto` | `internal/recruiter/interfaces/http` |
| `hosts` | `dto` | `internal/jobsources/interfaces/http` |
| `notifications` | `dto` | `internal/notifier/interfaces/http` |
| `postage` | `dto` | `internal/postage/interfaces/http` |
| `sources` | `dto` | `internal/jobsources/interfaces/http` |

**Note**: `hosts`, `sources` and (wave 2) `searches`, `roster` all belong to `jobsources`. They land in the same destination package — four files, one package, not four packages. Confirm no identifier collisions when they meet (`Handler`, `Mount` will collide; each keeps its distinct type name, as today).

**Note**: `contacts` is destined for `recruiter` on the basis of its route surface. Verify against `compose.go` wiring before moving — the handler's ports are declared locally, so the owning feature is not inferable from imports alone.

## Wave 2 — single or double feature dependency (15)

| Handler | Internal deps | Destination |
|---|---|---|
| `aifeature` | `aifeature`, `dto` | `internal/aifeature/interfaces/http` |
| `applications` | `applications/application`, `dto` | `internal/applications/interfaces/http` |
| `coach` | `coach/application`, `dto` | `internal/coach/interfaces/http` |
| `companies` | `companyintel/application`, `dto` | `internal/companyintel/interfaces/http` |
| `documents` | `generation`, `dto` | `internal/generation/interfaces/http` |
| `ghostjob` | `ghostjob/application`, `dto` | `internal/ghostjob/interfaces/http` |
| `interviewprep` | `interviewprep`, `dto` | `internal/interviewprep/interfaces/http` |
| `jobs` | `jobs`, `dto` | `internal/jobs/interfaces/http` |
| `keyword` | `keyword/application`, `dto` | `internal/keyword/interfaces/http` |
| `llm_settings` | `platform/llm`, `llmsettings`, `dto` | `internal/llmsettings/interfaces/http` |
| `outreach` | `outreach`, `dto` | `internal/outreach/interfaces/http` |
| `profiles` | `generation`, `profile`, `dto` | `internal/profile/interfaces/http` |
| `referral` | `referral`, `dto` | `internal/referral/interfaces/http` |
| `searches` | `jobsources/application`, `dto` | `internal/jobsources/interfaces/http` |
| `subscriptions` | `subscriptions`, `dto` | `internal/subscriptions/interfaces/http` |

**Two handlers import a second feature**: `llm_settings` (`platform/llm`) and `profiles` (`generation`). `platform/llm` is infrastructure, not a feature, so that one is fine as-is. `profiles` → `generation` is a genuine cross-feature dependency; confirm it goes through `generation`'s exported surface and not its internals (FR-012). If it does not, fix it in transit as with `roster`.

## Wave 3 — the real change (1) + the lock

| Handler | Internal deps | Destination | Action |
|---|---|---|---|
| `roster` | **`db/sqlcgen`, `dbutil`**, `jobsources/roster`, `dto` | `internal/jobsources/interfaces/http` | Move data access behind `jobsources/roster`'s boundary first, then move the handler |

Then enable `depguard` (contracts/depguard.md).

## Stays in `httpapi`

| File | Internal deps | Reason |
|---|---|---|
| `health` | none | Cross-cutting; pings three infrastructure dependencies through a local `Pinger`. Owned by no feature. |
| `router` | none | The router itself. |
| `middleware` | none | `requestLogger`, applied once by the router. |
| `helpers` | none | **Moves to `internal/httpx`**, exported. |

### Conflict with feature 026

Feature 026 adds a `PoolStatter` field to `HealthHandler` referencing `internal/db`. If 026 lands first, `health` staying in `httpapi` would make `httpapi` import `db` — not a *feature* package, so SC-001 still passes literally, but it weakens the invariant.

**Resolution**: `health` moves to its own `internal/health` package in wave 3 (T039), **unconditionally** — not depending on whether 026 has landed. A conditional resolution would make the outcome depend on merge order, which nobody will remember later. 026's `PoolStatter` targets `internal/health` in both orderings; whichever feature lands second adjusts only the wiring line in `cmd/server/compose.go`.

## Test files

19 test files move with their handlers, unmodified. **A test requiring edits is a signal the move changed behaviour** — stop and investigate rather than adjusting the test.

`router_test.go` stays. Its route-parity assertions become the primary guard for FR-006; extend it rather than replacing it.

## Totals

| | Count |
|---|---|
| Handlers moved | 22 |
| Handlers staying | 1 (`health`) |
| Distinct destination packages | 19 (four `jobsources` handlers share one) |
| Modules gaining an `interfaces/` layer | 16 (`generation`, `ghostjob`, `jobsources` already have one) |
| Test files moved | 19 |
| Files with real changes beyond imports | 2 (`roster`, `helpers`→`httpx`) |
