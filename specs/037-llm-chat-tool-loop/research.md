# Phase 0 Research: Multi-Turn Conversations and a Typed Tool Loop

**Feature**: `037-llm-chat-tool-loop` | **Date**: 2026-08-07

**Revised**: 2026-08-07 after an audit checked every claim against the tree, the proxy configuration
and the providers' wire formats. Six decisions were materially wrong, one of them load-bearing enough
that the feature was not implementable as written. See the corrections log at the end.

Thirteen decisions. All resolved; no NEEDS CLARIFICATION remains.

---

## R1 — Build it here; do not adopt `langchaingo` or a Go LangGraph port

**Decision**: Own the conversation type and the loop in `internal/platform/llm`. No new module
requirement (FR-020, SC-009).

**Status after audit: unchanged and confirmed.** This was the one decision that held up completely.
It is already recorded as a standing constraint in `specs/domains/llm-routing.md` § 2.3, so it
survives this directory being removed on ship.

**Rationale**: The comparison was made concretely against what is already in the tree, not in the
abstract. `langchaingo`'s `llms.Model` is *less* capable than `domain.Provider` on every axis this
codebase actually depends on:

| Capability in `domain.Provider` today | `langchaingo` equivalent |
|---|---|
| `ResponseModeStrict` + marshalled JSON Schema per call | no strict-schema abstraction; per-provider JSON-mode flags at best |
| `strictifySchema` / `makeNullable` — `additionalProperties:false`, nullable optional fields | none |
| `CompleteStructured[T]` — typed generic, schema cache, parse-and-retry, `Validator` hook | `outputparser`, string-typed and weaker |
| `WithServedModelCapture` reading `x-litellm-model-name`, `WithUsageCapture` reading the cost headers | no hook; fallback-tier and cost visibility would be lost |
| `Router` with `ProviderClass`, gateway ↔ local | no routing concept |

Adopting it would mean losing features 033 and 035 to gain a message slice. It also pulls a wide
transitive dependency tree into a deliberately tight `go.mod`.

The Go LangGraph ports are smaller still — a message-state graph with nodes and conditional edges,
without the durable checkpointer, the interrupt/resume, or the time-travel that make the Python
original worth having. The durability they lack, this platform already has and better: asynq on Redis,
Postgres, per-task deadline and heartbeat middleware.

**Alternatives rejected**:
- *Adopt `langchaingo`'s `llms.Model` as the port*: strict downgrade, see table.
- *Adopt a Go LangGraph port for orchestration*: trades typed Go for an in-memory graph DSL and gains
  no durability the queue does not already provide.
- *Vendor `langchaingo`'s `textsplitter` only*: genuinely the one useful leaf, but it belongs to
  retrieval work, not to this feature. Out of scope here.

---

## R2 — Conversation shape: OpenAI-style roles, because that is what the proxy already speaks

**Decision**: A `Message` with a role (`system`, `user`, `assistant`, `tool`), content, optional tool
calls on an assistant message, and a tool-call id on a tool message.

**Rationale**: The gateway adapter already marshals `{"role","content"}` pairs to
`/chat/completions`, and the routing service normalises every provider to that format. Inventing a
different internal shape would mean a translation layer whose only job is to undo the invention.

**Corrected — the local adapter is not the same format.** The original text implied one format
throughout. `internal/platform/llm/infrastructure/ollama/ollama.go` posts to `/api/chat` with its own
`chatRequest`/`chatResponse` pair, and its native tool-call representation is a `function` object with
a name and arguments and **no id field**. The domain type stays OpenAI-shaped; the Ollama adapter
translates, including assigning ids (R11).

**Alternatives rejected**: a provider-neutral abstract message type. Neutrality has no payoff when
the internal shape is already the shape one of two adapters speaks natively.

---

## R3 — `Complete` becomes a shim, and the shim must be provably transparent in **both** adapters

**Decision**: `Complete(ctx, prompt, opts)` builds `[system?, user]` and delegates to `CompleteChat`.
`CompleteJSON` does the same, keeping its response-format handling.

**Corrected — the numbers, the risk ranking, and the scope of the golden test were all wrong.**

