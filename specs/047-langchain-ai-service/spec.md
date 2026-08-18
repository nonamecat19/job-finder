# Feature Specification: Dedicated AI Orchestration Service

**Feature Branch**: `047-langchain-ai-service`

**Created**: 2026-08-18

**Status**: Draft

**Input**: User description: "lets migrate ai functionality to langchain, langgraph and langfuse. if needed you can create python microservice for that"

## Overview

Today every AI capability in the platform — job matching, ghost-job scoring, salary
inference, recruiter extraction, outreach drafting, keyword rephrasing, the multi-stage
resume/cover-letter generation pipeline, and embeddings — is orchestrated inside the Go
backend. Each capability re-implements its own prompt assembly, JSON parsing, retry
handling, multi-step sequencing and (for salary) tool-calling loop. Observability exists
only at the routing layer: the inference proxy records one record per model call, with no
notion of which business step a call belonged to, what the step before it produced, or why
a multi-step run ended the way it did.

The consequence is operational, and it is felt in two places. An AI run that produces a bad
result cannot be replayed or inspected step by step, so diagnosing it is guesswork. And
changing how a capability reasons — adding a step, reordering stages, changing a prompt —
is a backend code change with a rebuild and redeploy behind it, which makes prompt and
workflow iteration slow enough that it rarely happens.

This feature moves AI **orchestration** (prompt assembly, step sequencing, tool loops,
output validation, retries, tracing) out of the backend and into a dedicated AI
orchestration service, and makes every AI run a first-class, inspectable trace. Model
selection and provider failover remain where they are today — in the reviewed routing
configuration — so this is a change of where reasoning is assembled, not of which models
serve it.

It also replaces the platform's asynchronous backbone. Work is dispatched today through a
Redis-backed job queue whose consumers live only in the backend; a second service cannot
participate in it, and the queue is a task list rather than a record of what happened. This
feature migrates **every** queue — AI and non-AI alike — onto a message broker and makes the
platform event-driven: services publish events describing what occurred, and interested
consumers react. The orchestration service becomes a peer consumer rather than a callee, so
no backend worker is held open for the minutes a multi-step AI run can take.

The broker migration is stated here as one feature because the orchestration service depends
on it, but it is worth naming what that couples: it touches every queue and every worker,
including ingestion, enrichment and notification work that has nothing to do with AI, and it
replaces the retry, backoff and stuck-run recovery machinery those non-AI paths rely on.

## Clarifications

### Session 2026-08-18

- Q: How much of the AI surface migrates to the orchestration service? → A: All of it — every one of the 14 task keys, embeddings included, leaving no LLM call path in the backend.
- Q: Who owns provider selection and failover after the migration? → A: The existing routing layer, unchanged. The orchestration service is a client of it, requests by task key only, and holds no provider credentials.
- Q: Where do prompts and workflow definitions live? → A: In-repository, versioned with the orchestration service and changed by commit plus a restart of that service. Not in a remote prompt registry, not in the database.
- Q: How long are trace payloads containing resume and profile content retained? → A: 30 days, after which inputs and outputs are purged; structural metadata, timings, token counts, cost and outcome are retained indefinitely.
- Q: How does the backend invoke a capability? → A: It does not invoke it directly. All queues migrate off the current Redis-backed queue onto a message broker, and the platform becomes event-driven: the backend publishes work events, the orchestration service consumes AI work events and publishes result events, and the backend consumes those and persists. No synchronous call, no worker held open for the duration of a run.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An operator diagnoses a bad AI result end to end (Priority: P1)

An operator sees a resume generation that produced a weak result, or a job scored
implausibly high. They open the observability UI, find the run by user, job, or capability,
and see the whole run as one trace: every step in order, the exact inputs and outputs of
each step, how long each took, which model tier served it, what it cost, and — when a step
failed or was retried — the error and the retry. From that trace they can tell whether the
fault was the prompt, an intermediate step's output, or the model.

**Why this priority**: This is the capability that does not exist today at any level and
that every subsequent improvement depends on. It is also independently valuable: it can
ship for one capability and immediately pay for itself in diagnosis time, with no other
part of the migration done.

