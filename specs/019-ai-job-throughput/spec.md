# Feature Specification: AI Job Throughput & Stuck-Run Recovery

**Feature Branch**: `019-ai-job-throughput`

**Created**: 2026-07-28

**Status**: Draft

**Input**: User description: "lets increase speed of ai jobs. when use Ollama Cloude should not be any throttle, you can process 3 request at the same time. also speed up the local ollama match. the speed for now is too slow. i see 700+ jobs which completing very slow. also should be propper handling and max time before cancel (i have a lot of jobs which started before pc shutdowned and after pc start i see them working for 10+ hours without any change)"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Cloud-backed AI work runs in parallel (Priority: P1)

The user has a large backlog of AI work (700+ queued items needing match scoring, salary
inference, ghost-job scoring, or document generation) and has configured a hosted AI
provider for those tasks. Today every AI task is processed strictly one at a time, so the
backlog drains at the speed of a single request even though the hosted provider can serve
several concurrently. The user wants the backlog to drain several times faster whenever
the work is going to a hosted provider that has no single-request bottleneck.

**Why this priority**: Biggest throughput multiplier for the reported problem (a 700-item
backlog), needs no model or prompt changes, and delivers value even if nothing else in
this feature ships.

**Independent Test**: Configure an AI task to use the hosted provider, enqueue a batch of
items of that type, and observe that several are in-flight simultaneously and the batch
finishes in roughly a third of the previous wall-clock time, with no increase in failures.

**Acceptance Scenarios**:

1. **Given** an AI task type routed to a hosted provider and 30 queued items of that type,
   **When** the queue drains, **Then** up to 3 items are processed at the same time and
   total wall-clock time is at most 40% of the single-at-a-time baseline.
2. **Given** the same task type routed to the local model instead, **When** the queue
   drains, **Then** concurrency stays at the level that keeps the local runtime healthy
   and the user is not required to change any other setting.
3. **Given** parallel processing is active and the hosted provider starts rejecting
   requests for exceeding its quota, **When** rejections occur, **Then** the system backs
   off, does not lose the affected items, and reports the condition instead of failing the
   whole backlog.
4. **Given** an operator wants a different parallelism level, **When** they change the
   configured value, **Then** it takes effect on restart without code changes.

---

### User Story 2 - Stuck runs are cancelled instead of hanging forever (Priority: P1)

The user's machine was shut down mid-run. On restart, the activity list still shows runs
in a "running" state, unchanged for 10+ hours. They occupy the user's attention, make the
backlog look active when it is not, and never resolve on their own. The user wants any run
that exceeds a sane maximum duration — or that was orphaned by a restart — closed out
automatically with a clear reason, and the underlying work either retried or released so
it stops blocking the queue.

**Why this priority**: Ghost "running" entries corrupt the user's view of the system and
can hold queue slots; without this, any throughput gain is masked by phantom work.

**Independent Test**: Start a run, kill the process mid-run, restart, and confirm the
orphaned run reaches a terminal state within the recovery window rather than staying
"running" indefinitely.

**Acceptance Scenarios**:

1. **Given** a run marked "running" whose worker died (process killed, machine shut down),
   **When** the system starts back up, **Then** within 5 minutes the run is moved to a
   terminal state with a reason indicating it was interrupted.
2. **Given** a run that is genuinely still executing but has exceeded its maximum allowed
   duration, **When** the limit is reached, **Then** the work is aborted, the run is marked
   timed out with the elapsed time recorded, and downstream steps that only success should
   trigger are not started.
3. **Given** a run that timed out on a transient cause, **When** retries remain for that
   task type, **Then** it is retried under the normal retry policy rather than silently
   dropped.
4. **Given** the user is looking at the activity list, **When** runs have been auto-closed,
   **Then** they can distinguish "timed out", "interrupted", "failed", and "cancelled".

---

### User Story 3 - Local model matching completes noticeably faster (Priority: P2)

