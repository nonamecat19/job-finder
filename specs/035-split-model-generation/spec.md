# Feature Specification: Split-Model Resume Generation

**Feature Branch**: `035-split-model-generation`

**Created**: 2026-08-07

**Status**: Draft

**Input**: User description: "current ai model for resume generation is not good as needed... i think we can split the resume generation by multiple requests. for summary use better model and for picking right job achievements and other just use much simpler model"

## Clarifications

### Session 2026-08-07

- Q: May the page-fitting pass alter the premium-written summary? → A: No. Page fitting adjusts selection content only; the summary is immutable once written.
- Q: What threshold makes selection output "complete"? → A: Vacancy-required skills exactly (100% retained); nice-to-have skills 80%+.
- Q: What happens on a second consecutive selection shortfall? → A: Escalate the selection stage to the premium option; run completes, marked as escalated.
- Q: Does removing the automatic cover letter apply to job-triggered generation too? → A: Yes — both entry points; on demand from either.
- Q: Where must a substituted summary be visible? → A: Marker on the resume result surface plus the activity record; no interrupting dialog.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A summary worth the money, on a resume that costs cents (Priority: P1)

A user tailors their resume for a job. The professional summary at the top — the part a
recruiter reads first and the only part that is genuinely *written* rather than selected — is
produced by the strongest available generation quality. The rest of the document (which skills
appear and in what order, which achievements are picked from each job, how they are worded) is
produced by a far cheaper, faster option. The user sees no seam: one document, one wait, one
quality bar.

**Why this priority**: This is the whole feature. Today one model does all four jobs, so the
document is priced at the most expensive one, and choosing a cheap model to save money is what
produced a fabricated summary in evaluation. Separating the written part from the selected part
is what makes "good summary" and "cheap resume" stop being opposites.

**Independent Test**: Can be fully tested by running one tailoring pass and asserting the
summary was produced by the premium option while the selection work was produced by the
economy option, that the finished resume is complete, and that the measured cost per resume
falls below the pre-split baseline.

**Acceptance Scenarios**:

1. **Given** a master profile and a vacancy, **When** the user runs tailoring, **Then** the
   summary is produced by the premium generation option and the skill ordering, highlight
   selection and rephrasing are produced by the economy option, and the run records which
   option produced each part.
2. **Given** the split pipeline is live, **When** a tailoring run completes, **Then** the cost
   per resume is at most one fifth of the pre-split single-model baseline and the run completes
   at least twice as fast.
3. **Given** a completed run, **When** the user reads the resume, **Then** the summary opens
   with the derived years-of-experience figure, references skills the vacancy asks for, and
   cites an achievement that appears in the selected experience — with no fabricated skill,
   employer, metric or credential.
4. **Given** the user never learns the pipeline is split, **When** they compare the output to a
   pre-split resume, **Then** nothing about the document structure, section set or ordering has
   changed.

---

### User Story 2 - Cheap work that is caught when it cuts corners (Priority: P1)

The economy option does the mechanical work, and when it silently drops content — returns three
skill groups where the master has ten, or two achievements where five were asked for — the
system detects the shortfall and does not hand the user a hollowed-out resume.

**Why this priority**: Equal to P1 because it is the risk the split creates. Evaluation found
three cheap candidates that returned perfectly well-formed output containing a fraction of the
requested content: seventeen skill tokens where the master had a hundred and eighty-seven,
eighty-nine in another, six achievements where fifteen were expected. Nothing in the current
pipeline notices this, because the output is structurally valid. Shipping the cost saving
without this check is how the feature would quietly damage every resume.

**Independent Test**: Can be tested by feeding the pipeline a deliberately truncated selection
response and asserting the run detects the shortfall, retries or escalates, and never renders
the truncated document.

**Acceptance Scenarios**:

1. **Given** a selection response that drops a master skill the vacancy lists as required,
   **When** the pipeline verifies it, **Then** the shortfall is detected, the stage is retried,
   and a still-incomplete second response escalates rather than rendering.
