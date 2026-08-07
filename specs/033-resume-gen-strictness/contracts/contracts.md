# Contracts: Resume Generation Strictness & Model Improvement

**Feature**: 033-resume-gen-strictness
**Date**: 2026-08-07

This feature changes **internal Go contracts** (function signatures, request shapes) and
**one external contract** (the LiteLLM proxy request for the `generation` task). It adds no
HTTP endpoints and changes no DTOs. Each contract below is the thing an implementer must
satisfy; the plan does not prescribe the code inside.

---

## C1. `VerifyRendercvGrounding` — extended signature

**File**: `apps/api/internal/generation/domain/rendercv_grounding.go:95`

**Current**:
```go
func VerifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel) []string
```

**After**:
```go
func VerifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel, analysis VacancyAnalysis) []string
```

The `analysis` parameter supplies `RequiredSkills`/`NiceToHaveSkills` so the
moderate/aggressive adjacency check (data-model.md §AdjacentSkillAllowed) can evaluate whether
a non-master token is a vacancy-required adjacent skill. Under `GroundingStrict`, `analysis`
is ignored (today's strict behaviour is unchanged — master tokens only).

**Contract**:
- Returns a string per violation. Empty slice = grounded.
- Violation strings are stable, machine-greppable prefixes:
  - `skill "<token>" (<label>) not in master profile` — strict, or moderate with no adjacency
  - `experience "<company>" highlight not grounded in master: "<truncated>"` — all levels
  - `company "<name>" not in master profile` — unchanged
  - `unexpected section "<key>" added to merged resume` — unchanged
  - `project "<name>" not in master profile` — unchanged
- The function is pure (no I/O, no side effects) and deterministic for a given input.
- Performance: O(skills × tokens + experience × highlights × master_bullets). Must not add
  more than 5ms to a typical run (SC-006).

---

## C2. `DropUngroundedSkillTokens` — called on the primary pass

**File**: `apps/api/internal/generation/domain/rendercv.go:178`

**Current call sites**: `service.go:359` (after expand), `service.go:398` (after condense).

**After**: also called at `service.go:232` (after the primary `MergeTailored` +
`ApplyHardLimits`, before `VerifyRendercvGrounding`). Signature unchanged.

**Contract**:
- Mutates `doc` in place: removes any skill entry whose tokens are not all in
  `MasterSkillTokens(pool)`.
- Logs nothing itself — the caller logs the intervention on the activity row (FR-010).
- Idempotent: running it twice produces the same result.

---

## C3. `StripUngroundedHighlights` — new, mirrors `StripStructureViolations`

**File**: `apps/api/internal/generation/domain/rendercv_structure.go` (add)

```go
func StripUngroundedHighlights(master, merged RendercvMaster) RendercvMaster
```

**Contract**:
- For every experience highlight in `merged` that does not pass `lcsCovered` against the
  master's highlights for that company, replace it with the master bullet that has the highest
  word-overlap. If no master bullet exists for that company (should not happen — the company
  match is checked first), drop the highlight.
- Returns a new `RendercvMaster` (does not mutate input).
- Logs nothing itself — the caller logs each replacement on the activity row.
- Called only after a re-prompt failed to fix the drift (same two-step pattern as
  `fixStructureIntegrity`: verify → re-prompt → verify → strip).

---

## C4. `buildSelectPrompt` — cleaned prompt

**File**: `apps/api/internal/generation/application/rendercv_llm.go:109-218`

**Contract**:
- No reference to `sectionsToDrop`, `ExperienceOrder`, or `Drop` (removed struct fields).
- The "HARD RULES" section matches the `TailoredSections` struct fields exactly: `summary`,
  `skills` (by index), `experience` (by company, highlights only), `projects` (by name),
  `skillGroupsToAdd`, `skillGroupsToRemove`, `skillChanges`. No phantom fields.
- The grounding-level rule (`LevelRules[level]`) is unchanged in wording.
- The `prevViolations` feedback loop is unchanged (feeds grounding violations back into the
  re-prompt).

---

## C5. `CompleteOptions.ResponseMode` — new field

**File**: `apps/api/internal/platform/llm/domain/port.go:18-23`

```go
type ResponseMode int
const (
    ResponseModeJSON   ResponseMode = iota  // zero value: json_object (current behaviour)
    ResponseModeStrict                       // json_schema with strict: true
)

type CompleteOptions struct {
    System      string
    Temperature *float64
    MaxTokens   *int
    Model       string
    ResponseMode ResponseMode
}
```

**Contract**:
- Zero value (`ResponseModeJSON`) preserves today's behaviour exactly. Every existing caller
  that does not set `ResponseMode` keeps working with `json_object`.
- `ResponseModeStrict` is only meaningful for `CompleteJSON` (structured calls). For
  `Complete` (plain text), the field is ignored.

---

## C6. Gateway `chatRequest.ResponseFormat` — upgraded shape

**File**: `apps/api/internal/platform/llm/infrastructure/gateway/gateway.go:49-56, 150-164`

**Current**:
```go
type chatRequest struct {
    ...
    ResponseFormat map[string]string `json:"response_format,omitempty"`  // {"type": "json_object"}
}
```

**After**: see data-model.md §StrictSchemaRequest. The `ResponseFormat` is a pointer to a
struct that can express `json_object` or `json_schema` with `strict: true`.

**`CompleteJSON` builds the format from `opts.ResponseMode`**:
- `ResponseModeJSON` → `{"type": "json_object"}` (byte-identical to today's request).
- `ResponseModeStrict` → `{"type": "json_schema", "json_schema": {"name": "<type>", "schema": <schema>, "strict": true}}`.

The schema is passed from `CompleteStructured` (which generates it via `jsonschema.ReflectFromType`)
to the provider through a new field on `CompleteOptions` or a sibling. The cleanest path:
`CompleteStructured` sets `opts.ResponseMode = ResponseModeStrict` and attaches the schema;
the gateway adapter builds the `json_schema` object. The schema is cached (`schemaCache`,
`port.go:82`).

**Contract**:
- For `ResponseModeJSON`, the wire payload is byte-identical to today — no provider sees a
  change for non-generation tasks.
- For `ResponseModeStrict`, the schema's `additionalProperties` is `false` (enforced by
  `jsonschema.Reflector` config or an explicit marshal step). The model cannot emit unexpected
  fields.
- Capability: every model in the `generation` chain in `gateway/config.yaml` must support
  `json_schema` strict mode, verified at implementation time (config-time verification, per
  030-C5). The `drop_params: true` setting means an unsupported param would be silently
  dropped — the guard is the verification, not runtime detection.

---

## C7. `max_completion_tokens` — set for generation

**File**: `apps/api/internal/generation/application/rendercv_llm.go` (call sites)

**Contract**:
- Every `selectAndTailor`, `retailorForStructure`, `expandContent`, `condenseContent` call
  sets `CompleteOptions.MaxTokens` to an explicit cap (default ~4096, tuned to the largest
  expected `TailoredSections` payload).
- The gateway adapter already forwards `MaxTokens` when non-nil (`gateway.go:144-146`).
- `analyzeVacancy` and `writeCoverLetter` may set their own caps (smaller payloads).

---

## C8. Gateway config — generation chain (re-evaluated, not restructured)

**File**: `gateway/config.yaml`

**Contract**:
- The five-tier pattern (`generation` → `generation-cerebras` → `generation-groq` →
  `generation-cohere` → `local`) is unchanged in structure.
- The primary model (`openrouter/deepseek/deepseek-v4-pro` today) may change based on the R6
  benchmark results. The change is a single `model:` line edit, no other config change.
- Every model in the generation chain must support `json_schema` with `strict: true`
  (extends the existing 030-C5 contract that every model supports `json_object`).
- The chain still terminates at `local` (Ollama) — Constitution V, 030-FR-008.
- `request_timeout: 60` per attempt is unchanged; the worst-case arithmetic
  (`5 × 2 × 60 = 600s < 15m safetyNetTimeout`) still holds.

---

## C9. No new HTTP endpoints

This feature adds no routes to `httpapi.NewRouter` and no handlers to
`internal/generation/interfaces/http/` or `internal/tailoring/interfaces/http/`. The existing
`POST /api/tailoring`, `GET /api/tailoring/{draftId}`, etc. (spec §4.1) are unchanged — they
surface the same draft/proposal shape. The strictness changes are invisible to the HTTP
client except that the proposals it sees are more often grounded.

---

## C10. No DTO changes — no tygo regeneration

`TailoredSections`, `ShapeConfig`, `VacancyAnalysis`, `EditProposalDto`,
`ActivityRunDto` — all unchanged. No `make tygo-generate` needed. The `packages/shared`
generated file is untouched. The dashboard needs no change.