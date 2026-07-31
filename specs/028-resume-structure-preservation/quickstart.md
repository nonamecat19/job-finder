# Quickstart — Feature 028: Strict Resume Structure Preservation During AI Tailoring

**Feature**: 028 — [spec](spec.md) | [plan](plan.md) | [contract](contracts/structural-invariants.md) | [data model](data-model.md)

Runnable validation scenarios that prove the three structural-integrity invariants hold end-to-end. All checks are `go test` unit tests against the tailoring/merge/verifier layer; no LLM call, no Docker, no browser required for the core invariants (the text-years check has a unit-testable regex path and an LLM-dependent re-prompt path covered separately).

## Prerequisites

- Go toolchain (apps/api/.golangci-version)
- `pnpm install` + `pnpm --filter @job-finder/shared build` (so tygo-generated types resolve)
- Local Ollama running (for the two end-to-end scenarios that invoke the LLM) — optional for unit tests

## Scenario 1 — Block sequence is immutable (unit test)

**Proves**: Invariant 1 (FR-001, FR-002, FR-010). AI cannot drop/reorder blocks; a reordered master is preserved as-is.

1. In `apps/api/internal/generation/domain/rendercv_test.go`, construct a master `RendercvMaster` with `cv.sections` in order `[experience, education, skills, summary, projects]` (non-canonical, simulating a user-reordered master) and a `_order` key reflecting that order.
2. Build a `TailoredSections` (post-028 shape: no `SectionsToDrop`) with a new summary and per-company highlights.
3. Call `MergeTailored(master, payload)`.
4. **Assert**: the merged `cv.sections["_order"]` equals `[experience, education, skills, summary, projects]` (master order, unchanged). The `projects` section is still present. No section was added, removed, renamed, or reordered.
5. **Assert**: `merged.cv.sections.summary` equals the payload summary; each experience entry's `highlights` equal the payload highlights; all other fields (company, dates, skill group labels) are byte-for-byte the master's.

**Run**: `make test-go` (or `go test ./internal/generation/domain/ -run TestMergeTailoredPreservesBlockOrder`).

## Scenario 2 — Experience order and identity are preserved (unit test)

**Proves**: Invariant 2 (FR-003). AI cannot reorder or drop jobs.

1. Construct a master with experience entries `[Acme, Globex, Initech]` in that order.
2. Build a `TailoredSections` with `Experience` entries in a *different* order `[Initech, Acme, Globex]` (simulating an LLM that tries to reorder most-relevant first) and with `Highlights` for only two of the three (simulating an attempt to drop the third by omission).
3. Call `MergeTailored`.
4. **Assert**: the merged experience section lists companies in master order `[Acme, Globex, Initech]`. No entry was dropped. Only `highlights` changed for the entries the payload covered; the omitted entry retains its master `highlights` verbatim.

**Run**: `go test ./internal/generation/domain/ -run TestMergeTailoredPreservesExperienceOrder`

## Scenario 3 — Dates are unchanged (unit test)

**Proves**: Invariant 3 part (a) (FR-009). AI cannot alter experience dates.

1. Construct a master with an experience entry having `start_date: "2019-01"`, `end_date: "2023-06"`.
2. Build a `TailoredSections` with new `highlights` for that company.
3. Call `MergeTailored`.
4. **Assert**: the merged entry's `start_date` and `end_date` are byte-for-byte `"2019-01"` and `"2023-06"`.

**Run**: `go test ./internal/generation/domain/ -run TestMergeTailoredPreservesDates`

## Scenario 4 — Text-asserted years contradiction is flagged (unit test)

**Proves**: Invariant 3 part (b) (FR-007). `VerifyStructureIntegrity` flags a summary asserting a years figure that contradicts the master's derivable total.

