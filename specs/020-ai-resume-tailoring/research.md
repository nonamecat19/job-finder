# Phase 0 Research: Constrained AI Resume Tailoring

Resolved unknowns and design decisions for feature 020, derived from the spec (`spec.md`), constitution, and codebase exploration (see plan.md "Source Code" for the surveyed files).

## R1 — LLM output schema: merged payload vs per-field proposals

**Decision**: The LLM continues to return a single structured payload (`TailoredSections` extended — see R3), but a new **proposal generator** (`internal/generation/rendercv_proposals.go`) translates it into a list of atomic `EditProposal` records keyed to the *current baseline*. Proposals are persisted and presented to the user; merging only happens on acceptance.

**Rationale**:
- Continuing to ask the LLM for one payload keeps the prompt small and the existing grounding verifier (`verifyRendercvGrounding`) reusable — the verifier validates the *payload*, not the proposals.
- A per-field proposal generator is deterministic Go: it diffs the payload against the baseline and emits one row per changed `summary`, per-company `highlights`, each skill-group add/remove, and each skill add/remove. Per-field granularity (FR-005), per-skill atomicity (clarification Q2), and re-run diff-against-current-baseline semantics (FR-010, clarification Q4) all live here, not in the prompt.
- This isolates the LLM prompt changes from the diff UX, so a future model swap doesn't break the review surface.

**Alternatives**:
- *Per-field structured output from the model directly* (one tool call per bullet) — rejected: 10-30× the round trips, blows the 60s latency budget (SC-007).
- *Prompt the LLM to output a JSON array of proposals* — rejected: re-implementing the grounding verifier per-proposal, plus the LLM is already good at the whole-payload form (existing `selectAndTailor`).

## R2 — Single-page PDF: which renderer

**Decision**: A new `internal/generation/singlepage` package built on **chromedp** (already a dependency). A new ATS-clean HTML template renders `dto.Resume` (the 009 structured shape) — **not** the legacy JSON-Resume HTML template (which lacks skill-group rendering) and **not** the `rendercv` CLI (whose margins/font-size live in the user's design config, outside the spec's allow-list, and which offers no measurement API). The fitter does a measure pass with `chromedp.Runtime.evaluate` against `document.documentElement.scrollHeight` vs the printable height (page height − 2×margin), then iterates over a bounded density ladder until the content fits or the minimum bound is hit.

**Rationale**:
- chromedp gives a real DOM measurement before `page.PrintToPDF`, which is required to apply density controls *before* producing the PDF (`rendercv` and the legacy `htmlToPDF` print blindly).
- `page.PrintToPDF` already produces selectable text PDFs in the existing `HtmlPdfRenderer` — satisfies FR-007 / ATS-readability.
- Building on `dto.Resume` (the editable schema 009 already validates) means single-page rendering and the user's hand-edit UI share one source of truth; the user's Typst theme/design is *not* used for tailored exports, which is acceptable because the user opts into a tailored resume specifically to deviate from their master layout.
- Density ladder stays within professional, ATS-readable norms (Assumptions): page A4, body font from 11pt → 9pt in 0.5pt steps, margins from 14mm → 8mm in 2mm steps, line-height from 1.4 → 1.1 in 0.1 steps, bullet spacing 4px → 0. The fitter selects the largest combination that fits, or reports impossibility.

**Alternatives**:
- *Extend `RenderCvRenderer` (rendercv CLI)* — rejected: requires editing the user's YAML design to inject density; the design block is outside the spec's allow-list and the CLI offers no measurement API; one can't "try smaller and re-render" within the 60s budget reliably.
- *Reuse legacy `resume.html` JSON-Resume template* — rejected: doesn't render skill groups (a US3 requirement), and the per-company highlights layout doesn't match how 009 Roundtrips through `dto.Resume`.
- *Client-side `window.print()` from the dashboard* — rejected: produces inconsistent output across users' browsers/printers, breaks SC-002 determinism and ATS text-PDF guarantees.

## R3 — Extending `TailoredSections` to express skill-group add/remove and per-field proposals

**Decision**: Extend `TailoredSections` (`apps/api/internal/generation/rendercv.go:42-90`) with new optional fields, keeping the existing fields backward-compatible so the ad-hoc generation path is unaffected:
- `Summary *string` — unchanged (the AI-tailored summary text).
- `Skills.SkillGroupsToAdd []TailoredSkillGroupAdd {Label, Details}` — new groups to add (populated from existing user skill tags only — verified by grounding).
- `Skills.SkillGroupsToRemove []string` — group labels to remove (matched against master labels; never auto-applied; user confirm per clarification Q2).
- `Skills.SkillChanges []SkillChange {GroupLabel, AddTokens, RemoveTokens, ReplaceDetails *string}` — per-skill edits within existing groups, and the only way individual skills are added/removed.
- `Experience []TailoredExperience` — unchanged shape (`{Company, Highlights []string, Drop bool}`), but the `Highlights` field is now interpreted as a *full replacement* of that company's baseline highlights, and the proposal generator diffs it bullet-by-bullet against the baseline to emit one proposal per changed bullet.
- No new fields touch job title, employer, dates, education, certs, links.

