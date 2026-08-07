# Feature Specification: Multi-Turn Conversations and a Typed Tool Loop

**Feature Branch**: `037-llm-chat-tool-loop`

**Created**: 2026-08-07

**Revised**: 2026-08-07 after an audit checked this specification against the tree. Three of its
anchors were wrong together — the named consumer, the loop's return type, and the capability-detection
mechanism. See research.md's corrections log. Corrections are marked inline as **Corrected:**.

**Status**: Draft

**Input**: User description: "Add messages to the port. Only real ergonomic gap: Complete(prompt string)
can't do multi-turn or tool-calling. Add CompleteChat(ctx, []Message, opts), keep Complete as a shim.
~80 LOC, no dep. / Tool loop in Go when you need one. ~150 LOC, typed, testable. langchaingo agents is
the leakiest part of that library."

## Clarifications

### Session 2026-08-07

- Q: Should a tool be allowed to change anything? → A: No. Every tool in this feature is read-only.
  A tool that writes, sends, or submits would put the constitution's no-auto-apply boundary inside a
  model's decision loop, and that boundary is non-negotiable.
- Q: What happens when the model serving a tool-using request cannot call tools? → A: The request
  fails loudly with a clear reason. It must never silently return prose that looks like an answer.
- Q: Is this feature complete without a consumer? → A: No. Capability with no user is unverifiable.
  One real read-only consumer ships with it and proves the loop end to end.
- Q: Does adding messages change any existing behaviour? → A: No. Every existing call site must
  produce a byte-identical request to what it produces today.

### Session 2026-08-07 (post-audit)

- Q: What does the loop return? → A: **A typed, schema-validated value**, not a string. Every LLM
  surface in this codebase consumes `CompleteStructured[T]`; a bare `Content string` has no consumer
  here and could not be wired into one. The loop's terminal step goes through the existing structured
  path, keeping strict schemas, the parse-and-retry loop and the `Validator` hook.
- Q: How is "this model cannot call tools" detected? → A: **By asking for a tool call and not getting
  one.** The first round is sent with `tool_choice: "required"`; a first response carrying no tool
  call means the declaration did not reach a model that could act on it. Comparing the served model
  against an expectation is not available — the application never learns which upstream served a
  request (`specs/domains/llm-routing.md`, 030-FR-004).
- Q: Is tool output trusted? → A: **No.** A read-only tool returning a stored job description is
  returning text scraped from an untrusted website. Read-only bounds what the tool *does*; it says
  nothing about what the tool's *output* can talk the model into. Tool results are untrusted data.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Conversations instead of single prompts (Priority: P1)

Any part of the platform that needs to talk to a model across more than one turn can do so: it hands
over an ordered conversation — system framing, what was asked, what the model said, what came back —
and gets the next turn. Everything that only ever needed a single prompt keeps working exactly as it
does today, unchanged and unaware.

**Why this priority**: This is the foundation. Every capability below is impossible without it, and
it is the smaller half of the work. Today the only way to reach a model is a single string, so a
follow-up question means re-serialising the whole history into one prompt by hand at every call site
— which nothing in the platform does, so nothing in the platform has follow-up questions.

**Independent Test**: Can be fully tested by conducting a three-turn exchange where the third turn
depends on information given only in the first, and separately by asserting that every existing
single-prompt call produces a request identical to the one it produces today — **in both adapters**,
not only the gateway one.

**Acceptance Scenarios**:

1. **Given** a conversation of several turns, **When** the next turn is requested, **Then** the model
   receives the full ordered history and its answer reflects information supplied in an earlier turn.
2. **Given** an existing single-prompt caller, **When** it runs unchanged, **Then** the request it
   produces is identical to the pre-change one, byte for byte, against **both** the gateway adapter
   and the Ollama adapter.
3. **Given** a conversation, **When** it is sent, **Then** the ordering, roles and content of turns
   are preserved exactly, with no turn merged, dropped, or reordered.
4. **Given** the structured-output path, **When** it is used, **Then** it still works exactly as
   today, including strict schema enforcement and the parse-and-retry loop.

---

### User Story 2 - A model that can look things up, within a fence (Priority: P1)

