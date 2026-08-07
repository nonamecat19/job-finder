# Phase 1 Data Model: Multi-Turn Conversations and a Typed Tool Loop

**Feature**: `037-llm-chat-tool-loop` | **Date**: 2026-08-07

**Revised**: 2026-08-07 after audit. §4's `Result` returned a string no consumer here can use, §6
modelled types for a package that does not exist, and §7 claimed a precedent that is not one. See
research.md's corrections log.

No database change. No migration, no table, no column, no DTO, no tygo regeneration. Everything below
is an in-process Go type.

---

## 1. Conversation types — `internal/platform/llm/domain/message.go` (NEW)

### `Role`

A string enum with exactly four values: `system`, `user`, `assistant`, `tool`. Matching the format the
routing proxy already speaks (research R2) so no translation layer exists to drift. The **Ollama**
adapter translates to and from its own `/api/chat` shape; the domain type is not neutral between the
two and does not pretend to be.

### `Message`

| Field | Type | Meaning |
|---|---|---|
| `Role` | `Role` | who produced this turn |
| `Content` | `string` | the text. Empty is legal on an assistant turn that only requests tools |
| `ToolCalls` | `[]ToolCall` | present only on an `assistant` turn |
| `ToolCallID` | `string` | present only on a `tool` turn; identifies which request this answers |
| `Name` | `string` | present only on a `tool` turn; the lookup's name |

Ordering is significant and preserved exactly (FR-005). Nothing in the stack may merge adjacent
turns, drop an empty-content assistant turn that carries tool calls, or rewrite a role.

### `ToolCall`

| Field | Type | Meaning |
|---|---|---|
| `ID` | `string` | ties a later `tool` message back to this request. Provider-assigned on the gateway path, **adapter-assigned** on the Ollama path (§2.3) |
| `Name` | `string` | the declared lookup being requested |
| `Arguments` | `json.RawMessage` | the **decoded** argument object |

**Corrected — `Arguments` is decoded by the adapter, not carried raw.** The previous version said
"raw, unvalidated. Validation happens in the registry, not here", which sounds careful and produces a
bug. An OpenAI-compatible response encodes tool-call arguments as a JSON **string** containing JSON:

```json
"arguments": "{\"bucket\":\"senior-backend-engineer|berlin|unknown\"}"
```

Unmarshalled straight into `json.RawMessage` that is a quoted string, and validating it against an
object schema refuses **every well-formed call** — surfacing on FR-009's refusal path as a false
refusal that looks like the registry working correctly. The adapter therefore unquotes before storing,
so `Arguments` always holds an object (research R11, FR-009a).

The original intent — that the domain type can *carry* a malformed request so a refusal can be
produced from it — is preserved: when unquoting fails, the adapter stores the raw bytes as-is and the
registry refuses them. Malformed input still reaches the refusal path; well-formed input no longer
does.

### `ChatResult`

The return of `CompleteChat`.

| Field | Type | Meaning |
|---|---|---|
| `Content` | `string` | the assistant's text, possibly empty |
| `ToolCalls` | `[]ToolCall` | zero or more requests |
| `FinishReason` | `string` | provider-reported; the loop reads it to distinguish "done" from "cut off" |

**`FinishReason` has no source today and one must be added.** `gateway.chatResponse` declares only
`Model`, `Choices[].Message.Content` and `Usage` — no `finish_reason`, no `tool_calls`. Same for
`ollama.chatResponse`, which declares only `Message.Content`. Both response structs gain the fields
(§2.3, tasks T006/T008). Until they do, `FinishReason` would silently be `""` on every call, which is
the kind of field that reads as implemented and is not.

A result may carry both content and tool calls at once. The loop treats that as "not final" — the
lookups are honoured and the exchange continues. Prose is never an answer (FR-023b).

---

## 2. Tool declaration types — `internal/platform/llm/domain/tool.go` (NEW)

### `ToolDef`

| Field | Type | Meaning |
|---|---|---|
| `Name` | `string` | unique within a toolset |
| `Description` | `string` | what the model is told it does |
| `ArgsSchema` | `string` | marshalled strict JSON Schema for the arguments |

### `NewTool[T any](name, description string, handler func(ctx, T) (string, error)) ToolDef`

