# Data Model: Single Source of Truth for Agent Context and Shared Types

No database entities. The model here is the **type inventory** of `@job-finder/shared` — which types exist, who owns each, and how each divergence is resolved — plus the ownership map for context documents.

## Ownership classes

Every export in the shared package falls into exactly one class after this change. Today a reader cannot tell which class a symbol belongs to; after the split, the file it lives in answers that.

| Class | File | Authority | Hand-edited? |
|---|---|---|---|
| **Generated** | `generated.ts` | Go DTOs via tygo | never |
| **Narrowing** | `index.ts` | derived from generated | field *names*; a field's type only where generation cannot express the constraint |
| **Consumer-only** | `consumer-only.ts` | hand-written, no backend counterpart | yes, explicitly |
| **Entry point** | `index.ts` | re-exports all three | structural only |

**Invariant**: no shape is defined in more than one class. A shape with a Go counterpart is Generated, optionally wrapped by a Narrowing. Fourteen types have no counterpart and are Consumer-only.

---

## Baseline inventory

| Measure | Count |
|---|---|
| `index.ts` interfaces / total exports | 70 / 91 |
| `generated.ts` interfaces / total exports | 80 / 115 |
| Shapes defined in both | 56 |
| — byte-identical (delete outright) | 17 |
| — drifted (resolve deliberately) | 39 |
| Hand-only, no generated counterpart | 14 |
| Consumer files importing the package | 47 |

---

## Divergence classes and resolutions

### Class A — nullability (82 fields, ~30 interfaces)

| Side | Form | Correct? |
|---|---|---|
| `index.ts` | `error: string \| null` | **yes** — matches the wire |
| `generated.ts` | `error?: string` | no — key is never absent |

Cause: Go declares `Error *string \`json:"error"\`` — a pointer with no `omitempty`, which marshals to an explicit `null`. tygo maps every `*T` to `field?: T` regardless of tag.

**Resolution**: the `Nullable` generic.

```ts
// packages/shared/src/nullable.ts
export type Nullable<T, K extends keyof T> =
  Omit<T, K> & { [P in K]: Exclude<T[P], undefined> | null };
```

```ts
// packages/shared/src/index.ts
export type ActivityRunDto = Nullable<Gen.ActivityRunDto,
  'step' | 'jobId' | 'sourceKey' | 'refId' | 'error' | 'startedAt' | 'finishedAt' | 'elapsedMs'>;
```

**Validation rules**
- In *this* class, only field **names** may appear. Nullability is fully expressible by the generic, so a restated field type here is a duplication defect (FR-003). Class B is the exception — see below.
- A new DTO field flows through with zero hand edits — only a nullability change needs a touch.
- The name list is derivable from the Go DTOs (pointer, no `omitempty`), so the comparison script that produced this baseline should be committed as a check. A missing entry then fails CI rather than waiting to be noticed.

### Class B — flattened aliases and unions (69 fields)

| Example | `index.ts` | `generated.ts` |
|---|---|---|
| `ActivityRunDto.op` | `ActivityOp` | `string` |
| `ActivityRunDto.meta` | `Record<string, unknown>` | `{[key: string]: any}` |
| `JobDto.status` | `ApplicationStatus \| 'hidden'` | `string` |
| `QueueBacklogDto.providerClass` | `'local' \| 'hosted' \| null` | `string` |

**Resolution, in order**:
1. `enum_style: union` in `apps/api/tygo.yaml` recovers Go const groups (`SourceKind`, `ApplicationStatus`, …) as real literal unions at generation time. Apply and re-measure **before** writing any narrowing.
2. Whatever remains — a consumer-side narrowing with no Go const group behind it, such as `JobDto.status`'s extra `'hidden'` — is layered:
   ```ts
   export type JobDto = Omit<Gen.JobDto, 'status'> & { status: ApplicationStatus | 'hidden' };
   ```
   This **does** restate one field's type, and FR-003 permits it: the constraint is not expressible by generation, and only the narrowed field is written. What FR-003 forbids is copying the whole field list, or restating a field the narrowing does not change. Expect a handful of these, not thirty — if the count grows, the constraint probably belongs in the Go DTO instead.
3. `Record<string, unknown>` versus `{[key: string]: any}` is a strictness difference, not a shape difference; prefer the stricter hand-written form via `type_mappings` if expressible, otherwise a narrowing.

### Class C — genuinely divergent fields (3)

