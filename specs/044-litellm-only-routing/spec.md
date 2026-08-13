# Feature Specification: LiteLLM-Only Inference and Per-Scenario Model Assignment

**Feature Branch**: `044-litellm-only-routing`

**Created**: 2026-08-12

**Status**: Draft

**Input**: User description: "all the ai requests should use litellm only (no direct ai call at all). lets discuss different ai models for different types of scenario"

## Clarifications

### Session 2026-08-12

- Q: How far does "no direct AI call" go? → A: **Everything through the gateway, including embeddings.** The in-application Ollama client is deleted; the gateway address becomes required configuration.
- Q: Constitution Principle V requires the system to serve AI tasks locally when the gateway is unconfigured or unreachable. How is that reconciled? → A: **Amend Principle V and drop local-first.** Hosted inference becomes a hard dependency; the self-hosted local model is no longer a required fallback. This is a MAJOR constitution amendment and is in scope for this feature.
- Q: Does the Ollama deployment stay in the routing config? → A: **No — removed entirely**, including for embeddings. Embeddings move to a hosted embedding provider reached through the gateway, which means a vector-dimension migration and a full re-embed.
- Q: Which embedding provider? → A: **Cohere `embed-v4.0` at 1024 dimensions.** Cohere is already a configured provider, so no new credential is introduced.
- Q: What is the chain-ordering rule now that "terminate locally" is gone? → A: **Per scenario.** Scenarios whose output a human reads and sends lead with the quality model; mechanical and structured scenarios lead with free tiers.
- Q: How are the concrete models chosen? → A: **Pinned in this specification now, and each user-facing scenario's pin must be confirmed by a live evaluation run before the feature is done.**
- Q: What does production look like, given it has no gateway service today? → A: **Production runs its own gateway service** from the same routing configuration as development.

## Context

Two facts sit behind this feature.

The first is that "all AI goes through the gateway" is currently *almost* true and therefore not
true. Every provider-specific client was already deleted; what remains is one self-hosted model
client the application still calls directly, on two paths — embeddings, always, and every chat task
whenever the gateway address is unset. So the system has two inference paths with different
failover, different observability and different cost accounting, and the second one is the default
in the shipped example environment.

The second is that model assignment is coarser than the work it serves. Resume generation was split
into per-stage keys and demonstrably benefited. Everything else did not get that treatment: salary
inference, outreach drafting and recruiter extraction share one `default` chain, even though one of
them runs a tool-calling exchange, one writes prose a human will send to another human, and one
extracts fields from scraped HTML. Those three want different models and currently cannot have them.

This feature closes both: one inference path, and one named scenario per kind of AI work with a
model chosen for that kind.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Every AI request goes through one path (Priority: P1)

An operator wants to know, without qualification, where the platform's AI traffic goes, what it
cost, and which model answered. Today they can answer that for chat calls when the gateway is
configured, and cannot answer it at all for embeddings or for a deployment running without the
gateway. After this change there is exactly one outbound inference path, so every request — chat,
structured, tool-calling and embedding — appears in the observability record with a served model, a
cost and a trace that ties it to the run that caused it.

**Why this priority**: It is the whole of the stated request, and every other story in this
specification depends on there being a single path to assign models on.

**Independent Test**: Start the platform with no gateway address configured — it must refuse to
start with a message naming the missing setting, rather than quietly serving AI work by another
route. Then start it with the gateway configured, run one job through matching, one through resume
generation and one salary inference, and confirm every resulting AI call — embeddings included —
appears in the observability record.

**Acceptance Scenarios**:

1. **Given** the platform is configured with no gateway address, **When** it starts, **Then** it
   fails startup with an error naming the missing configuration, and no AI work is attempted.
2. **Given** the platform is running normally, **When** a job is ingested and scored, **Then** both
   the embedding call and the fit-scoring call appear in the observability record, each with a
   served model and a cost figure.
3. **Given** the gateway is unreachable, **When** an AI task runs, **Then** the task fails with a
   provider-unavailable reason recorded on its activity record and is retried under its existing
   policy — it is never served by a second, unrecorded path.
4. **Given** any AI call site in the application, **When** the code is inspected, **Then** no
   provider address other than the gateway's is reachable from it.

---

### User Story 2 - Each kind of AI work runs on a model chosen for it (Priority: P1)

An operator wants the model serving each kind of work to match what that work actually is. Resume
summary writing and salary-band inference should not be interchangeable, and today's configuration
makes three unrelated scenarios — salary, outreach, recruiter extraction — share one chain. After
this change every distinct kind of AI work has its own name in the routing configuration, its own
ordered chain, and a recorded reason for the model class it was given.

