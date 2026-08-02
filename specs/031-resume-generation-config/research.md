# Phase 0 Research: Configurable Resume Generation Shape

**Feature**: `031-resume-generation-config` | **Date**: 2026-08-02

All Technical Context unknowns are resolved below. No `NEEDS CLARIFICATION` markers remain.

---

## R1. Where the hardcoded shape currently lives

**Finding** (source survey, not a decision):

| Dimension | Current value | Location |
|-----------|---------------|----------|
| Summary length (tailor) | "3-4 sentences" | `generation/application/rendercv_llm.go:139` |
| Bullets per experience entry | "TOP 8-10 most relevant highlights" | `generation/application/rendercv_llm.go:133` |
| Summary length (expand) | "4-5 sentences" | `generation/application/rendercv_llm.go:210` |
| Bullets per entry (expand) | "2-3 more … aim for 10-12 per job" | `generation/application/rendercv_llm.go:211` |
| Summary length (condense) | "2-3 tight sentences" | `generation/application/rendercv_llm.go:261` |
| Bullets per entry (condense) | "TOP 5-6 most relevant per job" | `generation/application/rendercv_llm.go:262` |
| Target pages | literal `2` / `1` / `<= 2` | `generation/application/service.go:252,256,290` |
| Skills volume | "keep all relevant keywords (do not trim)" | `generation/application/rendercv_llm.go:137` |
| Projects | not addressed by any prompt; pass through `MergeTailored` untouched | `generation/domain/rendercv.go:251-294` |

**Implication**: parameterising the six prompt literals plus the three page-loop literals covers every configurable dimension in the spec except projects and section-disable, which need new code paths.

---

## R2. Persistence mechanism for the config

**Decision**: One singleton row in a new table `ResumeShapeSetting`, typed columns (not JSON blob), goose migration `00034`, accessed via sqlc.

**Rationale**:
- Matches the established precedent exactly: `AutoGenerateSetting` (migration `00019`) was a singleton row with a `CHECK ("id" = 'default')` guard, later generalised into `AiFeatureSetting` (`00023`). Constitution III requires sqlc-generated DB access; a typed-column table gives compile-time-checked field access with zero hand-written mapping.
- Typed columns make DB-level range guards (`CHECK`) possible as a second line of defence behind service-level validation (FR-004).
- Latest existing migration is `00033_drop_llm_task_setting.sql`, so `00034` is the next free sequential version (constitution constraint: never reuse a goose version).

**Alternatives considered**:
- *JSONB blob column*: fewer migrations when the config grows, but defeats sqlc typing, moves validation entirely into Go, and produces an untyped DTO boundary — rejected against constitution III.
- *Env-var / config file*: rejected outright — FR-001/FR-002 require change without restart, and `internal/config` values are read once at boot.
- *Per-profile config on the `Profile` row*: closer to where the master resume lives, but the spec's Assumptions scope this to a single global config; per-profile is explicit non-scope. Rejected as premature.

---

## R3. How the config reaches the generation pipeline

**Decision**: A narrow port on `generation/application.Service` — `ShapeProvider interface { Shape(ctx) domain.ShapeConfig }` — satisfied by `*resumeshape.Service`, resolved **once per generation run** and threaded as a value through tailoring, merging and rendering.

**Rationale**:
- Resolving once at the top of a run and passing the value satisfies the spec's in-flight edge case ("the in-flight job completes with the settings that were in effect when it started") for free, with no snapshotting machinery.
- A port (not a concrete dependency) keeps `generation` free of an import on the settings package and makes the existing fake-based tests in `generation/application/ports_test.go` trivially extendable.
- `resumeshape.Service` caches the row in memory and refreshes on update — the same load-at-boot-and-cache pattern `aifeature.NewService(ctx, p.DB.Queries)` already uses, so a generation run costs no extra DB round-trip.

**Alternatives considered**: passing `*resumeshape.Service` directly (creates a package dependency in the wrong direction); re-reading the DB at each pipeline step (extra queries, and settings could change mid-run, violating the edge case).

---

## R4. Enforcement strategy: prompt vs deterministic

**Decision**: Split by whether an approximate match is acceptable.

| Dimension | Enforcement | Why |
|-----------|-------------|-----|
| Summary length | Prompt only | Spec explicitly accepts "close, not exact"; a deterministic sentence-trim would cut mid-thought and damage readability. |
| Bullets per experience entry | Prompt (range) + deterministic upper-bound clamp | The range is a target, but the max must hold or the page budget is unpredictable. Clamping keeps the first N (the model already returns them in relevance order). |
| Skills volume | Prompt only | Detail strings are comma-joined prose; truncating them mid-list produces trailing-comma artifacts. |
| Section enable/disable | Deterministic only | Binary and non-negotiable; never entrusted to a model. |
| Project count | Deterministic (with LLM ranking input when limits are set) | Hard cap (FR-014). |
| Bullets per project | Deterministic truncate | Hard cap (FR-014). |
| Target pages | Render loop | Measured from the actual PDF, the only ground truth. |