Generic constructor. `ArgsSchema` is produced by the **existing** `schemaFor` + `strictifySchema`
path already used by `CompleteStructured` (research R4), including its `sync.Map` schema cache. So a
tool's declared arguments cannot drift from the struct its handler decodes into — they are the same
type.

The handler returns a `string` result. Structured tool results are the handler's business to marshal;
the loop treats a result as opaque, **untrusted** text bounded by size (FR-014, FR-024).

### 2.3 What the adapters must gain

Neither adapter can carry a tool call today. Both response structs are missing the fields, and the
gateway's `chat()` throws away everything except the content string:

```go
// gateway.go:120 — today
func (g *Provider) chat(ctx context.Context, req chatRequest) (string, error)
```

`chat()` performs the request, classifies errors, logs the served model, and reports served model and
usage — all of which `CompleteChat` needs — and then returns one string. It must be **split**: a
lower half returning the parsed response and its headers, and a thin wrapper preserving today's
`(string, error)` for the existing paths. This is a larger edit than "implement `CompleteChat`", and
task T006 is written accordingly.

| Adapter | Request gains | Response gains |
|---|---|---|
| gateway | `tools`, `tool_choice`, both omitted when unset | `choices[].message.tool_calls[]`, `choices[].finish_reason` |
| ollama | `tools` in Ollama's native shape, omitted when unset | `message.tool_calls[]` (a `function` object: `name`, `arguments`), `done_reason` |

**Ollama assigns no tool-call ids.** Its native format has no id field at all. The adapter synthesises
`call_<round>_<index>`: deterministic, unique within one exchange, never sent back to Ollama (which
does not read it), and sufficient for FR-015a's matching requirement.

---

## 3. Port change — `internal/platform/llm/domain/port.go`

```go
type Provider interface {
    ModelName() string
    Complete(ctx, prompt string, opts *CompleteOptions) (string, error)      // unchanged signature
    CompleteJSON(ctx, prompt string, opts *CompleteOptions) (string, error)  // unchanged signature
    CompleteChat(ctx, msgs []Message, opts *CompleteOptions) (ChatResult, error)  // NEW
    Embed(ctx, text string) ([]float32, error)
}
```

Twelve types implement this interface — 3 production, 9 test fakes, all enumerated in research R13.
The compile break is deliberate (contracts C1-5).

### `CompleteOptions` additions

| Field | Meaning |
|---|---|
| `Tools []ToolDef` | declared lookups for this call. Empty means no tool declaration is sent |
| `ToolChoice string` | `""` (omit entirely), `auto`, `none`, or `required` |

Zero values preserve today's behaviour exactly, the same backward-compatibility discipline
`ResponseMode` used in 033 and `TraceID`/`TaskKey` use in 036.

### Shim translation rules — the load-bearing detail (FR-002, FR-003, SC-001)

`Complete` builds `[system?, user]` and delegates. `CompleteJSON` does the same plus its response-format
handling. **What matters is not the construction — it is the seven differences a shared helper
erases.** Ranked by consequence (research R3), each with its own golden assertion:

| # | Must survive | Verified at |
|---|---|---|
| 1 | ollama `Complete` sends `MaxTokens` as `num_predict`; ollama `CompleteJSON` sends **no** `num_predict` at all | `ollama.go:143-145` vs `:156-162` |
| 2 | gateway `CompleteJSON` **always** sets `response_format`, including when `opts == nil`; `Complete` never does | `gateway.go:205-235` |
| 3 | The strict-schema retry fires on `ErrModelUnavailable` **or** `ErrInvalidResponse` — the latter raised on an unparsable 200 body and on zero choices, not only on a 400/422 | `gateway.go:157`, `:161`, `:246` |
| 4 | The retry re-calls `chat`, so one logical call logs and reports served model **and** usage twice | `gateway.go:243-247` |
| 5 | A schema-parse failure sets `strictMode = false` *and* downgrades to `json_object`, so the retry is **skipped** in that case | `gateway.go:228-233` |
| 6 | ollama `CompleteJSON` sets `Format: "json"`; `Complete` does not | `ollama.go:159` |
| 7 | `MaxTokens` is read behind `opts != nil`; `Temp`/`ModelOr`/`SystemPrompt` are nil-safe methods. `Complete(ctx, prompt, nil)` is live | `gateway.go:192`, `cmd/llmsmoke/main.go:37` |