**Why this priority**: This is the second half of the request and the part with direct output-quality
and cost consequences. It is independently valuable: it delivers even if the single-path work of
Story 1 slipped.

**Independent Test**: Run one of each kind of AI work and read the observability record grouped by
scenario name — each kind appears under its own name, served by the model this specification assigns
it. Changing one scenario's model must leave every other scenario's served model unchanged.

**Acceptance Scenarios**:

1. **Given** the routing configuration, **When** salary inference, outreach drafting and recruiter
   extraction each run, **Then** each is recorded under its own scenario name and each may be served
   by a different model.
2. **Given** a scenario whose output is written for a human to read and send, **When** it runs with
   all providers healthy, **Then** it is served by that scenario's quality model, not by a free tier.
3. **Given** a mechanical or structured scenario, **When** it runs with all providers healthy,
   **Then** it is served by a free tier.
4. **Given** an operator changes which model serves one scenario, **When** they edit the routing
   configuration and restart the routing service, **Then** the change takes effect with no
   application rebuild and no other scenario affected.
5. **Given** an AI request naming a scenario that does not exist in the routing configuration,
   **When** it is issued, **Then** it fails loudly rather than being served by an unnamed default.

---

### User Story 3 - Model assignments are confirmed by measurement, not assertion (Priority: P2)

Before this feature is called done, the model pinned to each scenario whose output a human reads has
been run against the existing scored evaluation corpus, and the resulting score, cost and latency
per candidate are recorded alongside the assignment. An assignment nobody measured is a guess with a
version number.

**Why this priority**: The pins are usable immediately without this, which is why it is P2 rather
than P1 — but an unmeasured pin is exactly the failure mode that produced this feature's own
predecessors.

**Independent Test**: For each user-facing scenario, a recorded comparison artifact exists naming
the candidates compared, the corpus cases used, and the score/cost/latency of each; the assignment
in the routing configuration matches the winner or records why it does not.

**Acceptance Scenarios**:

1. **Given** the feature is proposed as complete, **When** the assignments are reviewed, **Then**
   every user-facing scenario has a dated comparison artifact behind its pin.
2. **Given** a comparison shows a cheaper model scoring within tolerance of the pinned one, **When**
   the assignment is revisited, **Then** either the pin changes or the reason for keeping it is
   recorded with the artifact.

---

### User Story 4 - Existing job data keeps working across the embedding change (Priority: P2)

Embeddings move to a different provider and a different vector width. A user with a populated
database must not silently get worse matching. Every stored job and profile embedding is recomputed
under the new model, and until that recomputation finishes the affected records are handled
explicitly rather than compared across incompatible vectors.

**Why this priority**: It is a data-correctness consequence of a P1 decision. Skipping it does not
break a build; it degrades matching quietly, which is worse.

**Independent Test**: On a database populated before the change, run the upgrade; matching results
for a fixed sample of jobs before and after are compared and every job in the sample ends with a
current-model embedding.

**Acceptance Scenarios**:

1. **Given** a database of jobs embedded under the previous model, **When** the upgrade runs,
   **Then** every job and the profile end with an embedding produced by the new model.
2. **Given** the recomputation is partially complete, **When** matching runs, **Then** records
   without a current embedding are handled by an explicit rule and never compared against
   embeddings from a different model.
3. **Given** the recomputation has completed, **When** a fixed sample of jobs is re-scored, **Then**
   the ordering is compared against the pre-change ordering and any change is recorded rather than
   discovered later.

---

### Edge Cases

- **The gateway is unreachable at startup.** Startup does not depend on the gateway being up — only
  on it being *configured*. A configured but unreachable gateway fails individual AI tasks, and the
  application still serves its non-AI surface.
- **Every tier in a scenario's chain is exhausted.** The task fails with the existing terminal or
  retryable classification. There is no longer a self-hosted tier to absorb it — this is the
  behaviour change the constitution amendment authorises, and it must be visible on the activity
  record, not silent.
- **A scenario chain is declared with a single tier.** Rejected by the configuration guardrail: with
  the terminal local tier gone, a one-tier chain has no failover at all.
- **A tool-using scenario is pointed at a model that does not accept tools.** The request succeeds
  without tools and the answer looks normal. Detection stays where it already is — the exchange's
  required first round — and the tool-capability declaration remains documentation a test reads.