*The numbers.* The original claimed "thirteen packages call the structured path and four call
`Complete` directly". Counted against the tree: **six** packages call `CompleteStructured` across
**fourteen** call sites — `generation` (7), `recruiter` (3), `ghostjob`, `matching`, `outreach`,
`salary` (1 each). Direct production `Complete` callers are **two**:
`keyword/infrastructure/rephraseadapter/rephrase_adapter.go:23` and `cmd/llmsmoke/main.go:37`, plus
`application/router.go:62`'s passthrough. The exaggerated count made the shim sound more load-bearing
than it is and, worse, was used to argue against migrating callers — an argument that did not need a
false premise.

*The risk ranking.* The original named the temperature split (0.3 plain, 0.1 JSON) as the regression
a shim would cause. It is real but it is not the largest. Ranked by what a naive shared helper
actually breaks, verified line by line:

| # | Difference | Where | Why it matters |
|---|---|---|---|
| 1 | ollama `Complete` forwards `MaxTokens` as `NumPredict`; ollama `CompleteJSON` does **not** | `ollama.go:143-145` vs `:156-162` | A shared helper starts sending `num_predict` on every local JSON call. That is a **wire change on the terminal tier every chain ends at** — the one path Principle V guarantees. No prior 037 document mentioned it. |
| 2 | `response_format` is always set on gateway `CompleteJSON`, never on `Complete` — including when `opts == nil` | `gateway.go:205-235` | The `else` branch sets `json_object` unconditionally. A shim that sets it "when strict mode is off and opts is non-nil" silently drops it for `CompleteJSON(ctx, p, nil)`. |
| 3 | The "400/422 fallback" also fires on `ErrInvalidResponse` | `gateway.go:157`, `:161`, `:246` | `isResponseFormatRejection` matches `ErrModelUnavailable` **or** `ErrInvalidResponse`, and `chat()` raises the latter on an unparsable 200 body and on zero choices. Calling it "the 400/422 fallback", as every 037 document did, describes roughly half of when it fires. |
| 4 | The fallback re-calls `g.chat` | `gateway.go:243-247` | So `logServed`, `ReportServedModel` and `ReportUsage` fire a **second time** for one logical call. Moving the retry changes side-effect counts that 035 and 036 read. |
| 5 | A schema-parse failure sets `strictMode = false` **and** downgrades to `json_object` | `gateway.go:228-233` | The retry is therefore skipped in that case. Recomputing strictness from `opts` inside a helper resurrects a retry that does not happen today. |
| 6 | `Format: "json"` on ollama `CompleteJSON` only | `ollama.go:159` | Straightforward, and the easiest to preserve. |
| 7 | `MaxTokens` guarded by explicit `opts != nil`; `Temp`/`ModelOr`/`SystemPrompt` are nil-safe methods | `gateway.go:192`, `ollama.go:143` | `Complete(ctx, prompt, nil)` is a live shape (`cmd/llmsmoke/main.go:37`). A helper that dereferences uniformly panics on it. |

*The scope.* The original golden test (T001) was scoped to the gateway adapter alone. Risk #1 lives
entirely in the Ollama adapter, so the check meant to prove SC-001 would not have covered the change
most likely to break it. The golden test now covers both adapters.

**Mitigation**: golden request bodies captured before the change and asserted byte-for-byte after,
per adapter, with one named assertion per row above. Not a review reading — a test.

**Alternatives rejected**: deprecating `Complete` and migrating every caller. With the real numbers
(two direct callers) this is more viable than the original text implied, but `CompleteJSON` is the one
that matters and it has fourteen structured call sites behind it. Keeping the shim is still right; it
is simply not the fourteen-vs-thirteen landslide the original claimed.

---

## R4 — Tool declarations reuse the existing schema machinery

**Decision**: A tool's argument shape is a Go type; its JSON Schema comes from the same
`invopop/jsonschema` + `strictifySchema` path `CompleteStructured` already uses, including the schema
cache (`domain/port.go`, `schemaFor` / `strictifySchema` / `makeNullable`).

**Rationale**: 033 already solved "turn a Go type into a schema a strict provider accepts" —
all-properties-required, `additionalProperties:false`, nullable optionals, `$schema`/`$id` stripped.
A second schema path would drift from the first.

**Alternatives rejected**: hand-written schema literals per tool. Drift-prone and unchecked against
the Go type the handler actually decodes into.