**Independent Test**: Run one migrated capability end to end, then locate its trace in the
observability UI and confirm every step, its input, its output, its duration, its token
usage and its cost are visible, and that the trace carries the identifiers needed to tie it
back to the user and the job it ran for.

**Acceptance Scenarios**:

1. **Given** a migrated AI capability, **When** it completes successfully, **Then** exactly
   one trace exists for the run, containing one span per orchestration step in execution
   order, each with its input, output, duration, model tier, token counts and cost.
2. **Given** a migrated AI capability, **When** a step fails and the run fails after
   retries, **Then** a trace still exists, is marked failed, and shows the failing step, the
   error, and each retry attempt.
3. **Given** a completed run, **When** an operator searches by the associated user, job, or
   capability name, **Then** the run's trace is findable by those identifiers.
4. **Given** the observability collector is unavailable, **When** AI work runs, **Then**
   every run completes normally and no user-facing request is slowed or failed by the
   collector's absence.

---

### User Story 2 - A maintainer changes an AI workflow without a backend release (Priority: P2)

A maintainer wants to change how a capability reasons — reword a prompt, add a validation
step, reorder the stages of resume generation, or change how many tool-calling rounds are
allowed. They change it in the orchestration service's definition of that workflow and
restart that one service. The backend is untouched, its contract with the orchestration
service is unchanged, and no other capability is affected.

**Why this priority**: This is the productivity payoff of the migration, but it is only
realizable once at least one capability has moved, so it follows P1. It is independently
testable and demonstrable on a single migrated capability.

**Independent Test**: Change one prompt and add one step to a migrated capability, restart
only the orchestration service, and confirm the new behaviour takes effect, appears in
subsequent traces, and required no backend rebuild.

**Acceptance Scenarios**:

1. **Given** a migrated capability, **When** its prompt or step sequence changes and only
   the orchestration service restarts, **Then** subsequent runs use the new definition and
   the backend is unmodified.
2. **Given** a workflow change, **When** runs execute after it, **Then** their traces record
   which version of the workflow and prompt served them, so before/after runs are
   distinguishable.
3. **Given** an invalid workflow definition, **When** the orchestration service starts,
   **Then** it fails to start with a message naming the invalid definition, rather than
   starting and failing runs at request time.

---

### User Story 3 - Multi-step and tool-using capabilities keep their guarantees (Priority: P2)

The capabilities with real internal structure — the multi-stage resume/cover-letter
pipeline, and salary inference with its bounded tool-calling loop — run under the
orchestration service with the same guarantees they have today: bounded steps, bounded tool
rounds, untrusted tool output never treated as instructions, and a structured, schema-valid
result or an explicit failure.

**Why this priority**: These are the capabilities the migration exists for; single-shot
capabilities gain far less. But they are also the riskiest to move, so they follow the
capabilities that establish the pattern.

**Independent Test**: Run the multi-stage generation pipeline and salary inference through
the orchestration service against a fixed set of inputs and confirm results remain
schema-valid and within the documented quality baseline, that step and tool-round bounds
still hold, and that each intermediate step is visible as its own span.

**Acceptance Scenarios**:

1. **Given** a multi-step capability, **When** it runs, **Then** each stage is a separate
   span with its own input and output, and a failure in one stage fails the run with that
   stage identified.
2. **Given** a tool-using capability, **When** the model requests tool calls, **Then** the
   number of rounds stays within the configured bound, each tool call and result is its own
   span, and exceeding the bound ends the run with an explicit bound-exceeded failure rather
   than an unbounded loop.
3. **Given** tool output containing instruction-like text, **When** it is fed back to the
   model, **Then** it is handled as untrusted data and does not alter the run's
   instructions.

---

### User Story 4 - The migration is reversible per capability (Priority: P3)

Each capability moves independently. Until a capability is cut over, it runs the way it does
today. After cutover, an operator can return that one capability to its previous path
without redeploying anything else and without touching the capabilities already migrated.

**Why this priority**: Risk control rather than user-visible value. It is what makes the
other stories safe to ship incrementally, but it delivers nothing on its own.

**Independent Test**: Cut one capability over, verify it runs through the orchestration
service, revert it via configuration, and verify it runs the previous path again — with the
other capabilities unaffected in both directions.

**Acceptance Scenarios**:

