# Contracts: Djinni Preset-Search Rewrite

**Date**: 2026-07-28
**Spec**: [spec.md](../spec.md)

This feature exposes two interfaces whose external contract matters:
the **URL validator** (what an operator may save as a Djinni
subscription) and the **adapter run** (what the system does with a
saved preset). It also touches one **DB migration** contract. Internal
Go-to-Go interfaces (the adapter's `Search`/`FetchDetail` signatures,
`jobsources.Adapter`) are implementation, not contracts, and are not
re-specified here beyond their existing shape.

---

## C1. URL-shape contract (save-time validator)

`apps/api/internal/subscriptions/service.go` exposes, via the
`case "djinni"` arm of `validateSubscriptionURL`, a deterministic
accept/reject decision for any candidate Djinni subscription URL.

**Accept** iff **all** hold:

| Predicate | Required value |
|---|---|
| URL parses (Go `net/url`) | n/a |
| Host (case-insensitive, after trim) | `djinni.co` OR `www.djinni.co` |
| Path | `/jobs` OR `/jobs/` (trailing slash tolerant) |
| Query parameter `search_type` (case-sensitive) | `basic-search` |

**Reject** with a human-readable reason otherwise. The rejection reason
for `djinni.co/my/dashboard/subs/{id}/` URLs is **specifically**:

> "Djinni subscriptions support only preset-search URLs
> (`djinni.co/jobs/?search_type=basic-search&…`); dashboard URLs are no
> longer supported."

Examples (all post-rewrite):

| Input URL | Result |
|---|---|
| `https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote` | Accept |
| `https://www.djinni.co/jobs?search_type=basic-search&primary_keyword=Node.js` (no trailing slash) | Accept |
| `https://djinni.co/jobs/?primary_keyword=Golang` (no `search_type`) | Reject |
| `https://djinni.co/jobs/123-senior-go` (single posting) | Reject |
| `https://djinni.co/my/dashboard/subs/42/` | Reject (dashboard shape — specific reason above) |
| `https://djinni.io/jobs?...` (wrong host) | Reject (wrong host) |
| `https://example.com/...` (wrong host) | Reject |

**Query-param preservation contract**: any `q := u.Query(); q.Set("page",
"1")`-equivalent operation by the adapter MUST leave all *other*
params (including duplicates of `exp_level`, unrecognized keys, and
ordering) byte-for-byte identical to the saved URL. The adapter MUST NOT
interpret, normalize, drop, or re-order any non-`page` param. Required
by spec FR-002 and the edge case "unrecognized extra query parameter."

---

## C2. Adapter run contract (preset-search pagination)

