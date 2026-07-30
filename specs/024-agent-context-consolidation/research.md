# Phase 0 Research: Single Source of Truth for Agent Context and Shared Types

Every finding below was measured against the working tree or verified against upstream documentation. The two most consequential findings contradict assumptions in the spec.

---

## R1 — Can `index.ts` simply re-export `generated.ts`?

**Decision: No.** Not without losing type information that 47 consumer files depend on. The duplication is removed, but through a layered derivation rather than a plain re-export.

**Method**: parsed both files, extracted every `export interface`, compared field name / optionality / normalised type.

```
pairs=56  identical=17  drifted=39
null-union:         82 fields
other (alias lost): 67 fields
literal-union-lost:  2 fields
missing-field:       3 fields
```

**The dominant finding — generation is wrong about the wire format.**

```go
// apps/api/internal/dto/activity.go
Error      *string `json:"error"`       // pointer, NO omitempty
FinishedAt *string `json:"finishedAt"`
ElapsedMs  *int64  `json:"elapsedMs"`
```

Go marshals a nil pointer without `omitempty` to an explicit `null`. The server therefore always sends the key, sometimes with a null value.

```ts
// generated.ts — says the key may be ABSENT, and is never null
error?: string;

// index.ts — says the key is always present, sometimes null
error: string | null;
```

`index.ts` is correct; `generated.ts` is not, for **82 fields**. tygo maps `*T` → `field?: T` unconditionally, ignoring the `omitempty` tag. Deleting the hand-written file to satisfy FR-001 would satisfy the letter of the requirement while breaking SC-005 and misinforming every consumer about null handling.

**Rationale for the layered approach**: the nullability information genuinely does not exist in the generated output, so it has to come from somewhere. The cheapest honest form is a list of field *names* per interface, with types still derived:

```ts
type Nullable<T, K extends keyof T> = Omit<T, K> & { [P in K]: Exclude<T[P], undefined> | null };
export type ActivityRunDto = Nullable<Gen.ActivityRunDto, 'step' | 'jobId' | 'error' | 'startedAt' | 'finishedAt' | 'elapsedMs'>;
```

No field type is restated, so adding a DTO field still requires zero hand edits, and `scripts/tygo-check.sh` remains sufficient enforcement. Only a change in *nullability* needs a touch — which is the irreducible residue of the generator's limitation. This is precisely the layering FR-003 permits.

**Alternatives considered**:
- *Plain re-export* — loses 82 nullability facts, 67 alias narrowings and 2 literal unions. Rejected: violates SC-005.
- *Add `omitempty` to 82 Go fields* — makes the generated output self-consistent and lets a plain re-export work. **Rejected: changes the wire format** from `"error": null` to an absent key, which the plan's constraints forbid. This is the correct long-term fix if the API contract is ever revisited deliberately; recorded here so the option is not lost.
- *Post-generation transform script* — a sed pass over `generated.ts`. Rejected: adds a second generator to maintain, and makes `tygo-check.sh` compare against a mutated file.
- *Patch or fork tygo* — rejected outright; unmaintainable for one project.

---

## R2 — Can any of the loss be fixed at the source?

**Decision: Yes, partially. Set `enum_style: union` in `apps/api/tygo.yaml`.**

Current config has no `enum_style`, so Go const groups degrade:

```go
type SourceKind string
const (
    SourceKindAPI     SourceKind = "api"
    SourceKindScrape  SourceKind = "scrape"
    SourceKindSidecar SourceKind = "sidecar"
)
```

emits `export type SourceKind = string;` plus three consts — while `index.ts` hand-maintains the real union `'api' | 'scrape' | 'sidecar'`.

tygo documents `enum_style` with values `const`, `enum`, `union`. Setting `union` recovers these at generation time, removing them from the narrowing layer entirely.

**Sequencing**: this must run **before** the narrowing layer is written (plan P1 before P2). Writing narrowings first would paper over gaps the config change fixes for free, leaving permanent hand-maintained entries that did not need to exist.

**Verification**: the change is one line and fully reversible. `tsc --noEmit` across the 47 consumers is the gate. Expect it to fix the 2 literal-union losses and an unknown share of the 67 alias flattenings; the exact figure is measured during implementation, not guessed here.

**Not available**: no tygo option controls pointer-to-optional versus pointer-to-nullable. The README documents `path`, `output_path`, `indent`, `type_mappings`, `frontmatter`, `exclude_files`, `extends`, `enum_style`, `flavor` — none addresses nullability. `type_mappings` works per *type*, not per field, so it cannot express "this pointer is nullable, that one is optional".

---

## R3 — What are the 3 genuine divergences?

**Decision**: resolve each against the Go DTO, which is authoritative for the wire format — never against whichever file looks more complete.

| Field | State | Resolution basis |
|---|---|---|
| `JobDto.application` | in `generated.ts`, absent from `index.ts` | Present in the Go DTO ⇒ the hand-written file is stale. Adopt the generated form. |
| `JobDto.documents` | in `generated.ts`, absent from `index.ts` | Same. |
| `SearchQuery.subscriptionUrl` | in `index.ts`, absent from `generated.ts` | Absent from the Go DTO ⇒ either a consumer-only addition or dead. Check consumers; keep as consumer-only if used, delete if not. |

These are the concrete proof that the "update both files field-for-field" practice does not work: two fields have been in the API for some time and never reached the consumer-facing type.