Even with hosted work parallelised, the user often runs matching against the local model.
Each match currently takes long enough that a large ingest run takes hours. The user wants
per-item local matching latency materially reduced without degrading match quality.

**Why this priority**: Real pain, but depends on measurement and tuning rather than one
structural change, and the local path is inherently capacity-bound.

**Independent Test**: Run a fixed benchmark set of jobs through local matching before and
after, comparing median per-job latency and resulting scores.

**Acceptance Scenarios**:

1. **Given** a fixed set of 50 jobs scored against the local model, **When** matching runs
   after this change, **Then** median per-job time is at least 30% lower than the recorded
   baseline.
2. **Given** the same fixed set, **When** scores are compared before and after, **Then**
   score drift stays within the agreed tolerance and no job flips across a feature-trigger
   threshold purely because of this change.
3. **Given** several jobs are matched back to back, **When** they run, **Then** fixed work
   that does not vary per job is not redone for every single job.
4. **Given** the local runtime is saturated, **When** more work arrives, **Then**
   throughput degrades gracefully rather than causing timeouts across the board.

---

### User Story 4 - Backlog progress is visible (Priority: P3)

With a 700+ item backlog, the user cannot tell whether the system is working or wedged.
They want to see how many items are pending vs in-flight per AI task type, and a rough
completion estimate.

**Why this priority**: Diagnostic quality-of-life; the throughput and stuck-run fixes are
what actually solve the complaint.

**Independent Test**: Enqueue a large batch, then confirm pending/in-flight counts and an
estimate are visible and update as the backlog drains.

**Acceptance Scenarios**:

1. **Given** a large queued backlog, **When** the user checks progress, **Then** they see
   pending and in-flight counts per AI task type.
2. **Given** the backlog is draining, **When** the user re-checks, **Then** counts and the
   estimate reflect recent throughput.

---

### Edge Cases

- Hosted provider returns quota/rate rejections while 3 requests are in flight → all
  affected items back off and are retried; none are lost.
- A task type's provider setting switches from hosted to local while work is in flight →
  in-flight items complete under the old setting; new items pick up the new concurrency.
- Machine shut down while several runs are in-flight → all of them are recovered on next
  start, not just the first.
- A run legitimately needs longer than a short limit (large document generation) → limits
  are per task type, so slow types are not cut at the same threshold as fast ones.
- Laptop suspend / clock change → elapsed-time detection must not close healthy runs right
  after resume, and must not miss genuinely stale ones.
- The same job enqueued twice for the same task while parallelism is raised → concurrent
  processing must not produce conflicting stored results.
- User cancels an item mid-flight → it terminates promptly and lands in a terminal state,
  same as a timeout.
- Local runtime unreachable → items fail fast with a clear reason instead of hanging until
  the maximum duration.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST process AI work items concurrently, with the concurrency level
  determined per AI task type rather than a single global value.
- **FR-002**: System MUST allow at least 3 AI work items of a given type to be processed
  simultaneously when that type is routed to a hosted AI provider.
- **FR-003**: System MUST NOT apply outbound request pacing intended for scraped job-board
  hosts to AI provider traffic.
- **FR-004**: System MUST keep local-model work at a concurrency level that does not
  overload the local runtime, defaulting to the current conservative level and
  configurable by the operator.
- **FR-005**: Concurrency levels MUST be operator-configurable through existing
  configuration without code changes, with documented defaults.
- **FR-006**: System MUST enforce a maximum execution duration per AI work item, after
  which the item is aborted rather than left running.
- **FR-007**: Maximum execution duration MUST be configurable per task type, with defaults
  appropriate to each type's expected cost.
- **FR-008**: When an item exceeds its maximum duration, the system MUST record a terminal
  state with a timeout reason and the elapsed duration, and MUST NOT trigger downstream
  work reserved for success.
- **FR-009**: On startup, the system MUST detect runs left in a non-terminal state by a
  previous process (crash, shutdown, power loss) and move them to a terminal "interrupted"
  state within 5 minutes of startup.
