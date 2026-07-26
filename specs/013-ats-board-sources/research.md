# Phase 0 Research: Employer ATS Board Sources

## 1. Board vendor read endpoints

**Decision**: Read each vendor via its existing public, unauthenticated JSON API, one HTTP call
per employer per run:

| Vendor | Endpoint shape | Auth |
|---|---|---|
| Greenhouse | `GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true` | none |
| Lever | `GET https://api.lever.co/v0/postings/{token}?mode=json` | none |
| Ashby | `GET https://api.ashbyhq.com/posting-api/job-board/{token}` | none |
| Workable | `GET https://apply.workable.com/api/v1/widget/accounts/{token}` | none |
| SmartRecruiters | `GET https://api.smartrecruiters.com/v1/companies/{token}/postings` | none |

**Rationale**: All five publish stable, documented, public JSON endpoints designed for embeddable
job widgets — no login, no session, no JS rendering, matching FR-001 exactly and needing none of
Feature 014's fetch ladder. `content=true` (Greenhouse) / `mode=json` (Lever) return the full
posting description inline, satisfying FR-004 (no separate enrichment pass).

**Alternatives considered**: Scraping each vendor's rendered HTML board page — rejected, strictly
worse than the JSON API these vendors already expose for this purpose, and fragile to markup
changes (violates the "add a vendor without changing shared behavior" goal, FR-005).

## 2. Fan-out: one run reads many employers

**Decision**: `Adapter.Search(ctx, query, config) ([]dto.NormalizedJob, error)` is called once per
run per vendor, same as today. The board adapter's `Search` internally iterates the roster
(fetched from the new `roster.Service`, filtered to that vendor), calling the vendor endpoint once
per employer, capped by FR-007. Per-employer outcomes are collected into a run-scoped result and
returned to the ingestion handler alongside the flattened `[]NormalizedJob` — via a new optional
interface (see below), not by changing the existing `Adapter` signature.

**Rationale**: Existing `Adapter` interface takes a single `SearchQuery`; board sources are not
keyword-driven (Assumptions: "read in full on every run"), so `query` is effectively ignored by
these adapters and the roster is the real input. Keeping `Search`'s signature unchanged avoids
touching every existing adapter and the `Adapter` interface itself (Constitution III/no drift);
per-employer detail is exposed via a new optional interface:

```go
// EmployerReporter is implemented by adapters whose one Search call fans out
// over multiple employers and needs to report a per-employer outcome.
type EmployerReporter interface {
    LastRunDetail() []EmployerRunOutcome
}
```
`ingestion.Handler.ProcessTask` type-asserts for this after calling `Search`, same pattern already
used for `DetailNeeder`.

**Alternatives considered**: One `JobSource` row + one `SourceRun` per *employer* instead of per
vendor — rejected; spec Assumptions pins "each board vendor appears as its own source... roster is
shared and stored once," and FR-022 requires vendor-level (not per-employer) enable/disable/health
parity with existing sources.

## 3. Employer roster storage & candidate discovery

**Decision**: New tables `EmployerBoard` (the roster) and `BoardCandidate` (proposed/rejected),
detailed in data-model.md. Candidate discovery is a synchronous read-only query over the existing
`Job` table's `url` column — regex/host match per vendor (e.g. `boards.greenhouse.io/{token}`,
`jobs.lever.co/{token}`), extracting `(vendor, employerIdentifier)`, deduplicated against
`EmployerBoard` and already-seen `BoardCandidate` rows (FR-013, Edge Cases: "already in roster:
not offered again").

**Rationale**: Spec Assumptions state discovery "runs against listings already stored... costs
nothing and touches no third-party host" — a DB-only scan, no new fetch surface.

**Alternatives considered**: Crawling the web for employer boards — explicitly out of scope per
spec Assumptions.

## 4. Dedup/merge extension

**Decision**: Extend `ingestion/dedupe.go`'s existing `DedupeKey`/`CanonicalURL` matching with a
second-pass merge check specific to the board↔aggregator case: when a new posting's
`(company, title)` normalized match an existing `Job` row from a *different* source but the URLs
don't canonicalize identically (aggregator URL vs. board URL never match by construction), treat
it as the same opening if company+title match closely enough (existing embedding similarity
infrastructure — `Job.embedding` — is reused as the match signal, not reimplemented) AND the
existing job's `postedAt`/company are consistent. On merge: update `Job.url` to the board URL,
append the new source to a `sources`-seen list on the row (new column, see data-model.md), and
leave every other user-owned column (`status`, `Application`, `MatchResult`, `GeneratedDocument`
rows, all FK'd to `Job.id`) untouched — merge is an UPDATE to the existing row, never a delete+
reinsert, so FR-017 (preserve user state) holds by construction.

**Rationale**: Reusing the existing embedding column avoids introducing a second similarity
system; the existing FK structure (`Application`/`MatchResult`/`GeneratedDocument` all reference
`Job.id`) means an UPDATE-not-replace merge preserves everything for free.

**Alternatives considered**: Fuzzy title-string matching (Levenshtein/Jaro) instead of embeddings
— rejected as strictly weaker than the similarity signal already computed and stored for every
job; would risk FR-018's false-merge requirement (distinct jobs with identical titles at
different companies) more than the embedding+company-match combination does.

## 5. Stale flagging

**Decision**: `EmployerBoard.staleSinceRunCount` incremented each run that produces zero postings
for that employer, reset to 0 on any run with ≥1 posting found; `stale = staleSinceRunCount >=
threshold` (config constant, not user-configurable in v1 beyond the "configurable number" FR-014
already implies a single system constant is enough at launch — a per-employer override is not
required by any acceptance scenario).

**Rationale**: Matches FR-014 exactly; computed at read time from a counter column, no separate
scheduled job needed.