A model working on a question can ask for information it does not have — read a stored record, look
up a comparable — and continue to a typed answer. The set of things it may ask for is declared up
front, typed, and strictly read-only. The exchange is bounded: a maximum number of rounds, a
deadline, a per-lookup timeout, a result size bound and a spend ceiling. It cannot loop forever, it
cannot run up an unbounded bill, and it cannot do anything.

**Why this priority**: Equal to P1 because the fence *is* the feature. A tool loop without bounds and
without a read-only guarantee is the thing that puts a language model between the user and an
irreversible action, which this platform's first principle forbids outright. Shipping the loop and
adding the fence later is not an available order of work.

**Independent Test**: Can be tested by giving a model a question answerable only through a lookup,
asserting it performs the lookup and produces a valid typed answer; then by asserting that a model
which asks endlessly is stopped at the configured bound, that no tool in the registry can reach an
outward-facing package, and that a tool result full of instructions does not change what the loop
does.

**Acceptance Scenarios**:

1. **Given** a question answerable only via a declared lookup, **When** the loop runs, **Then** the
   lookup is performed, its result is returned to the model, and the final **typed** answer uses it.
2. **Given** a model that requests lookups without ever concluding, **When** the configured round
   limit is reached, **Then** the loop stops and reports that it hit the limit rather than continuing
   or returning a fabricated answer.
3. **Given** a deadline that expires mid-exchange, **When** it expires, **Then** the loop stops
   promptly, no further lookup is started, and the caller is told why.
4. **Given** a lookup that fails or returns nothing, **When** it does, **Then** the failure is
   reported back to the model as a result it can react to, and the loop continues within its bounds
   rather than aborting the whole exchange.
5. **Given** the declared set of lookups, **When** it is inspected, **Then** every one is read-only:
   none sends a message, submits an application, contacts an employer, writes to storage, or changes
   any user-visible state.
6. **Given** a model that asks for a lookup that was never declared, or asks with arguments that do
   not match the declared shape, **When** it does, **Then** the request is refused, the refusal is
   returned to the model as a result, and nothing is executed.
7. **Given** a lookup whose result contains text instructing the model to do something else —
   ignore its instructions, call a different tool, return a particular value — **When** the loop
   continues, **Then** the instruction has no effect on the toolset, the bounds, the round count or
   the schema the answer must satisfy, and the occurrence is recorded.
8. **Given** an exchange that reaches its spend ceiling, **When** it does, **Then** the loop stops
   and reports it, rather than continuing to call a hosted provider.

---

### User Story 3 - The capability is proven by something real (Priority: P2)

**Corrected: this story previously named a consumer that does not exist.** See "The fabricated
consumer" below.

Salary estimation is converted to use the loop. Today it hands a model one truncated prompt and asks
for a band. With the loop it can ask for the comparables it actually needs — a neighbouring seniority,
a neighbouring geography, the untruncated posting — and return the same typed band, better grounded.

**Why this priority**: A capability with no consumer is untested by construction. This is P2 rather
than P1 because the seam and the fence are what other work depends on, but the feature is not
finished without this — a loop nothing calls is a loop nobody has proven correct.

**Independent Test**: Can be tested by asking for a band on a posting whose bucket has no cached
comparable, and confirming the model performs two distinct lookups and returns a `SalaryBand` that
passes the same validation the non-loop path applies today.

**Acceptance Scenarios**:

1. **Given** a posting requiring two distinct lookups, **When** a band is estimated, **Then** both
   lookups are performed and the returned band is a valid, schema-conforming `SalaryBand`.
2. **Given** the same posting with the lookups failing, **When** a band is estimated, **Then** the
   estimation reports it cannot answer and **no band is persisted**, rather than persisting one
   produced from nothing.
3. **Given** the converted service, **When** its existing test suite runs, **Then** every previously
   passing case still passes — including the paths that never reach a model at all: parsing
   `salaryRaw`, a cache hit, a levels.fyi hit, and the blend of the two.

#### The fabricated consumer

The original version of this specification, and every document under it, named
`internal/interviewprep/application` as the first consumer, with a `service.go` converted to the loop
and three lookups added beside it. **That package does not exist and never did.** `internal/interviewprep`
is two files — `apiservice.go` and `interfaces/http/interviewprep.go` — with no `application/`
directory, no test file, and **no LLM call anywhere in it**. It is a deterministic keyword pipeline:
`keyword.DeriveQuestions` plus `keyword.SelectStories`, constructed at `cmd/server/compose.go:490`
as `NewService(jobs, diffs, stories, news)` with no `llm.Router` argument at all. Its endpoint is
`GET /jobs/{id}/interview-prep`, takes no question parameter, and returns a fully structured
`dto.InterviewPrepPackDto`.

