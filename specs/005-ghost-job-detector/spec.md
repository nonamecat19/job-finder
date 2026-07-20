# Feature Specification: Ghost-Job Detector

**Feature Branch**: `005-ghost-job-detector`

**Created**: 2026-07-20

**Status**: Draft

**Input**: User description: "Score every job 0-100 on likelihood of being a 'ghost job' — a posting the employer has no real intent to fill."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - See a ghost-job badge on the feed (Priority: P1)

A job seeker scanning the feed sees, next to each job's fit score, a badge indicating how likely that posting is to be a ghost job. Jobs the system is suspicious of are visually marked before the user invests time reading them; jobs with no suspicion carry no badge at all, so the feed stays quiet.

**Why this priority**: The feed is where the user spends time triaging. A badge there is the whole value of the feature in its cheapest form — it changes which job the user clicks next. On its own it is a complete, shippable increment: the detector can compute and persist a score with nothing else built.

**Independent Test**: Score a handful of jobs, open the feed, and confirm that jobs scoring 50-79 show a yellow badge, jobs scoring 80-100 show a red badge, and jobs scoring 0-49 show none. No detail page and no manual refresh button are needed.

**Acceptance Scenarios**:

1. **Given** a job with a stored ghost score of 85, **When** the user opens the feed, **Then** a red ghost badge appears next to that job's fit score, showing the numeric score.
2. **Given** a job with a stored ghost score of 62, **When** the user opens the feed, **Then** a yellow ghost badge appears next to that job's fit score.
3. **Given** a job with a stored ghost score of 20, **When** the user opens the feed, **Then** no ghost badge is rendered for that job.
4. **Given** a job that has never been scored, **When** the user opens the feed, **Then** no ghost badge is rendered and the card is otherwise unchanged from today's layout.
5. **Given** a job with a ghost score of 90, **When** the user opens the feed, **Then** the job is still listed in its normal position — it is neither hidden, dimmed, nor reordered.

---

### User Story 2 - Read why a job was flagged (Priority: P2)

The user opens a flagged job and finds a breakdown panel: the score, each contributing signal with its own value, and a plain-English explanation of why the posting looks (or does not look) like a ghost job. The user can disagree with the verdict on the evidence rather than being asked to trust a number.

**Why this priority**: A score with no reasoning is a number the user cannot act on and will not trust. But the badge (Story 1) is already useful as a triage hint before this exists, so this ships second. Independent of Story 3.

**Independent Test**: Open a scored job's detail page and confirm the panel shows the score, each signal's value, and a readable explanation — with no need for a refresh button to exist.

**Acceptance Scenarios**:

1. **Given** a scored job, **When** the user opens its detail page, **Then** a ghost-job panel shows the numeric score, the confidence, and the model that produced it.
2. **Given** a scored job, **When** the user opens its detail page, **Then** each of the four signals (repost count, days open, cross-board duplicate count, always-hiring count) is shown with its measured value.
3. **Given** a scored job, **When** the user opens its detail page, **Then** a plain-English explanation states which signals drove the score.
4. **Given** a job that has never been scored, **When** the user opens its detail page, **Then** the panel is absent (or shows an explicit "not scored yet" state) rather than rendering an empty or zero-valued panel.
5. **Given** a job scored with low confidence, **When** the user opens its detail page, **Then** the low confidence is visible, so a weak verdict is not read as a strong one.

---

### User Story 3 - Re-score a job on demand (Priority: P3)

Signals age: a posting that looked fine at week one may have been reposted twice by week six. The user presses a button on the job's detail page to recompute the ghost score against current data. Nothing recomputes on a schedule.

**Why this priority**: Convenience and freshness. Scores computed at ingestion are already useful (Stories 1-2); manual refresh is what keeps a long-lived job honest, but the feature works without it.

**Independent Test**: Open a scored job, note the score, press refresh, and confirm the stored score is recomputed and the panel updates in place.

**Acceptance Scenarios**:

1. **Given** a scored job, **When** the user presses the refresh button, **Then** the score is recomputed from current data and the panel shows the new result without a page reload.
2. **Given** a job that has never been scored, **When** the user presses the refresh button, **Then** a score is computed and stored for the first time.
3. **Given** a refresh is in flight, **When** the user presses the button again, **Then** the second press is ignored or disabled rather than starting a duplicate scoring run.
4. **Given** the scoring model is unreachable, **When** the user presses refresh, **Then** an error is shown, the previously stored score is left intact, and no partial result is persisted.
5. **Given** a job scored yesterday, **When** no one presses refresh, **Then** the score is unchanged today — no scheduled or background re-scoring occurs.