Row 1 is why the golden test covers **both** adapters. It is a wire change on the terminal tier every
chain ends at, and the original T001 — gateway-only — would not have seen it.

`CompleteStructured[T]` keeps its signature, its schema generation, its fence stripping, its
parse-and-retry count and its `Validator` behaviour verbatim. It gains a sibling (§4.1).

---

## 4. Loop types — `internal/platform/llm/application/toolloop/` (NEW)

### 4.1 The typed terminal — `CompleteStructuredChat[T]`

**Corrected: this is new, and it is what makes the loop usable.** The previous `Result{Content string}`
had no consumer in this codebase — all fourteen structured call sites go through
`CompleteStructured[T]` (research R10).

```go
// internal/platform/llm/domain
func CompleteStructuredChat[T any](
    ctx context.Context, p Provider, msgs []Message, opts *CompleteOptions,
) (T, error)
```

It shares the **body** of `CompleteStructured` — same `schemaFor(reflect.TypeOf(zero))`, same schema
attachment when `ResponseModeStrict` is set, same `stripFences`, same `structuredRetries` count of 2,
same `Validator` assertion, same immediate propagation of provider-level errors. The difference is
only where the schema instruction and the retry correction are appended: to a trailing `user` message
rather than to a prompt string.

`CompleteStructured` becomes the sibling called with `[system?, user]` — the same shim discipline R3
applies to `Complete`, one layer up, and covered by the same golden test.

### `Toolset`

A registry: name → `ToolDef` + handler. Responsibilities:

- Present declarations to the provider.
- Dispatch a `ToolCall` to its handler.
- **Refuse** an unknown name or arguments failing schema validation, producing a refusal *result*
  rather than an error that aborts the exchange (FR-009).
- Be **immutable after construction**. Nothing in the conversation can add, remove or alter a tool
  (FR-026).

### `Bounds`

| Field | Default | Requirement |
|---|---|---|
| `MaxRounds` | **4** | FR-010 |
| `PerToolTimeout` | 10s | FR-012 |
| `MaxResultBytes` | 32768 | FR-014 |
| `MaxTotalCostUSD` | 0.50 | FR-016a |
| overall deadline | **required** on `ctx` | FR-011, FR-011a |

**Corrected on two counts.**

*The round default drops from 8 to 4.* Eight was a round number. Four covers the two-lookup shape
SC-008 measures plus one recovery from a refusal.

*There is still deliberately **no** overall-deadline field — but the deadline is now required.* The
previous version said "the caller's context is the single deadline" and stopped there, treating "no
second timer" as equivalent to "bounded". It is not. `gateway/config.yaml`'s own comment records the
proxy's worst case as `5 tiers × 2 attempts × 60s = 600s` **per call**; four rounds against that is
forty minutes, eight was eighty. `Run` therefore returns an error immediately when `ctx.Deadline()`
reports no deadline. That bounds wall time without introducing the second competing timeout that
produced 030's 830-second hang.

*`MaxTotalCostUSD` is new.* `usageFrom` (`gateway.go:92`) already reads per-call cost from
`x-litellm-response-cost`; nothing was accumulating it. Checked between rounds, against the running
total in `Result`.

None of these is reachable from a prompt or a model response (research R6, FR-026).

### `RoundRecord` (FR-016)

| Field | Meaning |
|---|---|
| `Round` | 1-based index |
| `ServedModel` | **NEW** — captured via `WithServedModelCapture` for this round's provider call |
| `CostUSD` | **NEW** — captured via `WithUsageCapture` for this round's provider call |
| `Calls` | per call: name, outcome (`ok` / `refused` / `failed` / `timeout` / `truncated`), duration |
| `SuspectedInjection` | **NEW** — set when a result matched the injection heuristic (FR-027) |

**Corrected: the previous `RoundRecord` dropped the served model and the cost.** Both capture hooks
already exist and are already used by the gateway adapter on every call. A multi-round exchange that
records neither is invisible to exactly the per-tier and per-run visibility features 035 and 036
exist to provide — one exchange would appear as several unattributed calls.