**Rationale**:
- Backward-compatible: old `selectAndTailor` callers that ignore the new fields still work.
- The 009 schema admits skill groups with arbitrary labels and comma-tokenized details — `SkillGroupsToAdd`/`SkillGroupsToRemove` map cleanly. Multi-bullet diff lives in the Go proposal generator, not in the LLM, so devs can unit-test the diff independently of model nondeterminism.
- The grounding verifier (`verifyRendercvGrounding`) is extended minimally: (a) reject any `SkillGroupsToAdd` whose tokens are not all present in the user's master skill tags (`masterSkillTokens` already computed at `rendercv.go:362-371`), (b) reject any `SkillGroupsToRemove` label not in the master, and (c) reject any per-skill `AddTokens` whose tokens aren't in `masterSkillTokens`. This enforces "no fabricated skills" (FR-003/SC-005) at the verifier.

**Alternatives**:
- *Storing tailored resumes as merged master configs* (current behavior) — rejected by the spec: the user must accept/reject per field, so merged state discards the proposal boundary.
- *Prompting the LLM to emit proposals directly* — covered and rejected under R1.

## R4 — Persistence: new tables vs reuse `GeneratedDocument`

**Decision**: Introduce **two new tables** — `tailored_drafts` and `edit_proposals` — rather than overloading `GeneratedDocument`. See `data-model.md` for the schema.

**Rationale**:
- `GeneratedDocument` is a fire-and-forget merged blob with `version` + `pdfPath`; it carries no per-field review state, no "accepted/rejected" set, and no baseline pointer. Encoding the proposal lifecycle onto it would require either (i) one row per proposal (breaks ad-hoc queries and changes the meaning of `version`) or (ii) a jsonb blob duplicating the proposal list, which loses sqlc typing and the ability to `WHERE accepted=false` query.
- A dedicated `tailored_drafts` row per `(profile_id, job_id, is_adhoc)` carries the baseline snapshot (the master-at-creation-time content hash + the persisted baseline `RendercvMaster` after each acceptance), provenance, and the current state. A dedicated `edit_proposals` row per `(draft_id, field_type, field_key)` carries the per-proposal before/after/acceptance/traceability fields.
- New migration `00030_tailoring_drafts.sql` (next available goose version — current migrations top out at `00023_ai_feature_setting.sql`; verify at write time and pick the next unique non-duplicate number; constitution requires unique sequential).
- New `internal/db/queries/tailoring.sql`, `make sqlc-generate`, `make sqlc-check` gate.

**Alternatives**:
- *Single `tailoring_drafts` table with jsonb proposals* — rejected for queryability and the typed-contracts principle.
- *No persistence; proposals live in the SPA only* — rejected: re-runs must compare against the current baseline (FR-010) and the user may close the tab mid-review (503/network) — server-side durability is the only correct answer.

## R5 — HTTP route shape & worker wiring

**Decision**: New `TailoringHandler` with `Mount(r chi.Router)` exposing:
- `POST /api/jobs/{jobId}/tailor-resume` — enqueue a tailoring run on the existing `generate` queue (`TypeGenerate`) with an extended payload (`tailoringDraftID` carrying from the activity), or a thin sub-type. Activity recorder writes one activity row for the run; the user-facing API returns the new `tailored_draft.id` + activity id for polling.
- `GET /api/tailoring/{draftId}` — fetch the draft + its proposals + current baseline summary.
- `POST /api/tailoring/{draftId}/proposals/{proposalId}` `{action: accept|reject}` — apply or revert a field; mutates `tailored_drafts.baseline` and the proposal row in one tx.
- `POST /api/tailoring/{draftId}/export-pdf` — invoke the new single-page fitter; returns the produced document id (existing `GET /api/documents/{id}/pdf` already serves the file).
- `GET /api/tailoring/{draftId}/export-status` — short-poll the fitter outcome (fits / blocked-with-feedback / in-progress).

Wiring follows the existing pattern exactly (see plan.md "Source Code" snippet): add `composeTailoring`, add `app.Tailoring`, append `app.Tailoring.Mount` to the `NewRouter` variadic list in `servers.go:buildServers`. The handler is a thin HTTP/DTO translator; all business logic lives in `internal/tailoring`.