- **FR-010**: System MUST also detect and close out non-terminal runs that exceed the
  maximum duration while the process keeps running, not only at startup.
- **FR-011**: Terminal states MUST be distinguishable by the user: succeeded, failed,
  timed out, interrupted, cancelled.
- **FR-012**: Aborted or interrupted work MUST either be retried under the task type's
  existing retry policy or released — never left holding a queue slot.
- **FR-013**: System MUST reduce median local-model matching time per job by at least 30%
  relative to a recorded baseline, without materially changing match scores.
- **FR-014**: System MUST avoid repeating per-batch work that does not vary between jobs
  within a matching run.
- **FR-015**: When a hosted provider signals quota exhaustion or rate rejection under
  parallel load, the system MUST back off and retry affected items rather than failing the
  backlog.
- **FR-016**: System MUST expose pending and in-flight counts per AI task type so backlog
  progress is observable.
- **FR-017**: Raising concurrency MUST NOT produce conflicting or duplicated stored results
  when the same job is processed more than once concurrently.
- **FR-018**: All behaviour changes MUST preserve full operation against the local model
  with no hosted provider configured.

### Key Entities

- **AI work item**: One unit of AI processing for one job (match scoring, salary inference,
  ghost scoring, document generation). Has a task type, target job, provider routing, retry
  budget, and maximum duration.
- **Activity run**: User-visible record of an AI work item's execution — state, start time,
  finish time, current step, termination reason. Drives the activity list.
- **Task-type policy**: Per-task-type settings governing speed and safety — concurrency,
  maximum duration, retry budget.
- **Provider routing**: Per-task-type choice of which AI backend serves the work (local
  model vs hosted provider), which determines applicable concurrency.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A backlog of 700 AI work items routed to a hosted provider completes in at
  most 40% of the wall-clock time it takes today.
- **SC-002**: At least 3 hosted-provider AI items are observably in flight simultaneously
  during backlog drain.
- **SC-003**: Median per-job local matching time drops by at least 30% versus the recorded
  baseline on the same 50-job benchmark set, with match scores within the agreed tolerance
  and zero feature-threshold flips.
- **SC-004**: No run remains in a non-terminal state longer than its configured maximum
  duration plus 5 minutes, under any shutdown or crash scenario.
- **SC-005**: After an abrupt machine shutdown, 100% of previously in-flight runs reach a
  terminal state within 5 minutes of the next startup.
- **SC-006**: Backlog drain under the new concurrency shows no higher failure rate than the
  single-at-a-time baseline, within normal variance.
- **SC-007**: For any auto-closed run, the user can tell why it ended (timed out,
  interrupted, failed, cancelled) without inspecting logs.

## Assumptions

- "Ollama Cloud" means the hosted AI provider(s) already configurable for AI tasks; the
  parallelism rule applies to any task routed to a hosted provider, not one named vendor.
- 3 simultaneous requests is the default for hosted-provider work, as stated by the user;
  a configurable default, not a hard cap.
- Local-model concurrency stays at its current conservative default because the local
  runtime is the bottleneck; local speedup comes from reducing per-job work, not from
  raising local parallelism.
- "Max time before cancel" defaults are per task type — minutes for scoring tasks, longer
  for document generation; exact values tuned during planning against observed durations.
- Stuck-run recovery is time-based (elapsed since start / last progress update), not
  dependent on an external process supervisor.
- Existing retry policies per task type are kept as-is; this feature changes when a run
  terminates, not how many times it may be retried.
- Match-quality tolerance for SC-003 is agreed during planning against the benchmark set;
  no change that flips a job across a feature-trigger threshold is acceptable.
- Pacing for scraped job-board hosts is unchanged; this feature only ensures it never
  applies to AI provider traffic.
- The 700+ backlog is normal ingest volume, not duplicate enqueueing; deduplication of
  enqueued work is out of scope.