---

### Edge Cases

- **Posting has no `postedAt`** → the days-open signal is reported as unknown, not as zero days and not as infinite days. The score is computed from the remaining signals and confidence is reduced.
- **Posting is the only one from that company** → the always-hiring signal is 1 (itself) and MUST NOT count as evidence of ghosting. A single posting from a company the user has seen once is the normal case, not a red flag.
- **A recruiter legitimately cross-posts one JD to several boards** → identical descriptions under different sources is a weak signal on its own. It MUST NOT by itself push a job into the red band; it only compounds with repost count or always-hiring.
- **Company name unparseable** (empty, placeholder `"Unknown"`, or punctuation-only) → the always-hiring signal is skipped rather than grouping every unnamed company together. A placeholder company MUST NOT cause dozens of unrelated jobs to be treated as one employer.
- **Job description is empty or a teaser only** → the cross-board duplicate signal is skipped; a short teaser matches too many unrelated postings to be evidence.
- **The scoring model returns a malformed or out-of-range result** → nothing is persisted and the previous score, if any, survives.
- **A job is deleted** → its stored signal row is removed with it; no orphan rows.
- **Every signal is unknown** (no `postedAt`, one-off company, empty description, first appearance) → the system MUST decline to score rather than emit a confident 0 or a confident 50.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST compute, for a job, a ghost-job score in the range 0-100 where a higher value means a higher likelihood the posting is not intended to be filled.
- **FR-002**: System MUST derive the score from four measured signals: (a) **repost count** — how many times the same posting identity has re-appeared across ingestion runs; (b) **days open** — elapsed time since the posting date; (c) **cross-board duplicates** — the same description text appearing under a different source within the last 60 days; (d) **always-hiring count** — how many postings from the same company in the last 90 days remain unprogressed.
- **FR-003**: System MUST identify "the same posting" for the repost signal using the existing deduplication identity already used at ingestion, so the detector and ingestion never disagree about what counts as the same job.
- **FR-004**: System MUST treat a posting older than 45 days with no user progression as contributing to suspicion, and MUST NOT treat age alone as sufficient for a high score.
- **FR-005**: System MUST scope the cross-board duplicate signal to a rolling 60-day window and the always-hiring signal to a rolling 90-day window, so ancient history does not permanently condemn a company.
- **FR-006**: System MUST count a company's postings as "unprogressed" for the always-hiring signal only when they never advanced past initial discovery — a posting the user shortlisted, generated documents for, applied to, interviewed for, or received an offer on is progression and MUST NOT count toward the signal.
- **FR-007**: System MUST persist, per job, the score, the measured value of every signal, a confidence value, and the identifier of the model that produced the score.
- **FR-008**: System MUST keep the ghost-job result separate from the fit/match score, so a change to one never mutates or invalidates the other.
- **FR-009**: System MUST store at most one ghost result per job, replacing any prior result on re-score rather than accumulating history.
- **FR-010**: System MUST reject and discard a scoring result whose score falls outside 0-100, persisting nothing and leaving any prior result intact.
- **FR-011**: System MUST report a confidence value alongside the score and MUST lower it when a signal could not be measured.
- **FR-012**: System MUST display a ghost badge on each feed card next to the existing fit-score badge: yellow for a score of 50-79, red for 80-100, and no badge below 50.
- **FR-013**: System MUST show, on the job detail page, a breakdown of the score, each signal's measured value, the confidence, the model, and a plain-English explanation.
- **FR-014**: System MUST provide a manual, per-job control to recompute the ghost score, and MUST NOT recompute scores on a schedule.
- **FR-015**: System MUST NOT hide, dim, reorder, auto-reject, or otherwise act on a job because of its ghost score. The score informs the user; the user decides. This is a badge-only feature.
- **FR-016**: System MUST NOT contact the employer, the job board, or any third party as part of scoring. Scoring reads only data the system already holds.
- **FR-017**: System MUST leave a job with no stored ghost result rendering exactly as it does today, on both the feed and the detail page.
- **FR-018**: System MUST tolerate a scoring failure for one job without affecting any other job's score, display, or ingestion.
- **FR-019**: System MUST base the plain-English explanation only on the measured signals, never on invented facts about the employer, the role, or hiring intent.
- **FR-020**: System MUST run scoring against the locally hosted model, with no dependency on a third-party paid inference API.

### Key Entities