So research R8's rationale ("the natural two-lookup question"), plan.md's file tree, contracts §7 and
tasks T038–T044 all described editing files that do not exist, in a package with nothing to convert.
This is recorded rather than quietly replaced because the failure was not a typo: a consumer was
chosen from recollection and then reasoned about for five documents without once being opened.

---

### User Story 4 - Tool use survives the failover chain, or fails honestly (Priority: P2)

When the model that normally serves a tool-using task is unavailable and a fallback answers instead,
either the fallback can call tools too, or the request fails with a clear reason. What must never
happen is a fallback that ignores the declared lookups and returns confident prose that looks like a
real answer.

**Why this priority**: This platform's routing guarantees that some other model will answer when the
first cannot, and terminates every chain at the self-hosted model. That guarantee is exactly what
makes silent tool-ignoring possible — and `drop_params: true` in the proxy configuration makes it
*silent by design*: an unsupported `tools` parameter is dropped and the request succeeds. It is P2
because it only bites during a provider outage — but when it bites, the failure mode is an answer
that is wrong and looks right.

**Independent Test**: Can be tested by pointing a tool-using chain's first tier at a model with no
tool support and asserting an explicit failure naming the limitation — never a plausible prose answer
produced without the declared lookups.

**Acceptance Scenarios**:

1. **Given** a tool-using request served by a fallback that supports tools, **When** it runs, **Then**
   the lookups are performed and a valid typed answer is returned.
2. **Given** a tool-using request served by a model that cannot call tools, **When** it runs, **Then**
   the request fails with a reason naming the limitation, and no answer — prose or typed — is
   returned.
3. **Given** a task configured to use tools, **When** its routing configuration is inspected, **Then**
   every tier of its chain carries a recorded, tested statement of whether it is tool-capable, and
   the statement's status as **documentation rather than enforcement** is written down beside it.
4. **Given** the self-hosted terminal tier, **When** every hosted tier is unavailable, **Then** it
   serves the tool-using request if it is tool-capable, and otherwise the request fails explicitly.

---

### Edge Cases

- What happens when a model asks for several lookups in one turn? All are performed, each result is
  returned identified against the request it answers, and the round counts as one round.
- What happens when a lookup takes a long time? Each has its own time bound, independent of the
  exchange's overall deadline, so one slow lookup cannot consume the whole budget.
- What happens when a model requests the same lookup with the same arguments repeatedly? Repetition
  is bounded like everything else; the round limit stops it, and the repetition is recorded so the
  behaviour is visible rather than merely absorbed.
- What happens when a lookup returns a very large result? Results are size-bounded, and truncation is
  reported to the model as truncation rather than silently passed off as the whole answer.
- What happens when a model returns both prose and a lookup request in the same turn? The lookup
  is honoured and the exchange continues; prose is never the answer, because the answer is typed.
- What happens if someone later adds a lookup that writes something? It must be rejected at review
  and by an automated structural check, not caught by a human noticing.
- What happens if someone later adds a *new package* of lookups and forgets to register it? The
  structural check must fail, not pass silently. A check that only inspects packages it already knows
  about is a check that a new package escapes by existing.
- What happens when a tool returns text that reads as an instruction to the model? Nothing changes:
  the toolset, the bounds and the answer's schema are fixed before the exchange starts and no tool
  result can alter them. The occurrence is recorded.
- What happens when the model's arguments arrive in the provider's encoded form rather than as an
  object? They are decoded before validation, so a well-formed request is not mistaken for a
  malformed one and refused.
- What happens when the serving provider does not assign ids to tool calls? The adapter assigns
  them, deterministically and uniquely within the exchange, so every result can still be matched to
  its request.
- What happens to the existing single-prompt and structured-output paths? Nothing. They are unchanged
  in behaviour and on the wire, in both adapters.

## Requirements *(mandatory)*

### Functional Requirements

#### The conversation seam

- **FR-001**: The platform's model-access seam MUST accept an ordered multi-turn conversation, not
  only a single prompt string.
