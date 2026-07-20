# Feature Specification: Salary Inference for Postings That Hide Compensation

**Feature Branch**: `006-salary-inference`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "When a posting hides comp, predict a salary band from title + geo + company size + levels.fyi-like data. Auto-filter jobs below the user's floor."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See an inferred band instead of "salary hidden" (Priority: P1)

A job seeker opens their feed. Most Ukrainian and remote-EU postings show no compensation at all. Instead of a blank salary field, each such job now carries an estimated salary band — a min, a max, a currency — with a visible indicator of how much to trust it. The user can tell at a glance which estimates are grounded in real observed data and which are a rough guess.

**Why this priority**: This is the whole feature's value in one increment. Without the band there is nothing to filter on, nothing to break down, and nothing to tune a floor against. It ships alone and is immediately useful — a feed where two thirds of cards said nothing about money now says something about all of them.

**Independent Test**: Ingest a batch of jobs where `salaryRaw` is null, run inference, and confirm each job's card shows a band and a confidence indicator. No floor needs to be configured and no detail page needs to be opened.

**Acceptance Scenarios**:

1. **Given** a job whose posting states no compensation, **When** inference runs, **Then** the job carries a minimum, a maximum, a currency, a confidence value, and a named source, and the card displays the band rather than "salary hidden".
2. **Given** a job whose posting states compensation in a parseable form, **When** inference runs, **Then** the stated compensation is used directly and the job is marked as coming from the posting itself, not from an estimate.
3. **Given** a job whose inferred band carries a confidence below the low-confidence threshold, **When** the card renders, **Then** the band is visibly marked as low confidence rather than presented with the same weight as a well-grounded estimate.
4. **Given** no source can produce any band for a job at all, **When** inference completes, **Then** the job is left with no band, the card falls back to today's behavior, and the run does not fail.
5. **Given** inference has already run for a job, **When** it runs again without new information, **Then** the stored band is not recomputed and no additional LLM call is made.

---

### User Story 2 - Hide jobs that pay below the user's floor (Priority: P2)

The user sets a salary floor. Jobs whose inferred or stated band falls entirely below that floor disappear from the feed by default, so the feed stops being dominated by roles the user would never take. A single toggle brings them back when the user wants to see the whole market.

**Why this priority**: Depends on Story 1 producing a band, but delivers the second half of the user's stated need — "auto-filter jobs below the user's floor". Independently shippable once bands exist.

**Independent Test**: Set a floor, load the feed, and confirm below-floor jobs are absent; toggle the filter off and confirm they reappear, visually marked.

**Acceptance Scenarios**:

1. **Given** a configured floor and a job whose band maximum is below it, **When** the feed loads with default filters, **Then** the job is not in the results.
2. **Given** the same job, **When** the user toggles the below-floor filter off, **Then** the job appears in the results and its card carries a visible below-floor marker.
3. **Given** a floor of zero, **When** the feed loads, **Then** no job is filtered on salary grounds — a zero floor disables the filter entirely.
4. **Given** a job with no band at all (inference produced nothing), **When** the floor filter is active, **Then** the job is still shown — absence of a band is never treated as below floor.
5. **Given** a job filtered out by the floor, **When** its record is inspected, **Then** its status is unchanged and still `found` — filtering is a view concern, never a state change.
6. **Given** a job whose band straddles the floor (minimum below, maximum above), **When** the floor filter is active, **Then** the job is shown — only bands entirely below the floor are hidden.

---

### User Story 3 - Understand where an estimate came from (Priority: P3)

On a job's detail page the user expands a breakdown of the salary estimate and sees which sources contributed, what each one predicted independently, and how confident each was — enough to decide whether to trust the number before using it in a salary negotiation.

**Why this priority**: Trust-building and debugging, not core function. The feed is usable without it, but a number with no provenance is a number the user will eventually stop believing.

**Independent Test**: Open a job with a blended estimate and confirm the breakdown names each contributing source with its own band and confidence.

**Acceptance Scenarios**:

1. **Given** a job whose band was blended from more than one source, **When** the user opens the detail page, **Then** each contributing source is listed with the band and confidence it contributed.
2. **Given** a job whose band came from a single source, **When** the breakdown renders, **Then** that one source is named and no blending is implied.
3. **Given** a job with a stated, parsed compensation, **When** the breakdown renders, **Then** it identifies the posting itself as the source and shows no estimate.

---

### Edge Cases

