# Implementation Plan: Single Source of Truth for Agent Context and Shared Types

**Branch**: `024-agent-context-consolidation` | **Date**: 2026-07-29 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/024-agent-context-consolidation/spec.md`

## Summary

Three workstreams. Measurement during planning changed the shape of two of them:

1. **Shared types** — the naive "make `index.ts` re-export `generated.ts`" plan **would lose type information**. Of the 56 duplicated shapes, 39 already disagree, and the dominant disagreement is 82 fields typed `T | null` by hand but `T?` by the generator. The hand-written form is the *correct* one — the Go DTOs use bare `*string` with no `omitempty`, so the API sends `"error": null`, never an absent key. Re-exporting would tell 47 consumer files the opposite. Design: fix what generation *can* express (`enum_style: union`), and layer the rest through a single generic rather than restating shapes.
2. **Documentation** — one operative rule per topic; add the branch/PR and worktree rules; correct four false claims this feature owns (a non-existent `plans/` directory, a non-existent `plan/` directory inside the constitution, and two references naming a single DTO file where there are ten). The two false Python claims are owned by the companion workflow-gates feature, which changes what the quality command runs — see the Dependencies table in spec.md.
3. **Agent stack** — the real defect is not two committed copies. **Neither** stack's prompts are committed: `.claude` is excluded by the user's global ignore file, `.opencode` by the repo's. The only committed configuration declares `opencode`, a stack that no longer exists here. A fresh clone gets a manifest pointing at nothing and zero speckit commands.

## Technical Context

**Language/Version**: TypeScript 5.6 (`packages/shared`, `apps/dashboard`); Go 1.26 (`apps/api/internal/dto`); Markdown and JSON for context documents

**Primary Dependencies**: tygo 0.2.21 (existing pin); no new dependency

**Storage**: none

**Testing**: `tsc --noEmit` across the workspace, `vitest run`, `go test ./...`, plus the existing `scripts/tygo-check.sh` drift gate

**Target Platform**: development tooling only — nothing ships to a runtime

**Project Type**: monorepo refactor of shared types + documentation consolidation

**Performance Goals**: none. Success is measured in eliminated duplication, not speed.

**Constraints**:

- **No runtime behaviour may change.** The wire format is fixed by the Go DTOs; this feature must not alter a single JSON key or value. Any change that would is out of scope by definition.
- **No consumer import may change** (FR-005). 47 dashboard files import `@job-finder/shared`; the public surface is frozen.
- **Type strictness must not regress** (SC-005). This is the binding constraint — see the Nullability section below.
- `.claude` is unstaged-by-default via `core.excludesFile=/home/nnc/.gitignore_global`, which lists `.claude`. Committing anything under `.claude/` needs an explicit negation in the repository's own `.gitignore`.

**Scale/Scope**: `packages/shared/src/index.ts` 70 interfaces / 91 exports; `generated.ts` 80 interfaces / 115 exports; 56 shapes defined in both; 39 of those already drifted; 82 nullability divergences; 3 genuinely missing fields; 47 consumer files

## Measured baseline

Every number below was computed against the working tree, not estimated. The implementation should re-run the same comparison to confirm nothing shifted.

| Finding | Count | Notes |
|---|---|---|
| Shapes defined in both files | 56 | of 70 hand-maintained interfaces (80%) |
| — byte-identical | 17 | safe to delete outright |
| — already drifted | 39 | each needs a deliberate resolution (FR-007) |
| `T \| null` (hand) vs `T?` (generated) | 82 fields | the dominant class; hand-written form is correct |
| Named alias flattened to `string` | 67 fields | e.g. `ActivityOp`, `ActivityState`, `Record<string, unknown>` → `{[key: string]: any}` |
| Literal union flattened to `string` | 2 fields | `JobDto.status`, `QueueBacklogDto.providerClass` |
| Fields present on one side only | 2 | `JobDto.application`, `JobDto.documents` — `SearchQuery.subscriptionUrl` is present in the Go DTO and generated output (not a divergence) |
| Hand-maintained types with no generated counterpart | 14 | genuinely consumer-only |
| Consumer files importing the shared package | 47 | import surface must stay frozen |

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Assessment |
|---|---|
| I. No Auto-Apply, Ever | **N/A** — no code path touching applications. |
| II. Grounded Generation | **N/A**. |
| III. Typed Contracts Across Service Boundaries | **This feature exists to enforce it.** Principle III forbids hand-maintained duplicate type definitions; 56 exist today, documented as required practice by `AGENTS.md`. Post-change, `index.ts` holds only consumer-only types and declarative narrowings — no duplicated shapes. |
| IV. Test Discipline Per Language | **Pass.** Existing suites gate the change; no test is removed or weakened. |
| V. Local-First, Self-Hosted by Default | **N/A** — no inference involved. |
| Governance — amendments re-check templates | **Applies, and is triggered.** Principle III's wording is *not* touched — the repository is brought into compliance instead. But `constitution.md:96` claims design documents live under a `plan/` directory that does not exist, and FR-015 requires stated rules to be true. Correcting it is an amendment, so the same change must bump the version (1.0.0 → 1.0.1, PATCH), update the Sync Impact Report, and re-check `.specify/templates/*` and the speckit skills (FR-016, FR-017). |

**Post-Phase-1 re-check**: passing. The design removes hand-maintained duplicates rather than blessing them, which moves the repository toward Principle III rather than away.

**Violations requiring justification**: none. Complexity Tracking omitted.

**Correction to an earlier draft of this plan**: it stated the constitution needed no amendment. That held for Principle III but overlooked the false `plan/` directory claim on line 96, which FR-015 puts in scope. The amendment is bookkeeping, not a change of principle — but skipping it would leave a corrected constitution carrying a stale version line, which is itself a new false statement.

## The nullability problem, and the chosen resolution

This is the crux of the feature and the reason it is not a one-line change.

**Observed**: `apps/api/internal/dto/activity.go` declares `Error *string \`json:"error"\`` — a pointer with **no** `omitempty`. Go marshals that to `"error": null`. tygo maps every `*T` to `field?: T` regardless of the tag, so `generated.ts` says `error?: string` — the key may be absent, and is never null. `index.ts` says `error: string | null`, which matches what the server actually sends. **The generated file is wrong about the wire format for 82 fields, and the hand-written file is right.**

Deleting the hand-written copy would therefore satisfy the letter of FR-001 while breaking SC-005 and silently misinforming 47 consumer files.

**Chosen resolution — three layers, in order:**

1. **Recover what generation can express.** Set `enum_style: union` in `apps/api/tygo.yaml`. Go const groups (`SourceKindAPI/Scrape/Sidecar`, `StatusFound/Shortlisted/…`) then emit as literal unions instead of `type SourceKind = string`. This alone fixes the 2 literal-union losses and several of the 67 alias flattenings, at the source, for free.
2. **Layer nullability declaratively.** Add one generic to the shared package:
   ```ts
   type Nullable<T, K extends keyof T> = Omit<T, K> & { [P in K]: Exclude<T[P], undefined> | null };
   ```
   `index.ts` then declares `export type ActivityRunDto = Nullable<Gen.ActivityRunDto, 'step' | 'jobId' | 'error' | …>`. Field *names* appear once; no field *type* is ever restated. This is exactly the layering FR-003 permits, and it keeps `tygo-check.sh` as sufficient enforcement — a new DTO field flows through automatically, and only a change in *nullability* needs a touch.
3. **Resolve the 2 real divergences explicitly** (FR-007). `JobDto.application` and `JobDto.documents` exist in the generated file but not the hand-written one. `SearchQuery.subscriptionUrl` was misattributed as one-sided: it is in the Go DTO (`SubscriptionURL`, `jobs.go:34`) and generated output, with zero dashboard consumers — so no decision is needed, the generated form is correct. Each is a decision, recorded in `data-model.md`, not an overwrite.

**Deliberately not chosen**: editing 82 Go DTO fields to add `omitempty`. That would make the generated output self-consistent, but it changes the wire format — an absent key instead of an explicit null — which the constraints forbid. Recorded in `research.md` as the correct long-term fix if the API contract is ever revisited.

## Project Structure

### Documentation (this feature)

```text
specs/024-agent-context-consolidation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 — the type inventory and each divergence's resolution
├── quickstart.md        # Phase 1 — how to verify nothing regressed
├── contracts/
│   ├── shared-types.md      # The frozen public surface of @job-finder/shared
│   └── doc-ownership.md     # Which document owns which rule
├── checklists/
│   └── requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
packages/shared/src/
├── generated.ts         # unchanged by hand; regenerated with enum_style: union
├── nullable.ts          # NEW — the Nullable<T, K> generic, ~6 lines
├── consumer-only.ts     # NEW — the 14 types with no backend counterpart, clearly labelled
└── index.ts             # REWRITTEN — re-exports + narrowings only; zero duplicated shapes

apps/api/tygo.yaml       # + enum_style: union

.specify/
├── init-options.json    # ai: opencode -> claude
├── integration.json     # installed_integrations: [opencode] -> [claude]
└── integrations/        # opencode.manifest.json removed

.gitignore               # negate the global .claude exclusion so agent config is committable
.claude/skills/          # speckit-* skills become tracked (currently untracked)

AGENTS.md                # corrected; becomes the owner of workflow rules
.specify/memory/constitution.md   # unchanged unless Principle III wording needs it
```

**Structure Decision**: split `index.ts` into three files by *authority* — generated (never hand-edited), consumer-only (hand-maintained, labelled), and narrowings (declarative, derived). Today a reader cannot tell which of the 91 exports is which; after the split, the file a symbol lives in answers that question. `index.ts` remains the single entry point so the public import surface is untouched.

## Phased Delivery

| Phase | Delivers | Spec coverage | Gate |
|---|---|---|---|
| **P1** | `enum_style: union`, regenerate, confirm no consumer breaks | prerequisite for US1 | `tsc --noEmit` + `vitest` green; `tygo-check.sh` green |
| **P2** | `Nullable<>` generic, split into `consumer-only.ts`, rewrite `index.ts`, delete 56 duplicates, resolve the 3 divergences | US1, FR-001..FR-009 | zero duplicated shapes; no import changed; all suites green |
| **P3** | Document ownership: AGENTS.md corrected/extended, constitution amended for the false `plan/` claim (with the FR-016 procedure), DTO file references corrected to the directory | US2, FR-010..FR-019 | every rule appears once; every claim true; constitution version bumped |
| **P4** | Single agent stack: `.gitignore` negation, commit `.claude/skills`, switch manifests to claude, delete opencode manifest | US3, FR-020..FR-025 | fresh clone gets working speckit commands |

P1 before P2 — regenerating with unions first means the narrowing layer only has to cover what generation genuinely cannot express, instead of papering over a fixable gap.

## Cross-feature dependency

Feature 023 plans to commit `.claude/settings.json` for its hooks (its FR-028). That is **blocked by the same global-ignore problem** this feature's P4 fixes. Whichever feature lands first must carry the `.gitignore` negation; the other then depends on it. Recorded in both plans.

Feature 023's `regen-tygo.sh` hook also only closes half the drift gap while the hand-maintained duplicate exists. After this feature, that hook is sufficient on its own.

## Risks

| Risk | Impact | Mitigation |
|---|---|---|
| `enum_style: union` changes more output than expected | Consumer breakage in P1 | It is a one-line config change, fully reversible; `tsc --noEmit` across 47 consumers is the check. Run it before writing any narrowing. |
| The `Nullable<>` list drifts from reality | Silent re-introduction of the original bug | The field-name list is derivable — the comparison script used for the baseline should be committed as a check, so a missing entry fails CI rather than waiting to be noticed. |
| The 3 divergences hide a real bug | Wrong resolution shipped | Each is decided against the Go DTO, which is authoritative for the wire format — not against whichever file looks more complete. |
| Committing `.claude/` exposes local settings | Leak of machine-specific config | Commit `.claude/skills/` and (for 023) `settings.json` only. `settings.local.json` stays ignored — it holds the 140-entry allowlist with a database password in it. |
| Consumer-only types are actually stale | Dead code retained and blessed | Check each of the 14 for consumers before labelling; unreferenced ones are deleted, not documented. |
