# Contract: Capabilities

**Feature**: 047-langchain-ai-service | Binding on `apps/ai/src/jobfinder_ai/capabilities`.

Each capability is a named unit with a typed input, a typed output, a declared layer, declared
bounds and a task key. The registry validates all five at startup (FR-007).

## C1. The registry

- **C1-1**: Every capability MUST declare: `name`, `task_key`, `layer`
  (`chain` | `graph_loop` | `graph_state`), input model, output model, bounds, and prompt
  module.
- **C1-2**: Its `task_key` MUST be one of the fourteen keys declared in
  `gateway/config.yaml`. An undeclared key is a startup error (FR-007), not a request-time 4xx.
- **C1-3**: Every one of the fourteen task keys MUST be requested by exactly one capability. A
  declared-but-unrequested key is a defect, carrying forward 044-C1-4.
- **C1-4**: Startup MUST fail, naming the capability, if its prompt module is missing, its
  models are unimportable, or its bounds are non-positive.

## C2. The capability table

| Capability | Task key | Layer | Input | Output | Bounds | Transport |
|---|---|---|---|---|---|---|
| `match` | `match` | chain | profile snapshot + posting | score 0–100, reasons, matched/missing skills | 1 call | event |
| `ghost` | `ghost` | chain | posting | score, signal list | 1 call | event |
| `rephrase` | `rephrase` | chain | keyword set + profile context | suggested keywords | 1 call | HTTP |
| `recruiter` | `recruiter` | chain | scraped profile text | structured contact fields | 1 call | HTTP |
| `outreach` | `outreach` | chain | contact + company intel + profile | drafted message | 1 call | HTTP |
| `summary` | `generation-summary` | chain | profile + posting | tailored summary | 1 call | event (stage) |
| `summary_premium` | `generation-summary-premium` | chain | as above | as above | 1 call | event (stage) |
| `summary_fast` | `generation-summary-fast` | chain | as above | as above | 1 call | event (stage) |
| `embed` | `embed` | chain | text | vector | 1 call | HTTP |
| `salary` | `salary` | graph_loop | posting + salary tools | salary band | max tool rounds, max nodes | event |
| `generation` | `generation`, `-analyze`, `-select`, `-select-premium` | graph_state | profile snapshot + posting + section config | resume sections / cover letter | max nodes, per-stage timeout | event |

**Coach chat is not in this table, deliberately.** It is a *caller* of the `rephrase`
capability, not a capability of its own: `coach.NewService(model RephraseModel)` takes the
rephrase model and makes a single call through the `rephrase` task key, with no tools and no
loop. Listing it separately would either invent a fifteenth task key or give `rephrase` two
capabilities, breaking C1-3. Its HTTP handler in the backend keeps calling the `rephrase`
capability, and it migrates when `rephrase` does.

Interview prep and company intel are absent for a simpler reason: neither makes a model call.
`interviewprep.NewService(jobs, diffs, stories, news)` and the company-intel service receive no
router. There is nothing in them to migrate.

- **C2-1**: `layer` MUST follow FR-039/FR-040 — `chain` iff the capability makes exactly one
  model call. A chain capability that grows a second call MUST become a graph in the same
  change.
- **C2-2**: `embed` is a capability like any other, but is invoked over HTTP because its callers
  (profile indexing, retrieval) are synchronous request paths. Because it is the
  highest-volume call in the platform and gains nothing from orchestration, the added hop MUST
  be measured against SC-005 **before** its cutover; a breach is a spec question about
  FR-019a's totality, not a number to relax.
- **C2-2a**: One capability MAY serve several backend callers — `rephrase` serves both the
  keyword path and coach chat. What C1-3 forbids is the inverse: two capabilities claiming one
  task key.
- **C2-3**: The generation graph spans four task keys — one per stage. Each stage node names its
  own key, so per-stage routing tuning stays a `gateway/config.yaml` edit (030-FR-005).

## C3. Input and output