- **FR-002**: The existing single-prompt entry points MUST remain available and MUST become thin
  translations onto the conversation form, with no change to their signatures or behaviour.
- **FR-003**: Every existing caller MUST produce a request identical to the pre-change one, byte for
  byte, with no caller required to change. **Corrected: this applies to the Ollama adapter as well as
  the gateway adapter.** The two adapters differ from each other today in ways a shared shim would
  erase — see FR-003a.
- **FR-003a**: The following differences between the two existing entry points MUST survive the
  change, each verified by an assertion rather than by reading the diff:
  1. The Ollama adapter's plain-text path forwards a max-token bound as `num_predict`; its JSON path
     does **not** send it at all. A shared helper that starts sending it is a wire change on the
     terminal tier every chain ends at.
  2. The gateway adapter's JSON path always sets a response format, including when no options are
     supplied at all; its plain-text path never does.
  3. The Ollama adapter's JSON path sets a JSON format flag; its plain-text path does not.
  4. The two paths' default temperatures differ — 0.3 plain, 0.1 JSON — and must not be unified.
  5. The max-token bound is read behind an explicit nil-options guard while other options are
     nil-safe. A call supplying no options at all is a live call shape and must keep working.
- **FR-004**: The existing structured-output path — strict schema enforcement, parse-and-retry, typed
  results and semantic validation — MUST continue to work unchanged.
- **FR-005**: Conversation turns MUST preserve their order, role and content exactly; no turn may be
  merged, dropped, reordered or rewritten in transit.
- **FR-005a**: The strict-schema retry on the structured path MUST fire in exactly the cases it fires
  in today and no others. It currently triggers on more than a rejected request — an unparsable
  success body and a success body with no choices reach it too — and a schema that fails to parse
  currently disables both the strict mode and the retry together. Restructuring must not resurrect a
  retry that does not happen today, nor drop one that does.
- **FR-005b**: The number of times a single logical call reports a served model, reports usage, and
  writes a request log line MUST NOT change. The strict-schema fallback issues a second request today
  and therefore reports twice; moving where the retry lives must not turn that into once or three
  times.

#### The tool loop

- **FR-006**: A caller MUST be able to declare a set of lookups a model may request, each with a name,
  a description, and a typed argument shape.
- **FR-007**: Every declared lookup MUST be read-only. None may send a message, contact an employer,
  submit an application, write to storage, or change any user-visible state.
- **FR-008**: The read-only guarantee MUST be enforced by an automated structural check over the
  **transitive** reachability of each lookup package, not by review convention and not by inspecting
  only what a file imports directly.
- **FR-008a**: The structural check MUST enumerate lookup-registering packages by discovery, not by a
  hand-maintained list alone, and MUST fail when it finds one it was not told about. A list is
  permitted as the declaration of intent; the discovery is what makes omission a failure.
- **FR-008b**: The set of packages a lookup may not reach MUST include every package that performs
  outbound network requests on the platform's behalf, not only the ones that send messages. Reaching
  the open internet from inside a model's decision loop is the same class of exposure as reaching a
  mail sender.
- **FR-008c**: The check's limits MUST be stated in the check itself. It cannot catch a lookup that
  builds its own outbound request from the standard library, and it cannot catch a lookup implemented
  as a closure over a capability its own package never imports.
- **FR-009**: A request for an undeclared lookup, or arguments that do not match the declared shape,
  MUST be refused without execution, and the refusal MUST be returned to the model as a result it can
  react to.
- **FR-009a**: Arguments MUST be decoded from the provider's transport encoding before they are
  validated. The OpenAI-compatible format delivers a tool call's arguments as a JSON-encoded string,
  not as a nested object; validating the string form would refuse every well-formed request.
- **FR-010**: The exchange MUST be bounded by a configured maximum number of rounds. Reaching it MUST
  stop the exchange and report that the limit was reached.
- **FR-011**: The exchange MUST respect the caller's deadline. On expiry it MUST stop promptly, start
  no further lookup, and report the reason.
- **FR-011a**: The exchange MUST refuse to start when the caller's context carries no deadline. Each
  provider call in the chain can take as long as the whole failover chain takes; without a caller
  deadline the round limit alone bounds the exchange at a number of hours. Requiring a deadline
  bounds wall time without introducing a second, competing timeout.
- **FR-012**: Each lookup MUST have its own independent time bound, so one slow lookup cannot consume
  the whole exchange budget.
