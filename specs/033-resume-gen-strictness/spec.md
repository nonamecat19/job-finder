# Feature Specification: Resume Generation Strictness & Model Improvement

**Feature Branch**: `033-resume-gen-strictness`

**Created**: 2026-08-07

**Status**: Draft

**Input**: User description: "i have problems with ai resume generation. we need to fix the strictness of the rules. investigate better models and ways to improve the prompt"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Grounded tailoring that stays inside the allow-list (Priority: P1)

A user opens a job listing and runs "tailor my resume". The AI edits land only in the
allowed fields (summary, experience highlights, skills) and every generated claim is
traceable to their master profile — no invented skill tokens, no fabricated metrics, no
reworded bullets that drift past a recognizable derivation of the master's wording. When the
user reviews the diff, nothing needs to be rejected for being off-the-wall; the edits are
clearly the same content, better aimed at the vacancy.

**Why this priority**: This is the core trust boundary (Constitution II). A resume with a
fabricated claim is the single biggest harm the product can do to a user — it damages their
credibility in a real hiring decision and is hard to detect before it's too late. Today the
moderate grounding level (the default) does not enforce skill tokens or highlight word-overlap
after the primary tailoring pass, so fabricated or unrelated skills can pass grounding. Closing
that gap is the highest-value change.

**Independent Test**: Can be fully tested by running a tailoring pass at the default grounding
level against a master profile and a vacancy that asks for skills the candidate does not have,
then asserting that none of those absent skills appear in the merged resume — the grounding
verifier catches and rejects/drops them. Delivers a verifiably grounded resume.

**Acceptance Scenarios**:

1. **Given** a master profile whose skills are {Go, Postgres, Docker} and a vacancy asking
   for "Terraform, Kubernetes, Snowflake", **When** the user runs tailoring at the default
   (moderate) grounding level, **Then** the merged resume contains only skills already present
   in the master profile and the run is flagged as grounded; if an unrelated skill slipped
   past the model, it is silently dropped from the rendered output and logged.
2. **Given** a master experience bullet "Led migration of monolith to microservices", **When**
   the AI rephrases it for the vacancy, **Then** the rephrased bullet has recognizable
   word-overlap with the original (not a fabricated rewrite), and a rephrasing that drifts too
   far is dropped with a grounding reason rather than surfaced as a proposal.
3. **Given** a tailoring run where the model returns a skill token absent from the master,
   **When** the grounding verifier runs after the primary tailoring pass (not only after the
   page-fitting passes), **Then** the violation is detected on the first pass and triggers a
   re-prompt before the resume reaches the review surface.

---

### User Story 2 - A prompt that enforces, not just suggests, the rules (Priority: P2)

The prompt sent to the model reflects the actual, enforced invariants — it does not reference
removed fields (`sectionsToDrop`), does not contradict the deterministic post-processing, and
uses the strongest available structured-output mode so the model is constrained by schema, not
by prose alone. The user benefits because the model produces violations far less often: fewer
re-prompts, fewer dropped proposals, faster runs, and a review surface showing only meaningful
edits.

**Why this priority**: The prompt and the schema are the first line of defense. Every
violation the model never produces is one the verifier never has to catch. Today the prompt
carries a stale reference to a removed field and uses the weaker `json_object` mode rather
than the stricter `json_schema` with `strict: true` that the gateway and capable models
support. Fixing this reduces the violation rate at the source.

**Independent Test**: Can be tested by capturing the exact prompt and schema sent to the model
for a tailoring run and asserting (a) no reference to removed struct fields, (b)
`response_format` is `json_schema` with `strict: true` for providers that support it, and (c)
the schema includes only the fields the merge code can actually apply. Delivers a
contract-accurate prompt.

**Acceptance Scenarios**:

1. **Given** the tailoring prompt is built, **When** the prompt text is inspected, **Then** no
   instruction references a struct field that was removed from `TailoredSections` (e.g.
   `sectionsToDrop`, `ExperienceOrder`, `Drop`).
2. **Given** a model/provider that supports `response_format: json_schema` with
   `strict: true`, **When** the structured-output call is made, **Then** the schema is a strict
   JSON Schema derived from the `TailoredSections` type (and the per-stage types) with
   `additionalProperties: false`, so the model cannot emit unexpected fields.
3. **Given** a provider that does not support strict JSON Schema, **When** the call is routed,
   **Then** the system falls back to `json_object` mode and relies on the existing JSON-parse
   retry loop — the capability gap is detected, not silently degraded into prose.

---

### User Story 3 - A generation model chosen for instruction-following and grounding (Priority: P3)

The model serving the `generation` task is the one best suited to producing structured,
constrained, grounded output — evaluated against the actual strictness rules, not just
general quality. The user benefits because the model that is weakest at following the rules is
no longer the one writing their resume.