### `Result[T]`

| Field | Meaning |
|---|---|
| `Value` | `T` — the typed, schema-validated answer. **Replaces `Content string`** |
| `Rounds` | `[]RoundRecord` |
| `TotalCostUSD` | **NEW** — sum across rounds (FR-016a) |
| `StopReason` | `answered` / `max_rounds` / `deadline` / `cost_ceiling` / `not_tool_capable` |

`StopReason` is what makes FR-010, FR-011, FR-016a and FR-017 observable rather than inferred from an
error string. `Value` is only meaningful when `StopReason == answered`; every other stop reason
returns the zero `T` **and** an error, so a caller cannot mistake a truncated exchange for an answer.

### Signature

```go
func Run[T any](ctx context.Context, p domain.Provider, ts *Toolset,
                msgs []domain.Message, opts *domain.CompleteOptions, b Bounds) (Result[T], error)
```

---

## 5. State machine

```text
   ┌──────────────────────────────────────────────────────┐
   │ ctx has no deadline? ──► error, nothing sent  (FR-011a)│
   │ append system framing (untrusted-data notice, FR-025) │
   │ round := 0, cost := 0                                 │
   └────────────────────────┬─────────────────────────────┘
                            ▼
        ┌──────────────────────────────────┐
        │ round++                          │
        │ round > MaxRounds?  ─────────────┼──► stop: max_rounds     (FR-010)
        │ ctx expired?        ─────────────┼──► stop: deadline       (FR-011)
        │ cost > MaxTotalCostUSD? ─────────┼──► stop: cost_ceiling   (FR-016a)
        └───────────────┬──────────────────┘
                        ▼
        CompleteChat(msgs, opts + Tools + ToolChoice)
          round 1 → tool_choice "required"          (FR-017, research R12)
          round n → tool_choice "auto"
        wrapped in WithServedModelCapture + WithUsageCapture   (FR-016)
                        │
        ┌───────────────┴───────────────────┐
        │ ToolCalls empty?                  │
        │   ├─ yes, and round == 1 ──► stop: not_tool_capable  (FR-017)
        │   │        no answer returned, prose discarded
        │   ├─ yes, and round > 1  ──► TERMINAL STEP           (FR-023)
        │   │        CompleteStructuredChat[T](msgs) → Value
        │   │        its own retries + Validator apply
        │   │        failure here fails the exchange (FR-023b)
        │   └─ no  ──► dispatch ALL calls (one round, FR-015)
        └───────────────┬───────────────────┘
                        ▼
   per call: decode args → validate → run with PerToolTimeout
             → bound to MaxResultBytes → scan for injection markers
   any outcome (ok / refused / failed / timeout / truncated)
   becomes a `tool` message, wrapped as untrusted data  (FR-013, FR-014, FR-024)
                        │
                        └──► append assistant turn + all tool turns, loop
```

Failure never leaves the loop as an exception: a refused call, a failed call, a timed-out call and a
truncated result all become `tool` messages the model can react to. Only the five stop reasons end the
exchange, and only one of them — `answered` — produces a value.

### 5.1 Why round one is special

Round one sends `tool_choice: "required"`; every later round sends `"auto"`. That asymmetry is the
whole of FR-017's detection (research R12): under `required` a tool-capable model must emit a tool
call, so its absence is diagnostic. Under `auto` it is not — a model that chose not to look anything
up and a model whose `tools` array was silently dropped by the proxy produce identical responses.

The cost is that round one always performs at least one lookup (FR-017a). For salary this is not
waste: the loop is only reached after every deterministic route has already missed.

---

## 6. Untrusted tool output

**New section. The previous data model had none**, which left the largest unexamined risk in the
feature entirely unmodelled: a read-only lookup returning a stored job description returns text
scraped from a third-party website by `internal/retrieval`, and that text goes straight into the
model's context as a `tool` message.

Read-only bounds what a lookup *does*. It places no bound on what a lookup's *output* can talk the
model into.