- **`salaryRaw` present but unparseable** (unknown currency symbol, malformed range, "competitive", "договірна") → parsing yields nothing; the job falls through to estimation exactly as if compensation were absent. The unparsed original text is preserved and still displayed.
- **LLM disagrees sharply with the observed-data cache** → both bands enter the blend at their own confidences; the disagreement lowers neither, but the resulting wide band is itself the signal. The breakdown (Story 3) exposes the disagreement rather than hiding it.
- **The title+geo+company-size bucket is empty in every source** → no band is stored, the card falls back to today's behavior, and the job is never filtered out by the floor.
- **The posting has no geo** → the geo dimension degrades to a catch-all bucket; estimates remain possible but at reduced confidence, since geo is the strongest predictor after title.
- **Floor is zero or unset** → the floor filter is inert; every job is shown regardless of band.
- **A currency other than the floor's currency** → bands are compared against the floor only after conversion to a common currency; a band whose currency cannot be converted is never filtered out.
- **The external reference dataset is unreachable at startup** → the system starts anyway and runs on whatever is already cached plus the other two sources; an unreachable dataset degrades quality, never availability.
- **The observed-data cache is empty on a fresh install** → the self-hosted source contributes nothing; estimates come from the other two sources until enough jobs with stated compensation have been ingested.
- **A stated compensation is a monthly figure while the floor is annual** → periods are normalized before any comparison; a monthly-vs-annual mismatch must never silently produce a 12× error.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST produce, for a job whose posting states no compensation, an estimated salary band consisting of a minimum, a maximum, a currency, a confidence between 0 and 1, and the name of the source that produced it.
- **FR-002**: System MUST derive estimates from three independent sources — a language-model prediction over the posting text, an external public compensation reference dataset, and the system's own cache of compensation observed in already-ingested postings.
- **FR-003**: System MUST have each source emit its own band and its own confidence independently, without any source seeing another's answer.
- **FR-004**: System MUST combine multiple sources' bands into one final band by weighting each source's band by its own confidence and normalizing those weights, and MUST record the combined result as blended.
- **FR-005**: System MUST set the final confidence to the sum of the contributing sources' confidences, capped so it never exceeds 1.
- **FR-006**: System MUST mark a band whose final confidence falls below a low-confidence threshold so the interface can present it as untrustworthy rather than authoritative.
- **FR-007**: System MUST parse compensation stated in an already-ingested posting into a minimum, a maximum, and a currency, and MUST store that parsed result on the job.
- **FR-008**: System MUST treat a parsed, posting-stated compensation as ground truth for its title-and-geo bucket, taking precedence over any estimate for that same job.
- **FR-009**: System MUST leave a job with no band, rather than a fabricated one, when no source can produce an estimate, and MUST NOT fail the run.
- **FR-010**: System MUST persist the resulting band on the job itself, since a band describes exactly one posting.
- **FR-011**: System MUST record which source or combination of sources produced each stored band, distinguishable between the language model, the external dataset, the observed-data cache, and a blend.
- **FR-012**: System MUST cache the external reference dataset locally after first retrieval and MUST serve subsequent lookups from that cache rather than re-retrieving per job.
- **FR-013**: System MUST key the external dataset cache by the combination of job title, geography, and company-size bucket, so a lookup for a job resolves to a bucket rather than to an exact posting.
- **FR-014**: System MUST accept a user-configured salary floor through the same configuration surface the rest of the system uses, changeable without a redeploy.
- **FR-015**: System MUST exclude from the feed, by default, jobs whose band lies entirely below the configured floor.
- **FR-016**: System MUST offer a toggle that reveals below-floor jobs, and MUST visibly mark each revealed job as below floor.
- **FR-017**: System MUST NOT change a job's status as a result of salary filtering; a filtered job remains in the `found` state and stays retrievable.
- **FR-018**: System MUST treat a floor of zero as disabling the floor filter entirely.
- **FR-019**: System MUST NOT filter out a job that has no band, regardless of the floor.
- **FR-020**: System MUST normalize currency and pay period before comparing any band against the floor, and MUST NOT filter out a band whose currency cannot be normalized.
- **FR-021**: System MUST present, on a job's detail view, the source or sources behind its band together with each source's own band and confidence.
- **FR-022**: System MUST NOT recompute a band for a job that already has one unless the underlying inputs have changed, so that repeated runs do not repeat model calls.
- **FR-023**: System MUST confine a failure of any one source (model unavailable, dataset unreachable, cache empty) to that source, still producing a band from whichever sources did succeed.
- **FR-024**: System MUST preserve and continue to display the posting's original compensation text alongside any parsed or estimated band, never replacing it.
- **FR-025**: System MUST run inference using only the self-hosted model runtime, with no dependency on a third-party paid inference API.

### Key Entities