**Rationale**:
- Reuses the existing `generate` worker, `queue.Gate` admission, `activity.Recorder`, and `llm.Router` — no new asynq task type, no new concurrency governance to design.
- POST/accept-reject as separate endpoints (rather than PATCH on the whole draft) keeps each mutation idempotent and auditable, matches how `applications` are patched.
- Short-polling on export-status is simpler than WebSocket for a one-shot fit attempt and stays within TanStack Query's existing pattern.

**Alternatives**:
- *WebSocket push for proposal readiness* — rejected: nothing else in the dashboard uses WS, premature complexity for a ≤60s flow.
- *Synthesize the draft on the existing `POST /documents/tailor` ad-hoc path* — rejected: that path writes a merged `GeneratedDocument` with no proposal review state; layering the new flow on top would conflate two persistence models.

## R6 — Dashboard review UI

**Decision**: New `apps/dashboard/src/features/tailoring/` module, mounted in the existing job-detail panel and reusable for the "paste vacancy" ad-hoc flow. Components:
- `TailoringPanel` — triggers a run, shows indeterminate progress after 30s, navigable (run is non-blocking per FR-013).
- `ProposalReview` — grid of `EditProposal` cards grouped by field type; each card shows before/after diff text, a traceability chip ("from master:summary" / "from job posting: required_skills"), and accept/reject radio.
- `ExportSinglePage` — calls `/export-pdf`, polls status, on "blocked-with-feedback" renders the actionable message listing shorten-targets; on success streams the PDF (existing `documents/{id}/pdf` endpoint).
- `hooks.ts` — TanStack Query mutations/queries mirroring the new HTTP contract.

**Rationale**:
- The 009 profile feature already laid down the `features/profile` module shape with `SectionList`/`SectionEditor`; the tailoring module mirrors it so the codebase stays stylistically consistent.
- Diff cards are derived purely from the `EditProposal` DTO; no client-side diff computation, preserving the typed-contracts principle (client never reasons about LLM output shape).

**Alternatives**:
- *Render proposals inline in the existing profile editor* — rejected: the profile editor is for hand-editing the master resume; tailoring is a distinct lifecycle with accept/reject semantics that don't belong in the master-edit surface.

## R7 — Re-run baseline semantics (FR-010, clarification Q4)

**Decision**: `tailored_drafts.baseline` is initialized to the master-at-creation-time. On each accepted proposal, the baseline is updated in place within the same DB transaction as the `accept` mutation. On a re-run:
- The handler creates a **new draft row** (so the user can roll back the whole re-run), seeding `baseline` from the *current* `tailored_drafts.baseline` of the prior draft for the same `(profile_id, job_id)`.
- The proposal generator diffs the new LLM payload against that current baseline. Already-accepted edits are part of the baseline and therefore not re-surfaced (the user's explicit ask — clarification Q4 Q/A).
- The user retains the old draft to roll back to if the re-run disappoints.

**Rationale**: matches the Q4 answer exactly; avoids "ghost" re-proposals of accepted edits; gives an explicit rollback point; the proposal generator stays simple (diff against whatever the draft's `baseline` says).

**Alternatives**:
- *Re-use the same draft row across re-runs* — rejected: destroys the rollback affordance and conflates run boundaries.
- *Diff against the master always* — rejected by Q4.

## R8 — Grounding & meaning-preservation for highlights (edge case)

**Decision**: The current `verifyRendercvGrounding` checks identity of employers/dates/degrees across the merged master; it does not yet police "the AI didn't add new facts to a bullet." For the allow-list highlights edits, the proposal generator AND the grounding verifier together enforce:
- Per-company `highlights` in the LLM payload MUST be a permutation/rewording of the existing bullets for that company. The Go diff emits one "modified bullet" proposal per pairwise change (longest-common-subsequence over the existing bullet set; for any bullet that the LLM "drops," a *remove* proposal is emitted; for any AI-bullet that doesn't LCS-match an existing baseline bullet, the proposal is **dropped** with a `dropped:grounding_violation` traceability reason and the user sees nothing for it — meeting the edge-case in spec.md:86).
- Token-level heuristics reject bullets containing novel numbers (metrics), novel employer/timeline references, or novel credentials absent from the master bullet set.

**Rationale**: keeps the LLM free to *reword* but prevents it from *inventing scope*, which is the exact behavior the spec's edge case calls out. Heuristics are conservative (some valid rewordings will be rejected) rather than permissive — that tradeoff matches the Grounded Generation principle (better to under-tailor than to fabricate).

**Alternatives**:
- *LLM-as-judge post-pass* — rejected: extra LLM round trip, blows the latency budget, and judges are unreliable on small factual deltas.
- *Permit any rewording, surface a "verify" warning* — rejected: violates SC-005 (zero accepted edits with claims not in master).