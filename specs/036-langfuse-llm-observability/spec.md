# Feature Specification: Self-Hosted LLM Observability

**Feature Branch**: `036-langfuse-llm-observability`

**Created**: 2026-08-07

**Status**: Draft

**Input**: User description: "Observability — Langfuse via LiteLLM callbacks. Config-only, zero Go change,
gets you cost/latency/trace per task key + which fallback tier actually served."

## Clarifications

### Session 2026-08-07

- Q: Does "config-only, zero Go change" hold for the whole feature? → A: No. It holds for per-call
  visibility (US1), which is the MVP. Correlating the several calls of one resume run into a single
  trace requires the application to emit a correlation identifier, so US2 is explicitly scoped as a
  small application change and is not claimed as config-only.
- Q: Prompts contain the user's master profile — real name, employers, contact details. Is that
  acceptable to store? → A: Only in a self-hosted collector inside the deployment's own trust
  boundary, with a stated retention window and no third-party egress. Sending prompt bodies to a
  hosted observability SaaS is out of scope and forbidden.
- Q: What happens when the collector is down? → A: Nothing user-visible. Inference must never fail,
  slow down, or block on trace delivery. Observability is strictly best-effort.

### Session 2026-08-07 (post-audit revision)

An audit checked this spec's claims against the codebase and the collector's own source. Four
premises were false and the requirements above were revised rather than patched over:

- Q: Does the collector group records by the requested task key? → A: **No.** It records the serving
  deployment. Per-task grouping must be created by sending the key as request metadata, which is an
  application change — so per-task reporting moved out of the zero-change scope (FR-012a).
- Q: Is the retention window a configuration setting? → A: **No.** Automated retention is an
  enterprise feature; OSS self-hosts prune for themselves. Retention is now an owned job (FR-008).
- Q: Are collector credentials isolated from the application the way provider credentials are? →
  A: **No, and the cited precedent is itself broken.** The application container ingests the whole
  environment file, so provider keys are already exposed. FR-007 now requires fixing the channel
  rather than describing it as safe.
- Q: Do embeddings bypass the proxy only under some configuration? → A: **They bypass it always.**
  FR-013 now requires the affected call sites to be named.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Every AI call is visible without touching application code (Priority: P1)

The operator opens a local dashboard and sees every AI call the platform made: which task asked for
it, which model actually answered, how long it took, what it cost, how many tokens went in and out,
and whether it succeeded. They did not rebuild the application to get this. They edited deployment
configuration and restarted one service.

**Why this priority**: This is the whole feature at its cheapest. Today the platform's economics are
inferred from log lines and hand-run benchmarks; nobody can answer "what did last week cost" or
"which task is slow" without reading logs. Because routing already sits behind a single proxy that
sees every call, this visibility is obtainable without adding a single line to the Go service.

**Independent Test**: Can be fully tested by running one tailoring pass and one job match with the
collector enabled, then confirming the dashboard shows one record per AI call, each carrying the
task key that was requested, the model that served it, duration, token counts and cost — with no
change to any Go source file.

**Acceptance Scenarios**:

1. **Given** the collector is enabled, **When** any AI task runs, **Then** a record appears for
   every call, carrying the requested task key, the model that served it, duration, prompt and
   completion token counts, cost, and success or failure.
2. **Given** a task's primary provider is unavailable and the call is served by a later tier,
   **When** the record is read, **Then** it shows both what was asked for and what actually answered,
   so silent degradation is visible rather than inferred.
3. **Given** the operator enables the collector, **When** they do so, **Then** no application source
   file changes, no migration runs, and no rebuild is required — only deployment configuration and a
   restart of the routing service.
4. **Given** a call fails on every tier, **When** the record is read, **Then** the failure and its
   reason are recorded rather than the call being absent.

---

### User Story 2 - The several calls of one run read as one story (Priority: P2)

A resume run makes several AI calls in sequence. The operator wants to see them as one run — the
vacancy analysis, the content selection, the summary, any retry or escalation — grouped, in order,
with a total cost and total duration for the run, not as unrelated records they must reassemble by
timestamp.

**Why this priority**: Per-call records answer "what did this call cost". Only grouped records
answer "what did this resume cost" and "where did those forty seconds go", which are the questions
that actually drive tuning. It is P2 rather than P1 because it requires an application change and
US1 delivers real value without it.