---

## R5 — Read-only enforced structurally — and the claimed precedent was not the mechanism claimed

**Decision**: A test resolves each lookup package's **transitive** dependency closure with
`go list -deps` invoked through `os/exec`, and fails if the closure contains any forbidden package.

**Corrected — the two "precedents" are neither of them import-graph walks.** The original called this
"the third instance of an established idiom" and cited two files. Opened:

- `internal/arch_test.go:46` parses each file with `parser.ImportsOnly` and compares each import
  string against **one** constant, `github.com/go-chi/chi/v5`. It resolves nothing transitively. It
  is a direct-import scan for a single package, and it is a perfectly good one — for that job.
- `internal/outreach/nosend_test.go` reads file **contents** as strings and greps for the tokens
  `"net/smtp"`, `"mailto:"`, `".Send("`, `"SendMail"`, `"smtp."`. It touches no imports at all and
  parses no Go.

Neither performs a transitive walk, so C5-1's "any transitive import" had no precedent in this tree
and the reassuring "third instance of an established idiom" framing was doing work the code did not
support. The idiom that exists is *a test that reads the tree and fails*; the specific mechanism was
invented here and has to be justified on its own.

**Why `go list -deps` and not `x/tools/go/packages`**: `golang.org/x/tools` is **not** in
`apps/api/go.mod`. Adding it to satisfy the fence would break SC-009's no-new-dependency check with
the very control that exists to protect a constitutional principle. `go list` ships with the
toolchain, needs no module requirement, and `go test` cannot run without that toolchain being present
anyway.

**The failure mode of the mechanism itself**: if `go list` cannot be invoked, the test MUST fail, not
skip. A fence that turns itself off when the environment is unusual is a fence that is off.

**Limits, stated honestly** (all three go in the test file, per FR-008c):
1. It catches a lookup package that *reaches for* a forbidden package. It cannot catch a lookup that
   builds its own outbound HTTP request out of `net/http` and the standard library.
2. **Handlers are closures.** A handler defined inside a service that already holds an outreach client
   can call it without the lookup file importing anything at all. The import graph cannot see a method
   value on an already-injected dependency. This is the largest hole and it is the reason the
   complementary control — a small, enumerated, reviewed toolset — is not optional.
3. It sees packages, not call paths. A lookup package that legitimately imports a package with both
   read and write functions is indistinguishable from one that calls the write function.

**Corrected — the forbidden list was too short.** The original listed `internal/notifier`,
`internal/outreach`, `internal/postage` and the write paths of `internal/applications`. Missing:
`internal/retrieval` performs outbound HTTP, drives a headless browser and calls FlareSolverr
(`retrieval/browser.go`, `retrieval/flaresolverr.go`, `retrieval/transport.go`), and
`internal/jobsources` fetches third-party boards. A lookup importing either passes the original fence
and reaches the open internet from inside a model's decision loop. Both are now forbidden (FR-008b).

**Alternatives rejected**:
- *A `ReadOnly() bool` marker on the tool interface*: self-attested by the thing being constrained.
  Worthless against the failure it is meant to prevent.
- *Narrow the fence to direct imports, matching `arch_test.go` exactly*: cheaper, needs no subprocess,
  and would be honest if the weaker guarantee were stated. Rejected because one indirection —
  a lookup package importing a helper package that imports `retrieval` — defeats it entirely, and
  that indirection is the normal shape of this codebase, not a contrived evasion.
- *Review only*: fails the non-negotiable test above.

---

## R6 — Bounds are caller configuration with conservative defaults, plus a spend ceiling

**Decision**: Round limit, per-lookup time bound, result size bound and **total spend ceiling** are
set by the caller with defaults; none is influenceable by a prompt or a model. The overall deadline
remains the caller's context, and the exchange **refuses to start without one**.

**Corrected — the original bounds did not bound the two things that cost money and time.**