`apps/api/internal/jobsources/adapters/djinni.go`'s `Search(ctx, query,
config)` for `query.SubscriptionURL ≠ ""`:

| Step | Behavior |
|---|---|
| 1 | Parse `query.SubscriptionURL`. Set `page=1` (overriding any saved `page=N`). Preserve all other params. |
| 2 | Loop `i = 1..50` (`djinniMaxSubscriptionPages`): |
| 2a | `FetchHTML(ctx, url, headers={})` — anonymous (no session cookie, no `DJINNI_EMAIL`/`DJINNI_PASSWORD`). The retrieval ladder may escalate to browser/FlareSolverr rungs. |
| 2b | Parse with `parseDjinniCards` → list of `dto.NormalizedJob` (`SourceKey="djinni"`, `ExternalID` = path tail, `URL` = abs-resolved). |
| 2c | If the list is empty → stop (success). |
| 2d | If the first card's href equals the previous page's first card's href → stop (redirect-loop guard). |
| 2e | Else append the cards, `i++`, continue. |
| 3 | Return the concatenated `[]dto.NormalizedJob`. |

**Failure modes** (per spec FR-015 / SC-006, *unchanged* from the
generic ingestion failure posture):

| Event | Verdict | Source health |
|---|---|---|
| `FetchHTML` returns a block/challenge response | `verdict="blocked"`, recorded with human-readable reason | After 3 consecutive failures → source flagged `unhealthy` |
| All cards fail to parse (response shape change) | `verdict="results returned but none interpretable"` — a distinguishable failure | Same health-flag threshold |
| Zero matching jobs returned (legitimately empty search) | `verdict="ok"` with count `0` | Healthy |
| Single-page search (page 2 returns empty) | `verdict="ok"` with actual count, no loop, no error | Healthy |

**Concurrency / pacing**: requests are sequential within a run (no
parallel-page fetch). The enrich queue's default delay
(`DJINNI_DETAIL_DELAY_MS`, kept at `1500`) paces detail fetches per
FR-016. The `Search` inter-page delay follows the existing
`paginateDjinni` convention (unchanged).

**No side effects**: `Search`/`FetchDetail` MUST NOT submit
applications, send messages, or otherwise act on a listing — discovery
only (constitution Principle I, FR-018).

---

## C3. DB migration contract (`00027_drop_djinni_dashboard_subs.sql`)

A goose migration under `apps/api/internal/db/migrations/` numbered
`00027` (next sequential, per the constitution's unique-sequential
rule; never reused/replaced). Plain SQL (no Go migration plumbing),
up + down.

### Up

1. Create the audit table `DjinniLegacySubAudit` (see
   data-model.md (`data-model.md`, removed on merge — see git history) §3 for the column list).
2. `DELETE FROM "Subscription" WHERE "sourceKey" = 'djinni' AND "url"
   LIKE '%/my/dashboard/subs/%' RETURNING "id", "name", "url",
   "createdAt"`; insert each returned row into
   `DjinniLegacySubAudit` with `deletedAt = now()`.

### Down

A no-op (commented "deletion is irreversible — audit rows remain as
the record"). Goose requires a Down block; the convention is an empty
`-- +goose Down` followed by a comment. The audit table is **not**
dropped on Down (the record must persist).

### Idempotency

The migration is idempotent by construction: re-running Up on a clean
DB creates an empty audit table and deletes zero rows (no match).

### Constraints

- The migration MUST NOT delete preset-search subscriptions (the
  `WHERE url LIKE '%/my/dashboard/subs/%'` predicate excludes them by
  construction).
- The migration MUST NOT touch `JobSource` rows (the `JobSource("djinni")`
  row remains; the adapter still runs preset searches against it).
- The migration MUST NOT alter any column on `Subscription` or
  `JobSource` (no crypto round-trip; the orphaned `sessionCookie` blob
  in `JobSource.config` is documented inert — see R6).

---

## C4. Dashboard display contract

`apps/dashboard/src/features/sources/djinniSearchSummary.ts`'s
`summarizeDjinniBasicSearch(url: string): string | null` is **the**
contract for the Subscriptions list row label. Its post-rewrite behavior
is unchanged: it returns a human-readable summary of a preset URL's
filters, or `null` if the URL is not a preset URL.

### Input

A `string` (the subscription's `url`). The helper gates on:

- Host: `djinni.co` or `www.djinni.co`.
- Path: `/jobs` or `/jobs/`.
- Query `search_type === "basic-search"`.

If any predicate fails → returns `null`.

### Output (the summary string)

Format: `"<keyword> · $<salary> · <expSummary> · <employment>"`, with
each segment **omitted** if not present in the URL (spec FR-012).
Segments are joined by `" · "` (space, middle dot, space), in a fixed
order (keyword, salary, expSummary, employment) — **not** URL order.

### `expSummary` rules (the range-vs-list contract)

1. Tokenize `exp_level` query values (each is one string like `"2y"`).
2. Deduplicate, preserving first-occurrence order.
3. For each token: must end with `"y"` and `parseInt(prefix)` cleanly.
4. If **any** token is non-parseable → return the **sorted unique raw
   tokens** joined by `", "` (e.g. `"2y, lead"`).
5. Otherwise: sort the parsed years ascending.
6. Single value → `"<N> years"`.
7. Consecutive run (`years[i] - years[i-1] === 1` for all `i`) →
   `"<min>–<max> years"` with the **en-dash** `"\u2013"` (U+2013), not
   a hyphen.
8. Non-consecutive → `"<a>, <b>, ... years"`.
9. Empty input → `""`.

A return value of `null` from `summarizeDjinniBasicSearch` means the URL
is not a preset URL (e.g. a malformed djinni.co URL, a wrong-host URL,
or — post-migration — a leftover dashboard URL the operator pasted into
a side channel).

### Examples

| Input | Output |
|---|---|
| `?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote` | `Node.js · $3000 · 2–5 years · remote` |
| `?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote` | `Golang · $1500 · 1–3 years · remote` |
| `?search_type=basic-search&primary_keyword=Golang` (no other filters) | `Golang` |
| `?search_type=basic-search&primary_keyword=Go&exp_level=1y&exp_level=3y` (gap) | `Go · 1, 3 years` |
| `?search_type=basic-search&exp_level=senior` (non-`Ny` token) | ` · senior` (note the leading separator gap if keyword is absent — verified edge) |
| `https://djinni.co/jobs/123-senior-go` (not preset) | `null` |
| `https://djinni.co/my/dashboard/subs/42/` (post-migration) | `null` |

The row in `SourcesPage.tsx` will display whatever this helper returns;
a `null` falls back to the raw `url` (the existing fallback).