- **Job Signal**: A scored judgement attached to a job, of a named kind. For this feature the kind is *ghost*. Carries a 0-100 score, the measured signal breakdown, the producing model, and a creation timestamp. At most one per (job, kind). Deliberately generic so future signal kinds (e.g. salary-realism, seniority-mismatch) reuse the same shape rather than each adding a table.
- **Ghost Signal Breakdown**: The measured evidence behind one ghost score — repost count, days open (or unknown), cross-board duplicate count, always-hiring count, per-signal notes, and the confidence. Every number is a measurement, not a model opinion.
- **Ghost Score Result**: The producing model's structured output — the 0-100 score, the breakdown it was given, the confidence, and the plain-English explanation.

Reused unchanged: **Job** (its deduplication identity, posting date, company, description, source key), **Application** (its status, as the record of user progression), **Match Result** (untouched — the fit score is a separate concern).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user scanning the feed can tell a flagged job from an unflagged one without clicking into it, in under one second per card.
- **SC-002**: For a job the user opens from the feed, the reason for its flag is fully readable on the detail page without leaving that page or consulting logs.
- **SC-003**: 100% of stored ghost results carry a score in 0-100, a confidence, a model identifier, and a value (or an explicit "unknown") for every one of the four signals — no partial rows.
- **SC-004**: A company with exactly one posting in the system is never flagged on the always-hiring signal alone — 0 false flags from that path.
- **SC-005**: A posting with no posting date is still scored, using the remaining signals, and reports reduced confidence rather than failing.
- **SC-006**: Re-scoring the same job twice with unchanged underlying data yields the same signal measurements both times (the measurements are deterministic even though the model's prose is not).
- **SC-007**: Zero jobs are hidden, reordered, or auto-rejected by this feature — the feed's job ordering and membership are byte-identical with the feature on and off.
- **SC-008**: A job with no ghost result renders identically to today on the feed and the detail page — zero visual regressions for unscored jobs.
- **SC-009**: A scoring failure on one job leaves 100% of other jobs' scores and the ingestion pipeline unaffected.
- **SC-010**: Deleting a job leaves zero orphaned signal rows.
- **SC-011**: The manual refresh returns an updated panel within 30 seconds under normal local-model load, or surfaces an error — it never hangs silently.

## Assumptions

- **"Ghost job" is a likelihood, not a verdict**: The system estimates; it never asserts that an employer is acting in bad faith. The UI language and the stored explanation reflect that.
- **Badge only, no auto-hide** *(user-approved)*: The user explicitly chose that this feature never takes an action on a job. Auto-hiding suspect jobs was considered and rejected — a wrongly flagged real job that disappears is a lost opportunity the user never learns about.
- **Thresholds 50/80 are the starting bands** *(user-approved)*: Yellow at 50-79, red at 80-100, nothing below 50. These are product decisions, not derived from a calibration set, and are expected to be tuned once real scores exist.
- **A separate table, not more columns on the fit result** *(user-approved)*: Ghost detection and fit scoring answer different questions and change on different schedules. Keeping them apart means a fit re-score never disturbs a ghost score and vice versa (FR-008).
- **The four signals are proxies, not proof**: Each has an innocent explanation — a slow hiring process, an agency posting to several boards, a genuinely growing company. Hence the LLM blends them rather than a fixed threshold rule firing on any one, and hence confidence is a first-class output.
- **User progression is the ground truth for "always hiring"**: The system cannot see the employer's applicant tracking system. The only evidence it holds about whether anything ever came of a posting is the user's own application status history.
- **Signals are computed from data already held**: No new scraping, no employer contact, no third-party enrichment (FR-016).
- **Scoring happens at ingestion and on demand only**: No scheduled re-scoring in this feature (FR-014). If periodic re-scoring turns out to be wanted, it is a later feature with its own spec.
- **Description similarity is fuzzy, not exact**: Boards reformat and re-wrap the same JD. Exact-string matching would find almost nothing, so a similarity hash is assumed; its exact form is a plan/research decision, not a spec commitment.
- **Single-user, self-hosted scale**: Signal queries run over one user's job corpus (thousands of rows, not millions). Query strategy is chosen for clarity first.

## Dependencies

- The existing job-deduplication identity computed at ingestion — the repost signal is meaningless if it disagrees with what ingestion considers a duplicate.
- The existing application status history — the only available record of whether a posting ever progressed.
- The existing locally hosted model runtime and its structured-output path, already used by fit scoring.
- The existing feed and job-detail screens, which this feature extends rather than replaces.
- Jobs having a posting date populated where the board supplies one — the days-open signal degrades to "unknown" without it.