*Wall time.* The original set `MaxRounds: 8` and said the caller's context is the deadline. But the
proxy's own worst case per call is `tiers × (1 + num_retries) × request_timeout = 5 × 2 × 60 = 600s`,
documented in `gateway/config.yaml`'s own comment. Eight rounds against that worst case is **eighty
minutes** of provider time inside one exchange. Every current caller does run under a queue deadline
(`AI_TASK_TIMEOUT_*`, 5–15 minutes), which is why the exposure is latent rather than live — but the
loop is a platform component and the next caller may not. Requiring `ctx.Deadline()` to be set makes
the bound explicit without adding a second timer that could disagree with the first. That second-timer
shape is what produced 030's 830-second hang, and avoiding it remains the right instinct; the original
just mistook "no second timer" for "bounded".

*Money.* `usageFrom` (`gateway.go:92`) already captures per-call cost from `x-litellm-response-cost`.
The original `Result` had no aggregate cost field, so an eight-round exchange against a premium tier
had no ceiling and no total. Both are added.

**Defaults**, revised down: **4 rounds** (not 8), per-lookup 10s, result 32 KB, spend ceiling
$0.50 per exchange. Four rounds is enough for the two-lookup shape SC-008 measures plus one recovery
from a refusal; eight was chosen because it is a round number.

**Alternatives rejected**:
- *Unbounded until the context expires*: a model looping on one cheap tool would burn the entire
  task deadline making requests, and the failure would read as "slow" rather than "looping".
- *A model-adjustable budget*: puts the fence under the control of the thing being fenced.
- *A `MaxWallClock` field on `Bounds`*: the second competing timeout, rejected on the 830-second
  precedent. Requiring a caller deadline achieves the bound without the second clock.

---

## R7 — Tool capability in routing configuration is **documentation, not enforcement**

**Decision**: Every tier of a tool-using task's chain carries `model_info.supports_function_calling`
in `gateway/config.yaml`, asserted by the existing config test. The annotation's status as metadata
is written down beside it, in the file and in the domain document.

**Corrected — the original decision claimed a control that does not exist.** The original text said
capability "becomes a declared, tested property of `gateway/config.yaml`, exactly as 035 had to do for
reasoning bounds", and FR-018 required every tier to "be declared tool-capable in configuration, or
the task must be ineligible for that tier" — phrasing that reads as though the proxy enforces
eligibility. It does not. Three verified facts:

1. **`drop_params: true` is set** (`gateway/config.yaml:213`). An upstream that does not accept
   `tools` gets the request with `tools` removed, and answers. Successfully. This is precisely the
   capability trap `specs/domains/llm-routing.md:118-125` already documents for `response_format` —
   "the fallback chain will not rescue that, because the request succeeded" — and 037 walked into the
   same trap one section further down the same file.
2. **`model_info: {supports_function_calling: true}` is real LiteLLM YAML**, and it is real *metadata*.
   It informs the proxy's model-info endpoint and cost/capability bookkeeping. It does not make the
   proxy refuse a request or skip a tier, and it does not stop `drop_params` from dropping.
3. **No `model_info` block exists anywhere in `gateway/config.yaml` today.** This feature introduces
   the first one, so there is no existing convention to follow — which is exactly why the annotation's
   meaning has to be written down rather than assumed.

So the annotation buys one real thing: a Go test can read it, and adding a tier without it fails that
test. That is worth having — it makes "did anyone think about tools when they added this tier?" a
build-time question. It buys nothing at runtime.

**The coupling nobody would notice.** Every chain terminates at the single shared `local` deployment
(030-FR-008). There is one `local` entry in the file, used by every task. Annotating one tool-using
chain therefore annotates the terminal tier **for every task in the system**, asserting the
self-hosted model is tool-capable everywhere. That is a claim about the deployed local model, not
about one chain, and it must be stated where it is made (FR-018a).

**Runtime backstop**: R12, which is the only mechanism that actually catches the drop.

**Alternatives rejected**:
- *Probe capability at startup*: extra live calls, staleness, and it still cannot cover a chain that
  fails over mid-request.
- *Set `drop_params: false` for tool-using tasks*: `drop_params` is a global `litellm_settings` key,
  not per-deployment. Turning it off globally changes behaviour for every existing call and would
  need its own feature and its own regression argument.
- *Silently fall back to a no-tool prompt*: the exact behaviour FR-017 forbids.

---

## R8 — First consumer: **salary estimation**

**Decision**: Convert `internal/salary/application` to the loop, with read-only lookups over the
salary comparables cache and the stored posting.

