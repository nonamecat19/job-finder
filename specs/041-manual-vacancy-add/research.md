# Phase 0 Research: Manual Vacancy Add by URL

Eight decisions, in dependency order. Each records what was chosen, why, and what was
rejected. Line references are to `master` at the time of planning.

---

## D1 — Reading a standalone posting needs a new adapter port

**Decision**: Add `domain.PostingReader` to the jobsources adapter contract:

```go
type PostingReader interface {
    // MatchesPostingURL reports whether this URL is a single posting on this
    // adapter's host — false for search pages, listings and unrelated paths.
    MatchesPostingURL(rawURL string) bool
    // ReadPosting reads one posting page into a complete NormalizedJob.
    ReadPosting(ctx context.Context, rawURL string, config map[string]any) (dto.NormalizedJob, error)
}
```

Optional, like the existing `DetailNeeder` / `Credentialed` / `EmployerReporter` interfaces
(`jobsources/domain/adapter.go:16-31`), with a `domain.AsPostingReader(a Adapter)` helper.
Implement it for **Djinni** in this feature; every other adapter is left without it and
degrades to the fill-in path.

**Rationale**: The obvious reuse — `FetchDetail` — cannot do the job. Nine adapters have it
(djinni, dou, workua, indeed, remoteok, jobleads, glassdoor, wellfound, jobgether), but every
one returns a *patch* type carrying only description, salary, location, remote and postedAt.
`DjinniDetailPatch` (`adapters/djinni.go:210-217`) has no title and no company, because
enrichment applies it to a job whose title and company already came from a search card
(`enrichment/handler.go:153-176`). Manual add starts from a bare URL, so those fields must
come from the posting page itself. There is no way around adding extraction.

Making the port optional rather than extending `Adapter` keeps all 20-odd adapters compiling
untouched and makes "this host has no reader" a first-class, testable state — which FR-023
requires anyway.

**Alternatives considered**:

- *Extend `FetchDetail` to return a full `NormalizedJob`.* Rejected: it is called on the hot
  enrichment path for nine sources; widening its contract means touching all of them plus
  their tests, for a feature that needs one.
- *Generic schema.org/JSON-LD `JobPosting` extractor for all hosts.* Rejected outright by
  FR-024, and rightly — coverage is uneven and silent misextraction produces junk vacancies
  that look real. It remains a clean future addition behind the same port.
- *Reuse `Search()` with the posting URL as `SubscriptionURL`.* Rejected: adapters treat that
  as a listing page and parse cards; a posting page yields zero cards, which the Djinni loop
  reads as a legitimate empty result (016-FR-003).

---

## D2 — URL→source resolution belongs on the adapter, not in a central table

**Decision**: Resolution walks the registry and asks each `PostingReader` whether it claims
the URL, via `MatchesPostingURL`. First claim wins; the registry's order is deterministic
(`domain.Registry.order`). No central host→source map is introduced.

Rejection of search URLs (FR-005) is the same call: a Djinni preset-search URL is on
`djinni.co` but is not a posting, so `MatchesPostingURL` returns false, and the resolver
reports **not a job posting** rather than **no reader for this host** — the host is known, the
shape is wrong. Distinguishing these two is exactly what FR-018 asks for.

**Rationale**: The repo already has two host-matching mechanisms and neither fits. `roster.
MatchVendor` (`jobsources/roster/urlmatch.go`) maps ATS board URLs to employer identifiers,
not postings. The subscription validators (`subscriptions/application/service.go:117-135`)
validate *search* URLs and live in the wrong module — asking them about postings inverts
their purpose. Keeping the knowledge next to the parser that depends on it means a new
source's URL shapes and its extraction land in one file with one fixture test.

**Alternatives considered**:

- *A `map[host]sourceKey` in the manualadd module.* Rejected: a third place that knows about
  hosts, guaranteed to drift from the adapters. `domains/codebase-structure.md` § 3 exists to
  prevent precisely this.
- *Reuse the subscription URL validators inverted.* Rejected: they encode "is this a valid
  saved search", which is a different question with different answers.

---

## D3 — `Subscription.kind` discriminates manual from crawling

**Decision**: Add `kind text NOT NULL DEFAULT 'crawl'` to `Subscription`, with `'manual'` as
the only other value. Manual rows carry an empty `url`, are excluded from scheduling by a
`kind = 'crawl'` predicate in the scheduler query, and are created implicitly on first manual
add for a source (`EnsureManualSubscription(sourceKey)`, unique per source).

Guards, enforced in `subscriptions/application/service.go`:

- `Create` rejects `kind = 'manual'` from the HTTP surface — implicit creation only (FR-015).
- `Update` rejects any change to a manual row's `url` or `cron` (FR-015).
- `Delete` and disable are refused while jobs reference the row (FR-016).
- `validateSubscriptionURL` is skipped for manual rows — there is no URL to validate.