---

## R4 — Are there really two agent stacks?

**Decision: No — and the actual state is worse.** Neither stack's prompts are committed.

```
$ git ls-files .claude          →  0 files
$ git ls-files .opencode        →  0 files
$ git check-ignore -v .claude/skills
  /home/nnc/.gitignore_global:2:.claude    .claude/skills
$ cat .gitignore | grep opencode
  .opencode
```

Committed configuration declares the stack that no longer exists:

```json
// .specify/init-options.json      // .specify/integration.json
{"ai": "opencode", …}              {"installed_integrations": ["opencode"], …}
```

Committed under `.specify/`: manifests for `claude`, `opencode` and `speckit`, the bash scripts, the templates, the constitution. **Not** committed: any actual command prompt, from either stack.

**Consequence**: a fresh clone gets a manifest pointing at a stack that is not present and zero speckit commands. The 12 speckit skills that this session is running from exist only on the maintainer's machine. That is a reproducibility failure, not merely a drift risk.

**Decision**: declare `claude`, commit `.claude/skills/`, drop the opencode manifest and the `.opencode` ignore entry.

**Blocker to clear first**: `.claude` is excluded by `core.excludesFile = /home/nnc/.gitignore_global`. A repository `.gitignore` takes precedence over the global one, but a file cannot be re-included if a parent *directory* is excluded — so the negation must un-exclude the directory before the contents:

```gitignore
!.claude/
.claude/*
!.claude/skills/
!.claude/settings.json      # for feature 023's hooks
```

`.claude/settings.local.json` and `.claude/worktrees/` stay excluded — the former holds a 140-entry allowlist containing a database password.

**Cross-feature note**: feature 023 needs this same negation for its committed hook settings. Whichever lands first carries it.

---

## R5 — Which rules live in which document?

**Decision**: `AGENTS.md` owns operational rules; the constitution owns principles. Where both currently speak, the constitution states the principle and `AGENTS.md` states the procedure that implements it — never a second, differently-worded rule.

**Contradiction to resolve**:

> Constitution III: "Hand-maintained duplicate type definitions across apps are not permitted; regenerate instead of hand-editing generated files."
>
> AGENTS.md: "`index.ts` is hand-maintained (not auto-imported from the tygo-generated `generated.ts`), so update both when adding a DTO field."

After this feature, the practice `AGENTS.md` documents no longer exists, so the sentence is deleted rather than reconciled. **Principle III itself is not amended** — the repository is brought into compliance with it, which is the cheaper and more honest path: the principle was right and the practice was wrong.

**The constitution is amended anyway, for an unrelated reason.** Line 96 states that design documents are written under a `plan/` directory:

```
$ test -d plan && echo yes || echo NO      →  NO
$ test -d plans && echo yes || echo NO     →  NO
```

Neither exists; plans live at `specs/<nnn>-<slug>/plan.md`. FR-015 requires every stated rule to be true, and an edit inside the governing principles is an amendment however small, so FR-016's procedure applies: version 1.0.0 → 1.0.1 (PATCH — wording only, no principle redefined), `Last Amended` updated, Sync Impact Report extended, and `.specify/templates/*.md` plus the `speckit-*` skills re-checked in the same change.

`AGENTS.md:11` carries the same class of error — "`plans/` — implementation plans derived from specs" — and needs no amendment procedure, being an ordinary document.

**False claims to remove** (both trace to a Python component the repository does not contain):
1. `AGENTS.md`: "`make test-lint` — full test suite (Go + React + Python) + lint".
2. `package.json`: `"test:python": "make test-python"` — the target does not exist; the script fails on invocation.

**Rules to add to `AGENTS.md`** (FR-013, FR-014): the branch-and-pull-request rule, and worktree rules — which working copy is authoritative, how isolated copies are created and retired. Measured: 11 registered `manual-*` worktrees, plus 9 stale `agent-*` directories under `.claude/worktrees/` that are **not** registered git worktrees at all, plus one prunable entry — 13 entries in `git worktree list` against 20 directories on disk. No documented lifecycle for any of them.

**A third class of false claim** (FR-019): `AGENTS.md` names the backend data-transfer types as living in `apps/api/internal/dto/dto.go`, on lines 17 and 22. Measured: the directory holds **11** non-test files, **10** of which declare exported DTO structs. Feature 023's edit-time hook already filters on `apps/api/internal/dto/*.go`, so the instructions currently contradict the tooling, and an agent following the text edits the wrong file.

The constitution is **not** affected by this one — `grep -n "dto" .specify/memory/constitution.md` returns nothing, so it makes no file-level claim to correct. An earlier draft of this research asserted otherwise; that assertion was itself the same class of error it was describing.

---

## Sources

- [tygo README / configuration](https://github.com/gzuidhof/tygo) — `enum_style`, `type_mappings`, and the absence of any nullability option
- Repository evidence: `apps/api/internal/dto/activity.go`, `apps/api/tygo.yaml`, `packages/shared/src/{index,generated}.ts`, `.specify/init-options.json`, `.specify/integration.json`, `git ls-files`, `git check-ignore -v`, `~/.gitignore_global`
- Baseline comparison performed with an ad-hoc AST-free parser over both TypeScript files; the plan recommends committing this comparison as a CI check so the narrowing list cannot silently drift
