# Feature Specification: Cerebras Free-Tier Model Toggle

**Feature Branch**: `001-cerebras-model-toggle`

**Created**: 2026-07-23

**Status**: Draft

**Input**: User description: "i need full support of cerebras, just need a toggle in dashboard settings to change models to such from cerebras free tier"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Switch inference to Cerebras from Settings (Priority: P1)

As the operator of the self-hosted job-finder, I want a control in dashboard Settings that
switches the app's language-model inference to Cerebras free-tier models, so that scoring,
generation, and other AI tasks run on Cerebras's fast hosted models instead of my local
Ollama, without editing config files or restarting the stack.

**Why this priority**: This is the core ask. Everything else (choosing a specific model,
validation, fallback) only has value once the operator can actually flip inference over to
Cerebras from the dashboard. Delivered alone it is a usable MVP.

**Independent Test**: From a clean install running on Ollama, open Settings, enable the
Cerebras toggle, then trigger an AI task (e.g. score a job). The task completes using a
Cerebras model and the Settings screen reflects "Cerebras" as the active provider.

**Acceptance Scenarios**:

1. **Given** the app runs with all tasks on Ollama and a Cerebras credential is configured,
   **When** the operator uses the one-action "switch all to Cerebras" control, **Then** all
   four chat tasks (matching, generation, rephrase, ghost-job) run against Cerebras and the
   Settings screen shows each as Cerebras.
2. **Given** tasks are on Cerebras, **When** the operator switches all back to Ollama,
   **Then** subsequent AI tasks run against Ollama again.
3. **Given** the operator switched tasks to Cerebras, **When** they reload the dashboard or
   the stack is restarted, **Then** the selection persists and remains active.
4. **Given** no Cerebras credential is configured, **When** the operator sets a task to
   Cerebras, **Then** the Settings screen shows the credential is missing and the task keeps
   running on Ollama.

---

### User Story 2 - Per-task provider and model selection (Priority: P2)

As the operator, I want to assign each chat task (matching, generation, rephrase, ghost-job)
to Ollama or to a specific Cerebras free-tier model independently, so I can put heavy tasks
on Cerebras's fast models while keeping others local, and trade speed vs quality per task.

**Why this priority**: The one-action switch (Story 1) is the MVP. Per-task control adds
real value for tuning cost/speed/quality but is not required to demonstrate Cerebras support.

**Independent Test**: Set generation to a Cerebras free-tier model and matching to Ollama,
save, then run both tasks and confirm each reports the provider/model it was assigned.

**Acceptance Scenarios**:

1. **Given** a configured credential, **When** the operator opens Settings, **Then** each of
   the four chat tasks shows a provider choice (Ollama / Cerebras) and, for Cerebras, a
   selector of supported free-tier models with a default preselected.
2. **Given** the operator assigns different providers/models per task and saves, **When**
   those tasks run, **Then** each uses its assigned provider/model and the choices persist
   across reloads and restarts.

---

### User Story 3 - Understand and recover from provider/credential problems (Priority: P3)

As the operator, I want clear feedback when Cerebras cannot be used (missing/invalid
credential, model unavailable, rate limit / quota exhausted), so that AI tasks do not
silently fail and I know what to fix.

**Why this priority**: Improves robustness and trust but is not needed to demonstrate the
core switch. Free-tier accounts hit rate/quota limits, so this materially affects real use.

**Independent Test**: With an invalid or missing credential, enable Cerebras and observe a
clear error at the point of enabling and/or when a task runs; the app remains operational.

**Acceptance Scenarios**:

1. **Given** no valid Cerebras credential, **When** the operator tries to enable Cerebras,
   **Then** they see a clear message that a credential is required and the provider is not
   switched.
2. **Given** Cerebras is active but a request is rejected for rate limit or quota, **When**
   an AI task runs, **Then** the failure is surfaced to the operator with an actionable
   message rather than being silently dropped.

---

### Edge Cases

- **Embeddings**: Cerebras does not serve embedding models. Enabling the Cerebras toggle
  MUST NOT break embedding-dependent features (similarity/vector search); embeddings
  continue to run on the existing Ollama endpoint regardless of the chat provider toggle.
- **In-flight work**: Jobs already queued/running when the provider is switched mid-flight
  (Assumption below: switch applies to newly started tasks; running tasks finish on the
  provider they started with).
- **Selected model no longer offered**: A previously selected free-tier model is removed
  from Cerebras's free tier — the app falls back to the default supported model and informs
  the operator.
- **Credential entered but endpoint unreachable**: Enabling should validate reachability and
  report failure instead of appearing to succeed.
- **Concurrent setting changes**: The setting is global to the single self-hosted instance;
  last write wins.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST support Cerebras as a first-class language-model provider for
  all existing chat/completion AI tasks (job matching/scoring, document generation, keyword
  rephrase suggestions, ghost-job detection).