1. **Given** a partially migrated system, **When** some capabilities are cut over and others
   are not, **Then** both sets function normally and each is traceable to the path it ran.
2. **Given** a migrated capability, **When** it is reverted to the previous path by
   configuration, **Then** it produces results as before, with no code change and no effect
   on other capabilities.
3. **Given** the orchestration service is unreachable, **When** a migrated capability's work
   is dispatched, **Then** the work fails and retries under the existing retry policy rather
   than hanging, silently succeeding, or returning a substituted result.

---

### User Story 5 - Work is dispatched as events that survive restarts (Priority: P1)

Every unit of asynchronous work in the platform — ingestion, matching, enrichment,
generation, salary inference, ghost scoring, notification — is published as an event to the
broker rather than pushed onto a backend-local job queue. Any service in the stack can
consume the events it cares about, so the orchestration service picks up AI work as a peer.
Work published while a consumer is down waits in the broker and is processed when it
returns; work that fails repeatedly lands somewhere an operator can find it rather than
disappearing or retrying forever.

**Why this priority**: The orchestration service cannot consume work at all until this
exists, so it precedes every other story in sequence. It is also independently valuable and
independently testable on the current backend alone, before any AI capability moves.

**Independent Test**: With no AI capability yet migrated, run all existing work types
through the broker, restart consumers mid-flight, and confirm no unit of work is lost, none
is processed twice in a way that duplicates stored data, and repeatedly failing work reaches
the dead-letter destination.

**Acceptance Scenarios**:

1. **Given** work is published while its consumer is stopped, **When** the consumer starts,
   **Then** the work is processed, with no loss and no manual intervention.
2. **Given** a consumer crashes mid-processing, **When** it restarts, **Then** the unit of
   work is redelivered and processed to completion, and the stored result is the same as if
   it had never crashed.
3. **Given** a unit of work fails past its retry budget, **When** the budget is exhausted,
   **Then** it is routed to a dead-letter destination with its failure reason retained, and
   an operator can list and inspect what is there.
4. **Given** the same unit of work is delivered more than once, **When** it is processed
   again, **Then** stored results are not duplicated or corrupted.
5. **Given** the broker is unreachable, **When** the backend attempts to publish, **Then**
   publication fails visibly and the originating request reports an error, rather than
   silently dropping the work.

---

### Edge Cases

- **Orchestration service down or unreachable**: queued AI work fails and retries under the
  existing per-queue policy; user-facing requests that depend on it report a clear failure.
  No fabricated or substituted result is ever returned.
- **Observability collector down, slow, or full**: runs complete unaffected. Tracing is
  best-effort and never on the critical path; records may be lost, and losing them is
  preferable to delaying or failing a run.
- **Model returns unparseable or schema-invalid output**: the run retries within its bound
  and then fails explicitly; a partially parsed result is never persisted.
- **A single step hangs**: per-step and whole-run time bounds end the run with a timeout
  failure that is visible in the trace, rather than occupying a worker indefinitely.
- **Duplicate dispatch of the same unit of work**: re-running produces a new trace and does
  not corrupt or duplicate stored results.
- **Broker unreachable when publishing**: the publish fails loudly; a user-facing request
  that depends on it reports an error, and the work is never silently dropped.
- **Broker unreachable when consuming**: consumers reconnect without operator action, and
  events published in the meantime are processed on reconnection.
- **Event redelivered after a consumer crash**: processing is idempotent per unit of work —
  a redelivered event produces the same stored state, not a second row or a second charge.
- **Result event arrives for work the backend no longer has** (the job was deleted, the user
  removed their profile): the result is discarded without error, and the discard is
  observable.
- **Poison event that fails every attempt**: it is dead-lettered with its failure reason
  after its retry budget, never retried indefinitely and never blocking the events behind it.
- **Events arriving out of order** (a result event for a superseded run): the backend accepts
  only the result matching the current run for that unit of work and discards stale results.
- **In-flight work at the moment of broker cutover**: no unit of work is lost or processed
  twice while both the old queue and the broker are briefly live.
- **Very large inputs** (long resumes, long job descriptions): the run either completes or
  fails with an explicit too-large failure; it never truncates silently in a way the trace
  does not record.