2. **Given** a selection response retaining under 80% of the master skills matching the vacancy's
   nice-to-have list, **When** the pipeline verifies it, **Then** the shortfall is detected; at 80%
   or above no shortfall is reported.
3. **Given** a selection response containing fewer achievements per job than the configured
   minimum, **When** the pipeline verifies it, **Then** the shortfall is detected and recorded
   with the reason on the run's activity record.
4. **Given** a complete, correctly-sized selection response, **When** the pipeline verifies it,
   **Then** no shortfall is reported and no retry occurs — the check does not fire on healthy
   output.

---

### User Story 3 - The summary is verified as its own artifact (Priority: P2)

Because the summary is now produced separately, it is checked separately: every claim in it
must trace to the master profile or to the achievements just selected. A summary that invents a
skill, asserts a metric the profile does not support, or contradicts the derived years figure is
caught, re-prompted once, and stripped with a logged reason if the retry also fails.

**Why this priority**: The summary is where the observed fabrication happened, and it is the
highest-visibility sentence on the document. Separating it from the bulk selection makes it
cheap to verify precisely, which was impractical when it arrived buried in a large combined
response.

**Independent Test**: Can be tested by running the summary stage against a master profile and a
vacancy demanding skills the candidate lacks, then asserting the resulting summary contains none
of them and that any violation was logged.

**Acceptance Scenarios**:

1. **Given** a vacancy asking for skills absent from the master profile, **When** the summary is
   produced, **Then** it contains no skill absent from the master and the run is recorded as
   grounded.
2. **Given** a summary asserting a total-years figure that contradicts the one derived from the
   profile's dates, **When** the summary is verified, **Then** the violation is caught and a
   single re-prompt is issued.
3. **Given** a re-prompted summary that still violates, **When** verification fails a second
   time, **Then** the offending claim is stripped, the intervention is logged with its reason,
   and the resume is still delivered.

---

### User Story 4 - Cover letters only when asked for (Priority: P2)

A user who wants only a tailored resume gets only a tailored resume. The cover letter is
produced when they ask for one, against the resume they already have.

**Why this priority**: Every run currently writes a cover letter whether or not it is wanted,
which is a paid generation call and a share of the wait on every single tailoring. Removing it
from the default path is a direct cost and latency saving independent of the model split.

**Independent Test**: Can be tested by running a tailoring pass and asserting no cover letter is
produced, then requesting one for that resume and asserting it is produced and stored against
the same job.

**Acceptance Scenarios**:

1. **Given** a user runs tailoring, or job-triggered generation runs in the background, **When**
   the run completes, **Then** a resume is produced, no cover letter is produced, and the run is
   faster and cheaper than one that produced both.
2. **Given** a completed resume, **When** the user requests a cover letter for it, **Then** one
   is produced from that resume and the vacancy, and is retrievable alongside the resume.
3. **Given** a user requests a cover letter twice for the same resume, **When** the second
   request completes, **Then** the behaviour matches the existing document versioning rules and
   no duplicate is silently created.

---

### User Story 5 - The operator retunes stages without touching the application (Priority: P3)

The operator decides which option serves each stage — economy work, premium summary — by editing
the deployment's routing configuration and restarting that one service. Adding a new candidate
includes declaring how that candidate's deliberation is switched off, because a model left to
deliberate freely consumes its whole output budget and returns nothing.

**Why this priority**: Protects the operational contract the platform depends on and encodes the
single most expensive lesson from evaluation. It is P3 because it preserves an existing
guarantee rather than delivering new user-visible value.

**Independent Test**: Can be tested by repointing one stage at a different option, restarting
only the routing service, and asserting the pipeline uses it with no application rebuild,
migration or code change.

**Acceptance Scenarios**:

1. **Given** the operator changes which option serves a stage, **When** the routing service is
   restarted, **Then** the next run uses it, with no application deployment and no migration.
2. **Given** a stage's option is unavailable, **When** a run reaches that stage, **Then** it
   proceeds down that stage's fallback chain, which terminates at the self-hosted option, and the
   run completes.
3. **Given** any stage's configuration is inspected, **When** it is read, **Then** it declares how
   that option's deliberation is bounded, and no provider credential is readable through the
   application.

