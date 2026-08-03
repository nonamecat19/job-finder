> **ARCHIVED — SHIPPED — FR-008 deferred by design.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/platform-operations.md`](../../domains/platform-operations.md) — read that first.
>
> **FR-008 (pool waiter metrics) was explicitly descoped with no task raised. It remains a known gap.**

---
# Feature Specification: Explicit Database Connection Capacity

**Feature Branch**: `026-db-pool-capacity`

**Created**: 2026-07-30

**Status**: Clarified

**Input**: User description: "Configure the database connection pool explicitly instead of relying on the driver default, size it against the sum of worker concurrency plus HTTP headroom, and make saturation observable."

## Clarifications

### Session 2026-07-30

Resolved with recommended defaults (no blocking questions raised; each decision below had a
clearly preferable option given the project's single-user, self-hosted constraints).

- Q: How is the default capacity determined — a fixed number, or derived? → A: **Derived.** When
  the capacity setting is unset, the system computes it as the sum of every worker pool's size plus
  a fixed interactive reserve. A fixed default would silently go stale the moment any concurrency
  default changed, which is precisely how the current problem arose. An explicit setting always
  overrides the derived value.
- Q: How does the system know what the database server itself permits? → A: **A declared setting**,
  defaulting to the standard server default of 100 connections. The system validates its own
  capacity against the declared value. Interrogating the server at startup was rejected: it adds a
  pre-connection round trip to answer a question whose answer the operator already knows.
- Q: Is interactive traffic protected by a separate pool, or by headroom? → A: **Headroom plus a
  bounded acquisition wait.** A second pool doubles the tuning burden and splits capacity that is
  usually better shared, for a deployment with one user. A bounded wait converts starvation from a
  hang into a fast, attributable error.
- Q: Where do the waiting-caller metrics in FR-008 live, given no metrics surface exists? → A:
  **Deferred, with a substitute.** FR-008 is deferred to the observability feature. Its intent is
  met in the interim by the readiness report (FR-007) and by a periodic saturation log entry
  (FR-009). Recorded in plan.md Complexity Tracking; FR-008 is explicitly not in scope for this
  feature's task list.
- Q: Should acquisition wait time be tunable? → A: **Yes, with a conservative default**, because
  the right value differs by an order of magnitude between an interactive request and a background
  worker that can afford to wait.

## Problem Statement

The backend opens its database connection pool without stating how many connections it may use, so the pool silently adopts the driver's built-in default — a small number derived from the machine's processor count, as few as four on a modest host. Every part of the process draws on that one pool: the six background worker pools, the ingestion scheduler, the periodic liveness sweep, and every incoming dashboard request.

The worker pools alone are sized from configuration and today total fifteen concurrent slots at their defaults, with the AI-facing pools sized to their *maximum* possible concurrency at startup. Those fifteen slots can therefore all be busy at once, all wanting a connection, while the pool is willing to hand out four. The rest queue. Nothing reports that they are queuing: a request waiting for a connection is indistinguishable, from the outside, from a slow query. The user experiences this as a dashboard that becomes unresponsive during a large ingestion run, and there is nothing in the system that tells them why.

The relationship is also unguarded in the other direction. Raising any concurrency setting — cloud AI concurrency, ingest concurrency — increases contention for a pool that was never told about the change, so a setting that reads like "go faster" can make the whole process slower. Conversely, sizing the pool generously without regard to what the database itself allows would let the process exhaust the database's own connection limit and fail at connect time rather than degrade.

This feature makes the pool's capacity an explicit, validated, observable decision instead of an accident of the host's processor count.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The dashboard stays responsive during heavy background work (Priority: P1)

A user browses the dashboard while a full ingestion cycle and several scoring jobs are running. Their page loads and filters respond at normal speed. The background work does not consume so much of the database's capacity that interactive requests are left waiting behind it.

**Why this priority**: This is the user-visible symptom the feature exists to remove. It is also the reason the fix is worth doing before any throughput work: making ingestion faster while the pool stays undersized just moves the contention earlier.

**Independent Test**: Saturate every background worker pool, then issue interactive dashboard requests and measure their response time against the same requests on an idle system.

**Acceptance Scenarios**:

1. **Given** every background worker pool is fully occupied, **When** a user loads a dashboard page, **Then** the page responds within its normal time budget rather than waiting for background work to finish.
2. **Given** the system is under sustained background load, **When** interactive requests arrive continuously, **Then** none of them fail or time out solely because no database connection was available.
3. **Given** background workers and interactive requests are competing, **When** connections are scarce, **Then** interactive requests are not starved indefinitely behind background work.

---

### User Story 2 - Connection capacity is stated, validated, and consistent with concurrency settings (Priority: P1)

An operator sets the system's concurrency options in configuration. The database connection capacity is an explicit setting alongside them, with a documented default that already accommodates the shipped concurrency defaults. If the operator raises concurrency past what the stated capacity can support, the system tells them at startup rather than degrading silently.

**Why this priority**: Equal to Story 1 because it is what stops the problem returning. Without a validated relationship between concurrency and capacity, the next person to raise a concurrency setting reintroduces the same contention.

**Independent Test**: Set concurrency higher than the configured capacity allows and confirm the system reports the conflict at startup, naming both settings. Then set a consistent pair and confirm it starts.

**Acceptance Scenarios**:

1. **Given** the system starts with default settings, **When** it opens its database connection pool, **Then** the pool's maximum size comes from an explicit configured value, not from the driver's default.
2. **Given** an operator raises a concurrency setting beyond what the configured connection capacity supports, **When** the system starts, **Then** it reports the inconsistency, naming the concurrency setting, the capacity setting, and the minimum capacity required.
3. **Given** the shipped default settings, **When** the system starts, **Then** the default capacity is sufficient for the sum of all worker pool sizes plus a reserve for interactive requests, and startup produces no warning.
4. **Given** an operator sets a capacity larger than the database server itself permits, **When** the system starts, **Then** it fails or warns clearly rather than discovering the limit later under load.
5. **Given** an operator sets an implausible capacity such as zero or a negative number, **When** the system starts, **Then** it rejects the value with a message naming the setting.
6. **Given** capacity, idle-connection and connection-lifetime settings exist, **When** an operator consults the project's configuration documentation, **Then** each is listed with its default and its effect.

---

### User Story 3 - Connection exhaustion is visible instead of silent (Priority: P2)

When the system is short of database connections, an operator can see it. The readiness report and the system's operational metrics show how much of the pool is in use, how many callers are waiting, and how long they wait. A slow dashboard caused by connection starvation is diagnosable without reading source code.

**Why this priority**: Lower than the first two because it does not itself fix the contention, but it is what turns a future recurrence from a mystery into a five-second diagnosis — and it is what proves stories 1 and 2 actually worked.

**Independent Test**: Deliberately configure capacity below what the workload needs, generate load, and confirm the saturation is visible in the readiness report and metrics before any user complains.

**Acceptance Scenarios**:

1. **Given** the system is running, **When** an operator requests the readiness report, **Then** it includes the database pool's total capacity, connections currently in use, and connections idle.
2. **Given** callers are waiting for connections, **When** an operator inspects the system's metrics, **Then** the number of waiting callers and the time they spend waiting are both reported.
3. **Given** the pool has been saturated continuously for a sustained period, **When** an operator inspects the system, **Then** the saturation is recorded in the log with enough detail to identify it as a capacity problem rather than a slow query.
4. **Given** the pool is healthy and under-used, **When** an operator inspects the readiness report, **Then** it reports the pool as healthy without warnings.

---

### Edge Cases

- The configured capacity exceeds what the database server permits, so some connections fail to open only once demand is high enough to need them.
- The database is restarted underneath a running process. The pool must recover to its configured capacity without the process being restarted.
- A connection is broken by an idle timeout on a network device between the process and the database. Such connections must be retired rather than handed to a caller and failing.
- A single long-running query holds a connection for the duration of a whole ingest run, reducing effective capacity for everything else.
- The host has an unusually high processor count, so the old implicit default happened to be adequate; the explicit value must not silently reduce capacity on such a host below what the workload needs.
- Concurrency settings are changed at runtime through the dashboard rather than at startup; capacity validation must account for the maximum a runtime change can reach, not only the value at boot.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST set the database connection pool's maximum size from an explicit configuration value rather than accepting the driver default.
- **FR-002**: The system MUST provide configuration for minimum retained connections, maximum connection lifetime, and maximum connection idle time, each with a documented default.
- **FR-003**: The default maximum pool size MUST be at least the sum of all background worker pool sizes at their default configuration, plus a reserve for interactive requests.
- **FR-004**: The system MUST validate at startup that the configured concurrency settings cannot demand more connections than the configured capacity allows, and MUST report any inconsistency identifying both settings and the required minimum.
- **FR-005**: The system MUST reject invalid capacity values — zero, negative, or below the minimum needed for the process to function — at startup, naming the offending setting.
- **FR-006**: The system MUST ensure that an interactive request competing with background work for connections either obtains one, or fails within a bounded time with an error identifying connection-capacity exhaustion. It MUST NOT block indefinitely. *(Availability under load — that interactive requests obtain a connection rather than failing — is carried by the interactive reserve and measured by SC-002, not by this requirement.)*
- **FR-007**: The readiness report MUST include the database pool's configured capacity, connections in use, and connections idle.
- **FR-008**: **[DEFERRED — not in scope for this feature; no task, by design.]** The system MUST expose, as operational metrics, the number of callers waiting for a connection and the time spent waiting. Deferred to the observability feature because no metrics surface exists to expose them through; FR-007 (readiness statistics) and FR-009 (saturation log) carry the diagnostic intent in the interim. See plan.md Complexity Tracking.
- **FR-008a**: The system MUST bound how long an **interactive request** waits for a connection, with a configurable limit, and MUST fail with an error identifying connection-capacity exhaustion rather than waiting indefinitely. Background workers are out of scope for this bound — they remain bounded by their existing per-task deadlines, because there is no single connection-acquisition choke point to wrap without shimming generated data-access code (see research.md R5 and data-model.md §8).
- **FR-009**: The system MUST record a log entry when the pool has been fully saturated for a sustained period, distinguishing capacity exhaustion from slow queries.
- **FR-010**: The system MUST retire connections that exceed their configured lifetime or idle time rather than handing a stale connection to a caller.
- **FR-011**: The system MUST recover to its configured capacity without a process restart after the database becomes unavailable and returns.
- **FR-012**: The project's configuration documentation MUST list every new setting with its default and its effect.
- **FR-013**: Capacity validation MUST account for the highest concurrency reachable through runtime settings changes, not only the values present at startup.

### Key Entities

- **Connection pool**: The shared, bounded set of database connections used by every part of the process. Gains an explicit capacity, retention and lifetime policy.
- **Worker pool**: One background task type's fixed set of concurrent slots, sized from configuration. The sum of these across all task types is the primary driver of required capacity.
- **Capacity budget**: The derived relationship between total worker concurrency, interactive reserve, and configured pool size, checked at startup.
- **Pool health snapshot**: The point-in-time view of capacity, in-use, idle and waiting counts, surfaced through the readiness report and metrics.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: With every background worker pool fully occupied, interactive dashboard requests complete within 150% of their idle-system response time.
- **SC-002**: Under sustained full background load, zero interactive requests fail or time out because no connection was available.
- **SC-003**: Starting the system with a concurrency setting that exceeds the configured capacity produces a startup message identifying both settings, in 100% of such configurations.
- **SC-004**: An operator can determine whether the system is short of database connections from the readiness report alone, without consulting source code or logs.
- **SC-005**: The shipped default configuration starts cleanly with no capacity warning on hosts with any processor count from one upward.
- **SC-006**: After the database is restarted underneath a running system, full capacity is restored within 60 seconds with no process restart.

## Assumptions

- A single database serves the whole system; multiple databases or read replicas are out of scope.
- The database server's own connection limit is the operator's responsibility to configure; the system validates against a stated value rather than discovering it.
- Interactive reserve is expressed as a fixed headroom above total worker concurrency rather than as a separate pool, since a second pool would double the tuning burden for a single-user self-hosted deployment.
- **The reserve's default value is provisional.** It is chosen as a plausible starting point, not derived from a latency model, and SC-001 is the thing that decides whether it is right. If measurement misses SC-001, raising the reserve and re-measuring is the expected remedy — not a redesign. This is called out because the rest of this specification presents the reserve as settled, and it is not.
- The metrics named in this specification are assumed to be delivered by, or alongside, the wider observability work; if that work has not landed, the readiness report is the minimum acceptable surface for FR-007 and the metric requirements in FR-008 depend on it.
- Existing concurrency settings keep their current names and defaults; this feature adds capacity settings alongside them rather than replacing them.
- Runtime concurrency changes made through the dashboard remain bounded by the same configured maximums that exist today.
- No dashboard or API contract changes are required beyond additional fields in the readiness report.