- **Traces containing personal data**: profile and resume content appearing in trace payloads
  is operator-only and purged after 30 days, while the run's metrics survive. A trace older
  than the limit is still findable and still shows what happened, timings and cost — its step
  inputs and outputs are simply gone.
- **Partial migration during deploy**: a capability whose two paths are briefly both live
  must not process the same unit of work twice.

## Requirements *(mandatory)*

### Functional Requirements

#### Orchestration service

- **FR-001**: The system MUST provide a dedicated AI orchestration service that owns prompt
  assembly, step sequencing, tool-calling loops, output validation and retry behaviour for
  every migrated AI capability.
- **FR-002**: The backend MUST invoke AI capabilities by capability name and typed input
  only, carrying no prompt text, no step sequence, and no model or provider identity. This MUST
  be enforced by an automated check, not by convention — a leaked prompt string in a payload
  still produces a working system, so nothing else would catch it.
- **FR-003**: Every capability MUST return a typed, schema-validated result or an explicit,
  classified failure. Unstructured or partially parsed output MUST NOT be returned as
  success.
- **FR-004**: Failures MUST be classified into the categories the backend already
  distinguishes — rate-limited, credential rejected, insufficient credits, model
  unavailable, provider unavailable, invalid input, bound exceeded, timeout — and each
  category MUST carry whether it is retryable, so retry and skip behaviour is preserved
  through the new messaging layer rather than re-decided in it.
- **FR-005**: The orchestration service MUST enforce bounds on every run: maximum steps,
  maximum tool-calling rounds, per-step timeout and whole-run timeout. Exceeding any bound
  MUST end the run with an explicit bound-exceeded failure.
- **FR-006**: Tool results and any other content fetched during a run MUST be treated as
  untrusted data and MUST NOT be able to change the run's instructions.
- **FR-007**: The orchestration service MUST validate its workflow and prompt definitions at
  startup and refuse to start when one is invalid, naming the offending definition.
- **FR-008**: The orchestration service MUST hold no direct database credentials for the
  platform's primary data store; all business data it needs MUST arrive in the request from
  the backend.

#### Model access

- **FR-009**: All model and embedding calls made by the orchestration service MUST go
  through the existing routing layer, by task-key name only. The single-inference-path
  invariant and the reviewed routing configuration remain the sole place provider and model
  selection lives.
- **FR-010**: The set of task keys, their failover chains and their provider-diversity
  guarantees MUST be unchanged by this feature. Chain ordering, retry-on-tier, cooldown and
  provider-diversity logic MUST NOT be re-implemented in the orchestration service; it
  observes a task key's chain as a single call whose failure is already chain-exhausted.
- **FR-010a**: The orchestration service MUST NOT bypass the routing layer for any model or
  embedding call under any condition, including routing-layer unavailability. There is no
  direct-to-provider path and no local fallback tier.
- **FR-011**: The orchestration service MUST hold no third-party model-provider credentials;
  its only inference credential is the one used to reach the routing layer.

#### Observability

- **FR-012**: Every AI run MUST produce exactly one trace containing one span per
  orchestration step, in execution order, each recording its input, output, duration, model
  tier, token counts and cost.
- **FR-013**: Traces MUST be produced for failed runs as well as successful ones, and MUST
  show the failing step, its error, and each retry attempt.
- **FR-014**: Every trace MUST carry the identifiers needed to find it operationally:
  capability name, the user it ran for, the entity it ran against (job, application, or
  resume) and the queued task it came from.
- **FR-015**: Traces MUST record the version of the workflow and prompt definitions that
  served the run, resolved to the committed revision of the orchestration service, so any run
  can be tied to the exact definition text that produced it.
- **FR-015a**: Prompt text and workflow definitions MUST live in the repository alongside the
  orchestration service and MUST be changed by commit. The service MUST NOT fetch prompt or
  workflow definitions from a remote registry or the database at runtime, so no external
  service can alter run behaviour and no external outage can affect a run.
- **FR-016**: Trace emission MUST be best-effort and off the critical path: a collector that
  is unavailable, slow, or rejecting writes MUST NOT delay or fail any run.
- **FR-017**: Existing routing-layer records MUST remain in place, and a run's trace MUST be
  correlatable with the routing-layer records of the model calls it made.