- **FR-013**: A lookup that fails or returns nothing MUST be reported back to the model as a result
  rather than aborting the exchange.
- **FR-014**: Lookup results MUST be size-bounded, and truncation MUST be reported to the model as
  truncation.
- **FR-015**: Several lookups requested in one turn MUST all be performed, each result identified
  against the request it answers, counting as a single round.
- **FR-015a**: Every tool result MUST be matched to its request by an identifier. Where the serving
  provider assigns none — the local model's native tool format does not — the adapter MUST assign one,
  unique within the exchange and stable across the request that carries it back.
- **FR-016**: The exchange MUST record, per round, which lookups were requested, whether each
  succeeded, how long each took, **which model served that round, and what that round cost**. A
  record that drops the served tier and the cost breaks the per-tier and per-run visibility features
  035 and 036 depend on.
- **FR-016a**: The exchange MUST be bounded by a maximum total spend, checked between rounds, and MUST
  stop and report when it is reached.

#### Typed termination

- **FR-023**: The exchange MUST produce a typed, schema-validated result, not free text. Its terminal
  step MUST go through the existing structured-output path so strict schema enforcement, the
  parse-and-retry loop and the semantic validation hook all apply to the loop's answer exactly as
  they apply to a single structured call today.
- **FR-023a**: The type the exchange produces MUST be the caller's own result type. A caller
  converting an existing structured call to the loop MUST NOT have to change what it returns.
- **FR-023b**: When the terminal structured step fails after its retries, the exchange MUST fail. It
  MUST NOT fall back to returning the model's prose.

#### Untrusted tool output

- **FR-024**: Lookup results MUST be treated as untrusted data. A read-only guarantee bounds what a
  lookup *does*; it places no bound on what a lookup's *output* contains, and a stored job description
  is text collected from a third-party website.
- **FR-025**: The conversation MUST frame lookup results as data rather than as instructions,
  including an explicit statement to the model that content inside a result is never a directive.
- **FR-026**: No content of a lookup result may change the declared toolset, any bound, the required
  answer schema, or which lookups are permitted. These are fixed before the exchange begins and are
  not re-read from the conversation.
- **FR-027**: The exchange MUST record when a lookup result contains content resembling an injected
  instruction, so the attempt is visible rather than merely ineffective.

#### Honest failure across the chain

- **FR-017**: **Corrected in mechanism.** A tool-using request served by a model that cannot call
  tools MUST be detected by requiring a tool call on the exchange's first round and observing that
  none was returned. The exchange MUST then fail with a reason naming the limitation and MUST NOT
  return an answer. The previous mechanism — comparing the served model against an expected one —
  is not available: the application never learns which upstream served a request
  (`specs/domains/llm-routing.md`, 030-FR-004), and under a permissive tool choice a model that
  simply chose not to look anything up is indistinguishable from one whose tool declaration was
  dropped.
- **FR-017a**: The cost of that mechanism MUST be stated rather than hidden: the first round always
  performs at least one lookup, even when the model could have answered without one.
- **FR-018**: **Corrected in what it claims.** Every tier of a tool-using task's chain MUST carry a
  recorded declaration of whether that tier's model supports tool calling, asserted by a test that
  reads the routing configuration. The declaration is **documentation the test reads; it is not a
  control the proxy enforces.** The proxy is configured to silently drop parameters an upstream does
  not accept, so a tier that cannot take a tool declaration receives the request without one and
  answers successfully. Nothing in the configuration prevents that; only FR-017's runtime detection
  catches it. Any document or comment implying the declaration prevents the drop is wrong and must be
  corrected.
- **FR-018a**: Because every chain terminates at one shared self-hosted deployment, declaring a
  chain tool-capable necessarily declares that shared terminal deployment tool-capable for every
  other task too. This coupling MUST be stated where the declaration is made.
- **FR-019**: The self-hosted terminal tier MUST serve tool-using requests when it is tool-capable;
  when it is not, the request MUST fail explicitly rather than degrade silently.

#### Scope and enforcement

- **FR-020**: No third-party agent or orchestration framework may be introduced. The conversation
  form and the loop are owned by this codebase.
- **FR-021**: **Corrected in consumer.** The salary estimation service MUST be converted to use the
  loop and MUST estimate a band through at least two distinct read-only lookups.