**Corrected — the original consumer was fabricated.** R8 named `internal/interviewprep` and described
converting `application/service.go` with lookups in `application/tools.go`. Verified:
`internal/interviewprep` contains exactly two files — `apiservice.go` (package `interviewprep`, 268
lines) and `interfaces/http/interviewprep.go` (39 lines). There is no `application/` package, no test
file, and **no LLM call of any kind**. It is constructed at `cmd/server/compose.go:490` as
`NewService(jobs, diffs, stories, news)` — no `llm.Router` — and runs a deterministic
`keyword.DeriveQuestions` + `keyword.SelectStories` pipeline behind `GET /jobs/{id}/interview-prep`,
which takes no question parameter and returns a structured `dto.InterviewPrepPackDto`.

Every rationale R8 gave was therefore invented: there was no "prepare me for this interview" prompt to
convert, no lookups to add beside an existing model call, and no test suite to protect. The mistake
propagated into plan.md's file tree, contracts §7 and seven tasks (T038–T044), all naming files that
do not exist. It was never checked because it sounded right.

**Salary, verified before choosing it** (`internal/salary/`):

| Requirement | Evidence |
|---|---|
| Holds an LLM provider | `application/service.go:27` — `llmc llm.Provider`, injected at `cmd/server/compose.go:357` as the `default` router |
| Already makes a structured call | `service.go:129` — `llm.CompleteStructured[domain.SalaryBand]` |
| Has a real test suite to protect | `application/service_test.go`, plus `internal_test.go`, `integration_test.go`, `live_test.go` |
| Read-only in the tool sense | Its two writes — `UpdateJobSalary`, `UpsertSalaryCache` — happen in `Infer`, *after* and *outside* the model call. The lookups wrap only `GetSalaryCacheByBucket` and `GetJobByID` |
| Genuinely benefits from lookups it cannot be handed up front | See below |
| Its return type is a natural `T` | `domain.SalaryBand` has `jsonschema` tags and implements `Validate()`, so FR-023's terminal step exercises strict schema **and** the `Validator` hook |

*Why the lookups cannot be handed up front.* `Infer` reaches the model only after every deterministic
route misses: `salaryRaw` did not parse, and the cache has no row for **this exact bucket**
(`title|location|size`, `service.go:214`). At that point the useful information is in *neighbouring*
buckets — a different seniority of the same title, the same title in a nearby market — and which
neighbour is apt depends on reading the posting. The caller cannot enumerate them in advance without
guessing; the model can ask. The second lookup is the posting itself, which the current prompt
truncates to 4000 characters (`service.go:118`) whether or not the salient text survives the cut.

That is a real two-lookup shape, which is what SC-008 measures, and it is the shape the original
consumer only claimed to have.

**Alternatives considered** (all four read, not recalled):
- *`ghostjob/application`*: holds a provider and has tests, but `Score` is a single-shot judgement over
  a posting it is already handed whole. Nothing to look up.
- *`matching/application`*: holds a provider, but its inputs are the job and the profile, both already
  loaded, and it is the hottest LLM path in the system (one call per job). Putting a multi-round loop
  on the highest-volume path to prove a capability is the wrong first consumer.
- *`outreach/application`*: read-only by constitutional mandate and well tested — but it is
  *the* Principle I surface, guarded by its own no-send test. Making the first tool loop's consumer
  the one package whose entire reason for existing is "must never send" adds risk for no gain.
- *`recruiter/application`*: three call sites and genuinely multi-step, but its lookups reach outward
  to scraping and third-party sources, entangling the first proof of the loop with the least reliable
  subsystem in the tree — and, after R5's corrected forbidden list, with a package the fence forbids.
- *No consumer, ship the capability alone*: rejected in clarification. An unexercised loop is an
  unverified loop.

**Consequence for the blast radius**: `salary.fakeLLM` (`application/service_test.go:50`) is both one
of the twelve fakes that must gain the new method (R13) and part of the consumer's own suite. That
overlap is convenient, not a problem — but it means the consumer's tests cannot be treated as an
independent check of the interface change.

---

## R9 — Streaming stays out

**Decision**: Every call remains a single complete response.

**Rationale**: Nothing in the platform consumes tokens incrementally — generation writes documents,
matching scores jobs, both behind a queue. Streaming would add a second response path through the
adapter, the router and the loop for no current consumer.

