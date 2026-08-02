# Feature Specification: Gateway-Owned Model Routing

**Feature Branch**: `030-litellm-model-routing`

**Created**: 2026-07-31

**Status**: Draft

**Input**: User description: "remove ai model settings from dashboard, all the models logic ideally should be moved to litellm side. try to use free tier models first (in env you can see keys for Cerebras, Groq, Cohere), when they are not available switch to openrouter models"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator stops managing models in the dashboard (Priority: P1)

An operator opens Settings and no longer sees a per-task AI model/provider picker. Model choice is no longer a product decision surfaced in the product; every AI task (job matching, resume/cover-letter generation, keyword rephrase, ghost-job detection, and the catch-all "other" task) simply works, with routing decided outside the dashboard by the gateway configuration.

**Why this priority**: This is the visible half of the request and it removes the confusing failure mode where a user picks a provider whose credential is missing and silently gets a different model. Delivered alone, it already simplifies the product surface.

**Independent Test**: Open Settings; confirm the "AI models" tile is gone while the "AI features" tile and "Danger zone" tile still work; run a match and a generation and confirm both still produce results.

**Acceptance Scenarios**:

1. **Given** the dashboard Settings page, **When** the operator loads it, **Then** no AI model or provider selection controls are shown and no credential-missing banner tied to such a choice appears.
2. **Given** the operator previously had per-task assignments saved, **When** the system is upgraded, **Then** those saved assignments have no effect on which model serves any task and no stale state is presented anywhere in the dashboard.
3. **Given** the dashboard Settings page, **When** the operator loads it, **Then** the remaining settings tiles render and function exactly as before.

---

### User Story 2 - Free-tier providers used first, aggregator only as backup (Priority: P1)

For each AI task the system first attempts free-tier hosted providers (Cerebras, Groq, Cohere) whose keys are present in the environment. Only when those are unavailable — key absent, provider erroring, rate-limited, or over quota — does the request move on to OpenRouter models. If no hosted provider can serve the request, the locally hosted model serves it, so the system never becomes unusable because an external provider is down.

**Why this priority**: This is the cost/robustness half of the request. It is independently valuable even if the dashboard were unchanged, because it converts a manual per-task choice into an automatic preference order.

**Independent Test**: With all provider keys present, issue AI requests and confirm a free-tier provider serves them; remove/invalidate the free-tier keys and confirm the same requests are served by OpenRouter without user intervention; remove every hosted key and confirm requests are served locally.

**Acceptance Scenarios**:

1. **Given** at least one free-tier provider key is configured and healthy, **When** any AI task runs, **Then** the request is served by a free-tier provider.
2. **Given** the first free-tier provider returns a rate-limit or server error, **When** an AI task runs, **Then** the request is automatically retried on the next entry in the chain and the caller receives a successful result without a user-visible failure.
3. **Given** every free-tier provider is unavailable, **When** an AI task runs, **Then** the request is served by an OpenRouter model.
4. **Given** every hosted provider (free tier and OpenRouter) is unavailable, **When** an AI task runs, **Then** the request is served by the local model.
5. **Given** a provider key is absent from the environment, **When** the system starts, **Then** that provider is skipped in the preference order rather than causing a startup failure or a request error.

---

### User Story 3 - Routing is changeable in one place (Priority: P2)

An operator who wants to change which model serves a task edits a single gateway configuration file and restarts the gateway; nothing in the application backend, database, or dashboard changes. Task names (matching, generation, rephrase, ghost-job, default) remain the stable vocabulary the application uses to ask for work, and the configuration maps each name to an ordered chain of provider/model choices.

**Why this priority**: Makes the "models logic lives on the gateway side" outcome real and maintainable, but the system already behaves correctly without this being polished.

**Independent Test**: Change the model assigned to one task in the gateway configuration, restart only the gateway, and confirm the new model serves that task while other tasks are unchanged and the application was never redeployed.

**Acceptance Scenarios**:

1. **Given** a gateway configuration change for one task, **When** only the gateway is restarted, **Then** that task uses the new model and no application deployment or database change was required.
2. **Given** the gateway configuration, **When** an operator reads it, **Then** every task name the application can request is present with an explicit ordered provider chain.

---

### Edge Cases