- **The embedding provider is unavailable.** Embedding-dependent work (ingestion prefilter, profile
  save) fails and retries; it does not fall back to a differently-shaped vector.
- **A previously-embedded record is compared against a newly-embedded one.** Prevented by Story 4's
  explicit rule; vectors from two models are never scored against each other.
- **An operator sets the retired per-task local model variables.** They no longer exist; startup
  reports unknown configuration rather than ignoring it.

## Requirements *(mandatory)*

### Functional Requirements

**Single inference path**

- **FR-001**: The application MUST issue every AI request — chat, structured, tool-calling and
  embedding — to the gateway. No other inference endpoint may be reachable from application code.
- **FR-002**: The gateway address MUST be required configuration. Startup MUST fail with an error
  naming the setting when it is absent, rather than selecting an alternative path.
- **FR-003**: The in-application client for the self-hosted model runtime MUST be removed, together
  with its configuration surface (endpoint, credential, keep-alive, embedding endpoint, and every
  per-task local model variable).
- **FR-004**: Embeddings MUST be requested by scenario name through the gateway and MUST appear in
  the observability record on the same terms as chat calls. The standing exclusion of embeddings
  from observability coverage is revoked.
- **FR-005**: Auxiliary tools and diagnostics that issue AI requests MUST use the same path; no tool
  in the repository may construct a provider client of its own.
- **FR-006**: With every provider reachable only through the gateway, the distinction between local
  and hosted execution used to size concurrency MUST be retired, and its configuration key with it.
  A single hosted concurrency setting governs.

**Per-scenario model assignment**

- **FR-007**: Every distinct kind of AI work MUST have its own scenario name in the routing
  configuration. The shared catch-all serving salary inference, outreach drafting and recruiter
  extraction MUST be split into one name per scenario and retired as a name.
- **FR-008**: The application MUST request AI work by scenario name only, carrying no provider or
  model identity. This is unchanged and remains binding.
- **FR-009**: The routing configuration MUST fail loudly on a request naming an undeclared scenario.
  It MUST NOT route an unknown name to any default.
- **FR-010**: Every scenario MUST resolve to an ordered chain of at least two tiers spanning at
  least two distinct providers. The previous rule that every chain terminates at the self-hosted
  model is revoked and replaced by this one.
- **FR-011**: Chain ordering MUST follow one stated rule: scenarios whose output a human reads and
  sends lead with that scenario's quality model; mechanical, extractive and structured scenarios
  lead with **the cheapest tier measured to do the job**, which is usually but not always a free
  tier. Each scenario's classification MUST be recorded with its assignment, and a mechanical
  scenario leading with a paid tier MUST record the measurement that justifies it.

  *Why not "always a free tier": the two resume stages measured on 2026-08-07 are led by a paid
  aggregator model that did the mechanical work as well as the premium one at a fraction of the
  premium price — cheaper per unit of quality than the free tier it displaced. A rule that forbade
  that would force a repin against the only evidence anyone has collected.*
- **FR-012**: Every scenario's assignment MUST record the model class it was given and why, so that
  a later change argues against a stated reason rather than an absence.
- **FR-013**: Each scenario serving a tool-calling exchange MUST declare tool capability on every
  tier of its chain, and MUST NOT include a tier not published as tool-capable.
- **FR-014**: Each scenario consuming structured output MUST have every tier of its chain capable of
  the structured-output mode that scenario requests. **This is a review obligation, not a machine
  check**: because unsupported parameters are silently dropped, a non-capable tier answers in prose
  and the request *succeeds*, so no fallback and no parser-side assertion can catch it. Adding a
  tier to such a chain MUST include verifying the capability against the provider's live catalogue
  and pinning the verification with a dated comment.
- **FR-015**: Each tier whose model family deliberates MUST declare how that deliberation is
  bounded. A tier without such a declaration is a configuration error, not a default.
- **FR-016**: Changing which model serves a scenario MUST remain one configuration edit plus a
  restart of the routing service — no application rebuild, no migration, no dashboard action.
- **FR-017**: The configuration guardrail MUST fail the build when a requested scenario has no
  chain, a chain has fewer than two tiers or draws on a single provider, a chain names an undeclared
  tier, a required capability or deliberation declaration is missing, or a credential appears
  literally in the configuration file.

**Embeddings**

- **FR-018**: Embeddings MUST be produced by a hosted embedding model reached through the gateway,
  at a fixed vector width declared in configuration.