**Alternatives rejected**: adding streaming "while we are in here". A second code path with no user
is a second code path to keep correct.

---

## R10 — The loop terminates into a **typed** result, not a string

**Decision**: `Run[T]` is generic. Lookup rounds go through `CompleteChat`; the terminal step goes
through the structured path, producing the caller's own `T` with strict schema, parse-and-retry and
`Validator` all applying.

**Corrected — `Result{Content string}` had no consumer in this codebase.** The original data model
returned a bare string. Every LLM surface here consumes `CompleteStructured[T]`: generation
(`VacancyAnalysis`, `TailoredSelection`, `TailoredSummary`), matching (`FitResult`), ghostjob
(`GhostJobResult`), outreach (`DraftOutput`), recruiter (`extractedContact`, `extractedContactList`),
salary (`SalaryBand`). Fourteen call sites, zero of which can use a string. A loop returning
`Content string` could not have been wired into any consumer without the consumer re-parsing prose —
which is the exact failure mode 033 exists to eliminate.

This is why the return type and the consumer choice are one decision, not two: `T` is the consumer's
existing return type. For salary, `T = domain.SalaryBand` and the converted service's signature does
not change (FR-023a).

**Mechanism**: `CompleteStructured[T]` today takes a `prompt string` and builds its own single-message
request. It gains a conversation-shaped sibling that takes `[]Message` and shares the *same* body —
same `schemaFor`, same fence stripping, same `structuredRetries` count, same `Validator` invocation.
`CompleteStructured` becomes the sibling called with `[system?, user]`, which is the same shim
discipline R3 applies one layer down, and it is covered by the same golden test.

**What the loop does with prose**: nothing. A model that answers in prose without calling a tool on
round one is FR-017's `not_tool_capable`. A model that stops calling tools on a later round triggers
the terminal structured step against the accumulated conversation. Prose is never returned as an
answer (FR-023b).

**Alternatives rejected**:
- *Return `Content string` and let each consumer parse*: reintroduces the pre-033 world for every
  loop caller.
- *Two entry points, `Run` for text and `RunStructured[T]` for types*: a text entry point with no
  consumer is dead code with a maintenance cost, and it is the one someone would reach for first.

---

## R11 — Tool-call identity: the OpenAI encoding trap, and the Ollama id gap

**Decision**: The adapter normalises both providers into the domain's `ToolCall{ID, Name, Arguments}`.
Specifically: the gateway adapter **decodes** the arguments string before storing it, and the Ollama
adapter **synthesises** ids.

**Corrected — two provider details the original data model glossed.**

*Arguments are a JSON-encoded string, not an object.* An OpenAI-compatible response carries
`"arguments": "{\"bucket\":\"senior-engineer|berlin|unknown\"}"` — a JSON **string** whose contents
are JSON. Unmarshalled into `json.RawMessage` naively it yields a quoted string, and validating that
against an object schema refuses every well-formed call. The original data model's framing —
"`Arguments json.RawMessage` … raw, unvalidated. Validation happens in the registry" — actively
invites the naive implementation, and FR-009's refusal path is where the bug would surface, as a
false refusal that looks like the fence working correctly. The adapter now unquotes before storing, so
`Arguments` always holds an object; if unquoting fails, the raw bytes are stored and the registry
refuses, preserving FR-009 for genuinely malformed input.

*Ollama assigns no ids.* Its native format returns `message.tool_calls[].function` with `name` and
`arguments` and no id field. But C1-7 requires every `tool` message to carry the id of the request it
answers, and nothing in the original documents authorised inventing one. The Ollama adapter now
assigns `call_<round>_<index>` — deterministic, unique within an exchange, and never sent back to
Ollama, which does not read it. The gateway adapter preserves the provider's id verbatim.

**Alternatives rejected**:
- *Match results to requests positionally on the Ollama path*: works until one call in a multi-call
  turn is refused and the positions shift.
- *Drop the id requirement for Ollama*: the loop would need two matching strategies, and the terminal
  tier is the one path that must not be the special case.

---

## R12 — Capability detection: require a tool call on round one

