# Phase 1 Contracts: Multi-Turn Conversations and a Typed Tool Loop

**Feature**: `037-llm-chat-tool-loop` | **Date**: 2026-08-07

**Revised**: 2026-08-07 after audit. §4's loop returned a string, §5's fence claimed a precedent that
does not exist, §6 asserted proxy behaviour that was never verified, and §7 contracted a consumer that
does not exist. See research.md's corrections log.

Eight contracts. No HTTP API changes; the converted consumer is a background worker with no endpoint
of its own.

---

## 1. The `Provider` interface

```go
type Provider interface {
    ModelName() string
    Complete(ctx context.Context, prompt string, opts *CompleteOptions) (string, error)
    CompleteJSON(ctx context.Context, prompt string, opts *CompleteOptions) (string, error)
    CompleteChat(ctx context.Context, msgs []Message, opts *CompleteOptions) (ChatResult, error)
    Embed(ctx context.Context, text string) ([]float32, error)
}
```

- **C1-1**: `Complete` and `CompleteJSON` MUST keep their exact signatures. No call site may be
  required to change (FR-002, FR-003).
- **C1-2**: **Corrected in scope.** Seven differences between the existing entry points MUST survive,
  each with its own assertion. The temperature split is one of them and is **not** the largest risk:

  | # | Invariant | Source |
  |---|---|---|
  | 1 | ollama `Complete` sends `num_predict` when `MaxTokens` is set; ollama `CompleteJSON` MUST send **no** `num_predict`, ever | `ollama.go:143-145` vs `:156-162` |
  | 2 | gateway `CompleteJSON` MUST set `response_format` on **every** call, including `opts == nil`; `Complete` MUST never set it | `gateway.go:205-235` |
  | 3 | The strict-schema retry MUST fire on `ErrModelUnavailable` **and** on `ErrInvalidResponse` — the latter raised by an unparsable 200 body and by zero choices, not only by a 400/422 | `gateway.go:157`, `:161`, `:246` |
  | 4 | One `CompleteJSON` call that takes the retry MUST continue to emit **two** `logServed` lines, **two** `ReportServedModel` calls and **two** `ReportUsage` calls | `gateway.go:243-247` |
  | 5 | A schema-parse failure MUST continue to disable strict mode **and** skip the retry | `gateway.go:228-233` |
  | 6 | ollama `CompleteJSON` MUST set `Format: "json"`; `Complete` MUST not | `ollama.go:159` |
  | 7 | `Complete(ctx, prompt, nil)` and `CompleteJSON(ctx, prompt, nil)` MUST not panic | `cmd/llmsmoke/main.go:37` |

  Rows 1 and 6 are Ollama-side, which is why C2-9 extends the golden test to that adapter. Row 1 in
  particular is a wire change on the terminal tier every chain ends at.
- **C1-3**: `Complete` MUST emit `[system?, user]` in that order, omitting the system message
  entirely when the system prompt is empty — not sending an empty one.
- **C1-4**: `CompleteStructured[T]` MUST keep its signature and behaviour: same schema generation,
  same fence stripping, same `structuredRetries` count of 2, same `Validator` invocation, same
  immediate propagation of provider errors (FR-004).
- **C1-5**: Every implementation of `Provider` MUST implement `CompleteChat`. There are **twelve** —
  3 production (`gateway.Provider`, `ollama.Provider`, `application.Router`) and 9 test fakes, all
  enumerated in research R13. A fake that does not implement it is a compile error, which is the
  intent. The interface MUST NOT be softened with an embedded default to avoid the break.
- **C1-6**: `CompleteChat` MUST NOT mutate the caller's message slice, and MUST NOT reorder, merge or
  drop turns (FR-005).

### Message-ordering rules

- **C1-7**: A `tool` message MUST carry the `ToolCallID` of the request it answers. Where the provider
  supplies no id — Ollama's native format has no id field — the **adapter** MUST assign one (C2-10).
  This was previously required without authorising any mechanism to satisfy it on the local path.
- **C1-8**: An `assistant` message with tool calls and empty content MUST be preserved, not dropped
  as "empty".
- **C1-9**: The conversation MUST be sent in slice order. No stack, no reversal, no dedup.

### The typed terminal