**Rationale**: The clarification put attribution on the subscription, and `Job.subscriptionId`
already exists (migration `00021_job_subscription.sql`, backfilled by `00028`), is already a
feed filter (`joblist.sql:9`), and is already set at insert by the ingest path. Attribution is
therefore free — no new `Job` column, no backfill, no risk to the dedupe key. A `kind`
discriminator is the minimum addition that keeps a non-crawlable row out of the scheduler.

Cron stays `NOT NULL` with its existing default rather than becoming nullable: manual rows
carry the default string, and `kind` — not a null cron — is what excludes them. Fewer null
branches in existing code that already reads `Cron` unconditionally
(`subscriptions/application/service.go:265`).

**Alternatives considered**:

- *A `manualAddedAt` timestamp on `Job` instead of subscription attribution.* Rejected by the
  clarification, and it would mean a second notion of origin alongside `subscriptionId`.
- *One global manual subscription across all sources.* Rejected: `Subscription.sourceKey` is a
  FK to `JobSource.key`, so a row belongs to exactly one source. One per source also gives
  FR-013 its natural per-source counts.
- *`enabled = false` to keep manual rows out of the scheduler.* Rejected: disabled means
  "paused by the operator", and reusing it makes the manual row look re-enablable.

---

## D4 — A registered no-op `manual` source backs unknown-host fill-ins

**Decision**: Register a `ManualAdapter` in the adapter registry with `Key() = "manual"`,
kind `manual`, `Search()` returning a permanent error ("the manual source is not crawlable"),
and `HealthCheck()` returning healthy. It implements no `PostingReader`. Fill-in vacancies
for hosts with no reader carry `sourceKey = "manual"` (FR-012a) and hang off the manual
subscription under that source.

**Rationale**: `Job.sourceKey` is `NOT NULL` and `Subscription.sourceKey` is a FK to
`JobSource.key`, so a fill-in vacancy on an unknown host needs *some* source row. It also has
to be a registered adapter or it vanishes from the sources list — `Service.List` iterates
`registry.All()` and drops DB rows with no adapter (`jobsources/application/service.go:79-101`),
and `GetByKey` refuses unregistered keys (`:103-107`). A tiny no-op adapter satisfies both
without special-casing either function.

`Search()` failing loudly rather than returning empty is deliberate: nothing should ever
schedule it (D3 already prevents that), and a silent empty result would look like a healthy
zero-result crawl.

**Alternatives considered**:

- *Nullable `Job.sourceKey` for manual vacancies.* Rejected: every consumer would need a null
  branch, for one edge case.
- *Store the real host as a free-text source key.* Rejected: `sourceKey` joins to
  `JobSource`; unregistered values break the sources list, health, and enrichment dispatch.
- *Special-case `manual` outside the registry.* Rejected: two functions already documented
  above would need exceptions, and future registry consumers would silently miss it.

---

## D5 — Persist and dedupe move out of the asynq worker

**Decision**: Move `persist.go` and `dedupe.go` from
`jobsources/interfaces/worker/` to a new `jobsources/application/ingest/` package, exporting
`ingest.NewPostingBatch`, `ingest.PersistBatch` and `ingest.DedupeKey`. The asynq handler and
the synchronous manual-add service both call it. Their tests
(`persist_test.go`, `persist_integration_test.go`, `dedupe_test.go`, `merge_test.go`) move
with them unchanged.

**Rationale**: Manual add must produce a vacancy that is byte-for-byte what a crawl produces —
same dedupe key, same insert, same repost bookkeeping, same activity rows. That code is
`persistBatch` (`worker/persist.go:74`) and `DedupeKey` (`worker/dedupe.go`), currently
unexported inside an interfaces-layer package. A synchronous HTTP service importing an asynq
worker package inverts the layering the module structure exists to enforce (024, 027):
`interfaces/` adapts the outside world in, and nothing should depend on it inward.

The move is mechanical — package clause, import fixes, and exporting three identifiers. No
logic changes, and the existing integration tests are the safety net.

**Alternatives considered**:

- *Export `worker.PersistBatch` and import the worker package from manualadd.* Rejected:
  cheapest diff, wrong direction. It makes the feature module depend on an asynq entry point
  it never uses and cements a layering violation that the next caller inherits.
- *Reimplement persistence in manualadd.* Rejected outright — two copies of the dedupe rule is
  precisely the drift the spec's FR-009 ("no field downstream must special-case") forbids in
  behaviour, and `domains/codebase-structure.md` forbids in code.
- *Enqueue an asynq ingest task and poll for it.* Rejected by FR-003a: the operator must get
  the vacancy, the reason, or the form as the direct result of submitting.

---

## D6 — The 30-second bound is one context, covering everything

**Decision**: The HTTP handler wraps the request context in
`context.WithTimeout(ctx, 30*time.Second)` and passes it through resolution, pacing, fetch,
parse and persist. On `context.DeadlineExceeded` the service reports the `timed_out` failure
kind, having written nothing. Post-ingest enqueues (match, ghost, enrich) happen after the
vacancy is committed and are explicitly **not** covered by the deadline (FR-003d) — they are
fire-and-forget `EnqueueContext` calls on a background context, mirroring
`handler.enqueueInserted` (`worker/handler.go:230-241`).