**Decision**: The exchange sends `tool_choice: "required"` on its first round. A first response with
no tool call means the tool declaration did not reach a model that could act on it → stop with
`not_tool_capable`, no answer returned. Subsequent rounds use `tool_choice: "auto"`.

**Corrected — the original mechanism violated the routing contract and could not distinguish the
case it was meant to catch.** R7's runtime backstop said: "if a response contains no tool call when
the loop required one **and the serving model is not the expected one**, fail". Two problems:

1. **The application has no expected model to compare against.** `specs/domains/llm-routing.md`
   (030-FR-004) states the application requests AI work by task name only, carrying no provider or
   model identity, and 030-FR-012 confines what it learns to a `served_model` log line. Building
   control flow on "is the served model the one we expected" reintroduces exactly the per-task model
   knowledge 030 removed, and § 5 ends with "do not reintroduce provider/model selection".
2. **Under `tool_choice: auto` the signal is not there.** A model that legitimately decided it could
   answer without a lookup produces the same response shape as a model whose `tools` array was dropped
   by the proxy: no tool calls. No comparison recovers the difference.

Requiring a tool call removes the ambiguity: under `required`, a tool-capable model **must** emit one,
so its absence is diagnostic. It works without knowing which upstream answered, which keeps the
routing contract intact.

**The cost, stated** (FR-017a): round one always performs at least one lookup, even when the model
could have answered from the prompt. For salary that is not waste — the loop is only reached after
every deterministic route missed, so a comparables lookup is always worth making. For a future
consumer where it is waste, that consumer should not use the loop.

**Alternatives rejected**:
- *Compare against the expected model*: see above.
- *A probe call before the exchange*: doubles latency and cannot cover a mid-request failover.
- *Trust the config annotation*: R7 — it is metadata, and `drop_params` overrides intent silently.

---

## R13 — Blast radius of the interface change: twelve implementors

**Decision**: Adding `CompleteChat` to `domain.Provider` breaks twelve types at compile time. That
compile break is the intent — it is what stops a fake from silently not implementing the seam — and
each is fixed deliberately rather than by weakening the interface with an embedded default.

Enumerated against the tree, all under `apps/api/internal/`:

| # | Type | Location | Kind |
|---|---|---|---|
| 1 | `gateway.Provider` | `platform/llm/infrastructure/gateway/gateway.go:21` | production |
| 2 | `ollama.Provider` | `platform/llm/infrastructure/ollama/ollama.go:19` | production |
| 3 | `application.Router` | `platform/llm/application/router.go:20` | production |
| 4 | `domain.fakeProvider` | `platform/llm/domain/port_test.go:16` | test fake |
| 5 | `application.stubProvider` | `platform/llm/application/router_test.go:11` | test fake |
| 6 | `generation.stageProvider` | `generation/application/stage_routing_test.go:17` | test fake |
| 7 | `ghostjob.fakeLLM` | `ghostjob/application/service_test.go:82` | test fake |
| 8 | `matching.noopLLM` | `matching/application/integration_test.go:20` | test fake |
| 9 | `outreach.fakeLLM` | `outreach/application/service_test.go:41` | test fake |
| 10 | `recruiter.fakeLLM` | `recruiter/application/posting_test.go:16` | test fake |
| 11 | `recruiter.multiSourceFakeLLM` | `recruiter/application/service_test.go:25` | test fake |
| 12 | `salary.fakeLLM` | `salary/application/service_test.go:50` | test fake **and** part of the consumer's suite (R8) |

**Alternatives rejected**: a separate `ChatProvider` interface the gateway and ollama adapters
implement and fakes do not. It avoids the compile break, but then `toolloop.Run` takes a different
type than `Router` satisfies, and the router — the only thing consumers hold — would need its own
assertion path. The compile break is the cheaper honesty.

---

## Resolved unknowns summary

