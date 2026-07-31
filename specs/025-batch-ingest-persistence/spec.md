# Feature Specification: Batched, Atomic Ingest Persistence

**Feature Branch**: `025-batch-ingest-persistence`

**Created**: 2026-07-30

**Status**: Clarified

**Input**: User description: "Batch ingest persistence: replace the per-job N+1 dedupe/insert loop in the ingest worker with a batched, transactional persist path."

## Clarifications

### Session 2026-07-30

Resolved with recommended defaults. Each had a clearly preferable option given the existing schema
and the project's constraints.

- Q: What is the unit of atomicity — the whole run, or a chunk? → A: **The whole run, delivered as
  chunks inside one transaction.** Chunking is a statement-size concern, not a consistency
  boundary; letting a chunk commit independently would reintroduce exactly the partial-run state
  FR-004 exists to eliminate.
- Q: How is at-most-once repeat-sighting counting achieved across retries? → A: **By recording
  which run last incremented each posting**, so a retry of the same run is recognised and skipped.
  Rejected: making the ingest task non-retryable (loses genuine transient recovery), and
  timestamp-window heuristics (wrong across a slow retry).
- Q: Bulk insert mechanism — copy-style bulk load, or a single multi-row statement? → A: **A single
  multi-row statement built from arrays.** The copy-style path cannot express `ON CONFLICT`, and
  conflict handling is mandatory here because `Job.dedupeKey` carries a unique constraint and
  concurrent runs collide on it (FR-013). One statement also returns which rows were actually
  inserted, which is needed to queue downstream work for new postings only.
- Q: Is queueing downstream work inside the transaction? → A: **No.** The queue is a separate
  system and cannot participate in a database transaction; enrolling it would mean holding the
  transaction open across network calls to Redis. Work is queued after commit, and the existing
  stranded-job sweep remains the safety net — this is stated in Assumptions and is unchanged from
  today's guarantee.
- Q: Default chunk size? → A: **500 postings per statement**, tunable. Chosen against PostgreSQL's
  65535-parameter limit with ~14 columns per posting; 500 leaves a wide margin without making the
  statement count meaningful for realistic run sizes.

## Problem Statement

When a job source finishes returning results, the system stores them one at a time. For every single posting it asks the database whether that posting is already known, then issues a second write to either record the repost or create the new row — and for postings from employer-board sources, a third lookup that scans the entire job table because no index supports the way that lookup compares company names. A run returning several hundred postings therefore performs many hundreds of separate database round trips in sequence, and the cost grows in direct proportion to how many jobs a source returns. The database is idle between round trips and the source run takes far longer than the work justifies.

The same loop is also not atomic. Nothing groups the postings from one run into a single unit of work, so a failure partway through leaves the run's results half-stored: some postings recorded, some not, and the run itself marked as failed. Because the ingest task is retried automatically after a failure, the retry re-walks the postings that were already stored and counts each of them as a *fresh sighting* of the posting — inflating the "how many times have we seen this job" counter that the ghost-job detector relies on. A source that fails near the end of a large run can therefore corrupt the signal quality of every posting it had already processed, and it does so silently.

There is already a facility in the codebase for grouping writes into one atomic unit, but only one feature uses it. There is also already a compensating background sweep that finds jobs which were stored but never queued for scoring, which confirms the non-atomicity is known and currently worked around rather than fixed.

This feature makes storing a run's results a bounded, batched, atomic operation: a fixed small number of database interactions regardless of how many postings a run returns, and an outcome where a run's results are either recorded or not, never half-recorded and never double-counted.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A large source run completes quickly (Priority: P1)

A user triggers a search against a source that returns a large number of postings. The system recognises which postings it already knows and which are new by consulting the database a fixed small number of times rather than once or more per posting, then records all the new postings and all the repeat sightings in bulk. The run finishes in a time governed by how long the source took to respond, not by how many postings it returned.

**Why this priority**: This is the whole point of the feature and the largest single throughput gain available in ingestion. It is independently valuable even if nothing about atomicity changes, because the current cost is unbounded in the size of the result set.

**Independent Test**: Run a source that returns several hundred postings against a real database, count the database interactions the run performs, and confirm the count does not grow with the number of postings. Compare wall-clock storage time against the current behaviour.

**Acceptance Scenarios**:

1. **Given** a source run returns N postings, none of which are known, **When** the results are stored, **Then** the number of database interactions used to store them does not increase as N increases.
2. **Given** a source run returns N postings, all of which are already known, **When** the results are stored, **Then** the number of database interactions used does not increase as N increases, and every posting's repeat-sighting count is incremented exactly once.
3. **Given** a source run returns a mixture of known and unknown postings, **When** the results are stored, **Then** each new posting is created once, each known posting is recorded as seen again once, and the run reports accurate found/new totals.
4. **Given** a run returns postings from an employer-board source, **When** the system looks for an existing posting from a different source to merge into, **Then** that lookup is served by an index rather than by scanning the whole job table, and it is performed for the whole batch rather than per posting.
5. **Given** a run returns an unusually large number of postings, **When** the results are stored, **Then** the work is split into bounded chunks so that no single database interaction grows without limit.

---

### User Story 2 - A failed run leaves no partial or double-counted results (Priority: P1)

A source run fails partway through storing its results — the connection drops, the process is stopped, or a write is rejected. The user later sees the run marked as failed, and the postings from that run are either all present or all absent. When the system automatically retries the run, postings that were already recorded are not counted as having been seen an additional time, so the "seen this many times" signal stays truthful.

**Why this priority**: Equal to Story 1 because it is a correctness defect, not just a performance one. The inflated sighting counter feeds the ghost-job detector, so the current behaviour silently degrades a user-visible quality signal, and it degrades it worst exactly when a source is flaky — which is most of them.

**Independent Test**: Force a failure partway through storing a run's results, confirm no partial results are visible, then allow the retry to run and confirm every posting's repeat-sighting count matches the number of genuine distinct runs that saw it, not the number of attempts.

**Acceptance Scenarios**:

1. **Given** a run's results are being stored, **When** a failure occurs partway through, **Then** none of that run's postings are visible afterwards, and the run is reported as failed.
2. **Given** a run failed partway through and is retried automatically, **When** the retry stores the same postings, **Then** each posting's repeat-sighting count reflects one sighting for that run in total, regardless of how many attempts it took.
3. **Given** a run is retried after a failure, **When** it completes, **Then** the run's found and new totals describe the successful attempt and are not the sum of all attempts.
4. **Given** the process is stopped abruptly while storing a run's results, **When** it restarts, **Then** the database contains no half-stored run, and the next run over the same source produces the correct new/repost classification.

---

### User Story 3 - Newly stored postings still reliably reach scoring (Priority: P2)

Every new posting stored by a run is subsequently scored, enriched, or ghost-checked according to its source, exactly as before. Batching the storage does not cause any posting to be stored and then forgotten, and does not cause any posting to be queued for scoring twice.

**Why this priority**: This is a regression guard rather than new value, but it protects the most user-visible consequence of ingestion — a job that never appears in the scored feed. A background sweep already exists to recover stranded jobs; this feature must not increase how often that sweep has work to do.

**Independent Test**: Store a batch containing both list-only postings and full postings, and confirm each is routed to enrichment or to scoring exactly once, with none stranded and none duplicated.

**Acceptance Scenarios**:

1. **Given** a batch of new postings from a source whose results carry full descriptions, **When** the batch is stored, **Then** each posting is queued for scoring and for ghost checking exactly once.
2. **Given** a batch of new postings from a source whose results are list-only stubs, **When** the batch is stored, **Then** each posting is queued for detail retrieval exactly once and is not scored twice against two different versions of the same posting.
3. **Given** a batch is stored successfully but the queueing step fails afterwards, **When** the existing recovery sweep next runs, **Then** the affected postings are picked up and scored, and no posting is lost.
4. **Given** a run's storage is rolled back, **When** the run ends, **Then** no scoring, enrichment or ghost-check work is queued for postings that were not actually stored.

---

### Edge Cases