1. Construct a master with experience entries whose dates derive to a total of **5 years** (e.g., two entries: 2019–2021 and 2021–2023).
2. Construct a *merged* `RendercvMaster` (post-merge) whose `cv.sections.summary[0]` is `"Senior engineer with over 12 years of experience."`.
3. Call `VerifyStructureIntegrity(master, merged)`.
4. **Assert**: the returned violations contain exactly one `StructureTotalExperienceYears` violation pointing at the summary path, with a message citing both the asserted 12 and the derived 5.

**Run**: `go test ./internal/generation/domain/ -run TestVerifyStructureIntegrityFlagsYearsAssertion`

## Scenario 5 — Text-asserted years absence is not flagged (unit test)

**Proves**: Invariant 3 part (b) negative case. A summary with no numeric years claim is not a violation.

1. Master as in Scenario 4 (5-year total).
2. Merged `summary[0]` = `"Senior backend engineer specializing in distributed systems."` (no number).
3. Call `VerifyStructureIntegrity`.
4. **Assert**: zero violations.

**Run**: `go test ./internal/generation/domain/ -run TestVerifyStructureIntegrityNoYearsAssertion`

## Scenario 6 — Prompt no longer asks for reorder/drop (unit test)

**Proves**: the prompt contract (R7). `buildSelectPrompt` does not instruct the LLM to reorder experience, drop jobs, or drop sections.

1. Build a master and a `VacancyAnalysis`, call `buildSelectPrompt`.
2. **Assert** the prompt string does NOT contain `"Reorder experience"`, `"drop: true"`, or `"Decide which sections to drop"`.
3. **Assert** the prompt string DOES contain `"Keep experience entries in the EXACT order"`, `"never set drop"`, and `"Do not drop, add, rename, or reorder any resume section"`.

**Run**: `go test ./internal/generation/application/ -run TestBuildSelectPromptNoReorderOrDrop`

## Scenario 7 — TailoredSections struct has no drop/reorder fields (compile gate)

**Proves**: the data-model contract (R1). The removed fields are gone.

1. The Go compiler is the gate: any code referencing `TailoredSections.SectionsToDrop`, `.ExperienceOrder`, or `TailoredExperience.Drop` fails to compile after the struct change. This is enforced automatically by `make test-go` / `make lint-go`.

**Run**: `make test-go` (compile failure is the success signal if any stale reference remains).

## Scenario 8 — TS types regenerated (cross-boundary gate)

**Proves**: constitution Principle III (typed contracts). The TS mirror reflects the removed fields.

1. Run `make tygo-generate`.
2. **Assert** `packages/shared/src/generated.ts` `TailoredSections` type has no `sectionsToDrop`, `experienceOrder`, or `drop` fields.
3. If `packages/shared/src/index.ts` hand-mirrors `TailoredSections`/`TailoredExperience`, **assert** the three fields are removed there too (per AGENTS.md).
4. `make test-lint` passes (ESLint + go).

**Run**: `make tygo-generate && make test-lint`.

## Scenario 9 — End-to-end tailoring produces a structurally-faithful resume (integration, optional)

**Proves**: the full pipeline with a live LLM respects all three invariants.

1. Start the backend (`make run-backend`) and local Ollama.
2. Trigger a resume generation (job-scoped or ad-hoc) against a vacancy that emphasizes an older role.
3. **Assert** the generated `generated_documents.content` (the merged `RendercvMaster`):
   - Block order equals the master profile's `rendercv_config` block order.
   - Experience entries appear in master order.
   - Experience `start_date`/`end_date` equal the master's.
   - The summary contains no numeric years-of-experience figure contradicting the master's derived total (if the LLM asserted one, the re-prompt/strip path ran — check the activity log for the intervention).

**Run**: `make run-backend`, then trigger via the dashboard or API; verify `content` jsonb and the activity row.

## Done definition for this feature

All scenarios 1–8 pass under `make test-lint` (which runs `lint-go`, `lint-web`, `test-go`, `test-react`). Scenario 9 is a manual/CI integration check (not part of `test-lint`).