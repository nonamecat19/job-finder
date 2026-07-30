# Contract: Document Ownership

One operative rule per topic (FR-010). A rule has exactly one home; other documents may point at it, never restate it in their own words. Restatements are how the current contradiction arose.

## Owners

| Topic | Owner | May be referenced by |
|---|---|---|
| Product trust boundary (no auto-apply) | `constitution.md` I | README |
| Grounded generation | `constitution.md` II | — |
| Type-sharing **principle** | `constitution.md` III | `AGENTS.md` |
| Type-sharing **procedure** (how to regenerate) | `AGENTS.md` | — |
| Test discipline **principle** | `constitution.md` IV | `AGENTS.md` |
| What the quality command covers | `AGENTS.md`, matching the `Makefile` recipe | — |
| Branch and pull-request rule | `AGENTS.md` | enforced by feature 023 |
| Worktree lifecycle | `AGENTS.md` | — |
| Migration numbering | `constitution.md` Tech Constraints | — |
| Local-first inference | `constitution.md` V | README |
| Commit conventions | `AGENTS.md` | — |

**Split rule**: the constitution says *what must be true and why*; `AGENTS.md` says *what to run*. When both must mention a topic, the constitution states the principle and `AGENTS.md` states the procedure implementing it. Two statements of the same rule, in different words, is the defect.

## Statements to delete

| Location | Claim | Status | Owner |
|---|---|---|---|
| `AGENTS.md:22` | "`index.ts` is hand-maintained (not auto-imported from the tygo-generated `generated.ts`), so update both when adding a DTO field" | contradicts Principle III; the practice ceases to exist after this feature | this feature |
| `AGENTS.md:11` | "`plans/` — implementation plans derived from specs" | **neither `plans/` nor `plan/` exists**; plans live at `specs/<nnn>-<slug>/plan.md` | this feature |
| `AGENTS.md:17,22` | data-transfer types live in `apps/api/internal/dto/dto.go` | **10 of the 11** files in that directory declare exported DTO structs; feature 023's hook filter already uses `apps/api/internal/dto/*.go`, so the instructions contradict the tooling. The constitution makes no file-level claim here | this feature |
| `constitution.md:96` | "Design/plan docs … are written under `plan/`" | **that directory does not exist**. Correcting it is an amendment of the governing principles, so it triggers the version bump and Sync Impact Report | this feature |
| `AGENTS.md` | "`make test-lint` — full test suite (Go + React + Python) + lint" | no Python in the repository; no lint runs today | **feature 023** — it changes what the target runs |
| `package.json` | `"test:python": "make test-python"` | the make target does not exist; the script fails on invocation | **feature 023** |

## Statements to add to `AGENTS.md`

**Branch and pull request** (FR-013):
> Work reaches `master` only via a branch and a pull request whose checks are green. Never commit on `master`. Create a branch first: `git checkout -b <nnn>-<slug>`.

**Worktrees** (FR-014): which working copy is authoritative, how an isolated copy is created, and when it is retired. The repository currently holds 11 registered `manual-*` worktrees, 9 stale `agent-*` directories under `.claude/worktrees/` that are not registered git worktrees at all, and one prunable entry — an agent cannot tell a live one from an abandoned one.

**Type sharing**, replacing the deleted sentence:
> Shared types are generated. Add the field to the Go DTO in `apps/api/internal/dto/`, run `make tygo-generate`, done. `packages/shared/src/index.ts` re-exports and narrows; it never restates a shape. Hand-written types with no backend counterpart live in `consumer-only.ts`.

## Amendment procedure

The constitution's own governance section requires that any amendment bump the version, update the Sync Impact Report, and re-check `.specify/templates/*.md` and the installed speckit skills in the same change (FR-016).

**Principle III is not amended.** It was correct; the practice violated it. Bringing the repository into compliance is the honest outcome — amending the principle to bless the duplication would have written the bug into law.

**But the constitution is still amended**, for a different reason: line 96 claims design documents live under a `plan/` directory that does not exist. FR-015 requires every stated rule to be true, and a correction inside the governing principles is an amendment regardless of how small it is. So the procedure applies:

- **Version** 1.0.0 → 1.0.1 — PATCH, since this is wording/clarification and no principle is added, removed or redefined
- **Last Amended** set to the landing date
- **Sync Impact Report** header block updated with the correction
- `.specify/templates/*.md` and the `speckit-*` skills re-checked for now-stale references in the same change

A corrected constitution carrying a stale version line and an unchanged Sync Impact Report would itself be a new false statement — exactly the defect class this feature exists to remove.

## Verification

For each row in the Owners table, search every context document for statements on that topic. Two documents stating the same rule in different words is a failure of SC-006. Every statement in every context document must be true of the repository as it stands (SC-007) — verified statement by statement, not skimmed.