- **FR-018**: Trace payloads — the recorded inputs and outputs of each step, which contain
  profile and resume content — MUST be purged 30 days after the run. Structural metadata
  (step sequence, timings, model tier, token counts, cost, outcome, identifiers) MUST be
  retained beyond that, so cost and quality trends survive the purge.
- **FR-018a**: Purging MUST be automatic and enforced by the platform, not by operator
  discipline, and MUST be verifiable: an operator can confirm that no payload older than the
  limit remains.
- **FR-018b**: Trace access MUST be restricted to operators; traces MUST NOT be reachable by
  end users or from outside the stack.

#### Messaging and event flow

- **FR-026**: All asynchronous work in the platform MUST be dispatched through a message
  broker. Every existing queue MUST migrate — the AI queues and the non-AI ones (ingestion,
  enrichment, notification, scheduling) alike — and the previous Redis-backed job queue MUST
  be removed once migration completes, leaving exactly one asynchronous mechanism.
- **FR-027**: Services MUST communicate by publishing and consuming events. The backend MUST
  publish a work event per unit of work; the orchestration service MUST consume AI work
  events and publish a corresponding result event; the backend MUST consume result events and
  perform all persistence. The backend MUST NOT call the orchestration service synchronously
  for queued work, and MUST NOT hold a worker open for the duration of a run.
- **FR-028**: Every event MUST carry a stable identifier for the unit of work it concerns, a
  correlation identifier that ties a work event to its result event, an event type, and a
  schema version.
- **FR-029**: Event payloads MUST be typed contracts shared across services, versioned so
  that a consumer can reject an event it does not understand rather than misinterpreting it.
- **FR-030**: Delivery MUST be at-least-once, and every consumer MUST be idempotent per unit
  of work: a redelivered event MUST produce the same stored state, never duplicated rows,
  duplicated notifications, or repeated AI spend beyond one retry budget.
- **FR-031**: Each work type MUST have its own retry budget with backoff. Work that exhausts
  its budget MUST be routed to a dead-letter destination that retains the failure reason, and
  MUST NOT be retried indefinitely or block subsequent work.
- **FR-032**: An operator MUST be able to list what is in the dead-letter destination,
  inspect why each item failed, and re-dispatch an item once the cause is fixed.
- **FR-033**: Events MUST survive a broker restart: work accepted from a user and published
  MUST NOT be lost because the broker or a consumer restarted.
- **FR-034**: A publish that cannot reach the broker MUST fail visibly to its caller. Work
  MUST NOT be accepted-and-dropped, and a user-facing request whose work cannot be published
  MUST report an error.
- **FR-035**: Consumers MUST reconnect to the broker without operator intervention, and MUST
  resume processing events published while they were disconnected.
- **FR-036**: Stuck-run detection and recovery MUST be preserved: work that begins but never
  produces a result within its time bound MUST be detected and re-dispatched or failed, not
  left indefinitely in progress.
- **FR-037**: A result event whose unit of work no longer exists, or which is superseded by a
  newer run for the same unit of work, MUST be discarded without error and the discard MUST be
  observable.
- **FR-038**: The broker MUST be a stateful service in the self-hosted stack, MUST require
  authentication, and MUST NOT be reachable from outside it.

#### Migration and cutover

- **FR-019**: Capabilities MUST migrate independently; migrating one MUST NOT require
  migrating another.
- **FR-019a**: Every AI capability MUST migrate — all fourteen task keys (`match`, `ghost`,
  `rephrase`, `recruiter`, `salary`, `outreach`, `generation`, `generation-analyze`,
  `generation-select`, `generation-select-premium`, `generation-summary`,
  `generation-summary-premium`, `generation-summary-fast`, `embed`) including embeddings.
  When the migration completes, the backend MUST retain no code path that calls a model or
  embedding endpoint directly, and the routing layer MUST accept requests from the
  orchestration service only.
- **FR-020**: Each migrated capability MUST be switchable between the orchestration service
  and its previous path by configuration alone, without a code change and without affecting
  other capabilities.
- **FR-021**: For each migrated capability, results MUST be validated against a recorded
  pre-migration baseline on a fixed input set before cutover, and MUST stay within the
  documented tolerance for that capability.