| Unknown | Resolution |
|---|---|
| Framework or hand-rolled | Hand-rolled; `langchaingo` is a downgrade against 033/035 (R1) — the one original decision that held |
| Message shape | OpenAI-style roles for the domain; Ollama's native shape is an adapter concern (R2) |
| Backward compatibility | `Complete`/`CompleteJSON` shim onto `CompleteChat`, proven by golden tests in **both** adapters against seven named differences (R3) |
| Tool argument schemas | Existing `invopop/jsonschema` + `strictifySchema` path (R4) |
| Read-only enforcement | Transitive closure via `go list -deps` through `os/exec` — no new module. The cited precedents were a direct-import scan and a string grep, neither a transitive walk (R5) |
| Loop bounds | Caller config; 4 rounds, 10s per lookup, 32 KB, $0.50 ceiling; caller deadline **required**, no second timer (R6) |
| Tool capability in config | An annotation a **test** reads, not a control the proxy enforces. `drop_params: true` drops `tools` silently (R7) |
| First consumer | **Salary estimation.** The original consumer, interview preparation, does not exist (R8) |
| Streaming | Out of scope (R9) |
| What the loop returns | A typed `T` through the structured path — `Result{Content string}` had no consumer here (R10) |
| Tool-call identity | Gateway arguments are an encoded string and must be decoded; Ollama assigns no ids and the adapter must (R11) |
| Detecting a non-tool-capable server | `tool_choice: "required"` on round one; no tool call means not capable. Comparing served models is forbidden by the routing contract (R12) |
| Interface blast radius | Twelve implementors, 3 production and 9 fakes, all enumerated (R13) |

## Corrections log

This research was rewritten on 2026-08-07 after an audit checked its claims against the tree, the
proxy configuration and both providers' wire formats. Six decisions were materially wrong.

1. **R8 named a consumer that does not exist.** `internal/interviewprep/application` is not a package.
   The real `internal/interviewprep` has no LLM call at all, no test file and no service to convert.
   Its rationale, its three lookups, plan.md's file tree, contracts §7 and tasks T038–T044 all
   described editing files that were never there. The consumer is now `internal/salary/application`,
   chosen after reading it and four alternatives.

2. **R7 claimed configuration enforces tool capability.** `drop_params: true` is set in
   `gateway/config.yaml:213`, so an upstream that cannot take `tools` receives the request without it
   and answers successfully. `model_info.supports_function_calling` is real LiteLLM YAML but it is
   metadata a Go test can read, not a control the proxy applies — and no `model_info` block exists in
   the file today. This is the same capability trap `specs/domains/llm-routing.md:118-125` already
   documents for `response_format`, one section above where 037 walked into it.

3. **R7's runtime backstop required knowledge the application is forbidden to have.** It compared the
   served model against an expectation; 030-FR-004 states the application never learns which upstream
   served a request. It also could not distinguish a dropped `tools` array from a model that simply
   chose not to look anything up. Replaced by `tool_choice: "required"` on round one (R12).

4. **The loop returned a string.** No consumer in this codebase can use one — all fourteen structured
   call sites go through `CompleteStructured[T]`. The loop is now generic and terminates through the
   structured path (R10), which is also what ties the return type to the consumer choice.

5. **R5's "established idiom" was not the mechanism claimed.** `internal/arch_test.go` is a
   direct-import scan against one constant using `parser.ImportsOnly`; `outreach/nosend_test.go` is a
   string-token grep that never touches imports. Calling them precedent for a transitive import walk
   made an invented mechanism sound settled. The forbidden list also omitted `internal/retrieval`,
   which drives a headless browser and calls FlareSolverr — a lookup importing it passed the fence and
   reached the internet.

6. **R3's counts were wrong and its risk ranking mis-ordered.** "Thirteen packages call the structured
   path and four call `Complete`" is actually six packages over fourteen call sites, and two direct
   `Complete` callers. More seriously, the named regression risk — the temperature split — is not the
   largest: ollama's `Complete` sends `num_predict` and its `CompleteJSON` does not, so a shared helper
   changes the wire on the terminal tier, and the golden test was scoped to the gateway adapter only,
   leaving that path uncovered by the check meant to prove SC-001.

Two smaller ones, folded into R6 and R11 rather than listed as decisions overturned: there was no
bound on total spend or wall time despite an eighty-minute worst case, and the tool-call encoding and
id gaps were glossed as "raw, unvalidated" in a way that invited the bug.

**The pattern worth keeping**, and it is the same one 036's audit found: every one of these was a
confident claim about this repository's own layout, about a vendor's behaviour, or about a precedent
in this tree — made without opening the file. The consumer is the worst case, because five documents
of design were built on top of a package that was never once read.
