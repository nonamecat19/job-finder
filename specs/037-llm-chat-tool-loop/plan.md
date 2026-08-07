# Implementation Plan: Multi-Turn Conversations and a Typed Tool Loop

**Branch**: `037-llm-chat-tool-loop` | **Date**: 2026-08-07 | **Spec**: [spec.md](./spec.md)

**Revised**: 2026-08-07 after audit. The source-code tree below named a package that does not exist,
and the Phase 0 summary repeated three decisions that were overturned. See research.md's corrections
log.

**Input**: Feature specification from `/specs/037-llm-chat-tool-loop/spec.md`

## Summary

`domain.Provider` reaches a model through one string. That is the whole ergonomic gap: no follow-up
turn, no tool call, no way to hand a model back the result of a lookup. Everything else in the port
is strong — `CompleteStructured[T]` with a schema cache, `strictifySchema`, the `Validator` hook,
`WithServedModelCapture` — which is precisely why the fix is to extend this port rather than replace
it with `langchaingo`'s weaker one (research R1).

Add `CompleteChat(ctx, []Message, opts) (ChatResult, error)`. Make `Complete` and `CompleteJSON`
shims onto it, proven transparent by golden request tests **in both adapters** rather than by reading
the diff. Then add a bounded tool loop above the port: declared, typed, **read-only** lookups; a round
limit; a required caller deadline; per-lookup timeouts; size-bounded results; a spend ceiling — and a
**typed** terminal, because a loop returning a string has no consumer in this codebase.

Three things gate the design:

1. **Read-only is a constitutional boundary, not a design preference.** Principle I forbids the
   platform acting on a job on the user's behalf. A tool loop is a model deciding what to invoke, so
   the fence is enforced by a test resolving each lookup package's **transitive** dependency closure
   with `go list -deps`. *Corrected: the original plan called this "the third instance of an idiom"
   citing `internal/arch_test.go` and `internal/outreach/nosend_test.go`. Neither performs a
   transitive walk — the first is a direct-import scan against one constant, the second a string-token
   grep over file contents. The mechanism is invented here and justified on its own in research R5.*
2. **A fallback tier that cannot call tools returns a confident answer.** The routing chains fail over
   to free tiers and terminate at the local model. *Corrected: the original plan said tool capability
   "becomes a declared, tested property of `gateway/config.yaml`, exactly as 035 had to do for
   reasoning bounds". It does not work that way. `drop_params: true` (`gateway/config.yaml:213`)
   silently removes an unsupported `tools` array and the request succeeds — the same trap
   `specs/domains/llm-routing.md:118-125` documents for `response_format`. The annotation is
   documentation a test reads; the only runtime catch is requiring a tool call on the exchange's first
   round (research R7, R12).*
3. **The loop must terminate into a type.** All fourteen structured call sites in this codebase go
   through `CompleteStructured[T]`; none can consume a string. `Run` is generic and its terminal step
   reuses the existing structured path, so strict schemas, the retry loop and the `Validator` hook
   apply to the loop's answer exactly as they do to a single structured call (research R10).

## Technical Context

**Language/Version**: Go 1.26 (`apps/api`). No dashboard change.

**Primary Dependencies**: none added (FR-020, SC-009). Reuses `invopop/jsonschema`, already required.

**Storage**: none. No migration, no schema change, no new table.

**Testing**: `go test` — golden request-body comparison for the shim **in both adapters**, plus
side-effect-count assertions for behaviours a golden file cannot see; table tests for the loop's
bounds; a fake tool-calling provider for the loop; an injection test for untrusted tool output; a
structural test for the read-only fence; and a config test for tier capability. Enforced by the
`go-test` job in `.github/workflows/api-ci.yml`, whose paths filter this feature widens to cover
`gateway/**` (FR-030).

**Target Platform**: Linux server, self-hosted via Docker Compose

**Project Type**: Go platform-layer addition plus one converted consumer

**Performance Goals**: no measurable latency change on existing single-prompt and structured calls —
the shim adds a slice allocation, nothing more

**Constraints**: byte-identical requests for every existing caller, in both adapters; zero new module
requirements — including for the fence, which is why it uses `go list -deps` rather than
`x/tools/go/packages`; every declared lookup read-only; every bound set by the caller and unreachable
by a prompt or by tool output.