- **FR-019**: The stored vector width MUST be migrated to match, and every existing job and profile
  embedding MUST be recomputed under the new model.
- **FR-020**: Until a record has an embedding from the current model, it MUST be excluded from
  vector comparison by an explicit rule rather than compared across models.
- **FR-021**: The model that produced a stored embedding MUST be determinable, so a future model
  change can identify what needs recomputing without guessing.

**Deployment and governance**

- **FR-022**: Production MUST run the gateway from the same routing configuration as development,
  so there is one routing contract and no untested divergence.
- **FR-023**: Constitution Principle V MUST be amended to remove the local-first fallback guarantee,
  with the amendment's reasoning and version bump recorded in the constitution's own sync header.
  **Satisfied 2026-08-12** — `.specify/memory/constitution.md` is at **2.0.0**, Principle V is now
  "Self-Hosted Control Plane, Single Inference Path". The amendment landed **before** implementation
  deliberately: it authorises the deletions, so every intermediate commit is legal under the
  constitution in force at the time it was made.
- **FR-024**: Documentation MUST state plainly that prompt content — including profile data and
  generated application materials — is sent to third-party providers on every AI request, with no
  configuration under which it is not. The environment documentation MUST list every provider
  credential the gateway consumes.
- **FR-025**: The routing domain record MUST be updated in the same change: superseded rules (local
  terminal tier, local-first fallback, embeddings excluded from the gateway and from observability,
  the shared catch-all scenario) MUST be marked superseded where they are stated, not left standing
  alongside their replacements.

### Scenario catalogue *(the assignment table)*

Model class definitions:

| Class | Meaning |
|---|---|
| **Economy-structured** | Mechanical extraction, ranking, classification. Judged by whether the fields come out right, not by how they read. Free tier first. |
| **Quality-writing** | Prose a human reads and sends under their own name. Judged by how it reads. Quality model first. |
| **Tool-capable** | Runs a bounded tool-calling exchange. Every tier must accept a tools array. |
| **Embedding** | Vector production. Fixed width, no substitution. |

| Scenario | Class | Lead model | Chain after lead | Why this class |
|---|---|---|---|---|
| `match` | Economy-structured | `cerebras/gpt-oss-120b` | groq → cohere → openrouter | Runs on every ingested job — the highest-volume scenario in the system. Structured fit output; quality gains do not justify per-job premium cost. |
| `ghost` | Economy-structured | `cerebras/gpt-oss-120b` | groq → cohere → openrouter | Signal classification over posting text. |
| `rephrase` | Economy-structured | `cerebras/gpt-oss-120b` | groq → cohere → openrouter | Short keyword suggestions, cached. |
| `recruiter` *(new)* | Economy-structured | `cerebras/gpt-oss-120b` | groq → cohere | Contact-field extraction from scraped HTML. Purely extractive. |
| `generation-analyze` | Economy-structured | `openrouter/google/gemini-2.5-flash-lite` | cerebras → cohere | Measured 2026-08-07: the economy model matches the premium one on this stage at a fraction of the cost. |
| `generation-select` | Economy-structured | `openrouter/google/gemini-2.5-flash-lite` | cerebras → cohere | Ranking and picking, not writing. Largest structured output of the pipeline. |
| `generation-select-premium` | Quality-writing | `openrouter/anthropic/claude-sonnet-5` | claude-haiku-4.5 | Escalation after the economy model returns incomplete selection twice. |
| `generation-summary` | Quality-writing | `openrouter/anthropic/claude-sonnet-5` | haiku-4.5 → cerebras | The one stage where writing quality decides whether the document is usable. |
| `generation-summary-premium` | Quality-writing | `openrouter/anthropic/claude-opus-5` | sonnet-5 → cerebras | User-selected premium option. |
| `generation-summary-fast` | Economy-structured | `cerebras/gpt-oss-120b` | groq → cohere | User-selected fast option; the user has traded quality for speed explicitly. |
| `generation` (cover letter) | Quality-writing | `openrouter/anthropic/claude-sonnet-5` | haiku-4.5 → cerebras | **Changed from economy-first.** A cover letter is sent verbatim under the user's name; FR-011 puts it in the quality class. |
| `outreach` *(new)* | Quality-writing | `openrouter/anthropic/claude-sonnet-5` | haiku-4.5 → cerebras | Messages a human sends to another human. On-demand and low-volume, so quality-first is cheap in absolute terms. |
| `salary` *(new)* | Tool-capable | `cerebras/gpt-oss-120b` | groq → cohere → openrouter | Runs the bounded tool loop. Split out precisely so its tiers can be constrained to tool-capable models without constraining outreach and recruiter. |
| `embed` *(new)* | Embedding | `cohere/embed-v4.0` at 1024 dimensions | second tier declared at the same width or omitted | Already-configured provider, no new credential. Width fixed; a differently-shaped fallback is worse than a failure. |
| ~~`default`~~ | — | — | — | **Retired.** Split into `salary`, `outreach`, `recruiter`. No call site may name it. |
| ~~`local`~~ | — | — | — | **Retired.** The self-hosted terminal tier is removed with Principle V's local-first clause. |

