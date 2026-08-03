> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/retrieval-and-ingestion.md`](../../domains/retrieval-and-ingestion.md) — read that first.

---
# Feature Specification: Throttle-Only Rate Control

**Feature Branch**: `017-throttle-only-rate-control`

**Created**: 2026-07-28

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "remove conception of daily limits, use only throtling method to avoid ip ban. its should visible properly, like common thing, not an error"

## Overview

The system currently protects job-board hosts from over-fetching with **two** independent
mechanisms: a **per-host daily request budget** (a hard cap that stops fetching once
exhausted for the day) and **per-host request pacing** (a steady, jittered request rate that
spaces requests out over time).

The daily budget is the wrong tool. It does not prevent an IP ban — a burst of requests
inside the budget is exactly what triggers a ban, and a slow trickle beyond the budget
triggers nothing. What it does instead is stop collection partway through, and report that
stop to the user as a failure indistinguishable from a real block. Users see "budget
exhausted" alongside genuine "refused"/"challenged" outcomes and cannot tell that nothing
is actually wrong.

This feature removes the daily-budget concept entirely and makes request pacing the single
rate-control mechanism. Pacing becomes a visible, ordinary operational fact — presented the
way "current rung" is presented, as neutral status — never as an error, warning, or block.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Collection is never cut short by an artificial daily cap (Priority: P1)

A user has several job sources enabled against the same host. Earlier in the day, scheduled
runs already fetched a large number of pages from that host. The user now triggers a new
search, or a scheduled run fires.

Today, once the host's daily request count is reached, every further fetch is refused
locally, the run returns partial or no results, and the user is told the source was blocked.
The user must wait until the next day, with no way to tell the difference between "the board
turned us away" and "we turned ourselves away."

After this change, the run proceeds. Requests are still spaced out at the safe per-host rate,
so the run may take longer, but it completes and returns the jobs it found.

**Why this priority**: This is the core of the request. It is the only story that changes
what results the user actually gets, and it is independently valuable even if no UI text
changes at all.

**Independent Test**: Drive a host past what used to be its daily cap in a single session and
confirm the fetches keep succeeding, results keep arriving, and no outcome is reported as
deferred/blocked on account of volume.

**Acceptance Scenarios**:

1. **Given** a host that has already served more requests today than the former daily cap
   allowed, **When** a new ingestion run targets that host, **Then** the run fetches normally
   and returns results.
2. **Given** a long-running search that requires many pages from one host, **When** the run
   executes, **Then** every page is attempted, spaced by the per-host pacing rate, and none
   is skipped for volume reasons.
3. **Given** a host that genuinely refuses or challenges a request, **When** that happens,
   **Then** the run still reports a block for that host — real blocks are unaffected by this
   change.
4. **Given** a host that has entered cooling-off after repeated real blocks, **When** a run
   targets it, **Then** cooling-off still applies — it is a response to observed blocking,
   not a volume cap.

---

### User Story 2 - Pacing reads as normal operation, not as a problem (Priority: P2)

A user opens the host retrieval status view to understand why a run took a while. They want
to see, in plain terms, that the system is deliberately fetching slowly to stay under the
host's tolerance — and that this is the intended, healthy steady state.

Today the same panel shows a "Budget: 47/200" counter next to a reset timestamp, which reads
as a quota running down toward a failure, and the exhaustion outcome is rendered in the same
visual language as genuine blocks.

After this change, the panel shows the pacing in effect for that host as ordinary
informational status, styled like the neutral facts beside it — never in the warning or
danger treatment reserved for real blocks and cooling-off.

**Why this priority**: The user explicitly asked for correct presentation. It depends on
Story 1 having removed the budget, but it is separately testable and separately valuable —
without it, the feature is done but invisible.

**Independent Test**: Open the host retrieval status for a healthy host and confirm the
pacing information is present, understandable without prior knowledge, and carries no error,
warning, or danger styling; then confirm a genuinely blocked host still renders its warning
treatment.

**Acceptance Scenarios**:

1. **Given** a healthy host, **When** the user views its retrieval status, **Then** the
   pacing currently applied to that host is shown as neutral informational text.
2. **Given** a healthy host, **When** the user views its retrieval status, **Then** no quota,
   budget, allowance, or reset-time information appears anywhere in the view.
3. **Given** a host with a recorded block or an active cooling-off period, **When** the user
   views its retrieval status, **Then** those are still visually distinguished from ordinary
   pacing status.
4. **Given** a run that took longer because of pacing, **When** the user reviews that run's
   outcome, **Then** the run is reported as successful, not partial or blocked.

---

### User Story 3 - No leftover daily-limit surfaces anywhere (Priority: P3)

An operator configuring the system, or a user reading a run's history, should find no trace
of the removed concept: no daily-budget setting to tune, no stored per-host counters, no
reason strings mentioning exhausted budgets, and no documentation describing a daily cap.

**Why this priority**: Cleanup. The feature works without it, but a stale knob or a stale
paragraph will send the next person looking for a mechanism that no longer exists.

**Independent Test**: Search configuration, stored host state, user-visible reason text, and
project documentation for the daily-limit concept and confirm no live references remain.

**Acceptance Scenarios**:

1. **Given** the system's configuration surface, **When** an operator reviews available
   settings, **Then** no per-host daily request allowance setting is offered.
2. **Given** stored per-host retrieval state, **When** it is inspected, **Then** it holds no
   daily counter, allowance, or period-start values.
3. **Given** any run outcome shown to a user, **When** its reason text is read, **Then** it
   never cites an exhausted daily allowance.
4. **Given** project documentation, **When** rate control is described, **Then** it describes
   pacing (and block-triggered cooling-off) as the only mechanisms.

---

### Edge Cases

- **Host state records that already carry daily-counter values.** Existing stored state must
  remain usable after the change; the obsolete values are discarded rather than migrated, and
  no host is left in an unfetchable state because of them.
- **A host publishes its own crawl delay.** The system already records a per-host crawl delay
  where one is advertised. Pacing must continue to honour it, and the displayed pacing must
  reflect the delay actually in force for that host, not just the global default.
- **Many sources share one host.** Pacing is enforced per destination host, so two sources on
  the same board share one rate and cannot together exceed it. Removing the daily cap must not
  change this.
- **A run legitimately needs far more pages than before.** With no cap, a pathological search
  could run for a very long time. Runs remain bounded by their existing execution time limits;
  a run that hits its time limit reports what it collected, and this is a run-duration outcome,
  not a rate-control one.
- **A host starts refusing mid-run.** Real refusals still escalate through the existing
  retrieval fallbacks and still count toward cooling-off; nothing about that path changes.
- **Concurrent runs against one host.** Two runs starting at once share the same pacing and
  are serialised by it; neither fails for volume reasons.

## Requirements *(mandatory)*

### Functional Requirements

**Removing the daily limit**

- **FR-001**: The system MUST NOT impose any fixed maximum number of requests per host per
  day, or per any other fixed calendar period.
- **FR-002**: The system MUST NOT refuse, defer, or skip a fetch on the grounds of how many
  requests that host has already served within a time period.
- **FR-003**: The system MUST stop tracking per-host cumulative request counts and their
  reset periods for the purpose of limiting requests.
- **FR-004**: The system MUST remove the operator-facing setting that configured the default
  per-host daily request allowance.
- **FR-005**: Existing stored per-host retrieval state MUST remain valid and usable after the
  removal, with no manual operator intervention required and no host rendered unfetchable.

**Pacing as the sole rate control**

- **FR-006**: The system MUST continue to space outbound requests per destination host at a
  deliberately conservative steady rate, with randomised variation so the timing does not
  form a recognisable machine pattern.
- **FR-007**: Pacing MUST apply to every outbound request to a third-party host, regardless
  of which source or which stage of processing issued it.
- **FR-008**: Pacing MUST be enforced per destination host, so that multiple sources sharing
  a host share a single rate rather than each getting their own.
- **FR-009**: Where a host advertises its own preferred crawl delay, the system MUST respect
  it in preference to the default rate when it is the more conservative of the two.
- **FR-010**: When a request must wait for its pacing turn, the system MUST wait and then
  proceed, rather than failing the request.
- **FR-011**: The system MUST retain block-triggered cooling-off, which pauses a host after
  repeated genuine blocks. Cooling-off is a reaction to observed refusal, not a volume cap,
  and is out of scope for removal.

**Presentation**

- **FR-012**: The host retrieval status view MUST show the request pacing currently in force
  for the selected host.
- **FR-013**: Pacing status MUST be presented as ordinary informational status, using the
  same neutral visual treatment as other routine host facts, and MUST NOT use error, warning,
  or danger styling.
- **FR-014**: The host retrieval status view MUST NOT display any quota, budget, allowance,
  usage counter, or allowance-reset time.
- **FR-015**: Genuine blocks and active cooling-off MUST remain visually distinct from
  ordinary pacing status, so the user can tell a real problem from normal operation.
- **FR-016**: Pacing information MUST be expressed in terms a non-technical user can act on
  — how fast the system is fetching from this host, and that this is intentional — rather
  than as a raw internal figure alone.
- **FR-017**: No user-visible outcome, reason string, or log intended for users may describe
  pacing or an exhausted allowance as a failure, block, refusal, or deferral.
- **FR-018**: A run whose only notable event was waiting for pacing MUST be reported as a
  successful run.

**Consistency**

- **FR-019**: All project documentation describing how the system avoids IP bans MUST
  describe pacing and block-triggered cooling-off as the mechanisms, with no reference to a
  daily cap.
- **FR-020**: Automated tests asserting daily-limit behaviour MUST be removed or replaced
  with tests asserting the pacing behaviour that supersedes it.

### Key Entities

- **Host retrieval state**: What the system remembers about one job-board host — which
  retrieval approach currently works, the crawl delay it advertises, its recent block history,
  and whether it is cooling off. After this change it no longer holds request counters or
  allowance periods.
- **Host pacing status**: The rate currently applied to a host, and where that rate came from
  (system default, host-advertised delay, or per-host override). This is what the user sees in
  the status view.
- **Run outcome**: The user-facing verdict for one ingestion run. After this change, "blocked"
  and "partial" verdicts reflect only genuine host behaviour or run-duration limits, never the
  system's own rate control.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A host can serve an unlimited number of requests across a day without the
  system refusing any of them for volume reasons — verified by exceeding the former cap in a
  single session with zero volume-related refusals.
- **SC-002**: Zero runs report a blocked, partial, or deferred outcome attributable to the
  system's own rate control; every such report traces to genuine host behaviour or a run
  time limit.
- **SC-003**: A user viewing a healthy host's status can state, in their own words, how fast
  the system is fetching from it and that this is intentional — without consulting
  documentation or support.
- **SC-004**: No quota, budget, allowance, or reset-time information appears in any
  user-facing view, configuration surface, or documentation page.
- **SC-005**: The observed request rate to any single third-party host stays at or below the
  configured conservative pace, measured across a run that issues many requests, including
  when several sources target that host concurrently.
- **SC-006**: The rate of genuine blocks and cooling-off activations per host does not
  increase after the daily cap is removed, confirming that pacing alone is sufficient
  protection.

## Assumptions

- **Cooling-off stays.** Block-triggered cooling-off pauses a host only after it has
  demonstrably refused requests. It is a reaction to evidence, not a preemptive volume cap, and
  removing it would work against the stated goal of avoiding IP bans. Only the daily budget is
  removed.
- **The existing pacing rate is already adequate.** The current conservative per-host rate,
  its randomised variation, and its small opening burst are assumed sufficient to avoid bans
  on their own. This feature does not retune those values; if boards prove otherwise, tuning is
  a separate change.
- **The retrieval fallback ladder is unchanged.** How the system escalates between retrieval
  approaches when a host challenges or refuses a request is out of scope.
- **Obsolete stored counters are discarded, not migrated.** There is no user value in
  preserving historical daily-usage numbers, so they are dropped rather than archived.
- **Pacing is not made user-configurable.** The status view displays the rate in force; it does
  not offer the user a control to change it. Per-host rate overrides remain an operator-level
  concern.
- **Local and loopback destinations stay exempt.** Pacing is owed to third-party hosts; the
  system's own supporting services are unaffected.
- **"Visible properly, like a common thing" is read as neutral informational status** shown
  alongside the other routine host facts — not as a new prominent surface, dashboard widget, or
  notification.

## Out of Scope

- Retuning the per-host request rate, burst size, or jitter range.
- Adding user-facing controls to change pacing.
- Changes to the retrieval fallback ladder or challenge detection.
- Changes to how genuine blocks are detected, recorded, or escalated.
- Any rate limiting that applies to language-model providers rather than job-board hosts;
  those are separate systems with their own provider-imposed quotas and are unaffected.
- Global or cross-host concurrency limits.