- **Salary Band**: An estimate or observation of what a posting pays — a minimum, a maximum, a currency, and a pay period. The unit every source emits and the unit stored on a job.
- **Source Estimate**: One source's independent answer for one job — a salary band plus the confidence that source assigns to it, plus the source's identity. Never persisted on its own; it is the input to blending and the content of the Story 3 breakdown.
- **Blended Estimate**: The single band stored on a job after combining source estimates, carrying the combined confidence and a source label describing what went into it.
- **Salary Cache Entry**: A row of the external reference dataset reduced to one bucket — a title, a geography, and a company-size bucket, with the band observed for that bucket. Populated at startup from the external dataset and queried per job.
- **Salary Floor**: The user's minimum acceptable compensation, expressed in a single currency, against which bands are compared to decide feed visibility. Zero means no floor.
- **Job** *(existing, extended)*: Gains the stored band, its confidence, and its source label. No new table — a band belongs to exactly one posting.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: At least 90% of jobs whose postings state no compensation carry a salary band after inference has run over them.
- **SC-002**: Every stored band carries a non-null minimum, maximum, currency, confidence, and source — 100% of rows, no partial bands.
- **SC-003**: For jobs whose postings *do* state a parseable compensation, the parsed band matches the stated figures for at least 95% of them, measured against a hand-checked sample across every currency present in the ingested data.
- **SC-004**: Holding out jobs with stated compensation and estimating them as if hidden, the estimated band contains the true figure for at least 60% of held-out jobs, and the system's own reported confidence correlates with whether it did.
- **SC-005**: A user can set a floor and see the feed reflect it within one page load, with no re-ingestion and no re-inference.
- **SC-006**: With the default floor filter active, zero jobs whose band lies entirely below the floor appear in the feed; toggling the filter off restores 100% of them.
- **SC-007**: No job's status changes as a result of any salary filtering — verifiable as zero status transitions attributable to this feature.
- **SC-008**: A job's estimate breakdown lets a user identify which source drove the number without reading logs or querying the database.
- **SC-009**: Running inference twice over an unchanged set of jobs issues zero additional model calls on the second run.
- **SC-010**: The system starts and serves the feed normally when the external reference dataset is unreachable — zero startup failures attributable to it.
- **SC-011**: Adding this feature changes nothing about how existing jobs are ingested, scored, or displayed when no floor is set and no band exists — zero regressions in existing feed behavior.

## Assumptions

- **A band per job, not a table**: Compensation describes one posting, so the band lives in columns on the job rather than in a separate related table. This is the user's explicit decision and it rules out storing estimate history.
- **Blending is confidence-weighted averaging**: The user specified weighted averaging with confidences as weights and the summed confidence as the result. Alternative combination schemes (taking the highest-confidence source outright, taking the interval intersection) were considered and are recorded in research.md as rejected.
- **The low-confidence threshold is 0.3**: Below this, a band is shown but visibly discredited. It is a display concern only — a low-confidence band is still stored and still filtered against.
- **The floor is denominated in USD**: The user specified a single USD-denominated floor. Bands in other currencies convert to USD before comparison. Multi-currency floors are out of scope.
- **Filtering is a view concern**: Below-floor jobs are hidden, never hidden *and* mutated. The user explicitly rejected auto-hiding via status change, so the record stays in `found`.
- **The filter defaults to on**: The user chose default-filtered with a toggle, on the grounds that an unfiltered feed is the problem the feature exists to solve.
- **Existing compensation text stays**: `salaryRaw` continues to be stored and displayed. Parsing adds structure beside it; it never overwrites it.
- **The observed-data cache improves over time**: On a fresh install the self-hosted source contributes nothing. This is expected, not a defect — the source is a compounding asset that gets better as the corpus grows.
- **A dashboard settings surface does not yet exist**: The dashboard currently has feed, job-detail, profile, sources, status, tailor, and tracker features and no settings page. The floor's settings entry therefore requires a new surface, not an addition to an existing one. Scoped in plan.md.
- **Geo granularity is coarse**: Buckets are country- or region-level, not city-level. City-level buckets would be too sparse to populate from any of the three sources.
- **Company size is inferred, not known**: Postings rarely state headcount. The size bucket is derived from hints in the posting and the company name, and is the weakest of the three bucket dimensions.

## Dependencies

- The existing self-hosted model runtime and its structured-completion path, which every other inference flow in the system already uses.
- The existing job storage and its migration mechanism, for the new columns and the cache table.
- The existing feed listing and filtering path, which the floor filter extends.
- The existing configuration surface, for the floor value.
- An external public compensation reference dataset remaining publicly retrievable — an external dependency outside the project's control, and one whose terms of use are examined in plan.md's Constitution Check.
- A corpus of already-ingested postings carrying stated compensation, without which the self-hosted source has nothing to learn from.