- **FR-022**: The user-facing behaviour and stored data shapes of every capability MUST be
  unchanged by migration: same fields, same value ranges, same dashboard rendering. This MUST
  be verified per capability at its cutover — field sets and enum ranges compared, not assumed
  — not once for the migration as a whole.
- **FR-023**: When a capability is cut over, its previous orchestration code path MUST be
  removed once the cutover is confirmed, so exactly one implementation of each capability
  remains live at rest.
- **FR-024**: The full stack, including the orchestration service, MUST come up through the
  existing single-command local and production stack definitions, and the orchestration
  service MUST NOT be a startup dependency of any non-AI capability.
- **FR-025**: The orchestration service MUST have its own automated test suite, wired into
  the repository's merge gate alongside the existing per-language suites.

### Key Entities

- **AI Capability**: A named unit of AI work the backend can request (job matching,
  ghost-job scoring, salary inference, recruiter extraction, outreach drafting, keyword
  rephrasing, each resume/cover-letter generation stage, embeddings). Has a typed input, a
  typed output, a workflow definition, and bounds.
- **Workflow Definition**: The ordered steps, prompts, tools and validation rules that
  implement one capability. Versioned, so a trace can name the version that served it.
- **Run**: One execution of a capability for one unit of work. Has a status, a duration, a
  cost, and an ordered set of steps.
- **Step / Span**: One unit inside a run — a model call, a tool call, a validation, a
  transformation — with its own input, output, duration and outcome.
- **Trace**: The observability record of a run: the run plus all its steps plus the
  identifiers that make it findable.
- **Baseline Record**: The recorded pre-migration outputs of a capability over a fixed input
  set, used as the comparison point for cutover.
- **Work Event**: A published request for asynchronous work. Carries the work type, the
  identifier of the unit of work, a correlation identifier, a schema version, and the typed
  input the consumer needs.
- **Result Event**: The published outcome of a unit of work — success with a typed result, or
  a classified failure — carrying the correlation identifier of the work event it answers.
- **Dead-Letter Item**: A unit of work that exhausted its retry budget, retained with its
  failure reason for operator inspection and re-dispatch.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For 100% of migrated AI runs, an operator can reconstruct the full step
  sequence with per-step inputs, outputs, timings and cost from the observability UI alone.
- **SC-002**: An operator can locate the trace for a specific reported bad result in under 2
  minutes, starting from the user and the job it concerned.
- **SC-003**: Changing a prompt or adding a step to a migrated capability takes under 5
  minutes from edit to first run using the change, and requires restarting only the
  orchestration service.
- **SC-004**: Every migrated capability's results stay within its recorded pre-migration
  baseline tolerance on the fixed input set — for scored capabilities, no more than a 5%
  mean deviation and no change in accept/reject outcome on more than 5% of the set.
- **SC-005**: End-to-end completion time for each migrated capability grows by no more than
  10% at the median compared with its pre-migration measurement.
- **SC-006**: With the observability collector stopped, 100% of AI runs still complete, and
  median completion time stays within ±5% of the collector-running median over at least 20
  runs of the same capability on the same inputs.
- **SC-007**: With the orchestration service stopped, zero AI work events are lost — 100% are
  processed once it returns — and zero units of work hang indefinitely or return a
  substituted result.
- **SC-011**: Across a restart of the broker and of every consumer under continuous load,
  zero accepted units of work are lost and zero produce duplicated stored results.
- **SC-012**: 100% of work that exhausts its retry budget is inspectable in the dead-letter
  destination with its failure reason, and an operator can re-dispatch an item in under 2
  minutes.
- **SC-013**: After the broker migration, exactly one asynchronous dispatch mechanism exists
  in the platform; zero work types remain on the previous queue.
- **SC-014**: No backend worker remains occupied for the duration of an AI run; backend
  worker occupancy for AI work types drops to the time spent publishing and persisting,
  measured before and after.
- **SC-008**: Zero third-party model-provider credentials are readable by the orchestration
  service; zero primary-database credentials are readable by it.
- **SC-015**: Zero trace payloads older than 30 days exist in the observability store,
  verified by inspection; 100% of runs older than that still show their step sequence,
  timings and cost.