| Field | State | Resolution |
|---|---|---|
| `JobDto.application` | generated only | Go DTO has it ⇒ hand file is stale. Adopt generated. |
| `JobDto.documents` | generated only | Same. |
| `SearchQuery.subscriptionUrl` | hand only | Absent from the Go DTO. Check consumers: keep as Consumer-only if referenced, delete if not. |

These three are the proof that "update both files field-for-field" does not work — two API fields never reached the consumer type.

### Class D — identical (17)

Delete from `index.ts`, re-export from `generated.ts`. No decision needed.

---

## Consumer-only types (14)

Types with no generated counterpart. Move to `consumer-only.ts` with a header stating that they are hand-maintained by design and are not an exception to Principle III.

**Validation rules**
- Each must be checked for actual consumers before being retained. An unreferenced type is deleted, not documented (a "consumer-only" label on dead code blesses it permanently).
- Any that turn out to have a Go counterpart under a different name belong in Class A/B instead.
- Adding to this file is a deliberate act: a new type here needs a comment saying why it has no backend counterpart.

---

## Public surface (frozen)

`index.ts` remains the only entry point. All 91 exports keep their names and remain importable from `@job-finder/shared`.

**Validation rule (FR-005, SC-004)**: zero of the 47 consumer files may change an import. `tsc --noEmit` across the workspace plus `vitest run` is the check. A required import change means the refactor has leaked out of the package.

---

## Document ownership map

One operative rule per topic (FR-010). Where both documents speak, the constitution states the principle and `AGENTS.md` states the procedure — never a second, differently-worded rule.

| Topic | Owner | Other documents |
|---|---|---|
| Type-sharing principle | `constitution.md` III | `AGENTS.md` refers to it |
| How to regenerate types | `AGENTS.md` | — |
| Branch and pull-request rule | `AGENTS.md` | enforced by feature 023 |
| Worktree lifecycle | `AGENTS.md` | — |
| What the quality command covers | `AGENTS.md`, matching the Makefile | — |
| Test discipline principle | `constitution.md` IV | `AGENTS.md` refers to it |
| Migration numbering | `constitution.md` (Tech Constraints) | — |

**Statements to delete or correct** (each false today):

| Location | Claim | Why false | Owner |
|---|---|---|---|
| `AGENTS.md:22` | "`index.ts` is hand-maintained … update both when adding a DTO field" | the practice ceases to exist after this feature | this feature |
| `AGENTS.md:11` | "`plans/` — implementation plans derived from specs" | neither `plans/` nor `plan/` exists; plans live at `specs/<nnn>-<slug>/plan.md` | this feature |
| `AGENTS.md:17,22` | DTOs live in `apps/api/internal/dto/dto.go` | 10 of the 11 files in that directory declare exported DTO structs; feature 023's hook filter already uses the glob, so docs and tooling disagree. The constitution makes no such claim | this feature |
| `constitution.md:96` | "Design/plan docs … are written under `plan/`" | that directory does not exist | this feature |
| `AGENTS.md` | "`make test-lint` — full test suite (Go + React + Python) + lint" | no Python in the repository; no lint currently runs | **feature 023** |
| `package.json` | `"test:python": "make test-python"` | the make target does not exist | **feature 023** |

**Principle III is not amended** — it was right, the practice was wrong, and compliance is the fix.

**The constitution is nonetheless amended**, for the unrelated `plan/` correction above. FR-015 requires stated rules to be true, and any edit inside the governing principles triggers the FR-016 procedure regardless of size: version 1.0.0 → 1.0.1 (PATCH), `Last Amended` updated, Sync Impact Report extended, and `.specify/templates/*.md` plus the `speckit-*` skills re-checked in the same change.

---

## Agent stack configuration

| File | Now | After |
|---|---|---|
| `.specify/init-options.json` | `"ai": "opencode"` | `"ai": "claude"` |
| `.specify/integration.json` | `installed_integrations: ["opencode"]` | `["claude"]` |
| `.specify/integrations/opencode.manifest.json` | committed | deleted |
| `.specify/integrations/claude.manifest.json` | committed | retained |
| `.claude/skills/speckit-*` | **untracked** (global ignore) | committed |
| `.claude/settings.local.json` | untracked | stays untracked — holds a DB password |
| `.claude/worktrees/` | untracked | stays untracked |
| `.gitignore` `.opencode` entry | present | removed with the directory |

**Required `.gitignore` negation** — a file cannot be re-included while a parent directory is excluded, and `~/.gitignore_global` excludes `.claude`:

```gitignore
!.claude/
.claude/*
!.claude/skills/
!.claude/settings.json
```

**Validation rule**: a fresh clone must yield working speckit commands. That is the test for FR-020–FR-025, and it fails today.