**Rationale**: mirrors the existing architecture — feature 028 already moved section/job/order invariants from model instruction to deterministic merge because the model was unreliable at them, while leaving phrasing to the model. This decision extends that same line rather than inventing a new policy.

**Alternatives considered**: all-deterministic (rejected — mangles prose, and cannot lengthen anything); all-prompt (rejected — FR-014 requires hard limits, and 028's history shows the model does not reliably honour count instructions).

---

## R5. Disabling a section without breaking existing invariants

**Decision**: Remove the section key from `cv.sections` **and** its entry from the synthetic `_order` list, immediately after `MergeTailored`, before grounding verification.

**Rationale**:
- `ParseRendercv` writes the author's section order into a synthetic `_order` key (`rendercv_config.go:12,34-41`); leaving a stale entry there would name a section that no longer exists downstream.
- `VerifyRendercvGrounding` check #2 asserts merged sections are a **subset** of master sections (`rendercv_grounding.go:30-39`) — a deletion is already legal under that rule, so FR-020 needs no exemption carve-out in the grounding verifier. This was worth confirming before committing to the approach.
- `VerifyStructureIntegrity` only inspects `summary` and `experience` for years assertions (`rendercv_structure.go:121-158`); removing `skills` or `projects` cannot trip it. Disabling `summary` or `experience` is not offered, so the verifier keeps full coverage.

**Alternatives considered**: filtering at render time in `rendercv_renderer.go` (leaves the disabled section visible to grounding and to any future consumer of the merged master — rejected as a leaky half-measure); asking the LLM to drop it (feature 028 deliberately removed `SectionsToDrop` from `TailoredSections`; reintroducing it would regress that decision).

---

## R6. Projects: how to select and tailor without breaking grounding

**Decision**: Add `TailoredProject{Name, Highlights}` to `TailoredSections`. `MergeTailored` matches by normalised project name against the deep-cloned master, replaces **only** the `highlights` field, then deterministically keeps at most `ProjectMax` entries **in master order**, truncating each to `ProjectBulletMax`. Name, `url`, `start_date`, `end_date` come from the clone and are never read from the model payload.

**Rationale**:
- This is the identical shape of the existing experience path (`rendercv.go:277-291`: index master by normalised key, mutate the entry map in place, everything else passes through from the clone). Reusing it means the "model cannot corrupt identity fields" property is structural, not a rule to remember.
- Keeping master order for the retained subset matches the experience rule ("Keep experience entries in the EXACT order shown in the master; do not reorder") and satisfies FR-019 without any ordering logic.
- Selection: when a project limit is configured, the prompt asks for the relevant projects; the merge intersects the model's set with the master's and caps it. If the model returns too many or unknown names, the cap and the name lookup silently correct it, and the unknown name is reported by grounding.

**Grounding extension** (constitution II): merged project names ⊆ master project names (any addition is a violation, mirroring the company check at `rendercv_grounding.go:18-28`); at `GroundingStrict`, each project's highlight tokens must come from that project's own master bullet token pool.

**Alternatives considered**: letting the model return whole project objects (rejected — reopens the fabricated-URL/date surface that the experience path was deliberately designed to close); selecting projects purely deterministically by master order (simpler, but ignores vacancy relevance, which is the whole point of tailoring — rejected as a fallback rather than the primary path, though it *is* the fallback when the model's selection is empty).

---

## R7. Preserving today's behaviour by default (FR-003)

**Decision**: Defaults reproduce current literals exactly, and the projects path is **inert by default** — `ProjectMax = 0` and `ProjectBulletMax = 0` are documented sentinels meaning "unlimited". When both are `0`, no projects block is added to the prompt and no project truncation runs, so projects pass through verbatim exactly as they do today.

| Setting | Default | Reproduces |
|---------|---------|------------|
| `summaryLines` | `4` | "3-4 sentences" |
| `skillsEnabled` | `true` | current behaviour |
| `skillsMaxGroups` | `0` (all) | "do not trim" |
| `experienceBulletsMin` / `Max` | `8` / `10` | "TOP 8-10" |
| `targetPages` | `2` | literal `2` in `renderResume` |
| `projectsEnabled` | `true` | projects currently render |
| `projectsMin` / `Max` | `0` / `0` (all) | verbatim pass-through |
| `projectBulletsMax` | `0` (all) | verbatim pass-through |

**Rationale**: a `0`-means-unlimited sentinel keeps the schema flat (no nullable columns, no separate "limit enabled" booleans) and makes the default row a no-op through every new code path. This is what makes SC-002 (post-feature output matches pre-feature output) achievable by construction rather than by tuning.

**Alternatives considered**: nullable columns with `NULL` = unlimited (equivalent semantics but forces pointer types through the DTO and into TypeScript as `number | null`, for no gain); a separate `projectLimitEnabled` boolean (redundant with `max > 0`).

---

## R8. Page-target loop shape

**Decision**: Generalise `renderResume` to compare against `cfg.TargetPages` with a bounded attempt budget (`shapeAttempts = 2`, matching the existing `groundingAttempts = 2` idiom): render → count → if under target, expand; if over target, apply `CompactDesign` then condense → re-render → accept the best result reached. Final page count and any miss are written to the activity record via `rec.Step` / `rec.Ok` meta.

**Rationale**:
- The existing loop already implements exactly this control flow against the constant `2` (`service.go:239-305`); it degrades gracefully on every failure (`slog.Warn` + return what we have). Generalising preserves that behaviour, including FR-021 (never fail, return best result).
- `infrastructure.CountPages(pdfPath)` already provides the measurement; nothing new is needed to verify SC-005.
- Bounding attempts keeps worst-case LLM calls per generation unchanged from today.

**Alternatives considered**: iterating until the target is hit (unbounded LLM spend, and unreachable targets — a 1-page target with a large master — would never terminate; rejected, and FR-021 exists precisely to define this case).

---

## R9. Conflict resolution: page target vs section lengths (FR-016)

**Decision**: The page target wins. When the condense path runs, the summary/bullet targets are re-issued as *reduced* targets (one step below the configured range) rather than as the configured range, and `rec.Step` records `{"conflict": "page_target_overrides_section_lengths"}`.

**Rationale**: FR-016 states the precedence directly. Re-issuing reduced targets is necessary because otherwise the condense prompt would carry instructions that contradict its own purpose ("keep 8-10 bullets" while asked to shorten). Recording the conflict satisfies the observability half of FR-016 and feeds SC-005's "every miss reports the reason".

---

## R10. Recording config and shortfalls on the generation (FR-006, FR-017)

**Decision**: Use the existing `activity.Recorder` meta channel — `rec.Step(ctx, "resume shape config", meta)` at the start of the run, and a shortfall step whenever a configured minimum could not be met from the master. No schema change to `GeneratedDocument`.

**Rationale**: `Recorder.Step(ctx, label string, meta map[string]any)` (`activity/recorder.go:95`) already persists arbitrary metadata per generation and is already used by this exact pipeline for page-count and grounding events. Adding a column to `GeneratedDocument` would duplicate a working mechanism and require a second migration.

**Alternatives considered**: a `shapeConfig` JSONB column on `GeneratedDocument` (better for querying historical shape across documents, but nothing in the spec asks to query it — FR-006 only requires that a past result can be explained, which the activity trace already delivers).

---

## R11. Settings API and dashboard surface

**Decision**: `GET /v1/settings/resume-shape`, `PUT /v1/settings/resume-shape`, `DELETE /v1/settings/resume-shape` (reset to defaults), handled by `resumeshape/interfaces/http`, mounted alongside the existing `/v1/settings/ai-features` routes. Dashboard gets a `ResumeShapeCard` on the existing settings page, wired through `lib/api.ts` + `queryKeys.ts` + TanStack Query hooks in `features/settings/hooks.ts`.

**Rationale**: `AiFeatureHandler` (`aifeature/interfaces/http/aifeature.go`) is a direct template — same `Mount(r chi.Router)` shape, same `httpx.WriteJSON` / `httpx.DecodeJSON` / `httpx.WriteError` helpers, same validate-then-update ordering that gives FR-004's all-or-nothing rejection for free (validation precedes any write). The dashboard side mirrors `AiFeatureSettingsCard` + its hooks 1:1. `DELETE` for reset is chosen over `POST .../reset` because reset means "remove my overrides", which is the semantic `DELETE` carries.

**Alternatives considered**: `PATCH` for partial updates (rejected — FR-004 demands whole-payload validation with nothing stored on failure; a full `PUT` makes that the natural implementation); a dedicated settings page (rejected — one card on the existing page satisfies SC-009's "single place").

---

## Summary of decisions

| ID | Decision |
|----|----------|
| R2 | Singleton `ResumeShapeSetting` table, typed columns, goose `00034`, sqlc access |
| R3 | `ShapeProvider` port on the generation service; resolved once per run, passed by value |
| R4 | Prompt enforcement for prose targets, deterministic enforcement for counts and switches |
| R5 | Section disable = delete from `cv.sections` + `_order`, post-merge, pre-verify |
| R6 | `TailoredProject{name, highlights}`; identity fields from the master clone; grounding extended |
| R7 | Defaults reproduce today's literals; `0` = unlimited keeps the projects path inert by default |
| R8 | Page loop generalised to `targetPages` with a bounded attempt budget; never fails |
| R9 | Page target overrides section lengths; conflict recorded |
| R10 | Config + shortfalls recorded through the existing `activity.Recorder` meta |
| R11 | `GET`/`PUT`/`DELETE /v1/settings/resume-shape`, modelled on `AiFeatureHandler` + one dashboard card |