| Control | Mechanism | Requirement |
|---|---|---|
| Framing | The system turn states that content inside a `tool` result is data, never an instruction | FR-025 |
| Delimiting | Each result is wrapped in an unambiguous, non-guessable delimiter the result's own bytes cannot close | FR-025 |
| Fixed surface | `Toolset` and `Bounds` are captured before the first call and never re-read from the conversation. No result can add a tool, raise a bound, or change the answer schema | FR-026 |
| Typed answer | The answer is `T`, validated against a strict schema and `Validate()`. For `SalaryBand` there is no free-text field an injection can steer into — five numeric/enum fields, all range-checked | FR-023 |
| Visibility | A heuristic scan records `SuspectedInjection` on the round record | FR-027 |

The typed answer is the strongest of these and it is a consequence of R10 rather than a control
designed for this: a loop returning free text would have had no equivalent.

**What this does not claim**: the heuristic scan is a detector, not a filter. It records; it does not
sanitise. A model can still be *influenced* by result content in ways that stay inside the schema —
returning a plausible but wrong band, for instance. The control against that is Principle II's
grounding, not this feature.

---

## 7. Consumer types — `internal/salary/application/tools.go` (NEW)

**Corrected: this section previously modelled three lookups for
`internal/interviewprep/application`, a package that does not exist** (research R8). Nothing in it was
salvageable, because there was no service to convert and no LLM call to convert it from.

The consumer is salary estimation. Its LLM path (`application/service.go:107`, `llmInfer`) is reached
only after `salaryRaw` fails to parse **and** the exact bucket misses in both the ingested cache and
levels.fyi. It currently sends one prompt carrying the posting truncated to 4000 characters, and asks
for a `SalaryBand`.

Two read-only lookups, each with a typed argument struct:

| Lookup | Arguments | Reads | Why it cannot be handed up front |
|---|---|---|---|
| `lookup_comparable_bands` | `{title, location}` | `GetSalaryCacheByBucket` for a bucket composed from the arguments | The exact bucket already missed. Which *neighbouring* bucket is apt — a different seniority, a nearby market — depends on reading the posting, which the caller cannot enumerate without guessing |
| `get_posting_details` | `{job_id}` | `GetJobByID` — full description, company, location, remote flag | The current prompt truncates the description at 4000 characters whether or not the salient text survives the cut |

Both are reads. The service's two writes — `UpdateJobSalary` and `UpsertSalaryCache` — stay where they
are today, in `Infer`, **after** and **outside** the model call. No handler touches them (FR-007).

`T = domain.SalaryBand`, which already carries `jsonschema` tags and implements `Validate()`
(`salary/domain/port.go`), so the terminal step exercises strict schema **and** the `Validator` hook
without any new type. `llmInfer`'s signature is unchanged: it still returns
`(domain.SalaryBand, error)` (FR-023a).

### 7.1 The unavailable path (FR-022)

`Infer` currently wraps an `llmInfer` failure as `"salary: LLM inference failed: %w"` and returns —
persisting nothing. That is already the required behaviour and it must stay: when the lookups fail, or
the exchange stops for any reason other than `answered`, `llmInfer` returns an error and `Infer`
persists no band. The regression to guard against is a conversion that "helpfully" falls back to a
low-confidence band, which would be a Principle II fabrication written to the database.

---

## 8. The read-only fence (FR-008)

**Corrected in mechanism, in scope, and in location.**

### 8.1 Where it lives

`apps/api/internal/toolfence_test.go`, package `internal_test`, beside the existing
`internal/arch_test.go`.

**Not** in `platform/llm/application/toolloop/`, where T024 originally put it. A fence in the platform
package would force the platform layer to enumerate its consumers' tool packages, contradicting
plan.md's own structure decision — "the consumer keeps its own tools in its own package, so the
platform layer never learns what a job posting is". `internal/` is the level that already owns
tree-wide structural rules.

### 8.2 How it enumerates lookup packages (FR-008a)

Two mechanisms, each used where it is actually valid:

1. **Discovery — a direct-import scan.** A package that registers tools must import the toolloop
   package. Walking `internal/` with `parser.ImportsOnly` and collecting every package that imports
   `.../platform/llm/application/toolloop` finds all of them. This is `arch_test.go`'s mechanism used
   for the job it is genuinely good at: one target package, direct imports, no transitivity needed.