Every pin above is provisional under FR-026 below.

- **FR-026**: Each pin in the quality-writing class MUST be confirmed by a run of the existing scored
  evaluation corpus against at least two candidates before this feature is complete, with the
  per-candidate score, cost and latency recorded next to the assignment.

### Key Entities

- **Scenario**: A named kind of AI work. Owns a model class, an ordered provider chain, a
  classification rationale, and — where applicable — capability declarations. The only identifier
  the application ever sends.
- **Chain**: An ordered list of tiers for one scenario. At least two tiers, at least two providers,
  no self-hosted terminal.
- **Tier**: One model on one provider, with its credential reference, capability declarations and
  deliberation bound.
- **Embedding record**: A stored vector plus enough provenance to know which model produced it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of AI requests the platform issues over a full day — chat, structured,
  tool-calling and embedding — appear in the observability record with a served model and a cost
  figure. Not "all chat calls": all of them.
- **SC-002**: Starting the platform without a configured gateway fails within 5 seconds with a
  message naming the missing setting, and no AI work is attempted.
- **SC-003**: A reviewer can determine which model serves any kind of AI work by reading one file,
  in under 2 minutes, with no code inspection.
- **SC-004**: Changing the model behind any one scenario takes one file edit and one service
  restart, completes in under 5 minutes, and demonstrably changes the served model for that scenario
  and no other.
- **SC-005**: Salary inference, outreach drafting and recruiter extraction are observably served by
  independently changeable models — changing one leaves the other two's served model unchanged.
- **SC-006**: Every scenario is served by **its own assigned lead tier** in at least 95% of requests
  over a normal day — quality-writing scenarios by their quality model, mechanical scenarios by
  their assigned economy model. A scenario falling below that is either mis-assigned or has an
  unhealthy primary, and both are worth knowing.
- **SC-007**: After the embedding migration completes, 100% of jobs and the profile carry an
  embedding from the current model, and a fixed 50-job sample's match ordering before and after is
  recorded with any change explained.
- **SC-008**: No AI request in a full-day trace reaches a provider by any route other than the
  gateway — verifiable from the record alone, with no code audit.
- **SC-009**: Each quality-writing scenario's pinned model is backed by a dated comparison over the
  evaluation corpus naming at least two candidates.

## Assumptions

- **This removes the offline guarantee, deliberately.** After this change the platform cannot score
  or generate anything without reaching third-party providers. That is the answer given in
  clarification, and it is why FR-023 requires a MAJOR constitution amendment rather than a wording
  fix. It is recorded here so that a future reader finds a decision, not an accident.
- **Prompt content leaves the deployment on every request, with no opt-out.** Profile data, resume
  content and posting text are sent to hosted providers. FR-024 requires this to be documented in
  the user-facing environment documentation rather than inferred from the routing configuration.
- The gateway itself remains self-hosted and in-stack; "no direct AI call" means no application-to-
  provider call, not that the proxy is external.
- Provider credentials continue to live only in the gateway service's environment and remain
  unreadable through the application.
- The gateway's existing per-attempt timeout, retry, cooldown and observability settings are
  unchanged by this feature; only the model list, the chains and the scenario names change.
- Free-tier providers (Cerebras, Groq, Cohere) remain available on their current terms. If a free
  tier disappears, the chains degrade to paid tiers — a cost event, not an outage.
- Cohere `embed-v4.0` supports the declared 1024-dimension output; the width is fixed at
  configuration time and is not negotiated per request.
- The existing scored evaluation corpus and its live-comparison mode are sufficient to satisfy
  FR-026 without new scoring logic.
- Queue concurrency, task deadlines, activity states and worker retry semantics are unchanged apart
  from retiring the local-execution concurrency setting (FR-006).
- The user-selected resume summary options keep their three published choices; the option that
  currently means "self-hosted" is removed with the self-hosted tier and is not replaced.
