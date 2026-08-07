# Quickstart: Resume Generation Strictness & Model Improvement

**Feature**: 033-resume-gen-strictness
**Date**: 2026-08-07

This is a validation guide — runnable scenarios that prove the feature works end-to-end. It
does not include implementation code; that lives in `tasks.md` and the implementation phase.

## Prerequisites

- The full stack is up: `make up` (Postgres, Redis, Ollama, LiteLLM).
- At least one master profile with skills and experience exists (seed via `make seed` or the
  dashboard).
- The LiteLLM gateway is configured (`gateway/config.yaml`) with at least one provider key
  set, and `LITELLM_MASTER_KEY` is set.
- `make setup-hooks` has been run (so `gofmt`/`go vet` run on save).

## Scenario 1: Grounding at the default (moderate) level catches a fabricated skill

**Proves**: FR-001, FR-003, SC-002 — the moderate grounding level now enforces skill tokens on
the primary pass.

1. Pick a master profile whose skills are `{Go, Postgres, Docker}` (no Terraform).
2. Pick a vacancy that requires `Terraform, Kubernetes, Go, Postgres`.
3. Run tailoring: `POST /api/tailoring` with `profileId` and `jobId` (or an ad-hoc vacancy).
4. Poll `GET /api/tailoring/{draftId}` until state is `review`.
5. Inspect the activity trail for the run (`GET /api/activity/{activityId}`):
   - There is a step logging that ungrounded skill tokens were dropped, naming `terraform`
     and `kubernetes`.
6. Inspect the merged resume's skills: no `terraform` or `kubernetes` token appears.
7. **Pass**: the merged resume's skills ⊆ master skills, and the activity trail records the
   drops.

## Scenario 2: A drifted highlight is replaced, not emitted

**Proves**: FR-002, FR-003, SC-003 — a rephrased bullet below the word-overlap threshold is
detected and replaced with the closest master bullet.

1. Use a master profile with an experience bullet: `"Led migration of monolith to
   microservices"`.
2. Use a vacancy that asks for something unrelated (e.g. "frontend React work") so the model
   is tempted to rewrite the bullet away from the original.
3. Run tailoring (Scenario 1 steps 3-4).
4. Inspect the activity trail:
   - A "highlight drift" intervention is logged, naming the company and the drifted bullet.
5. Inspect the merged resume's experience highlights for that company:
   - The bullet is the closest master bullet (word-overlap), not the drifted rewrite.
6. **Pass**: no highlight in the merged resume is below the word-overlap threshold, and the
   activity trail records every replacement.

## Scenario 3: The prompt matches the data contract

**Proves**: FR-004 — no reference to removed struct fields.

1. Capture the prompt sent to the model for a tailoring run (add a debug log or use the
   existing `rec.Step` activity trail to dump the prompt, or read the test fixture).
2. Grep the prompt text for `sectionsToDrop`, `ExperienceOrder`, `Drop`.
3. **Pass**: zero matches. The "HARD RULES" section references only fields that exist in
   `TailoredSections`.

## Scenario 4: Strict JSON Schema is sent for generation

**Proves**: FR-005, FR-006 — `response_format` is `json_schema` with `strict: true` for
generation, and the schema has `additionalProperties: false`.

1. Set `LITELLM_MASTER_KEY` and enable LiteLLM debug logging (`--detailed_debug` on the
   litellm container, or inspect the gateway's outgoing request via a log step).
2. Run tailoring (Scenario 1 steps 3-4) with a model that supports `json_schema` (e.g.
   `deepseek-v4-pro` or the benchmark-selected primary).
3. Inspect the request body sent to LiteLLM:
   - `response_format.type` is `"json_schema"`.
   - `response_format.json_schema.strict` is `true`.
   - `response_format.json_schema.schema.additionalProperties` is `false`.
4. Run a `match` task (e.g. trigger a job match) and inspect its request:
   - `response_format.type` is `"json_object"` (unchanged — only generation upgraded).
5. **Pass**: generation uses strict schema; non-generation tasks are byte-identical to today.

## Scenario 5: Non-strict provider falls back to `json_object`

**Proves**: FR-006 — a provider that does not support `json_schema` does not silently degrade.

1. Point the `generation` chain's primary at a model that does not support `json_schema`
   (temporarily, in a dev config) — or simulate by removing `response_format` support in a
   mock provider.
2. Run tailoring.
3. The request falls back to `{"type": "json_object"}` and the existing JSON-parse retry loop
   in `CompleteStructured` handles any parse failure.
4. **Pass**: the run completes (or fails with a real error), never a silent prose degradation.

## Scenario 6: Model benchmark and selection

**Proves**: FR-007, SC-004 — the primary generation model is chosen from strictness data.

1. Run the benchmark fixture (a Go test or a `make` target that runs the tailoring pipeline
   against a fixed set of profiles × vacancies for each model in the generation chain).
2. Record: grounding violations, structural violations, JSON-parse failures, wall-clock time,
   per model.
3. The primary model is the one with the lowest combined violation rate that meets the 60s
   average bar (020-SC-007).
4. Update `gateway/config.yaml`'s `generation` primary to the selected model. Restart litellm.
5. Run tailoring again; the served-model log line (`served_model` in the gateway request log)
   shows the new model.
6. **Pass**: the selected model is documented with the benchmark results, and the chain still
   terminates at `local`.

## Scenario 7: No regression in run time

**Proves**: SC-006 — strictness checks do not add more than 10% to median run time.

1. Run the same master profile + vacancy through tailoring 10 times, before and after the
   feature.
2. Measure the median wall-clock time from the activity trail's first to last step.
3. **Pass**: the post-feature median is ≤ 110% of the pre-feature median.

## Test commands

```sh
make test-go                 # unit tests for the new grounding checks and prompt
make test-lint               # full merge gate (lint-go + lint-web + test-go + test-react)
make test-integration        # cross-service (if the strictness checks have an integration path)
```

The grounding-check unit tests (`apps/api/internal/generation/domain/*_test.go`) are the
primary verification: they construct a master, a merged resume, and assert the violation set.
The prompt test asserts the prompt text contains no removed field names. The gateway adapter
test asserts the `response_format` wire shape for both modes.