**Why this priority**: Model choice is the leverage point: a model that ignores negative
instructions ("do not fabricate") or struggles with JSON Schema will violate the rules no
matter how well the prompt is written. Today the generation chain is led by
`deepseek-v4-pro` with no documented rationale tying it to strictness benchmarks. A deliberate
evaluation against the grounding and structure rules lets the operator pick the model that
best obeys them.

**Independent Test**: Can be tested by running the same master profile + vacancy through each
candidate model on the generation chain, measuring the grounding-violation rate, structural
violation rate, and JSON-parse failure rate, and asserting the selected primary model is the
one with the lowest combined violation rate. Delivers an evidence-backed model selection.

**Acceptance Scenarios**:

1. **Given** a fixed master profile and vacancy, **When** each candidate model in the chain is
   run through the same tailoring pipeline, **Then** the run records the violation counts
   (grounding, structural, JSON-parse) per model, so the primary model is chosen from data.
2. **Given** the evaluated models, **When** the operator changes the primary generation model,
   **Then** the change is a single edit to `gateway/config.yaml` plus a restart, the chain
   still terminates at local Ollama, and no application rebuild or migration is needed.
3. **Given** the selected model is now serving generation, **When** a tailoring run completes,
   **Then** the served-model log line shows the chosen model, and the grounding/structure
   violation rate is at or below the rate recorded during evaluation.

---

### Edge Cases

- What happens when the model emits a valid JSON object but with a field the merge code
  ignores (e.g. an extra key not in `TailoredSections`)? Strict schema rejects it; non-strict
  mode must tolerate it without crashing.
- What happens when the master profile is nearly empty (one job, no skills)? The minimum
  bullet/skill floors cannot be met — generation uses what exists and records the shortfall
  (031-FR-017), and grounding checks pass at the same rate because there is nothing to
  fabricate against.
- What happens when every model in the generation chain is unreachable? The chain terminates
  at local Ollama (030-FR-008); if Ollama also fails, the run surfaces a clear error, never a
  partial or corrupted resume (020-FR-014).
- What happens when a candidate model supports `json_object` but not `json_schema`? The
  capability trap (030-C5) applies: it must not silently degrade. The chain must skip it for
  structured calls or the adapter must downgrade per-call and still validate the output.
- What happens when the grounding verifier rejects the primary pass and the re-prompt also
  violates? The run fails with all violations logged — it does not emit a partially-grounded
  resume. (Today the years-assertion case strips and logs; skill-token violations under
  moderate do not have an equivalent fallback and must gain one.)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The grounding verifier MUST enforce skill-token grounding on the merged resume
  after the primary tailoring pass, at the moderate and aggressive grounding levels — not only
  after the page-fitting passes. Today `VerifyRendercvGrounding` checks skill tokens only
  under `GroundingStrict`, and `DropUngroundedSkillTokens` runs only after expand/condense,
  leaving the primary pass unguarded at the default level.

- **FR-002**: The grounding verifier MUST detect a rephrased experience highlight that has
  drifted too far from the master's original bullet — below a minimum word-overlap threshold —
  at all grounding levels. Today the `lcsCovered` (≥50% word overlap) check exists in
  `VerifyTailoredSectionsGrounding` but has no production caller.

- **FR-003**: When a skill-token or highlight-overlap grounding violation is detected and a
  re-prompt does not fix it, the system MUST drop the offending content and log the
  intervention on the activity row — the same "strip and log" pattern the years-assertion check
  uses — rather than emitting an ungrounded resume or failing the whole run.

- **FR-004**: The prompt sent to the model MUST NOT reference struct fields that were removed
  from `TailoredSections` (e.g. `sectionsToDrop`, `ExperienceOrder`, `Drop`). The prompt
  contract must match the data contract the merge code actually honors.

- **FR-005**: For structured-output calls to providers that support strict JSON Schema, the
  request MUST use `response_format: { type: "json_schema", json_schema: { ..., strict: true } }`
  with `additionalProperties: false`, derived from the target Go type, instead of the weaker
  `json_object` mode. This constrains the model at the API level, not just in prose.

- **FR-006**: For providers that do not support strict JSON Schema, the request MUST fall back
  to `json_object` mode and the existing JSON-parse retry loop. The capability is detected per
  provider/model, never silently dropped — the capability trap (030-C5) must not recur.

- **FR-007**: The generation model chain MUST be re-evaluated against the strictness rules
  (grounding violation rate, structural violation rate, JSON-parse failure rate) before this
  feature ships. The primary model selected MUST be documented with the evaluation results,
  and the chain MUST continue to terminate at local Ollama (Constitution V, 030-FR-008).