**Independent Test**: Can be tested by running one tailoring pass and asserting the dashboard shows a
single run containing every call that pass made, in order, with a run-level total cost and duration,
and that a second concurrent run does not have its calls mixed in.

**Acceptance Scenarios**:

1. **Given** a tailoring run makes several calls, **When** the operator views the collector, **Then**
   those calls appear grouped under one run, in execution order, with a run-level total cost and
   duration.
2. **Given** two runs execute concurrently, **When** their records are read, **Then** no call is
   attributed to the wrong run.
3. **Given** a run retries or escalates a stage, **When** the run is read, **Then** the retry and the
   escalation appear as additional calls within the same run rather than as separate runs.
4. **Given** a run is grouped in the collector, **When** the operator cross-references it,
   **Then** the run can be matched to its activity record in the platform's own history.

---

### User Story 3 - Cost and degradation are answerable questions (Priority: P3)

The operator asks: what did generation cost this week, which task is slowest, and how often is the
primary provider for each task actually failing over? Each is answered from recorded data in under a
minute, without running a benchmark.

**Why this priority**: The reporting payoff of US1/US2. It is P3 because it is querying data the
earlier stories already record — no new collection, only the ability to read it usefully.

**Depends on US2**, not merely on US1. The collector groups by serving deployment, so without the
requested task key sent as metadata, two stages served by the same model answer as one bucket and
these questions cannot be asked per task at all.

**Independent Test**: Can be tested by generating a known mix of calls, then answering each of the
three questions from the collector and checking the answers against the known mix.

**Acceptance Scenarios**:

1. **Given** a period of recorded activity, **When** the operator asks for cost grouped by task key,
   **Then** the figures are returned from recorded data and reconcile with the per-call records.
2. **Given** a period of recorded activity, **When** the operator asks which task has the worst
   latency, **Then** a per-task duration distribution is available, not just an average.
3. **Given** a provider has been failing, **When** the operator asks how often each task was served
   by a non-primary tier, **Then** the rate is answerable per task key.

---

### Edge Cases

- What happens when the collector is unreachable or its storage is full? Inference continues
  unaffected. Trace delivery is best-effort: dropped records are acceptable, a blocked or failed AI
  call is not.
- What happens when the collector is not configured at all? The platform behaves exactly as it does
  today — no records, no errors, no start-up failure.
- What happens to a call that is served by the local self-hosted model? It is recorded like any
  other, with a cost of zero rather than a missing record, so local usage is not invisible.
- What happens when a prompt contains the user's real name, employers and contact details? It is
  stored only inside the deployment's own trust boundary, subject to the stated retention window,
  and is never transmitted to a third party.
- What happens when the collector's own storage grows without bound? Retention is bounded by a
  configured window; the deployment does not silently accumulate prompt bodies forever.
- What happens when a run makes a call outside the routing proxy — an embedding served directly by
  the local model? It is either recorded through the same path or explicitly documented as
  out of coverage; coverage must be stated, not assumed.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Every AI call routed through the platform's routing service MUST produce an
  observability record without any change to application source code.
- **FR-002**: Each record MUST carry the requested task key, the model that actually served the call,
  the duration, prompt and completion token counts, the cost, and the success or failure outcome.
- **FR-003**: When a call is served by a fallback tier rather than the task's primary deployment, the
  record MUST show both the requested key and the serving model, so degradation is observable.
- **FR-004**: Observability MUST be best-effort and non-blocking: a collector that is slow,
  unreachable, or unconfigured MUST NOT delay, fail, or alter any AI call.
- **FR-005**: The collector MUST run inside the deployment, self-hosted, with no prompt or completion
  content transmitted to a third-party service.
- **FR-006**: Enabling, disabling or repointing observability MUST be a configuration edit plus a
  restart of the affected service — no application rebuild, no migration, no code change.
- **FR-007**: Collector credentials MUST NOT be readable from inside the application container.
  Satisfying this requires narrowing how the application container receives its environment: it
  currently ingests the entire environment file, which already exposes every provider credential and
  is itself a standing violation of the platform's credential rule. This feature MUST NOT add a
  second secret to that channel while describing it as isolated.
- **FR-008**: Recorded data MUST be pruned beyond a configured window. The collector's own automated
  retention is an enterprise feature and is NOT available to this deployment, so the platform MUST
  own a scheduled pruning job that enforces the window. Retention MUST be stated in the deployment
  documentation — but MUST NOT be stated as a guarantee until the job enforcing it exists.
