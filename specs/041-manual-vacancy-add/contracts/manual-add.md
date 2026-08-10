# Contract: Manual Vacancy Add

Two HTTP endpoints and one Go port. Conventions follow the existing API: chi routing mounted
in `httpapi`, `httpx.WriteJSON` / `httpx.WriteError`, camelCase JSON, DTOs defined once in
`internal/dto` and regenerated into `packages/shared` by tygo.

---

## `POST /api/jobs/manual`

Add a vacancy by URL. **Synchronous** — the response *is* the outcome (FR-003a). Bounded at
30 seconds (FR-003b).

### Request

```json
{ "url": "https://djinni.co/jobs/123456-senior-go-engineer/" }
```

| Field | Type | Rules |
|---|---|---|
| `url` | string, required | Must parse and use `http`/`https`. Validated before any network request (FR-002). |

### Response envelope

Every non-5xx response is a `ManualAddResultDto` discriminated by `outcome`.

**`created` — 201**

```json
{
  "outcome": "created",
  "job": { "id": "…", "sourceKey": "djinni", "title": "Senior Go Engineer", "…": "…" }
}
```

The vacancy is committed and downstream work is enqueued. `job` is the full `JobDto` the feed
and detail views already consume — no manual-specific shape (FR-009).

**`duplicate` — 200**

```json
{
  "outcome": "duplicate",
  "job": { "id": "…", "…": "…" },
  "reason": "This vacancy is already in your feed."
}
```

Not an error (FR-007b). `job` is the *existing* vacancy so the client can navigate to it.

**`needs_fill_in` — 200**

```json
{
  "outcome": "needs_fill_in",
  "kind": "incomplete",
  "reason": "The page was read but has no description.",
  "draft": {
    "url": "https://…",
    "sourceKey": "djinni",
    "title": "Senior Go Engineer",
    "company": null,
    "remote": true
  }
}
```

`draft` carries whatever was extracted, so the operator completes rather than retypes
(FR-019). `kind` is `no_reader` or `incomplete`.

**`failed` — 422**

```json
{
  "outcome": "failed",
  "kind": "blocked",
  "reason": "djinni.co returned a bot challenge; the posting could not be read."
}
```

Nothing is stored and the same URL may be resubmitted immediately (FR-021).

### Failure kinds (FR-018)

Exactly six, each with a distinct operator-facing message. The client switches on `kind`, not
on prose.

| `kind` | HTTP | Meaning | Recovery offered |
|---|---|---|---|
| `invalid_url` | 400 | Not parseable, or not `http`/`https`. No network request made. | Fix the URL |
| `not_a_posting` | 422 | Host is known, but the URL is a search or listing page. | Create a subscription instead |
| `no_reader` | 200 | No adapter reads this host. | Fill-in form (P3) |
| `unreachable` | 422 | DNS failure, connection refused, 404, 410. | Retry, or fill-in |
| `blocked` | 422 | Bot challenge, login wall, 403/429 after the ladder exhausted its rungs. | Fill-in |
| `timed_out` | 422 | 30 s elapsed, including any pacing wait. | Retry |
| `incomplete` | 200 | Read, but missing title, company or description. | Fill-in form, pre-filled |

`no_reader` and `incomplete` are 200 because they are recoverable states carrying a draft, not
failures of the request. Until User Story 3 ships they return 422 with the same `kind` and no
`draft` — the client renders the reason and stops.

### Concurrency

Two simultaneous submissions of the same URL must yield one vacancy (FR-008). Guaranteed by
the existing `Job_dedupeKey_unique` constraint: the loser of the race gets a unique violation
inside its transaction, retries the lookup, and returns `duplicate`.

---

## `POST /api/jobs/manual/fill-in`

Save a hand-completed vacancy (User Story 3). No page is fetched.

### Request

```json
{
  "url": "https://example.com/careers/123",
  "sourceKey": "djinni",
  "title": "Senior Go Engineer",
  "company": "NovaTech",
  "description": "…",
  "location": "Kyiv",
  "remote": true,
  "salaryRaw": "$5000-7000",
  "postedAt": "2026-07-28T00:00:00Z"
}
```

| Field | Required | Notes |
|---|---|---|
| `url` | yes | Same validation as the add endpoint |
| `title`, `company`, `description` | yes | Rejected when missing or blank (FR-020) |
| `sourceKey` | no | Defaults to `manual` when the host has no reader (FR-012a) |
| everything else | no | Stored as given; `postedAt` is never defaulted to now (FR-017a) |