- **FR-008**: Changing the primary generation model MUST remain a single edit to
  `gateway/config.yaml` plus a restart — no application rebuild, no migration (030-FR-005).

- **FR-009**: The grounding verifier's enforcement MUST apply to every run, including re-runs
  on an already-tailored baseline (028-FR-008) and the expand/condense page-fitting passes, so
  a violation introduced by the expand or condense call is caught with the same rigor as the
  primary pass.

- **FR-010**: Every grounding and structural intervention MUST be logged with the reason on
  the activity row, so a user or operator can see what was caught and why — consistent with
  020-SC-005 and 028-SC-005's audit-trail expectations.

- **FR-011**: An unreachable local model, a timeout, or malformed model output MUST surface a
  clear error — never a partial or corrupted resume (020-FR-014). The strictness changes must
  not introduce a new path that violates this.

- **FR-012**: The max output token limit for generation calls MUST be set explicitly (sent as
  `max_completion_tokens`), so a long tailoring response is bounded by a deliberate value
  rather than the model's implicit default or the request timeout.

### Key Entities *(include if feature involves data)*

- **GroundingLevel**: the strictness tier (`strict`/`moderate`/`aggressive`) that governs how
  much the model may deviate from the master profile. Already exists; this feature extends what
  each level enforces, not the levels themselves.
- **GroundingViolation**: a detected breach of Constitution II on a generated resume — the
  offending field, the rule it broke, and the source field it failed to trace to. Already
  exists in the verifier; this feature adds new violation kinds (skill token under moderate,
  highlight drift) and a strip-and-log fallback for them.
- **GenerationModel**: the model serving the `generation` task key, selected via
  `gateway/config.yaml`. This feature re-evaluates which model best satisfies the strictness
  rules and records the rationale, but does not change the routing mechanism (030 governs
  that).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The grounding violation rate (ungrounded skill tokens, drifted highlights)
  detected by the verifier on the primary tailoring pass drops by at least 50% after this
  feature, measured against a fixed benchmark of master profiles and vacancies — because the
  prompt and schema now constrain the model, and the verifier now catches what it previously
  missed at the default level.
- **SC-002**: 100% of tailoring runs at the default (moderate) grounding level produce a merged
  resume whose skill tokens all trace to the master profile or are dropped before rendering —
  zero ungrounded skill tokens reach the user-facing review surface.
- **SC-003**: A user reviewing a tailored resume can confirm, by eye within 30 seconds, that
  every highlight is a recognizable rephrasing of their master bullet (not a fabricated rewrite)
  — the same 30-second bar 028-SC-006 sets for structural invariants, now extended to content
  grounding.
- **SC-004**: The selected primary generation model has a documented strictness evaluation
  (grounding + structural + JSON-parse violation rates) showing it is the best-performing
  model on the chain for instruction-following under the rules, not just general quality.
- **SC-005**: Changing the primary generation model remains under 5 minutes — one YAML edit
  plus a restart (030-SC-003), with no application rebuild or migration.
- **SC-006**: A tailoring run still completes in under 60 seconds on average against the local
  model (020-SC-007), and the added strictness checks do not increase the median run time by
  more than 10% — the checks are deterministic and do not add LLM round-trips beyond the
  existing grounding loop.
- **SC-007**: 100% of grounding and structural interventions are logged with a reason on the
  activity row, so an operator can audit every caught violation without a database query
  (consistent with 030-SC-006's "within 2 minutes" bar for served-model logs).

## Assumptions

- The existing `gateway/config.yaml` model list and LiteLLM proxy remain the routing mechanism
  (030 governs routing); this feature changes which model is primary and how the prompt/schema
  constrains it, not the routing architecture.
- The LiteLLM proxy and the provider models in the chain support
  `response_format: json_schema` with `strict: true` (documented in LiteLLM); the
  `drop_params: true` setting means an unsupported param is dropped, so capability detection is
  required per provider to avoid the capability trap (030-C5).
- The existing grounding loop (`groundingAttempts = 2`) and the years-assertion strip-and-log
  pattern are the templates for the new skill-token and highlight-drift fallbacks; the
  architecture for intervention does not change, only what it covers.
- The master profile is the single source of truth for grounding; no external corpus is used
  to validate claims (Constitution II).
- Structured-output strictness and model instruction-following are evaluated against the
  existing benchmark fixtures and vacancy samples in the repo, not a new external dataset.
- The review/acceptance surface (spec §4.1) and the per-proposal `dropped` lifecycle are
  scaffolded but not yet wired into the generation flow; this feature focuses on the generation
  strictness and does not assume the proposal review surface is live. Where the `dropped`
  lifecycle is referenced, it is for the future-wired path, and the current fallback (strip and
  log) is the operative behavior.