---

### Edge Cases

- What happens when the premium summary option is unavailable? The stage falls back down its
  chain and the run completes, and the resume carries a visible marker on the result surface
  saying the summary came from a fallback — the user is never silently handed an economy summary
  while believing otherwise.
- What happens when the economy option returns valid but truncated selection output twice in a
  row? The selection stage escalates to the premium option rather than rendering a hollowed-out
  resume. The run completes at a higher cost and is marked as escalated.
- What happens when the page target cannot be met by adjusting selection content alone? The
  page-fitting pass is not permitted to touch the summary, so it fits what it can and records the
  shortfall rather than rewording the premium-written summary to save space.
- What happens when the master profile is nearly empty? The shortfall check compares against what
  the master actually contains, so a small profile is not flagged for being small — only a
  response that returns less than its own master supports.
- What happens when every hosted option is unreachable? Every stage terminates at the self-hosted
  option and the resume is still produced.
- What happens when a newly added option deliberates without bound? Its stage returns empty
  output and every retry fails identically. The configuration must declare the bound, and a stage
  whose option produces no content must fail loudly rather than retry indefinitely.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Resume generation MUST be produced by separate stages — vacancy analysis, content
  selection and rephrasing, and summary writing — each requested independently rather than as one
  combined request. Page fitting is a conditional re-invocation of the selection stage, not a
  fourth stage.
- **FR-002**: Each stage MUST be addressable by stage name only. The application MUST NOT send,
  store, or expose a provider identity, model identifier, or provider credential.
- **FR-003**: The summary stage MUST be served by the premium quality option and the analysis,
  selection and page-fitting stages by the economy option, with the assignment owned by
  deployment configuration rather than application code.
- **FR-004**: The summary stage's request MUST carry only what a summary needs — the vacancy
  analysis, the derived years-of-experience figure, the selected achievements, and the leading
  skill groups — not the entire master profile.
- **FR-005**: The stage boundary MUST be enforced by the shape of what each stage returns: the
  selection stage cannot return a summary and the summary stage cannot return selection content.
- **FR-006**: The system MUST verify that selection output is complete relative to the master
  profile, with the bar set by how much the vacancy wants each item:
  - Every master skill matching a skill the vacancy **requires** MUST be retained — exact, no
    tolerance.
  - Master skills matching the vacancy's **nice-to-have** skills MUST be retained at 80% or above.
  - Per-job achievement counts MUST meet the configured minimum.

  Output falling short on any of these MUST NOT be rendered.
- **FR-007**: A detected selection shortfall MUST trigger one retry on the economy option; a
  second shortfall MUST escalate the selection stage to the premium option rather than render.
  The run completes, is marked as escalated on its activity record, and every shortfall and the
  escalation itself MUST be logged with its reason.
- **FR-008**: The summary MUST be verified independently: no skill absent from the master, no
  numeric metric the master does not support, and no years figure contradicting the derived one.
- **FR-009**: A summary violation MUST trigger one re-prompt; a violation surviving the re-prompt
  MUST be stripped and logged rather than failing the run or reaching the user.
- **FR-010**: The page-fitting pass MUST NOT alter the summary. It adjusts selection content only
  — achievements, skills and projects. The summary is written once by the premium stage and is
  immutable for the remainder of the run.
- **FR-011**: Every stage's fallback chain MUST terminate at the self-hosted option, and a run
  MUST complete even when every hosted option is unavailable.
- **FR-012**: When any stage is served by a fallback rather than its configured option, the run
  MUST record the substitution on its activity record. A substituted *summary* MUST additionally
  carry a visible marker on the resume result surface, where the user reviews the document — the
  activity record alone does not satisfy this.
- **FR-013**: Cover letter generation MUST be removed from every automatic generation path — both
  the ad-hoc tailoring surface and job-triggered background generation — and offered on demand
  against an existing resume from either entry point.
- **FR-014**: Each stage MUST carry an explicit output size limit sized to what that stage
  produces, and each stage's deployment configuration MUST declare how that option's deliberation
  is bounded.