- What happens when the gateway itself is unreachable or not configured? AI tasks fall back to the local model so core matching and generation keep working (constitution: local-first must remain operational).
- What happens when a provider succeeds but returns a response the task cannot use (for example malformed structured output)? The failure surfaces as a task failure under today's per-task retry policy rather than being retried across the chain indefinitely.
- What happens when a free-tier provider degrades mid-request (partial response, timeout)? The request advances to the next entry in the chain within the task's existing time budget; if the budget is exhausted the task fails normally.
- What happens to in-flight work during a gateway restart? Those requests fail and are retried by the existing worker retry mechanism; no data is lost.
- What happens when an operator lists a provider in a chain but the corresponding key is missing? That entry is skipped; it must not abort startup or poison the chain for other tasks.
- What happens to the historical per-task settings data after removal? It is removed with the feature; no dashboard or API surface reads it.
- What happens to embeddings? Embedding generation is unaffected and continues on the local embedding model.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The dashboard MUST NOT present any control for choosing an AI provider or model for any task, and MUST NOT present provider-credential status messages tied to such a choice.
- **FR-002**: The system MUST remove the read/update interfaces that exposed per-task provider/model assignments, including the curated hosted-model list used to populate the removed picker, so no client can set or read them.
- **FR-003**: The system MUST remove the stored per-task provider/model assignments; no runtime behaviour may depend on them after this change.
- **FR-004**: The application MUST request AI work by task name only (matching, generation, rephrase, ghost-job, default), carrying no provider or model identity in the request.
- **FR-005**: Provider and model selection MUST be expressed entirely in the gateway configuration, changeable by editing that configuration and restarting the gateway, with no application code, database, or dashboard change.
- **FR-006**: For every task, the configured selection order MUST attempt free-tier hosted providers (Cerebras, Groq, Cohere) before OpenRouter models.
- **FR-007**: The system MUST automatically advance to the next entry in a task's chain when the current entry is unavailable — missing credential, authentication failure, rate limit/quota exhaustion, timeout, or server error — without user intervention.
- **FR-008**: The final entry of every task's chain MUST be the locally hosted model, so AI tasks continue to work when every external provider is unavailable.
- **FR-009**: When the gateway is not configured or is unreachable, the application MUST serve AI tasks with the local model rather than failing the task.
- **FR-010**: Provider credentials MUST be supplied through environment configuration only; they MUST NOT be stored in the application database nor be readable through any application interface.
- **FR-011**: Absent optional provider credentials MUST NOT prevent system startup or cause request-time errors; the affected entries are skipped.
- **FR-012**: The system MUST record, per AI request, which provider and model ultimately served it, so an operator can determine effective routing from logs without querying an external service.
- **FR-013**: Existing per-task operational limits (the concurrency/admission control that today distinguishes local from hosted execution) MUST continue to apply, and MUST be derived without the application knowing which specific provider the gateway selected.
- **FR-014**: Embedding generation MUST remain on the local embedding model and MUST be unaffected by this change.
- **FR-015**: Environment-configuration documentation MUST list every provider key the gateway consumes (Cerebras, Groq, Cohere, OpenRouter) and MUST no longer describe dashboard-selectable providers or models.

### Key Entities

- **AI Task**: A named unit of AI work the application requests (matching, generation, rephrase, ghost-job, default). It is the only routing input the application supplies.
- **Routing Chain**: The ordered list of provider/model entries tried for a given AI task — free tier first, then OpenRouter, ending with the local model.
- **Provider Credential**: An environment-supplied key that makes one provider eligible; absence removes its entries from every chain.
- **Removed: Per-Task Provider Assignment**: The stored, dashboard-editable {task, provider, model} record. This feature deletes it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Zero AI provider or model choices are exposed anywhere in the dashboard, and no user action is required to keep AI features working after upgrade.
- **SC-002**: With free-tier keys configured and healthy, at least 95% of AI requests over a normal day are served by a free-tier provider, and aggregator usage occurs only after a free-tier attempt failed.
- **SC-003**: An operator can change which model serves any single task in under 5 minutes by editing one configuration file and restarting one service, with no application redeploy.
- **SC-004**: When the top-preference provider is fully unavailable, users see no failed AI task attributable to that outage — every such request completes on a later entry in the chain.
- **SC-005**: With no external provider reachable at all, matching and generation still complete successfully using the local model.
- **SC-006**: For any completed AI request, an operator can identify the provider and model that served it from system logs within 2 minutes.
- **SC-007**: The settings surface shrinks: the AI-model settings screen, its data store, and its interfaces are absent, and no regression appears in the remaining settings screens.

## Assumptions

- Free-tier ordering among Cerebras, Groq, and Cohere is a configuration detail; the requirement is only that all three precede OpenRouter. A default order is chosen at configuration time based on each provider's model quality and quota, and can be reordered without a spec change.
- Groq and Cohere keys already exist in the runtime environment (`GROQ_API_KEY`, `COHERE_API_KEY`) alongside `CEREBRAS_API_KEY` and `OPENROUTER_API_KEY`; this feature consumes them rather than introducing new credential provisioning.
- The locally hosted model remains available as today and is what satisfies the project's local-first, no-paid-API-dependency principle; it is placed last in every chain because the user explicitly asked for free hosted tiers to be tried first.
- Removal is clean, not a deprecation: the per-task settings storage, its interfaces, and its dashboard surface are deleted rather than kept read-only, because the dashboard is the only consumer.
- Structured/JSON output and any other capability the tasks rely on today are available from every provider placed in a chain; a provider that cannot serve a task's required output shape is simply not configured for that task.
- Per-task retry, timeout, and concurrency policies already implemented remain in force; this feature changes selection, not those policies.
- No user-facing migration or announcement is required; this is a single-operator self-hosted system.

## Out of Scope

- Adding a new dashboard surface for viewing gateway routing or provider health.
- Cost tracking, budgeting, or spend caps across providers.
- Changing prompts, task decomposition, or output schemas for any AI task.
- Moving embedding generation to the gateway.
- Quota-aware load balancing beyond ordered failover.