**Rationale**: The retrieval ladder and its per-host pacing already honour context
cancellation, so one deadline covers "waiting politely behind a crawl" and "the host is slow"
with no separate accounting — which is what FR-003b asks for. Atomicity is already there:
`persistBatch` runs inside `WithinTx` when a `TxRunner` is configured
(`worker/handler.go:215-228`), so a timeout mid-persist rolls back and FR-021 holds.

Using the *request* context as the parent means an operator who navigates away cancels the
work, which is the behaviour the "operator navigates away" edge case asks for.

**Alternatives considered**:

- *Separate budgets per stage (fetch 20 s, parse 5 s, persist 5 s).* Rejected: more knobs, no
  better outcome; the operator cares about one number.
- *Bypass pacing to fit the budget.* Rejected by FR-003c. Getting throttled off a source to
  save an operator 5 seconds is a bad trade.
- *Extend the deadline to cover matching.* Rejected by FR-003d — matching is an LLM call and
  can take minutes; the vacancy is useful before it finishes.

---

## D7 — `SourceRun` gains a subscription link and a trigger

**Decision**: Add to `SourceRun`:

- `subscriptionId uuid NULL` — FK to `Subscription`, `ON DELETE SET NULL`
- `trigger text NOT NULL DEFAULT 'scheduled'` — `'scheduled' | 'manual'`

Manual adds write a run like any ingest: insert on start, `FinishSourceRunOk` with
`found`/`new`, or `FinishSourceRunError` with the reason (FR-017e/f). `RecentSourceRunsForSource`
gains `AND "trigger" <> 'manual'` so failed manual adds never reach the
3-consecutive-failure health threshold (FR-017g, `worker/handler.go:295-315`). FR-013's count
and "most recent addition" come from `SUM("new")` and `MAX("startedAt")` over that
subscription's runs (FR-017h).

**Rationale**: `SourceRun` today records only `sourceId` and `searchId`
(`00001_init.sql:96-106`, plus `employerDetail` from 00025 and verdict columns from 00026) —
there is no way to attribute a run to a subscription, which FR-017h needs. The scheduled path
gets the same benefit for free: subscription-triggered crawls can finally be attributed too.
Backfilling the new column for historical runs is not attempted; null means "not attributable",
which is honest for rows written before the column existed.

`trigger` is a separate column rather than being inferred from `subscriptionId → kind`,
because the health query is on the hot path and a two-table join to answer "was this manual"
is worse than a column read.

**Alternatives considered**:

- *Derive manual-ness by joining `Subscription.kind`.* Rejected: extra join in a query that
  runs on every ingest failure.
- *No run record; derive counts from `Job` rows.* Rejected by the D5-adjacent clarification —
  it loses failures entirely, and diagnosing a flaky host after the fact is the main reason
  the operator wants the record.
- *A separate `ManualAddAttempt` table.* Rejected: a second run-shaped table that every
  run-listing view would have to union.

---

## D8 — Feed surfacing is an ordering term, never a stored date

**Decision**: `postedAt` is stored exactly as extracted, or null (FR-017a). The 24-hour
surfacing is a leading `ORDER BY` term in `ListJobsByScore` and `ListJobsByDate`:

```sql
LEFT JOIN "Subscription" s ON s."id" = j."subscriptionId"
...
ORDER BY (s."kind" = 'manual' AND j."ingestedAt" > now() - interval '24 hours') DESC,
         mr."score" DESC NULLS LAST, j."ingestedAt" DESC
```

The term is applied to the **default** ordering — `sort=score`, which is what the dashboard
sends when the operator has not chosen (`FeedPage.tsx:33,175`) — and omitted when the operator
explicitly picks `sort=date` (FR-017d). `Job.ingestedAt` is the add time for a manual vacancy,
so no new column is needed. `CountJobs` gets the same `LEFT JOIN` for the Manual filter but no
ordering term.

The Manual filter (FR-017) is a new `only_manual` narg: `s."kind" = 'manual'`, spanning all
sources — which the existing `subscription_id` filter cannot do, since there is one manual
subscription per source.

**Rationale**: Storing the add time in `postedAt` would corrupt every age-sensitive consumer —
post-age signals, ghost-job scoring — which FR-017b forbids. Keeping the boost in the query
means it expires by itself at the 24-hour mark with no cleanup job and no stored state
(FR-017d). A boolean expression sorts `true` before `false` under `DESC` in Postgres, so the
term costs one comparison.

**Alternatives considered**:

- *Fall back to the add time when `postedAt` is null.* Rejected: makes an unknown-age vacancy
  look fresh to ghost-job scoring, which reads posting age as a signal.
- *A `pinnedUntil` column.* Rejected: stored state needing expiry, when a query term is exact
  and self-clearing.
- *Sort manual adds to the top forever.* Rejected — the feed would accumulate a permanent
  manual band at the top, and FR-017d asks for the opposite.