2. **Declaration — a list in the test.** The packages the test expects to find, in the style of
   `gateway_config_test.go`'s `requestedGenerationGroups` ("listed explicitly so deleting a chain AND
   its group still fails rather than quietly satisfying the derived check").

**The fail-closed rule (C5-2)**: discovery ∖ declaration must be empty. A new tool package that
nobody added to the list is discovered by the scan, is absent from the list, and fails the test.
Declaration ∖ discovery must also be empty, so deleting a tool package without updating the list
fails too.

The previous version required failing closed without saying how the test would *enumerate* — and an
import walk alone cannot discover "this package registers tools" without something to look for. The
toolloop import is that something.

### 8.3 How it checks reachability (FR-008)

For each discovered package, resolve its **transitive** dependency closure:

```
go list -deps <package>      // invoked via os/exec
```

and fail if the closure contains any forbidden package.

`golang.org/x/tools/go/packages` would be the idiomatic choice and is not available: `x/tools` is not
in `apps/api/go.mod`, and adding it to build the fence would break SC-009 — the no-new-dependency
check — with the control that exists to protect a constitutional principle. `go list` ships with the
toolchain and needs no module requirement.

**If `go list` cannot be invoked, the test fails.** It does not skip. A fence that switches itself off
when the environment is unusual is a fence that is off.

### 8.4 The forbidden set (FR-008b)

| Package | Why |
|---|---|
| `internal/notifier` | sends notifications |
| `internal/outreach` | drafts and addresses employer contact |
| `internal/postage` | mail |
| `internal/applications` write paths | application state |
| **`internal/retrieval`** | **NEW** — outbound HTTP, headless browser, FlareSolverr (`browser.go`, `flaresolverr.go`, `transport.go`) |
| **`internal/jobsources`** | **NEW** — fetches third-party job boards |

The last two were missing. A lookup importing `internal/retrieval` passed the original fence and
reached the open internet from inside a model's decision loop — the same class of exposure as reaching
a mail sender, and arguably worse because it is not obviously a "sending" package.

### 8.5 What it catches, and the three things it does not (FR-008c)

**Catches**: a lookup package whose dependency closure reaches a forbidden package, at any depth.

**Does not catch**, all three stated in the test file itself:

1. A lookup that builds its own outbound HTTP request from `net/http`. No forbidden import appears.
2. **A closure over an already-injected capability.** Handlers are closures. One defined inside a
   service that already holds an outreach client can call it; the lookup's own package imports
   nothing. The import graph cannot see a method value on an injected dependency. This is the largest
   hole, and it is why "the toolset is small, enumerated and reviewed" is a required complementary
   control rather than a reassurance.
3. Packages, not call paths. A lookup package that legitimately imports something with both read and
   write functions is indistinguishable from one that calls the write function.

### 8.6 Whether it "fails the build" (SC-005, FR-030)

It is a `go test`, and `.github/workflows/api-ci.yml`'s `go-test` job runs `go test ./...` in
`apps/api` — so for a change under `apps/api/**` it does fail CI. No 037 document previously named a
CI configuration at all.

**But the trigger has a hole this feature must fix.** That job is gated on a `dorny/paths-filter`
whose `go` filter lists `apps/api/**`, `scripts/sqlc-check.sh` and `scripts/tygo-check.sh`.
`gateway/config.yaml` matches **none** of them. A pull request that only adds a tier to a routing
chain therefore skips `go-test` entirely, and the config guardrail — this feature's C6-2, and 035's
existing `checkChainsTerminateAtLocal` — never runs. Adding `gateway/**` to the `go` filter is part of
this feature (FR-030, task T035a).

---

## 9. What is deliberately not modelled

- No persistence of conversations. An exchange lives for one request.
- No streaming (research R9).
- No memory, no summarisation, no context-window management. If a conversation grows too long for the
  model, that is a caller concern, and the round limit bounds how long it can get.
- No parallel *exchanges*. Tool calls within one round run together; exchanges do not fan out.
- No `packages/shared` or dashboard type. Nothing here crosses a language boundary, and the converted
  consumer stores the same `SalaryBand` it stores today.
- No sanitisation of tool output. §6 detects and records; it does not rewrite what a lookup returned.
