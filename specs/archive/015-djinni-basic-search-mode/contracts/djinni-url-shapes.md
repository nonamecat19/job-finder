# Contract: Djinni Subscription URL Shapes

**Status**: authoritative contract for the two Djinni subscription shapes both `apps/api`
and `apps/dashboard` MUST recognize. Replacing the implicit "any URL the adapter can
scrape" convention with the explicit two-shape contract is what
`015-djinni-basic-search-mode` introduces.

**Authority**: spec FR-001, FR-002, FR-007, FR-009, FR-010; research.md R2, R5.

Both apps parse the saved `SubscriptionDto.url` string using the rules below. The Go
side uses the parse for **save-time validation** (reject `Unknown`) and **run-time mode
selection** (`Dashboard` → `scrapeDashboard`; `BasicSearch` → `scrapeBasicSearch`). The
TS side uses the parse for **display only**. Neither side modifies the saved URL.

## 1. Host

| Accepted hosts |
|---|
| `djinni.co` |
| `www.djinni.co` |

Any other host → `Unknown` → server rejects at save with:
`"djinni subscription url must be a djinni.co url"`.

## 2. Shape A — Dashboard mode

```
https://djinni.co/my/dashboard/subs/<id>/
```

| Part | Rule |
|---|---|
| `path` | `^/my/dashboard/subs/<id>/?$` where `<id>` is one or more URL-safe segments (typically digits, but a UUID-ish slug is allowed; the existing `url.Parse`-based check stays permissive on the id segment) |
| `query` | none required; any present (e.g. `?page=2`) is ignored for shape discrimination but preserved in the saved URL |

This shape maps to `DjinniModeDashboard`. The existing `scrapeSubscription` behavior
becomes `scrapeDashboard` unchanged (FR-013, SC-008).

## 3. Shape B — Basic-search mode

```
https://djinni.co/jobs/?search_type=basic-search&primary_keyword=<kw>&salary=<n>&exp_level=<Ny>&exp_level=<Ny>&...&employment=<emp>
```

| Part | Rule |
|---|---|
| `path` | `/jobs` or `/jobs/` (trailing slash optional) |
| `query.search_type` | MUST equal `basic-search`. Absent or any other value → NOT shape B. |
| `query.primary_keyword` | optional but expected; free text (e.g. `Node.js`, `Golang`). Display: rendered verbatim. |
| `query.salary` | optional; positive integer (Djinni's USD monthly net minimum). Display: rendered verbatim as `$<n>`. |
| `query.exp_level` | optional; REPEATED `Ny` token (e.g. `exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y`). Display: deduplicated set, sorted ascending, collapsed to `"min–max years"` when consecutive, else `"a, b years"`. |
| `query.employment` | optional; free text (e.g. `remote`). Display: rendered verbatim. |
| Other query params | preserved in the saved URL and re-issued by the run verbatim, but NOT interpreted and NOT displayed |

A URL whose path matches `/jobs/...` but is, by inspection, a single job posting (e.g.
`/jobs/<id>` where `<id>` is a numeric job ID *and* `search_type=basic-search` is absent)
is `Unknown` and rejected with:
`"djinni subscription url looks like a single job posting, not a search results page"`.

This shape maps to `DjinniModeBasicSearch`. The new `scrapeBasicSearch` reuses the
existing `fetchDoc` + `parseDjinniCards` + pagination guards (research.md R1).

## 4. `exp_level` display rule (shared by Go and TS)

Both apps implement the **same** pure function over the list of `exp_level` values
collected from the URL query (order-independent; duplicates removed; values
interpreted as integers):

- **None**: omit the levels field from display.
- **One**: `<n> years` (e.g. one `exp_level=3y` → `"3 years"`).
- **Consecutive set (≥ 2)**: `<min>–<max> years` with the en-dash `–`.
  - `2y,3y,4y,5y` → `"2–5 years"`
  - `1y,2y,3y` → `"1–3 years"`
  - `3y,1y,2y` (out of order) → `"1–3 years"` (sorted before comparison)
- **Non-consecutive set (≥ 2)**: `<a>, <b>[, <c>]... years` (sorted ascending, comma-space
  separated).
  - `1y,3y` → `"1, 3 years"`

A value that doesn't parse cleanly as `Ny` (some catastrophic upstream change) is treated
as a non-integer string and forced to the non-consecutive-list rendering (never mis-collapse
to a misleading range). This is the safe fallback documented in research.md R4.

The shared function shape:

- **Go** (`apps/api/internal/jobsources/adapters/djinni_searchmode.go`):
  `summarizeExpLevels(values []string) string` — used in run-time logging and any
  diagnostic strings the validator returns.
- **TS** (`apps/dashboard/src/features/sources/djinniSearchSummary.ts`):
  `summarizeExpLevels(values: string[]): string` — used in the `SubscriptionRow` label.

Both functions return identical output for identical input; both are unit-tested with the
SC-004 verbatim shape list (Node.js 2y–5y → `"2–5 years"`; Golang 1y–3y → `"1–3 years"`;
non-consecutive `1y,3y` → `"1, 3 years"`).

## 5. Cross-app invariants

- The source of truth for the mode is the URL the operator pasted; no `mode` column or
  parallel `SubscriptionDto.Mode` field is introduced (research.md R3).
- `SubscriptionDto.url` (existing field, Go and TS identical) is the **only** cross-app
  contract surface this feature adds. Both apps parse it with the rules above; no other
  field-level contract change.
- The Go side rejects unknown shapes at save time (`subscriptions.service.go`). The TS
  side renders `null` from the basic-search parser for any URL the server would
  have rejected — and falls back to the existing default label — so the client never
  crashes on a shape the server didn't accept.