- **FR-022**: The converted service MUST report that it cannot estimate when its lookups are
  unavailable, and MUST persist nothing, rather than persisting a band produced without them.
- **FR-030**: The automated checks this feature relies on MUST actually run in continuous
  integration for the changes they guard. Where a guarded file lies outside the paths that trigger the
  Go test job today, the trigger MUST be widened in the same change.

### Key Entities *(include if data involved)*

- **Conversation Turn**: one entry in an ordered exchange — who said it (framing, the asker, the
  model, a lookup result), what was said, and, for a lookup result, which request it answers.
- **Lookup Declaration**: a named, described, typed, read-only capability a model may request —
  its argument shape and its result shape.
- **Lookup Request**: a model's request for a declared lookup, with arguments in the provider's
  encoding and an identifier — assigned by the provider or by the adapter — that ties its eventual
  result back to it.
- **Exchange Bounds**: the round limit, the per-lookup time bound, the result size bound and the
  spend ceiling. The overall deadline is the caller's, not a separate value.
- **Exchange Record**: per round, the lookups requested, their outcomes, their durations, the model
  that served the round and what the round cost; plus the exchange's total spend and stop reason.
- **Typed Answer**: the caller's own result type, produced by the terminal structured step and
  validated against its schema and its semantic rules.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of existing model calls produce byte-identical requests after the change, verified
  by golden comparison **against both adapters**, with zero call sites modified to keep working. Each
  of FR-003a's five differences has its own assertion.
- **SC-002**: A three-turn exchange whose final turn depends on the first turn's content answers
  correctly, demonstrating the conversation form carries history.
- **SC-003**: An exchange with a model that never concludes stops at exactly the configured round
  limit — never later — in 100% of runs.
- **SC-004**: An exchange whose deadline expires stops within one lookup's time bound of expiry and
  starts no further lookup, in 100% of runs. An exchange started without a deadline is refused in
  100% of runs.
- **SC-005**: Zero declared lookups can transitively reach an outward-facing or state-changing
  package, enforced by an automated check that runs in continuous integration and fails it. A new
  lookup package the check was not told about fails it too.
- **SC-006**: 100% of undeclared or malformed lookup requests are refused without execution, and 0%
  of well-formed requests are refused because of how their arguments were encoded on the wire.
- **SC-007**: Zero tool-using requests return an answer when served by a model that did not call the
  tool it was required to call; 100% fail with a reason naming the limitation.
- **SC-008**: The converted salary service estimates a band through two distinct lookups and returns
  a valid `SalaryBand`; 100% of its existing tests still pass, including the four paths that never
  reach a model.
- **SC-009**: The conversation seam and the loop together add no third-party dependency to the
  project's module requirements. The read-only check in particular introduces no new module
  requirement.
- **SC-010**: Every exchange has a per-round record carrying lookups, outcomes, durations, served
  model and cost, and an exchange total, for 100% of exchanges.
- **SC-011**: A lookup result containing an embedded instruction changes nothing about the exchange —
  same toolset, same bounds, same schema, same round accounting — in 100% of runs, and is recorded in
  100% of runs.
- **SC-012**: 100% of tool results are matched to their request by an identifier, including on the
  local adapter, whose native format supplies none.

## Assumptions

- The routing service remains the single point of model access and the sole owner of model selection;
  this feature adds a request shape, not a second routing mechanism, and the application still never
  names a model.
- Tool calling is expressed in the same request format the routing service already speaks, so no
  new transport or protocol is introduced. The self-hosted model's native format differs and is the
  adapter's problem, not the loop's.
- The first consumer is a read-only estimation surface over the user's own stored data plus a public
  comparables cache, which is why the read-only constraint is a natural fit rather than a limitation.
  Any future consumer wanting a state-changing tool is a separate feature requiring its own review
  against the no-auto-apply principle.
- The structured-output work from features 033 and 035 — strict schemas, the retry loop, typed
  results — is the foundation this builds on and is not re-implemented. FR-023 depends on it directly.
- Streaming responses are out of scope. Every call remains a complete response.
- No dashboard-facing change. The converted service's stored output is the same `SalaryBand` it
  stores today.
- The round limit, per-lookup bound, size bound and spend ceiling are deployment configuration with
  conservative defaults, not values a model or a prompt can influence.