- **C3-1**: Inputs and outputs MUST be the generated models from contracts/events.md E7. Neither
  side hand-writes a mirror.
- **C3-2**: A capability MUST return a validated output object or raise a classified failure.
  Returning free text, or a partially populated object, is forbidden (FR-003).
- **C3-3**: Output values MUST stay within the ranges the dashboard already renders — an unchanged
  0–100 score, unchanged enum sets (FR-022). A capability that would widen a range needs a spec
  change, not a prompt change.
- **C3-4**: Validation failure is retried within the capability's bound and then fails with
  category `internal`.

## C4. Bounds (FR-005, FR-041)

- **C4-1**: Every capability MUST declare `max_model_calls`, and every graph capability MUST
  additionally declare `max_nodes`; loop capabilities MUST declare `max_tool_rounds`.
- **C4-2**: Bounds MUST be enforced by the runtime — LangGraph's recursion limit and per-node
  retry policy — not by counters in capability code.
- **C4-3**: Exceeding any bound MUST produce `bound_exceeded` naming the bound (E5-3), never a
  silently truncated result.
- **C4-4**: Every capability MUST declare a whole-run timeout, and every graph a per-node
  timeout. Both are configuration, both appear in the trace.
- **C4-5**: Bounds MUST be at least the current Go equivalents at cutover — the `salary` tool
  loop's existing `toolloop.Bounds` is the floor for its replacement, so behaviour cannot
  silently tighten.

## C5. Tool use

- **C5-1**: Tool results MUST be handled as untrusted data. Instruction-like text in a tool
  result MUST NOT alter the run's instructions (FR-006), preserving the property
  `toolloop/untrusted.go` enforces today.
- **C5-2**: Tools MUST be pure with respect to platform state — read-only lookups and
  computation. A tool MUST NOT write to the database or call an endpoint that does.
- **C5-3**: Each tool call and its result MUST be its own span (US3 scenario 2).
- **C5-4**: A tool that fails MUST return a structured error to the model within the loop, not
  crash the run — unless it exhausts the round bound, which is `bound_exceeded`.

## C6. Prompts

- **C6-1**: Prompt text lives under `apps/ai/src/jobfinder_ai/prompts/`, one module per
  capability, and is changed by commit (FR-015a).
- **C6-2**: No prompt may be fetched at runtime from a registry, the database, or any remote
  source (FR-015a).
- **C6-3**: Prompts MUST draw only from the input snapshot. Fabricated experience, skills or
  credentials are a Constitution II violation, and the trace's per-step input is what makes that
  auditable.
- **C6-4**: A prompt that assembles structured output MUST keep sending
  `response_format: {"type":"json_object"}`, because `drop_params: true` at the gateway silently
  drops it — the capability trap recorded in `specs/domains/llm-routing.md` § 2.1.

## C7. Model access

- **C7-1**: Capabilities MUST call models only through the shared gateway client, by task key
  (FR-009).
- **C7-2**: The client MUST have its own retries disabled; the gateway owns failover (research
  R3). A capability MUST NOT wrap a model call in its own retry loop for transport errors.
- **C7-3**: No provider SDK may appear in the dependency tree (FR-011). This is checkable and
  MUST be checked in CI.
- **C7-4**: Per-request timeout MUST exceed the gateway's `request_timeout` so the client never
  abandons a call the gateway is still serving.

## C8. Migration parity

- **C8-1**: Before a capability's cutover, its pre-migration outputs over a fixed input set MUST
  be recorded (FR-021).
- **C8-2**: After cutover, outputs MUST stay within tolerance: ≤5% mean deviation for scored
  capabilities and ≤5% of the set changing accept/reject outcome (SC-004).
- **C8-3**: Each capability MUST be switchable back to its Go path by configuration alone
  (FR-020), until C8-4.
- **C8-4**: Once a cutover is confirmed, the Go path MUST be deleted (FR-023). Two live
  implementations at rest is the state this rule exists to prevent.