- **SC-009**: 100% of migrated capabilities can be reverted to their previous path by
  configuration alone, verified for each one before its previous path is removed.
- **SC-010**: After the final cutover, exactly one live implementation exists per
  capability, with zero duplicated orchestration logic across services.

## Assumptions

- **The routing layer stays.** Model selection, failover chains and provider diversity
  remain owned by the existing reviewed routing configuration; the orchestration service is
  a client of it, not a replacement. This preserves the single-inference-path invariant and
  keeps every model call recorded and costed in one place.
- **Orchestration moves, business logic does not.** Persistence, authorization, queueing,
  scheduling, scoring thresholds and all non-AI logic stay in the backend. The orchestration
  service is stateless with respect to platform data.
- **The backend remains the entry point.** Users and the dashboard never call the
  orchestration service directly; it is reachable only from inside the stack.
- **Migration is incremental in sequence but total in scope**: capabilities move one at a
  time, each with its previous path retained behind configuration until its cutover is
  confirmed and then removed, and the migration is not complete until all fourteen task keys
  have moved (FR-019a). Single-shot capabilities and embeddings migrate for path-uniformity
  and complete trace coverage rather than for orchestration benefit; the added network hop
  is accepted for them, and SC-005's latency ceiling is what holds that cost in check.
- **A capability's baseline is recorded before it moves**, on a fixed input set, so
  "unchanged behaviour" is a measurement rather than a claim.
- **Retry, backoff, timeout and stuck-run recovery are re-implemented on the broker**, not
  carried over: the current policies live in the Redis-backed queue layer that this feature
  removes. Their *observable behaviour* per work type — how many attempts, what backoff, what
  counts as stuck — is preserved and treated as the acceptance target; the mechanism beneath
  it is new.
- **The event-driven backbone is a prerequisite, not a parallel track.** It is delivered and
  proven on existing non-AI work types before any AI capability moves, so a failure in the
  messaging migration is diagnosable without AI changes in the way.
- **Non-AI work types migrate to the broker but do not otherwise change**: same inputs, same
  stored results, same schedules.
- **The existing routing-layer observability callback stays enabled**, so this feature adds
  run-level tracing rather than replacing call-level records.
- **Introducing a service in a new language is accepted**, with the cost acknowledged: a new
  runtime in the stack, a new dependency and vulnerability surface, and a new test suite
  that must join the merge gate. The repository currently contains no service in a third
  language, and its stated technology constraints and per-language test discipline will need
  amending to admit one.
- **Trace payloads carry personal data** (resume and profile content), are operator-only, and
  are purged at 30 days while their metrics are kept. Diagnosis of a reported bad result is
  therefore possible for 30 days after the run, and cost/quality trend analysis indefinitely.

## Mandated Technologies

Named by the maintainer in the originating request and in clarification, and therefore
binding rather than a planning choice. The requirements above stay technology-agnostic on
purpose — they are what gets tested, and they must hold whatever serves them — but the
selection itself is not open:

| Concern | Mandated | Serves |
|---|---|---|
| AI orchestration framework | **LangChain** | FR-001 – FR-003: prompt assembly, model and tool abstractions, structured output parsing. Used alone for single-call capabilities |
| Multi-step / stateful workflows | **LangGraph** | FR-005, FR-007, FR-039 – FR-041, US3: explicit step graphs, bounded tool loops, per-step state. Used for the generation pipeline and salary inference — see the per-capability table below |
| Run-level observability | **Langfuse** | FR-012 – FR-018b: traces, spans, cost and token accounting, payload retention |
| Message broker | **RabbitMQ** | FR-026 – FR-038: durable queues, per-work-type retry with backoff, dead-lettering, at-least-once delivery |
| Orchestration service runtime | **Python** | The language LangChain and LangGraph are first-class in; the reason a separate service exists at all |

### Which layer serves which capability

LangChain and LangGraph are layers, not alternatives — LangChain's agent construction is
built on the LangGraph runtime. What varies per capability is how much of that runtime the
capability actually needs. A single-shot call wrapped in a one-node graph buys nothing and
costs indirection; a multi-stage pipeline expressed as a chain loses the per-stage state,
per-node retry and stage-level failure identification that US3 and FR-005 require.

