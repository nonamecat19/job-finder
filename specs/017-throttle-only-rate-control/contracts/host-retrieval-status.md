# Contract: Host Retrieval Status

**Feature**: `017-throttle-only-rate-control`
**Endpoint**: `GET /hosts/{host}/retrieval-status`
**Route**: `apps/api/internal/httpapi/hosts.go:28`
**Type source of truth**: `apps/api/internal/dto/jobs.go` → tygo → `packages/shared/src/generated.ts`

This is a breaking change to an existing response shape. Three fields are removed and one
object is added.

---

## Response: before

```jsonc
{
  "host": "djinni.co",
  "identityVersion": "v3",
  "currentRung": "direct",
  "lastBlockAt": "2026-07-27T09:14:02Z",
  "lastBlockReason": "challenged via direct",
  "coolingOffUntil": null,
  "budgetUsed": 47,          // removed
  "budgetLimit": 200,        // removed
  "budgetResetsAt": "...",   // removed
  "crawlDelaySeconds": 5
}
```

## Response: after

```jsonc
{
  "host": "djinni.co",
  "identityVersion": "v3",
  "currentRung": "direct",
  "lastBlockAt": "2026-07-27T09:14:02Z",
  "lastBlockReason": "challenged via direct",
  "coolingOffUntil": null,
  "crawlDelaySeconds": 5,
  "pacing": {
    "requestsPerSecond": 0.2,
    "intervalSeconds": 5,
    "source": "site-requested"
  }
}
```

---

## Field reference

### Removed

| Field | Was | Requirement |
|---|---|---|
| `budgetUsed` | `number` | FR-014 |
| `budgetLimit` | `number` | FR-014 |
| `budgetResetsAt` | `string` (RFC3339) | FR-014 |

No deprecation window. The dashboard is the only consumer and ships from this repo.

### Added: `pacing`

Always present. There is no unpaced third-party host (FR-007), so this is never null.

| Field | Type | Notes |
|---|---|---|
| `requestsPerSecond` | `number` | steady-state rate before jitter |
| `intervalSeconds` | `number` | `1 / requestsPerSecond`; the user-facing form (FR-016) |
| `source` | `"default" \| "site-requested" \| "override"` | provenance; see below |

Both `requestsPerSecond` and `intervalSeconds` are sent even though either derives from the
other. The rate is the machine-meaningful figure; the interval is what the UI shows. Deriving
in the client invites two clients rounding differently.

`source` values:

| Value | Meaning | Typical display |
|---|---|---|
| `default` | no host-specific signal | `~1 request every 1.4s (default)` |
| `site-requested` | derived from the host's advertised `Crawl-delay` | `~1 request every 5s (site-requested)` |
| `override` | explicit operator per-host rate | `~1 request every 10s (override)` |

Jitter (±25%) is deliberately not exposed. It is anti-fingerprinting machinery, and surfacing
a fluctuating number would undercut FR-016's requirement that the display be actionable.

### Retained

| Field | Type | Notes |
|---|---|---|
| `host` | `string` | |
| `identityVersion` | `string` | |
| `currentRung` | `string` | retrieval ladder unchanged |
| `lastBlockAt` | `string?` | genuine blocks only |
| `lastBlockReason` | `string?` | must never cite an exhausted allowance (FR-017) |
| `coolingOffUntil` | `string?` | cooling-off survives (FR-011) |
| `crawlDelaySeconds` | `number?` | raw advertised value; `0` means "asked, nothing advertised" |

---

## Presentation contract

Binding on the dashboard, not on the wire. These encode FR-013 and FR-015 — the difference
between "normal" and "problem" is the whole point of the feature.

| Field | Treatment | Rationale |
|---|---|---|
| `pacing` | neutral (`text-muted`), same row as `currentRung` | routine operational fact |
| `lastBlockAt` / `lastBlockReason` | warning (`text-warning`) | genuine host refusal |
| `coolingOffUntil` | danger (`text-danger`) | active back-off, host is untouchable |

Pacing MUST NOT carry error, warning, or danger styling, and MUST NOT be rendered with an
alert icon. It appears whether or not anything is wrong, exactly like the current rung.

---

## Internal contract: `ratelimit.Transport` rate resolution

Package-internal, listed because it is the seam between the pacing transport and the database.

```
RateResolver func(host string) (rps float64, source string, ok bool)
```

- Optional. A nil resolver means every host gets `DefaultRPS` — the current behaviour, and the
  reason existing `ratelimit` tests keep passing untouched.
- Consulted at **limiter construction**, not per request, then cached for a short TTL. Pacing
  must not put a query in the hot path.
- Returning `ok == false` falls through to `DefaultRPS`.
- The `ratelimit` package must not import `retrieval`, `db`, or any Postgres type. The resolver
  is injected by the composition layer.

### Resolution precedence

1. operator override → `override`
2. `crawl_delay_seconds` = `N > 0` **and** `1/N` slower than default → `site-requested` at `1/N`
3. otherwise → `default` at `DefaultRPS`

`crawl_delay_seconds == 0` means "asked, nothing advertised" and falls to case 3. It must never
resolve to an unbounded rate.

---

## Behavioural contract: pacing never fails a request

| Situation | Required behaviour | Requirement |
|---|---|---|
| Token unavailable | block until available, then proceed | FR-010 |
| Request context cancelled while waiting | return promptly; shutdown is not held up | existing |
| Loopback / localhost destination | not paced | spec Assumptions |
| Run delayed only by pacing | reported `success` | FR-018 |
| Any user-visible reason string | never cites pacing or an allowance as a failure | FR-017 |

Pacing produces no `PageOutcome`. After this change `PageDeferred` has exactly one producer —
cooling-off — and continues to be reported as a block, because it now unambiguously *is* one.