- **FR-008a**: The pruning job MUST be observable: each run MUST record what it deleted, and a
  failure to prune MUST be visible rather than silent. A retention guarantee nobody can see failing
  is not a guarantee.
- **FR-009**: Calls belonging to one logical run MUST be groupable into a single trace carrying the
  run's total cost and duration, with retries and escalations appearing as calls within that run.
- **FR-010**: The identifier that groups a run MUST be derivable from, or cross-referable to, the
  platform's own activity record for that run.
- **FR-011**: Concurrent runs MUST NOT have calls attributed to the wrong run.
- **FR-012**: Cost MUST be reportable grouped by task key over a time range, latency MUST be
  reportable as a per-task distribution, and the rate of non-primary-tier service MUST be reportable
  per task key. Grouping by task key MUST be created deliberately: the collector records the
  *serving deployment*, not the requested key, so the requested key MUST be sent as request metadata
  or the per-stage distinction feature 035 exists to create is lost.
- **FR-012a**: Because FR-012 requires request metadata, per-task reporting MUST NOT be claimed as
  part of the zero-application-change scope. It belongs with the correlation work.
- **FR-013**: Coverage MUST be documented explicitly and accurately: which AI calls produce records
  and which bypass the routing service and therefore do not. Embeddings bypass it unconditionally —
  not under some configuration — and the specific call sites MUST be named rather than implied.
- **FR-014**: A call served by the local self-hosted model MUST produce a record with a zero cost
  rather than no record.
- **FR-015**: The platform MUST start and operate normally when observability is entirely
  unconfigured, and its test suites MUST NOT require a collector.

### Key Entities *(include if data involved)*

- **Call Record**: one AI call — requested task key, serving model, timing, token counts, cost,
  outcome, and the prompt and completion content.
- **Run Trace**: an ordered group of call records belonging to one logical unit of platform work
  (one tailoring pass, one match), with run-level totals and a cross-reference to the platform's
  activity record.
- **Retention Policy**: the configured window after which call records and their content are pruned.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of AI calls made through the routing service produce a record, verified by
  comparing record count against the platform's own call count over a fixed exercise.
- **SC-002**: Enabling US1 requires zero changed lines under `apps/api`, verified by diffing against
  the feature's base commit — not by inspecting the working tree, which carries unrelated changes and
  cannot distinguish this feature's edits from anything else.
- **SC-003**: Added latency attributable to observability is measured by comparing the same task key
  with callbacks configured against callbacks disabled, over at least 20 calls, using proxy-side
  timing. The bar is that the two distributions are indistinguishable at the median. A fully
  unreachable or unresponsive collector causes zero failed calls and no latency increase.
- **SC-004**: Once the requested task key is being sent as metadata, the operator can answer "what
  did each task cost over the last 7 days" in under one minute, grouped by task rather than by
  serving deployment, without running a benchmark or reading a log file.
- **SC-004a**: No two distinct task keys served by the same model collapse into one reporting bucket.
- **SC-005**: 100% of calls in a tailoring run appear grouped under that run, with zero
  cross-attribution across ten concurrent runs.
- **SC-006**: Zero prompt or completion bodies leave the deployment's own trust boundary, verified by
  inspecting the collector's named telemetry and phone-home settings — not by grepping for likely
  variable names.
- **SC-007**: The platform's existing test suites pass unchanged with no collector configured.
- **SC-008**: Records older than the retention window are absent, verified by writing a record with a
  backdated timestamp, running the pruning job, and confirming it is gone. Retention is proven by
  deletion, not by configuration.
- **SC-009**: Zero collector credentials are readable from inside the application container,
  verified by enumerating that container's environment.

## Assumptions

- The routing service remains the single point every AI call passes through, so proxy-level
  collection is sufficient coverage for everything except calls that deliberately bypass it. Any such
  bypass is enumerated by FR-013 rather than assumed absent.
- The collector is deployed as an additional service in the existing Docker Compose stack, reusing
  the deployment's existing datastores where its own requirements allow, rather than introducing a
  parallel infrastructure stack.
- The platform stays single-user and self-hosted, so observability is a global deployment concern
  with a single operator, not a per-tenant feature.
- Cost figures come from the routing service's own reported cost, the same figure feature 035 already
  captures; no external billing integration is introduced.
- Prompt and completion content is retained by default because grounding investigation is the main
  reason to have traces at all; the retention window, not redaction, is the control that bounds the
  exposure.
- No dashboard-facing (end-user) surface is added. Observability is an operator tool.