- A single run returns two postings that resolve to the same identity (same company, title and canonical link). The batch must record one posting and one repeat sighting, not two postings or a conflict failure.
- A posting arrives while a different run is concurrently storing the same posting. Exactly one record must result, and neither run may fail because of the collision.
- A run returns zero postings. The run must complete successfully with zero found and zero new, performing no storage work.
- A run returns more postings than can be sent to the database in a single interaction. The batch must be chunked, and a failure in a later chunk must not leave earlier chunks visible.
- An employer-board posting matches an existing posting from another source by company but the titles do not overlap. It must be stored as a new posting, not merged.
- A posting's raw payload is malformed and cannot be serialised. That posting must be skipped or stored with an empty payload without failing the whole batch.
- The database rejects the batch because the transaction exceeds a lock or statement timeout. The run must fail cleanly with no partial results and be retryable.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST determine which postings in a run's results are already known using a number of database interactions that does not grow with the number of postings in the run.
- **FR-002**: The system MUST create all new postings from a run in bulk rather than one statement per posting.
- **FR-003**: The system MUST record all repeat sightings from a run in bulk rather than one statement per posting.
- **FR-004**: The system MUST store all of a run's results as a single atomic unit, such that a failure at any point leaves none of that run's postings recorded.
- **FR-005**: The system MUST ensure that a posting's repeat-sighting count increases by at most one per source run, regardless of how many times that run is attempted.
- **FR-006**: The system MUST resolve, for a whole batch at once, whether employer-board postings should be merged into an existing posting from a different source, rather than resolving this per posting.
- **FR-007**: The lookup that matches postings by company name MUST be served by an index rather than by scanning the whole job table.
- **FR-008**: The system MUST deduplicate postings *within* a single batch before storing, so that duplicate identities in one run's results produce one posting.
- **FR-009**: The system MUST split batches larger than a configured maximum into bounded chunks, while preserving the all-or-nothing outcome across the whole run.
- **FR-010**: The system MUST queue each newly stored posting for exactly one downstream activity — scoring plus ghost checking for full postings, detail retrieval for list-only postings — and MUST NOT queue work for postings whose storage was rolled back.
- **FR-011**: The system MUST report accurate found and new totals for each run, describing the successful attempt only.
- **FR-012**: The system MUST continue to classify a posting's identity by the same rule as today, so that postings recorded before this change are still recognised as known afterwards.
- **FR-013**: Concurrent runs storing the same posting MUST result in exactly one stored posting, with neither run failing.
- **FR-014**: The system MUST record, per run, how long storage took and how many database interactions it used, so the improvement is observable rather than asserted.

### Key Entities

- **Source run**: One execution of one job source, carrying the found and new totals, its outcome, and now its storage duration. The unit of atomicity introduced by this feature.
- **Posting**: A discovered job, identified by a stable identity derived from company, title and canonical link, carrying a repeat-sighting count and the set of sources it has been seen on.
- **Repeat sighting**: The record that an already-known posting was seen again, which increments the posting's sighting count and refreshes its last-seen time. Must now be at-most-once per run.
- **Merge candidate**: An existing posting from a different source that an employer-board posting should be folded into rather than duplicated.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Storing the results of a run containing 500 postings takes no more than 5% of the wall-clock time it takes today.
- **SC-002**: The number of database interactions used to store a run is constant with respect to the number of postings, apart from chunking, and is at most 10 per chunk.
- **SC-003**: After ten consecutive forced mid-storage failures and automatic retries of the same run, every affected posting's repeat-sighting count is exactly one higher than before the run, not eleven.
- **SC-004**: No source run ever leaves partially stored results: in 100 randomised failure-injection trials, every trial ends with either all of the run's postings present or none of them.
- **SC-005**: The rate at which the existing stranded-job recovery sweep finds work to do does not increase after this change.
- **SC-006**: A full scheduled ingestion cycle across all enabled sources completes in measurably less time than before, with the reduction attributable to storage rather than to source response time.

## Assumptions

- The rule that determines a posting's identity is unchanged by this feature; changing it would invalidate every stored posting and is explicitly out of scope.
- The behaviour that a repeat sighting also refreshes the posting's last-seen time and backfills its subscription attribution is preserved.
- The existing background sweep that recovers postings stored but never scored remains in place; this feature reduces how often it is needed but does not replace it.
- Queueing downstream work is not part of the atomic storage unit, because the queue is a separate system; the recovery sweep remains the safety net for that gap.
- Atomicity is scoped to a single source run. Two different runs remain independent units and may interleave.
- **When two runs race to store the same posting, the loser records no repeat sighting for it.** The posting was genuinely new when that run classified it, so counting it as re-seen would be wrong; the sighting is recorded by the next run that encounters it. This is a small semantic change from today, where the losing path would have counted it, and it very slightly under-counts sightings in the rare concurrent case. Called out because the sighting count feeds the ghost-job detector.
- The maximum batch chunk size is a tunable with a sensible default rather than a value the user configures per source.
- Scraping sources remain best-effort and unstable upstream, per the project's architecture constraints; this feature improves how their results are stored, not how reliably they are obtained.
- No dashboard or API contract changes are required; this is entirely a backend ingestion change, observable only as speed and as more truthful sighting counts.