- **FR-002**: Dashboard Settings MUST expose controls that switch the chat provider between the
  existing local provider (Ollama) and Cerebras, per chat task (see FR-014), plus a one-action
  "switch all" control (FR-015).
- **FR-003**: When Cerebras is active, the system MUST use a Cerebras free-tier model, with a
  sensible supported model selected by default.
- **FR-004**: The provider/model selection MUST persist across dashboard reloads and full
  stack restarts.
- **FR-005**: The provider/model selection MUST take effect for AI tasks started after the
  change without requiring a manual service restart or config-file edit.
- **FR-006**: The system MUST continue to run embeddings on the existing Ollama endpoint when
  Cerebras is the active chat provider, since Cerebras offers no embedding models.
- **FR-007**: Users MUST be able to see which provider and model are currently active from the
  Settings screen.
- **FR-008**: The system MUST require a valid, configured Cerebras credential before any task
  can run on Cerebras, and MUST keep a task on Ollama (surfacing why) if the credential is
  missing or invalid.
- **FR-009**: The system MUST present a clear, actionable message when a Cerebras request fails
  due to missing/invalid credential, unavailable model, or rate-limit/quota exhaustion, rather
  than failing silently.
- **FR-010**: Users MUST be able to select among the supported Cerebras free-tier models when
  Cerebras is active (Story 2).
- **FR-011**: The Cerebras credential MUST be handled as a secret — never returned to the
  browser and never written to application logs. The dashboard MAY show only whether a
  credential is configured (present/absent), never its value.
- **FR-012**: Switching providers MUST NOT lose or corrupt any existing user data (profiles,
  jobs, generated documents).
- **FR-013**: The Cerebras credential MUST be supplied only through environment/deploy-time
  configuration (not entered or edited via the dashboard). The dashboard toggle governs
  provider/model selection; it never captures or displays the credential. If Cerebras is
  enabled while no credential is configured, the system MUST report that the credential is
  missing and keep the affected task(s) on Ollama.
- **FR-014**: Provider and model selection MUST be per chat task. The four existing chat tasks
  (job matching/scoring, document generation, keyword rephrase, ghost-job detection) MUST each
  be independently assignable to Ollama or a Cerebras free-tier model, so tasks can run on
  different providers simultaneously (e.g. generation on Cerebras, matching on Ollama).
- **FR-015**: The system MUST provide a clear way to move all four tasks to Cerebras (or back
  to Ollama) in one action, so the common "switch everything" case does not require setting
  each task individually.

### Key Entities *(include if feature involves data)*

- **LLM Provider Setting**: The persisted, instance-wide record holding, per chat task
  (matching, generation, rephrase, ghost-job), the assigned provider (Ollama or Cerebras) and
  the selected model. Single logical record for the single-operator self-hosted instance. The
  Cerebras credential is NOT part of this record — it comes from deploy-time configuration.
- **Supported Cerebras Model**: A known free-tier model the operator may select, with a
  display name and an indication of which is the default.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can switch inference from Ollama to a Cerebras free-tier model
  entirely from the dashboard in under 1 minute, with no file edits and no manual restart.
- **SC-002**: After enabling Cerebras, 100% of newly started chat AI tasks run against
  Cerebras; after disabling, 100% run against Ollama.
- **SC-003**: The selected provider and model survive a full stack restart 100% of the time.
- **SC-004**: When Cerebras is active, embedding-dependent features continue to work with no
  regression versus the Ollama-only baseline.
- **SC-005**: Every Cerebras failure caused by credential, model, or quota problems produces
  an operator-visible message; zero silent drops in acceptance testing.
- **SC-006**: The Cerebras credential never appears in the dashboard network responses or the
  application logs during testing.

## Assumptions

- **Single operator / self-hosted**: Per the project constitution (local-first, self-hosted,
  single user), the provider setting is global to the instance rather than per end-user.
- **Credential via deploy config**: The Cerebras API key is provided through environment/config
  at deploy time (e.g. a `CEREBRAS_API_KEY`-style variable), not through the dashboard. The
  toggle only selects providers/models; it never captures the secret.
- **Ollama remains the default**: Fresh installs continue to default to Ollama; Cerebras is
  opt-in via the toggle. This preserves the constitution's "local-first by default" principle,
  with Cerebras as an explicitly operator-chosen alternative.
- **Embeddings stay on Ollama**: Cerebras has no embeddings endpoint, so vector/similarity
  features depend on a reachable Ollama embedding endpoint even when Cerebras chat is active.
- **Free tier requires an account/credential**: "Free tier" refers to Cerebras's no-cost model
  access, which still requires an API credential; the toggle governs model selection, not
  bypassing authentication.
- **Curated model list**: The supported free-tier models are a curated set surfaced by the
  app (rather than an open free-text model field), so operators pick from known-good options.
- **Provider switch applies to new work**: Changing the setting affects AI tasks started after
  the change; tasks already running complete on the provider they started with.
- **Reuse existing pipeline**: This feature changes which provider/model backs the existing AI
  tasks; it does not add new AI task types or change their outputs' structure.