- **FR-015**: The application MUST enforce its own per-stage deadline rather than relying on the
  routing service's timeout, which has been observed not to be enforced.
- **FR-016**: Changing which option serves a stage, or adding a stage candidate, MUST be a
  configuration edit plus a restart of the routing service — no application rebuild, no migration,
  no code change.
- **FR-017**: Every run MUST record, per stage, which option was requested, which served it, the
  duration, and the measured cost, so the pipeline's economics are observed rather than estimated.
- **FR-018**: The split MUST NOT change the resume's structure, section set, section order, or any
  existing grounding rule — only which option produces which part.

### Key Entities *(include if data involved)*

- **Generation Stage**: a named unit of generation work (analysis, selection, summary, page fit)
  with its own request, its own returned shape, its own size limit, and its own quality option.
- **Selection Output**: the mechanically-produced part of a tailored resume — which skills, in
  what order, which achievements per job, and their rewording. Verified for grounding and for
  completeness against the master.
- **Summary Output**: the written part — a short professional summary. Verified independently for
  grounding, metric support and consistency with the derived years figure.
- **Stage Run Record**: per stage, per run: option requested, option served, whether substituted,
  duration, cost, and any intervention with its reason.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Cost per tailored resume falls to at most one fifth of the pre-split single-model
  baseline, measured over the shared benchmark.
- **SC-002**: A tailored resume completes at least twice as fast as the pre-split baseline,
  measured as median wall-clock time over the shared benchmark.
- **SC-003**: 100% of summaries are produced by the premium option or, when it is unavailable,
  carry a substitution marker on the surface where the user reviews the resume — no run silently
  downgrades the summary.
- **SC-004**: Zero resumes reach the user missing a skill the vacancy requires, retaining under 80%
  of the vacancy's nice-to-have skills, or falling below the configured achievements-per-job
  minimum; every shortfall is caught before rendering.
- **SC-005**: Zero summaries reaching the user contain a skill, employer, metric or credential
  absent from the master profile, measured over the shared benchmark.
- **SC-006**: A user comparing a split-pipeline resume with a pre-split one finds no difference in
  structure, section set or ordering, and no drop in summary quality on review.
- **SC-007**: An operator can retune which option serves any stage in under 5 minutes with one
  configuration edit and one service restart.
- **SC-008**: 100% of tailoring runs still complete when every hosted option is unavailable.
- **SC-009**: Every run has a per-stage record of requested option, serving option, duration and
  cost, so cost per resume is reported from measurement rather than estimated.
- **SC-010**: A default tailoring run produces no cover letter, and a cover letter requested
  afterwards is produced against the existing resume.

## Assumptions

- The routing service remains the sole owner of provider and model selection; this feature adds
  stage-named routes to it rather than introducing a second routing mechanism or letting the
  application name models.
- The stage-to-option assignment at ship time is the one measured on 2026-08-07: the economy
  option for analysis, selection and page fitting, and the premium option for the summary. Exact
  identifiers are an implementation decision, re-verified against the live catalogue at planning
  time, since availability and pricing move.
- The measured baseline for SC-001 and SC-002 is a full single-model run on the strongest option
  evaluated (~$0.113 and ~60 seconds per resume); the split's measured figures were ~$0.011 and
  ~20 seconds.
- Feature 033's grounding and strict-output work is the foundation this builds on; the fixes it
  required to make any generation succeed at all — a sufficient output budget for deliberating
  models, and an output schema the strict validators accept — are prerequisites, not part of this
  feature.
- Feature 034's user-facing option picker, if built, applies to the summary stage; the mechanical
  stages are not worth exposing as a user choice. 034's scope should be narrowed accordingly.
- Cost recording (FR-017, SC-009) depends on the cost figure the routing service already returns
  being captured rather than discarded; no external billing integration is assumed.
- The shared benchmark fixtures and vacancy samples already in the repository are the measurement
  basis; no new external dataset is introduced.
- The platform remains single-user and self-hosted, so stage configuration is global to the
  deployment rather than per-user.