- **C1-10**: `CompleteStructuredChat[T](ctx, p, msgs, opts) (T, error)` MUST share the body of
  `CompleteStructured[T]` — the same schema cache, strictification, fence stripping, retry count and
  `Validator` assertion. Two independent structured paths are forbidden; they would drift exactly as
  a second schema generator would (C2-2's reasoning, applied one layer up).
- **C1-11**: `CompleteStructured[T]` MUST become `CompleteStructuredChat[T]` called with
  `[system?, user]`, and MUST be covered by the same golden comparison as the other shims (SC-001).

---

## 2. Wire format — gateway adapter

When `opts.Tools` is non-empty, the outbound `/chat/completions` body gains:

```json
{
  "model": "<task key>",
  "messages": [ ... ],
  "tools": [
    { "type": "function",
      "function": { "name": "lookup_comparable_bands",
                    "description": "...",
                    "parameters": { "...strict schema..." } } }
  ],
  "tool_choice": "required"
}
```

- **C2-1**: When `opts.Tools` is empty, `tools` and `tool_choice` MUST be **absent** — not `null`,
  not `[]`. The body MUST be byte-identical to today's (SC-001).
- **C2-2**: Tool parameter schemas MUST come from the existing `schemaFor` + `strictifySchema` path,
  not a second schema generator (research R4).
- **C2-3**: `tool_choice` MUST be omitted when `ToolChoice` is empty, letting the provider default
  stand.
- **C2-4**: Tool calls in the response MUST be read from the choice's `message.tool_calls`, with the
  provider-assigned id preserved verbatim for C1-7.
- **C2-5**: The existing `response_format` handling MUST be unaffected. Declaring tools and
  requesting strict JSON are independent.
- **C2-6**: `WithServedModelCapture` and usage reporting MUST fire on `CompleteChat` exactly as they
  do on `Complete` today. A tool-using call that loses served-model visibility breaks 035 and 036.
- **C2-11**: **NEW — `chat()` must be split before `CompleteChat` can exist.** Today
  `func (g *Provider) chat(ctx, req chatRequest) (string, error)` (`gateway.go:120`) performs the
  request, classifies errors, logs, reports served model and usage, and then discards everything
  except `Choices[0].Message.Content`. `chatResponse` declares no `tool_calls` and no `finish_reason`,
  so `ChatResult.FinishReason` currently has no source at all. The required change:
  1. `chatResponse` gains `choices[].message.tool_calls[]` and `choices[].finish_reason`.
  2. `chat()` splits into a lower half returning the parsed response plus headers, and a thin wrapper
     preserving today's `(string, error)` for `Complete`/`CompleteJSON`.
  3. Error classification, logging and both capture hooks stay in the lower half so every path keeps
     them and C1-2 row 4's double-report survives.
- **C2-12**: **NEW — arguments MUST be decoded before they leave the adapter.** The response encodes
  `arguments` as a JSON **string** containing JSON. The adapter MUST unquote it so `ToolCall.Arguments`
  holds an object (FR-009a). If unquoting fails, the raw bytes MUST be stored unchanged so the
  registry's refusal path (C4-12) still receives genuinely malformed input. Storing the quoted string
  would cause **every** well-formed call to be refused.

### Ollama adapter

- **C2-7**: The ollama adapter MUST implement `CompleteChat` using Ollama's native tool format and
  MUST preserve the same absent-when-empty rule as C2-1. `chatResponse` gains `message.tool_calls[]`
  and `done_reason`; it declares only `Message.Content` today.
- **C2-8**: When the local model cannot call tools, the adapter MUST surface that as a
  distinguishable condition, not as an empty response (FR-019). In practice the loop's `required`
  first round (C4-14) is what detects it; the adapter must not mask it by fabricating a tool call.
- **C2-9**: **NEW — the golden request comparison MUST cover this adapter too.** C1-2 rows 1 and 6 are
  Ollama-side, and row 1 is a wire change on the terminal tier. A gateway-only golden test does not
  prove SC-001.
- **C2-10**: **NEW — the adapter MUST synthesise tool-call ids.** Ollama's format supplies none. Ids
  MUST be unique within an exchange, stable for the request that carries the result back, and MUST NOT
  be sent to Ollama, which does not read them.

---

## 3. Router

- **C3-1**: `Router.CompleteChat` MUST resolve provider and model exactly as `Complete` does, via the
  existing `resolve()` and `withModel`, with no new routing logic.
- **C3-2**: `Router` MUST NOT inspect, filter or rewrite tool declarations. It is a passthrough.
- **C3-3**: `Router.CompleteChat` MUST NOT choose a provider based on tool capability. Doing so would
  make the application aware of upstream capabilities, which 030-FR-004 forbids.

---

## 4. The loop

```go
func Run[T any](ctx context.Context, p domain.Provider, ts *Toolset,
                msgs []domain.Message, opts *domain.CompleteOptions, b Bounds) (Result[T], error)
```

- **C4-0**: **Corrected — `Run` is generic and returns a typed value.** The previous signature
  returned `Result{Content string}`, which no consumer in this codebase can use: all fourteen
  structured call sites go through `CompleteStructured[T]` (research R10). `Result[T].Value` is the
  caller's own result type.
- **C4-1**: `Run` MUST stop at exactly `b.MaxRounds` rounds — never one more — returning
  `StopReason: max_rounds` (FR-010, SC-003).
- **C4-2**: `Run` MUST honour `ctx` expiry, start no further lookup after it, and return
  `StopReason: deadline` (FR-011, SC-004).
- **C4-3**: `Bounds` MUST NOT contain an overall-deadline field. `ctx` is the single deadline; a
  second competing timeout is forbidden by design (research R6).
- **C4-3a**: **NEW —** `Run` MUST return an error before issuing any request when `ctx.Deadline()`
  reports no deadline (FR-011a). "No second timer" is not the same as "bounded": the proxy's own
  documented worst case is 600s per call, so four rounds without a caller deadline is forty minutes.
- **C4-4**: All tool calls in one assistant turn MUST be dispatched and MUST count as one round
  (FR-015).
- **C4-5**: Each dispatch MUST be bounded by `b.PerToolTimeout`, independently of `ctx` (FR-012).
- **C4-6**: A refused, failed, timed-out or truncated call MUST become a `tool` message the model can
  react to. None may abort the exchange (FR-013).
- **C4-7**: A result exceeding `b.MaxResultBytes` MUST be truncated **and** the truncation MUST be
  stated in the message content. Silent truncation is forbidden (FR-014).
- **C4-8**: `Run` MUST populate a `RoundRecord` per round with each call's name, outcome and duration,
  **plus the served model and the cost of that round's provider call** (FR-016, SC-010). Both come
  from `WithServedModelCapture` and `WithUsageCapture`, which the gateway adapter already reports to
  on every call. A record without them makes a multi-round exchange invisible to 035 and 036.
- **C4-9**: No value in `Bounds` may be derived from model output or prompt content (research R6).
- **C4-10**: A response with both content and tool calls MUST be treated as not final: the calls are
  honoured and the exchange continues.
- **C4-14**: **NEW —** round one MUST send `tool_choice: "required"`; later rounds MUST send `"auto"`
  (research R12, FR-017).
- **C4-15**: **NEW —** the exchange MUST accumulate cost across rounds and stop with
  `StopReason: cost_ceiling` when it exceeds `b.MaxTotalCostUSD` (FR-016a).
- **C4-16**: **NEW —** `Result[T].Value` MUST be meaningful only when `StopReason == answered`. Every
  other stop reason MUST return the zero `T` **and** a non-nil error, so no caller can mistake a
  truncated exchange for an answer.

### Terminal step

- **C4-17**: When a round after the first returns no tool calls, `Run` MUST produce its answer by
  calling `CompleteStructuredChat[T]` over the accumulated conversation (FR-023). It MUST NOT return
  the model's prose.
- **C4-18**: A failure of the terminal step after its own retries MUST fail the exchange (FR-023b).
  There is no prose fallback.
- **C4-19**: The terminal step MUST be given the same `CompleteOptions` strictness the caller
  requested, so `ResponseModeStrict` and the `Validator` hook apply to the loop's answer exactly as
  they apply to a single structured call today.

### Toolset

- **C4-11**: An unknown tool name MUST be refused without dispatch, and the refusal returned as a
  `tool` message (FR-009, SC-006).
- **C4-12**: Arguments failing schema validation MUST be refused without dispatch, with the validation
  reason in the refusal message — **after** the adapter's decoding step (C2-12), never against the
  wire encoding.
- **C4-13**: Tool names MUST be unique within a toolset; registering a duplicate MUST fail at
  construction, not at call time.
- **C4-20**: **NEW —** a `Toolset` MUST be immutable after construction, and `Bounds` MUST be captured
  before the first request. Neither may be re-read from the conversation (FR-026).

### Untrusted tool output

- **C4-21**: **NEW —** every `tool` message MUST carry its result wrapped in an unambiguous delimiter
  the result's own bytes cannot close, and the exchange's system framing MUST state that content
  inside a result is data and never an instruction (FR-024, FR-025).
- **C4-22**: **NEW —** no content of a tool result may alter the toolset, any bound, the round count,
  the tool choice, or the schema the answer must satisfy (FR-026). These are fixed before the first
  request.
- **C4-23**: **NEW —** a result matching the injection heuristic MUST set `SuspectedInjection` on that
  round's record (FR-027). This is a **detector, not a filter**: it records, it does not sanitise, and
  the contract must not be read as a claim that injected content is removed.

---

## 5. The read-only fence (FR-008, SC-005)

**Corrected in mechanism, scope, location and enforcement.** The previous version required "any
transitive import" on the strength of two precedents that perform no transitive resolution
(research R5).

- **C5-1**: The check MUST resolve each lookup package's **transitive** dependency closure with
  `go list -deps` invoked through `os/exec`, and MUST fail when the closure contains a forbidden
  package. `golang.org/x/tools/go/packages` MUST NOT be used: it is not in `apps/api/go.mod` and
  adding it would break SC-009 with the control that protects a constitutional principle.
- **C5-1a**: If `go list` cannot be invoked, the test MUST **fail**, not skip. A fence that switches
  itself off in an unusual environment is a fence that is off.
- **C5-1b**: The forbidden set MUST be `internal/notifier`, `internal/outreach`, `internal/postage`,
  the write paths of `internal/applications`, **`internal/retrieval`** and **`internal/jobsources`**.
  The last two were missing: `internal/retrieval` performs outbound HTTP, drives a headless browser
  and calls FlareSolverr, so a lookup importing it passed the original fence and reached the open
  internet from inside a model's decision loop (FR-008b).
- **C5-2**: The test MUST fail closed, by a **stated mechanism** rather than as an aspiration:
  1. **Discovery**: walk `internal/` with `parser.ImportsOnly` and collect every package that directly
     imports the toolloop package. A package that registers tools must import it, so this finds all of
     them — and direct-import scanning is exactly what `internal/arch_test.go` does well.
  2. **Declaration**: an explicit list in the test, in the style of `gateway_config_test.go`'s
     `requestedGenerationGroups`.
  3. **The rule**: the two sets MUST be equal. A discovered package absent from the list fails; a
     listed package no longer discovered fails too.
- **C5-3**: The test file MUST document all **three** limits, not one (FR-008c):
  1. It cannot catch a lookup that builds its own outbound request from `net/http`.
  2. **It cannot catch a closure over an already-injected capability.** Handlers are closures; one
     defined inside a service that already holds an outreach client can call it while the lookup's own
     package imports nothing. This is the largest hole and it makes the small-enumerated-reviewed
     toolset a required complementary control, not a reassurance.
  3. It sees packages, not call paths.
- **C5-4**: This test MUST land in the same change as the loop. A loop merged without its fence is a
  Principle I violation in the tree, however briefly.
- **C5-5**: **NEW — location.** The test MUST live at `apps/api/internal/toolfence_test.go`, beside
  `internal/arch_test.go`. It MUST NOT live in `platform/llm/application/toolloop/`, where it would
  force the platform package to enumerate its consumers' tool packages — contradicting plan.md's own
  structure decision that "the platform layer never learns what a job posting is".
- **C5-6**: **NEW — enforcement is real but its trigger has a hole.** `.github/workflows/api-ci.yml`
  runs `go test ./...` in `apps/api` under the `go-test` job, so this test does fail CI for changes
  under `apps/api/**`. That job is gated on a `dorny/paths-filter` whose `go` filter matches
  `apps/api/**`, `scripts/sqlc-check.sh` and `scripts/tygo-check.sh` — **not `gateway/`**. A pull
  request that only edits `gateway/config.yaml` skips `go-test` entirely, so C6-2's config assertion
  (and 035's existing chain-termination guardrail) never runs. Adding `gateway/**` to that filter is
  part of this feature (FR-030).

---

## 6. Routing tool-capability contract — `gateway/config.yaml` (FR-018)

**Corrected — this contract previously described the annotation as a control. It is documentation.**

```yaml
  - model_name: default
    litellm_params:
      model: cerebras/<verified>
      api_key: os.environ/CEREBRAS_API_KEY
    model_info:
      # 037: DOCUMENTATION, NOT ENFORCEMENT. LiteLLM reads model_info for its
      # model-info endpoint and cost bookkeeping; it does NOT refuse a request,
      # skip a tier, or stop drop_params from dropping an unsupported `tools`
      # array. This line exists so gateway_config_test.go can assert that
      # somebody considered tool capability when adding this tier. The only
      # mechanism that catches a dropped `tools` array at runtime is the loop's
      # required first round (C6-4).
      supports_function_calling: true
```

### What is true, verified

- **C6-0a**: `drop_params: true` is set (`gateway/config.yaml:213`). An upstream that does not accept
  `tools` receives the request **without** `tools` and answers successfully. This is the identical
  capability trap `specs/domains/llm-routing.md:118-125` already documents for `response_format` —
  "the fallback chain will not rescue that, because the request succeeded".
- **C6-0b**: `model_info: {supports_function_calling: true}` is valid LiteLLM YAML and is real
  metadata. It is **not** a control the proxy enforces.
- **C6-0c**: **No `model_info` block exists anywhere in `gateway/config.yaml` today.** This feature
  introduces the first one, so there is no established convention and the comment above is required,
  not decorative.

### Binding rules

- **C6-1**: Every deployment in a tool-using task's fallback chain MUST carry a
  `model_info.supports_function_calling` declaration. Its value MUST reflect a verified property of
  that upstream model, pinned with a dated comment in the style of the existing model-ID verification
  note.
- **C6-2**: A config test MUST assert C6-1 for every tool-using task key, in the same file and style
  as `checkChainsTerminateAtLocal` in `apps/api/internal/platform/llm/gateway_config_test.go`. That
  file and that function both exist and are the correct precedent.
- **C6-3**: The chain MUST still terminate at `local` (Principle V). If the local model is not
  tool-capable, the terminal behaviour is an explicit failure (FR-019), never an answer.
- **C6-3a**: **NEW —** there is exactly **one** `local` deployment, shared by every task
  (030-FR-008). Declaring a tool-using chain therefore declares the shared terminal tier tool-capable
  **for every task in the system**. That is a claim about the deployed local model, not about one
  chain, and the comment at the `local` deployment MUST say so (FR-018a).
- **C6-4**: Runtime backstop (FR-017, SC-007): the loop's first round sends `tool_choice: "required"`;
  a first response with no tool call MUST return `StopReason: not_tool_capable` with a reason naming
  the limitation, and MUST return no answer. **The previous C6-4 required "a reason naming the serving
  model" — that is not available.** The application never learns which upstream served a request
  (030-FR-004), and reintroducing that knowledge contradicts `specs/domains/llm-routing.md` § 5's
  standing instruction. The reason names the task key and the limitation, not the upstream.
- **C6-5**: Adding a tier to a tool-using chain without a capability declaration MUST fail the config
  test — subject to C5-6, which is why the CI path filter must be widened in the same change.

---

## 7. Consumer contract — salary estimation (FR-021, FR-022)

**Corrected — this section previously contracted `internal/interviewprep`, a package with no
`application/` directory, no test file and no LLM call.** Its C7-1 protected an endpoint shape that
was never going to change because nothing was being converted, and C7-4 pointed at "existing tests"
that do not exist. See research R8.

The consumer is `internal/salary/application`.

- **C7-1**: `llmInfer`'s signature MUST be unchanged: `(domain.SalaryBand, error)`. The conversion is
  internal to that method (FR-023a).
- **C7-2**: Its toolset MUST contain only reads: `lookup_comparable_bands` (reads
  `GetSalaryCacheByBucket`) and `get_posting_details` (reads `GetJobByID`). The service's two writes —
  `UpdateJobSalary` and `UpsertSalaryCache` — MUST stay in `Infer`, outside the model call, exactly
  where they are today.
- **C7-3**: When the exchange stops for any reason other than `answered`, `llmInfer` MUST return an
  error and `Infer` MUST persist **nothing** (FR-022). A conversion that falls back to a
  low-confidence band would write a Principle II fabrication to the database.
- **C7-4**: Every currently passing case in `salary/application/service_test.go` MUST still pass,
  including the four paths that never reach a model: `salaryRaw` parsing, an ingested-cache hit, a
  levels.fyi hit, and the blend of the two.
- **C7-5**: `T` MUST be `domain.SalaryBand`, whose `Validate()` MUST be exercised by the terminal
  step — that is what proves C4-19's claim that the `Validator` hook survives the loop.
- **C7-6**: `salary.fakeLLM` (`service_test.go:50`) is both one of the twelve fakes that must gain
  `CompleteChat` (C1-5) and part of this consumer's own suite. The consumer's tests therefore MUST NOT
  be counted as an independent check of the interface change.

---

## 8. CI enforcement contract (FR-030)

**New. No 037 document previously named a CI configuration**, while SC-005 claimed the fence "fails
the build".

- **C8-1**: The fence (C5-1) and the config assertion (C6-2) are `go test` cases run by the `go-test`
  job in `.github/workflows/api-ci.yml`.
- **C8-2**: That job's `dorny/paths-filter` `go` filter MUST be widened to include `gateway/**` in
  the same change. Without it, a pull request touching only `gateway/config.yaml` skips the Go test
  job and neither this feature's C6-2 nor 035's existing chain-termination guardrail runs.
- **C8-3**: Any statement in this feature's documents that a check "fails the build" MUST name the
  job that runs it. A guardrail whose CI trigger nobody checked is a guardrail whose status is
  unknown.