**Scale/Scope**: one package extended, one new package for the loop, one consumer converted, twelve
`Provider` implementations updated (3 production, 9 test fakes — research R13). The original "roughly
80 lines for the seam and 150 for the loop" estimate is retained for the loop but **understated the
seam**: `gateway.chat()` must be split and both adapters' response types extended before
`CompleteChat` can exist at all.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Principle | Assessment |
|---|---|
| **I. No Auto-Apply** | **The principle this feature is most exposed to.** A tool loop is a model choosing what to invoke; a write-capable tool would put that choice inside the trust boundary Principle I declares non-negotiable. FR-007 forbids it, FR-008 enforces it structurally over the transitive closure, and R5 documents all three things the enforcement does not catch — the largest being a handler closing over an already-injected capability, which no import graph can see. **PASS**, conditional on FR-008 landing with the loop and not after it, and on the enumerated-and-reviewed toolset being treated as a required complementary control rather than a reassurance. |
| **II. Grounded Generation** | Strengthened, with a new exposure named. A model that can look up the actual posting has less reason to invent it, and FR-016's per-round record makes what it read auditable. **But tool results are untrusted input** — a stored job description is text scraped from a third-party site — so FR-024–FR-027 add framing, delimiting, a fixed tool/bound/schema surface and injection recording. The typed terminal (FR-023) is the strongest control here: a `SalaryBand` has five numeric and enum fields, all range-checked, and no free-text field an injection can steer into. **PASS** |
| **III. Typed Contracts** | Entirely Go-internal. No DTO, no tygo regeneration, no `packages/shared` change. Tool argument shapes are Go types with schemas generated from them (R4), so the schema cannot drift from the struct the handler decodes into. **PASS** |
| **IV. Test Discipline** | `go test` only — `apps/api` is the sole app touched, so the cross-app `make test-lint` gate is not triggered by the boundary rule, though it is listed in Polish. The load-bearing tests are the golden request comparison (SC-001) and the read-only architecture test (SC-005). **PASS** |
| **V. Local-First** | The sharp edge, and sharper than the original assessment allowed. Every chain terminates at the **single shared** self-hosted deployment, so declaring one tool-using chain tool-capable declares that terminal tier tool-capable for every task in the system (FR-018a). A tool-using task must either be served by it or fail honestly — FR-019 — and the design chooses "fail with a named reason" over "silently answer without tools". Note also that the highest-ranked shim regression (C1-2 row 1, `num_predict` on the local JSON path) is a wire change on precisely this tier, which is why the golden test covers both adapters. **PASS** |

No violations. Complexity Tracking omitted.

**Post-Phase-1 re-check**: the design adds one package (`application/toolloop`), one method on an
existing interface, and one generic sibling to the existing structured entry point. The alternative —
an orchestration framework — was rejected on measured grounds in R1, not on preference; R1 is the one
Phase 0 decision the audit did not overturn. Still **PASS**.

## Project Structure

### Documentation (this feature)

```text
specs/037-llm-chat-tool-loop/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   └── contracts.md     # Phase 1 output
├── checklists/
│   └── requirements.md
└── spec.md
```

### Source Code (repository root)

