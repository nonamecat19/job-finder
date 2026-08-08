---

description: "Task list for the user-selectable summary model"
---

# Tasks: User-Selectable Summary Model

**Input**: Design documents from `/specs/034-resume-model-choice/`

**Prerequisites**: spec.md (rescoped), plan.md

**Scope note**: this list implements the **rescoped** feature — a choice over the summary stage
only. The spec's US1 as written ("pick the model that writes my resume") is answered by that; its
US2 (options measured, not guessed) is 038's harness and is not re-implemented here.

## Format: `[ID] [P?] Description`

---

## Phase 1: The catalogue and its guardrails

- [X] T001 Add `SummaryOption` and the four-option catalogue in `apps/api/internal/generation/domain/summary_option.go`: `standard` (task key `generation-summary`, the default), `premium`, `fast`, `local`. Each carries id, label, a one-line quality description and a relative cost indicator. Plan D1/D3. **Four options: standard (the pre-034 task key, unchanged), premium, fast, local**
- [X] T002 [P] Test the catalogue's invariants in `summary_option_test.go`: between 3 and 5 options, exactly one default, ids unique, every field non-empty, and the self-hosted option always present. Spec AC1. **TestSummaryCatalogueIsAUsableMenu plus three more in summary_option_test.go**
- [X] T003 Add the two new task keys `generation-summary-premium` and `generation-summary-fast` to `gateway/config.yaml` with fallback chains terminating at `local`, and bound reasoning on every openrouter deployment. Plan D1/D5. **generation-summary-premium and -fast, each chained through a second tier to local**
- [X] T004 Add both new keys to `requestedGenerationGroups` in `apps/api/internal/platform/llm/gateway_config_test.go`, so the existing invariants assert they have chains, terminate at local, and are declared. **Both added; the existing invariants now cover them, and the valid-config fixture grew to match**
- [X] T005 [P] Test that every catalogue option's task key is a group the gateway config declares and chains — the check that stops a Go-side catalogue drifting from the deployment artifact. Plan D3. **apps/api/internal/summarycatalogue_test.go, in package internal_test because it spans two layers the platform must not import across**

## Phase 2: Persistence

- [X] T006 Add migration `apps/api/internal/db/migrations/00039_summary_model_setting.sql`: singleton `SummaryModelSetting` table with the same `id = 'default'` guard as `ResumeShapeSetting`, an `optionId` text column defaulted to the catalogue default, and a seeded row. **00039, validated against the running Postgres in a rolled-back transaction**
- [X] T007 Add `apps/api/internal/db/queries/summarymodelsetting.sql` with `GetSummaryModelSetting` and `UpdateSummaryModelSetting`, then regenerate sqlc. **Both queries generated and exercised against the real schema**
- [X] T008 Implement `apps/api/internal/summarymodel/service.go` mirroring `resumeshape`: cached singleton, `Get`, `Update` (validating the id against the catalogue), `Reset`, and a `SummaryOption(ctx)` port method. **internal/summarymodel/service.go, mirroring resumeshape**
- [X] T009 [P] Unit-test the service: an unknown option id is rejected, a valid one is stored and cached, and a missing row falls back to the catalogue default rather than erroring. **Seven tests: unknown id rejected on write, stale id resolved to default on read, unreadable row non-fatal**

## Phase 3: The generation seam

- [X] T010 Add a `SummaryModelProvider` port to `apps/api/internal/generation/application/service.go` alongside `ShapeProvider`, and widen `GenerationRouters.Summary` to a per-option map. Plan Complexity row 2. **SummaryModelProvider port plus GenerationRouters.SummaryByOption; Summary stays a plain field so callers that know nothing about 034 are unchanged**
- [X] T011 Resolve the option **once at the top of a run**, beside `shapeConfig`, and pass the resolved provider down. A settings change mid-run must not alter the document being generated. Plan Structure Decision. **Resolved once at the top of tailorRendercvResume, beside the run trace**
- [X] T012 [P] Test the resolution seam: the chosen option's provider serves the summary stage, every other stage is untouched, and an unknown or unconfigured option resolves to the default rather than failing the run. Plan D5. **Six assertions in summary_option_routing_test.go, including that choosing a summary option disturbs no other stage**
- [X] T013 Record the chosen option on the run so the result can show which option produced it. `GeneratedDocument.summaryModel` (migration 00038) already records the served model; this adds the option id beside it. Spec AC2. **runProvenance.summaryOption, persisted via migration 00040 — the served model cannot answer which option was picked, since two options can land on the same upstream after a fallback**

## Phase 4: HTTP and the typed contract

- [X] T014 Add `SummaryModelOptionDto` and `SummaryModelSettingDto` in `apps/api/internal/dto/`, then run `make tygo-generate` so `packages/shared/src/generated.ts` carries them. Constitution III. **SummaryModelOptionDto, SummaryModelSettingDto, UpdateSummaryModelRequestDto; tygo-check passes**
- [X] T015 Implement `apps/api/internal/summarymodel/interfaces/http/handler.go`: `GET /settings/summary-model` returning the catalogue plus the current choice, and `PUT` to change it. Mirror the resume-shape handler. **GET/PUT/DELETE /v1/settings/summary-model**
- [X] T016 Accept an optional summary option on the tailoring request, apply it to that run, and persist it as the new default. Plan D4, spec AC2/AC4. **summaryOptionId on the tailoring request; applied to the run and persisted, with a failure to remember never costing the user the resume**
- [X] T017 [P] Handler tests: the catalogue is returned with exactly one option marked current, an unknown id is a 400, and a valid id round-trips. **Four handler tests; an unknown id is a 400 because the client picked it from a menu this API served**
- [X] T018 Wire it in `apps/api/cmd/server/compose.go`: one `Router` per hosted option, the service, and the handler mount. **One router per option built from the catalogue, so a new option wires itself**

## Phase 5: Dashboard

- [X] T019 Add the selector to the tailoring surface: the options with their labels, quality descriptions and cost indicators, with the current choice preselected. Spec AC1. **Summary writer selector on the tailoring surface, with the current option preselected and its description shown**
- [X] T020 Show which option produced a completed resume on the result surface. Spec AC2. **The finished resume names the option that wrote its summary**
- [X] T021 [P] Vitest coverage for the selector: renders every option, preselects the current one, and sends the choice on submit. **Four vitest cases, including that an untouched selector sends no option at all — which is what keeps the pre-034 request byte-identical**

## Phase 6: Polish

- [X] T022 [P] Document the choice in `specs/domains/resume-generation.md`: what the options are, why only the summary stage is exposed, and that measuring a candidate option is 038's job. **specs/domains/resume-generation.md, including why only the summary stage is exposed and that measuring the options is 038 job**
- [X] T023 Run `make test-lint` — must pass. **make test-lint exits 0; sqlc-check and tygo-check both clean**

## Dependencies

T001 blocks everything. T003/T004 before T005. T006→T007→T008→T009. T010/T011 before T012/T013.
T014 before T015/T017. T018 after T008 and T015. Phase 5 after T014.