| Capability (task key) | Layer | Shape |
|---|---|---|
| `match`, `ghost`, `rephrase`, `recruiter`, `outreach` | LangChain only | One model call: prompt → structured, schema-validated object |
| `generation-summary`, `generation-summary-premium`, `generation-summary-fast` | LangChain only | Single-shot summarization per stage invocation |
| `embed` | LangChain only | Embedding call through the gateway; no orchestration |
| `salary` | LangGraph — bounded agent loop | Model calls tools, results feed back, loop ends on no-tool-call or the round cap. Replaces the existing Go tool loop |
| `generation`, `generation-analyze`, `generation-select`, `generation-select-premium` | LangGraph — explicit state graph | The multi-stage pipeline: analyze → select (standard or premium) → summarize → assemble, with shared run state and conditional routing |

**Coach chat is not a separate capability**, despite reading like one. It calls
`RephraseModel` (`apps/api/internal/coach/application/service.go`), which resolves to the
`rephrase` task key — a single model call, no tools, no loop. It is a *caller* of the
`rephrase` capability, and migrates when `rephrase` does. Stated explicitly because an earlier
draft of this section described it as a bounded tool loop with its own key; it never was, and
the only tool loop in the codebase today is `salary`.

The rules this table encodes, stated so they survive the table:

- **FR-039**: A capability whose work is a single model call MUST NOT be implemented as a
  graph. Orchestration machinery is justified by multiple steps, branching or looping, and by
  nothing else.
- **FR-040**: Every capability with more than one model call — the generation pipeline and
  salary inference — MUST be implemented as an explicit graph whose nodes correspond
  one-to-one with the spans in its trace (FR-012), so a stage's failure names that stage
  (US3) without hand-written instrumentation.
- **FR-041**: Loop bounds (maximum tool rounds) and graph bounds (maximum node executions)
  MUST be configured per capability and MUST be enforced by the runtime, not by
  application-level counting, so no capability can loop unbounded (FR-005).

Two further notes that constrain planning rather than restate the table:

- **Langfuse is already deployed** in the stack (a `langfuse-web` / `langfuse-worker` /
  ClickHouse group) and is already wired as the routing layer's success and failure callback,
  producing one record per model call. This feature does not introduce it — it adds run-level
  tracing from the orchestration service on top of the call-level records that already exist,
  and FR-017 requires the two be correlatable.
- **Framework versions are pinned at planning time against current documentation**, not from
  recollection. LangChain's agent construction consolidated onto the LangGraph runtime in its
  1.0 release and the surface has moved quickly since; the layer split above is stable, the
  exact API names serving it are not.
- **LangChain's own provider integrations are not used for provider selection.** Model calls
  go to the existing gateway through an OpenAI-compatible client pointed at it, by task key
  (FR-009 – FR-010a). LangChain supplies the orchestration abstractions; the gateway keeps
  routing, failover and cost attribution.

## Dependencies

- The existing inference routing layer and its task-key contract.
- The existing observability collector already deployed for routing-layer records.
- The existing async queue's per-work-type retry, backoff and stuck-run recovery behaviour,
  which is the acceptance target for the replacement rather than a component that survives.
- RabbitMQ as a new stateful service in the self-hosted stack, with its own credentials,
  durability configuration and operational surface.
- The already-deployed Langfuse service group and its ClickHouse store, which gains
  run-level traces on top of the call-level records it holds today.
- Every existing worker and scheduler in the backend, all of which change dispatch mechanism.
- The recorded pre-migration baselines for matching and generation quality.
- The single-command local and production stack definitions, which must gain the new service
  without becoming a startup dependency for non-AI capabilities.

## Out of Scope

- Changing which models or providers serve any task key.
- Changing user-facing AI features, their outputs, or the dashboard.
- Introducing new AI capabilities that do not exist today.
- Moving non-AI logic (persistence, auth, scheduling, ingestion) out of the backend — non-AI
  work changes its dispatch mechanism only, and its logic stays where it is.
- Changing what non-AI work does, when it is scheduled, or what it stores.
- Exposing the broker or its management surface outside the stack.
- Replacing the existing evaluation approach with a new one; baselines are consumed, not
  redesigned.
- Exposing traces or the orchestration service to end users.