```text
apps/api/internal/platform/llm/
├── domain/
│   ├── message.go                    # NEW: Message, Role, ToolCall, ChatResult
│   ├── tool.go                       # NEW: ToolDef, argument-schema generation
│   └── port.go                       # + CompleteChat on Provider; + CompleteStructuredChat[T];
│                                     #   Complete/CompleteJSON/CompleteStructured become shims
├── application/
│   ├── router.go                     # + CompleteChat passthrough
│   └── toolloop/
│       ├── loop.go                   # NEW: the bounded loop, Run[T]
│       ├── registry.go               # NEW: declared toolset, dispatch, refusal of undeclared calls
│       ├── bounds.go                 # NEW: rounds, per-tool timeout, result size, spend ceiling
│       └── untrusted.go              # NEW: result framing, delimiting, injection heuristic
├── infrastructure/
│   ├── gateway/gateway.go            # chat() SPLIT; tool_calls + finish_reason on chatResponse;
│   │                                 #   CompleteChat; argument decoding
│   └── ollama/ollama.go              # CompleteChat, native tool format, synthesised call ids
├── gateway_config_test.go            # existing; + tool-capability assertion
└── llm.go                            # facade re-exports for the new types

apps/api/internal/salary/
├── application/
│   ├── service.go                    # llmInfer converted to toolloop.Run[domain.SalaryBand] (FR-021)
│   ├── tools.go                      # NEW: lookup_comparable_bands, get_posting_details
│   └── service_test.go               # existing; the regression surface the conversion must not break
└── domain/port.go                    # existing SalaryBand — the loop's T, already has Validate()

apps/api/internal/
├── arch_test.go                      # existing
└── toolfence_test.go                 # NEW: the read-only fence (FR-008)

gateway/config.yaml                   # model_info.supports_function_calling per tier — DOCUMENTATION
.github/workflows/api-ci.yml          # `go` paths filter widened to include gateway/**
```

**Structure Decision**: existing hexagonal layout. `Message`/`ToolDef` are domain types because they
describe the seam's contract; the loop is application because it orchestrates; the tool-format
translation is infrastructure because it is provider wire detail. The consumer keeps its own tools in
its own package, so the platform layer never learns what a job posting is.

**Corrected — two consequences of that last sentence the original plan violated.** First, the consumer
package named here was `internal/interviewprep/application`, which does not exist; the real consumer is
`internal/salary/application`, verified before choosing it (research R8). Second, the fence was placed
inside `platform/llm/application/toolloop/`, which would force the platform package to enumerate its
consumers' tool packages — the exact coupling this structure decision forbids. It now lives at
`internal/toolfence_test.go`, the level that already owns tree-wide structural rules.

## Phase 0: Research

See [research.md](./research.md). Thirteen decisions, all resolved; six of the original nine were
corrected after audit. The load-bearing ones:

- **R1**: the concrete capability table showing `langchaingo`'s `llms.Model` is a downgrade against
  what 033 and 035 already built. This is why FR-020 exists — and it is the one original decision the
  audit did not overturn.
- **R3**: the shim is where a silent behaviour change would hide. **Seven** differences between the
  existing entry points, ranked; the temperature split is the fourth. The largest is that ollama's
  `Complete` sends `num_predict` and its `CompleteJSON` does not — a wire change on the terminal tier,
  and outside the original gateway-only golden test.
- **R5**: the fence resolves transitive closure with `go list -deps` (no new module requirement), and
  states **three** limits, of which the largest — a handler closing over an already-injected
  capability — was previously undocumented.
- **R7**: tool capability in config is **documentation a test reads, not enforcement**. `drop_params`
  drops silently and the request succeeds.
- **R8**: **salary estimation** as the first consumer, verified by reading it. The original consumer
  was fabricated.
- **R10**: the loop terminates into a typed `T` through the existing structured path, because a
  string result has no consumer in this codebase.
- **R12**: capability detection by requiring a tool call on round one — the only mechanism available
  to an application that never learns which upstream served its request.

## Phase 1: Design

- [data-model.md](./data-model.md) — `Message`, `ToolCall`, `ChatResult`, `ToolDef`,
  `CompleteStructuredChat[T]`, `Bounds`, `RoundRecord`, `Result[T]`, the untrusted-output model, the
  fence's mechanism, and the shim's seven translation invariants.
- [contracts/contracts.md](./contracts/contracts.md) — eight contracts: the `Provider` interface
  change, the wire format for tools in both adapters (including the `chat()` split, argument decoding
  and synthesised ids), the router passthrough, the loop's contract and typed terminal, the read-only
  fence, the routing tool-capability contract as documentation, the salary consumer, and CI
  enforcement.
- [quickstart.md](./quickstart.md) — twelve runnable scenarios: prove the shim is byte-identical in
  both adapters, hold the retry's side-effect counts, run a three-turn exchange, produce a typed
  answer, exhaust every bound, refuse an undeclared call without refusing a well-formed one, prove
  tool output cannot steer the loop, see the fence fail three ways, force a non-tool-capable tier,
  estimate a salary band through two lookups, and confirm the checks actually run in CI.