### Responses

- **201** — `{ "outcome": "created", "job": { … } }`
- **400** — missing required fields, naming each: `{ "error": "title, description are required" }`
- **200** — `{ "outcome": "duplicate", "job": { … } }` when the dedupe key already exists

---

## Modified: `GET /api/jobs`

One new filter (FR-017):

| Param | Type | Effect |
|---|---|---|
| `onlyManual` | bool | Restricts to vacancies attributed to a Manual subscription, across all sources |

Ordering change: with the default `sort=score`, vacancies added by hand in the last 24 hours
sort first (FR-017c). With the explicit `sort=date`, they do not (FR-017d).

---

## Modified: `GET /api/subscriptions`

`SubscriptionDto` gains `kind` (`"crawl" | "manual"`), and manual rows additionally carry
`manualCount` and `lastAddedAt` (FR-013).

Write guards on existing endpoints:

| Endpoint | Behaviour on a manual row |
|---|---|
| `POST /subscriptions` with `kind: "manual"` | **400** — manual subscriptions are created implicitly (FR-015) |
| `PUT /subscriptions/{id}` changing `url` or `cron` | **400** — immutable on a manual row (FR-015) |
| `DELETE /subscriptions/{id}` with vacancies attached | **409** — would orphan vacancies (FR-016) |
| `POST /subscriptions/{id}/run` | **400** — a manual subscription is not runnable (FR-014) |
| `POST /subscriptions/run-all` | Skips manual rows silently (FR-014) |

---

## Go port: `domain.PostingReader`

In `apps/api/internal/jobsources/domain/adapter.go`, alongside the existing optional
interfaces (`DetailNeeder`, `Credentialed`, `EmployerReporter`):

```go
// PostingReader is implemented by adapters that can read a single posting page
// into a complete NormalizedJob. Optional: an adapter without it cannot serve
// manual add, and its hosts degrade to the fill-in path.
type PostingReader interface {
    // MatchesPostingURL reports whether rawURL is a single posting on this
    // adapter's host. False for search pages, listings, and other hosts.
    // Must not perform I/O.
    MatchesPostingURL(rawURL string) bool

    // ReadPosting reads one posting page into a NormalizedJob. Title, Company,
    // URL and Description are filled where the page provides them; the caller
    // decides whether the result is complete enough to store.
    ReadPosting(ctx context.Context, rawURL string, config map[string]any) (dto.NormalizedJob, error)
}

// AsPostingReader returns the adapter's PostingReader, if it has one.
func AsPostingReader(a Adapter) (PostingReader, bool) {
    pr, ok := a.(PostingReader)
    return pr, ok
}
```

### Contract for implementors

1. **`MatchesPostingURL` does no I/O and never panics** on malformed input — it is called for
   every registered adapter on every add, in registry order, first claim wins.
2. It returns **false for search URLs on its own host**. Djinni's
   `?search_type=basic-search` URL is a Djinni URL that is not a posting; the caller reports
   `not_a_posting` precisely because the host was claimed at host level but rejected at shape
   level.
3. **`ReadPosting` returns partial results rather than erroring** when the page loads but some
   fields are absent — that is what feeds the fill-in draft. It errors only when the page
   could not be read at all.
4. It **honours the context deadline** and returns `context.DeadlineExceeded` unwrapped enough
   for `errors.Is` to see it.
5. It **sets `SourceKey` to its own `Key()`** and resolves `URL` to an absolute, canonical
   form.
6. It **uses the same retrieval path as the adapter's other methods**, so pacing and ladder
   escalation apply unchanged (FR-003c).

### Djinni implementation notes

- `MatchesPostingURL`: host is `djinni.co` or `www.djinni.co`, path matches `/jobs/<digits>-`.
  Explicitly false for `/jobs` and `/jobs/` with a query — that is the preset-search shape
  that `validateDjinniSubscriptionURL` accepts.
- `ReadPosting` extends the selectors `FetchDetail` already uses
  (`adapters/djinni.go:219-250`) with title and company, which the detail path never needed.
  Description, salary, location, remote and `postedAt` reuse the existing selectors verbatim
  so a manually added Djinni vacancy matches an enriched crawled one (SC-002).
- Fetches anonymously, like the rest of the post-016 Djinni path.
- Tested against a saved fixture of a real posting page, following `djinni_test.go`.
