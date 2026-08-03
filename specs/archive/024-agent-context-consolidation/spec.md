> **ARCHIVED — SHIPPED.**
>
> Historical record, preserved as written. The requirements that still bind live in
> [`specs/domains/codebase-structure.md`](../../domains/codebase-structure.md) — read that first.

---
# Feature Specification: Single Source of Truth for Agent Context and Shared Types

**Feature Branch**: `024-agent-context-consolidation`

**Created**: 2026-07-28

**Status**: Shipped — see the ARCHIVED banner at the top of this file for supersessions.

**Input**: User description: "Resolve the contradictions in the AI agent's context and collapse to a single agent tooling stack (Tier B). Scope: (1) The constitution's Principle III forbids hand-maintained duplicate type definitions across apps and requires generated/shared types as the single source of truth, but AGENTS.md instructs agents that packages/shared/src/index.ts is hand-maintained and must be updated field-for-field alongside the tygo-generated generated.ts. An agent reading both gets contradictory law, and the current working tree has both files modified together — this is the structural cause of type drift. Decide and implement one answer: preferred approach is to make index.ts re-export from generated.ts (deleting the hand-maintained duplicate DTO definitions) so the existing tygo-check CI job becomes sufficient enforcement; the alternative is amending the constitution to bless the hybrid. Whichever is chosen, exactly one document states the rule and the other is updated to match. (2) The repo has two agent stacks installed simultaneously: .specify/init-options.json and integration.json declare ai=opencode with only the opencode integration installed, there is a .opencode/ directory, and .claude/skills/ also holds a full copy of the speckit-* skills. Two copies of the same prompts drift apart silently. Collapse onto a single stack (Claude Code) so there is one copy of the speckit commands and one integration manifest. (3) AGENTS.md must be corrected and made authoritative: it currently claims the test suite covers Python (the repo has none), and it must state the branch-and-PR rule and worktree rules. Goal: an agent reading the repo's context files gets exactly one consistent instruction per topic, and generated types cannot diverge from their source because no hand-maintained duplicate exists."

## Problem Statement

An AI coding agent working in this repository reads its instructions from several documents that disagree with each other, and edits shared types that exist in two places at once.

The governing principles document forbids hand-maintained duplicate type definitions across applications and names generation as the single source of truth. The agent instructions document tells the agent the opposite: that the shared type file is hand-maintained and must be updated field-for-field in parallel with the generated one. Measured today, 56 of the 70 shared type definitions — 80 percent — exist in both the hand-maintained file and the generated file, and the hand-maintained file does not read from the generated one at all. Both files are modified together in the current working tree. Forty-seven dashboard files consume these types. This is not a lapse of discipline; it is the documented process, and it is the structural cause of type drift between the two runtimes.

Separately, the repository has two agent tooling stacks installed at once. The specification tooling records one agent as the configured integration, while a complete second copy of the same command prompts lives in another agent's directory. Two copies of the same instructions drift apart silently, and no mechanism reconciles them.

Finally, the agent instructions contain a factual error — they describe the test suite as covering a language the repository does not contain — and omit the workflow rules an agent most needs.

The result is that the agent's behaviour depends on which document it happens to read. This feature makes every rule exist in exactly one place.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Shared types have exactly one definition (Priority: P1)

An author adds a field to a backend data-transfer type. The corresponding type consumed by the dashboard reflects the new field without anyone editing a second file. There is no hand-maintained copy of the same shape to fall out of step, so the existing drift check is sufficient to guarantee the two runtimes agree.

**Why this priority**: This is the highest-frequency defect class in the repository and the one the governing principles single out as the most common source of silent integration bugs. Every other item in this feature is documentation; this one removes an entire category of bug by construction.

**Independent Test**: Add a field to a backend data-transfer type, regenerate, and confirm the dashboard sees the new field with no second edit. Then attempt to reintroduce a duplicate definition of an already-generated shape and confirm it is rejected.

**Acceptance Scenarios**:

1. **Given** a type that is generated from a backend definition, **When** an author searches the shared package for its definition, **Then** exactly one definition exists.
2. **Given** an author adds a field to a backend data-transfer type and regenerates, **When** the dashboard is type-checked, **Then** the new field is visible to consumers with no additional hand edit.
3. **Given** an author hand-writes a duplicate of a generated shape into the shared package, **When** automated checks run, **Then** the duplicate is detected and rejected.
4. **Given** a type is narrower on the consumer side than generation can express — for example a fixed set of allowed values that generation flattens to a free-form value — **When** the consolidation is applied, **Then** the narrower form is layered on top of the generated definition rather than restated as a second full copy, so no field list is duplicated.
5. **Given** a type genuinely has no backend counterpart and exists only for the dashboard, **When** it is retained by hand, **Then** it is clearly separated from generated types and identified as consumer-only, so its hand-maintained status is not mistaken for an exception to the rule.
6. **Given** the consolidation is complete, **When** every consumer of the shared package is type-checked and its tests run, **Then** all pass, and no consumer import path had to change.

---

### User Story 2 - Every rule appears in exactly one document (Priority: P1)

An agent starts work and reads the repository's context documents. For every topic — how types are shared, how work reaches the trunk, what the quality command covers, how isolated working copies are used — it finds exactly one statement, and that statement is true of the repository as it actually is.

**Why this priority**: A contradiction between two authoritative documents is worse than a missing rule. Given a missing rule the agent asks or reasons; given a contradiction it silently picks one, and which one it picks varies between sessions. That is the definition of unpredictable behaviour, which is the problem this whole effort exists to fix.

**Independent Test**: Enumerate the rules stated across the context documents, and confirm no two documents state conflicting rules on the same topic, and that no stated rule is false of the repository.

**Acceptance Scenarios**:

1. **Given** the governing principles document and the agent instructions document, **When** both are read on the subject of shared type definitions, **Then** they agree, and only one of them states the operative rule while the other refers to it.
2. **Given** the agent instructions describe what the quality command covers, **When** the description is compared to what the command actually runs, **Then** the two match and no absent language is claimed.
3. **Given** an agent needs to know how its work reaches the trunk, **When** it reads the agent instructions, **Then** it finds the branch-and-change-request rule stated explicitly.
4. **Given** an agent is working inside an isolated working copy, **When** it reads the agent instructions, **Then** it finds explicit rules on which working copy is authoritative and how isolated copies are created and retired.
5. **Given** the governing principles are amended, **When** the amendment lands, **Then** dependent documents and command prompts are re-checked in the same change, as the governance section already requires.

---

### User Story 3 - One agent tooling stack, one copy of every prompt (Priority: P2)

The repository declares a single supported agent. The specification commands exist in exactly one location. Editing a command's behaviour requires one edit, and no second stale copy exists to be picked up by a future session.

**Why this priority**: Duplicate prompts drift silently and the drift is invisible until an agent behaves differently from the last time. It is P2 rather than P1 because the duplication is currently latent — both copies still agree — so it is a defect waiting to happen rather than one already causing harm.

**Independent Test**: Search the repository for the specification command definitions and confirm exactly one copy of each exists; confirm the declared integration matches the directory that actually holds them; run one specification command end to end and confirm it still works.

**Acceptance Scenarios**:

1. **Given** the specification command definitions, **When** the repository is searched, **Then** exactly one copy of each command exists.
2. **Given** the specification tooling's declared configuration, **When** it is compared to the agent stack actually present, **Then** they name the same stack.
3. **Given** the consolidation is complete, **When** a specification command is run end to end, **Then** it behaves as before, and the helper scripts and templates it depends on are still resolved.
4. **Given** the removed stack's directory is deleted, **When** the repository is searched for references to it, **Then** no configuration, script, or document still points at it.
5. **Given** a future contributor upgrades the specification tooling, **When** the upgrade runs, **Then** it targets the single declared stack and cannot reinstall the removed one by default.

---

### Edge Cases

- **Generation cannot express a needed constraint**: the generator flattens fixed-value sets to free-form values, losing the narrower types the dashboard currently relies on. Removing the hand-maintained copy naively would weaken type safety for every consumer — the resolution must preserve the narrowing without restating the shape.
- **Consumer-only types**: a minority of shared types have no backend counterpart. The rule must accommodate them explicitly rather than forcing an artificial backend definition or leaving them as an undocumented exception.
- **Two definitions of the same name that are not actually identical**: where the hand-maintained and generated versions of a type have already drifted, consolidating them changes the type consumers see. Each such divergence must be identified and resolved deliberately, not silently overwritten in whichever direction is convenient.
- **Consumer breakage at scale**: forty-seven files import from the shared package. The consolidation must keep the public import surface stable, or it becomes a repository-wide edit rather than a package-internal one.
- **In-flight work**: the shared type files are already modified in the working tree. The consolidation must define how uncommitted parallel edits are reconciled rather than discarding them.
- **The removed agent stack is still in use**: if any workflow, script, or habit still invokes the removed stack, deleting it breaks that path silently. References must be found before removal, not after.
- **Documentation regresses later**: nothing stops a future edit from reintroducing a contradiction. The rule "one topic, one document" needs an owner and ideally a check, or this feature's value decays.
- **Governance ordering**: if the governing principles document is the one that must change, the change must follow the amendment procedure that document itself defines, including a version bump and a re-check of dependent templates.

## Requirements *(mandatory)*

### Functional Requirements

#### Shared type consolidation

- **FR-001**: Each shared type that has a backend counterpart MUST have exactly one definition in the shared package.
- **FR-002**: Hand-maintained duplicates of generated shapes MUST be removed, not merely kept in sync.
- **FR-003**: Where a consumer requires a narrower form than generation can express, the narrowing MUST be derived from or layered on top of the generated definition, and MUST NOT restate the shape. Naming the individual fields a narrowing applies to is required and expected; restating a field's *type* is permitted only for the specific fields being narrowed, and only where generation cannot express the constraint. Copying a whole field list, or restating a field the narrowing does not change, is the duplication this feature removes.
- **FR-004**: Types with no backend counterpart MUST be retained, explicitly identified as consumer-only, and separated from generated types so their hand-maintained status is unambiguous.
- **FR-005**: The public import surface of the shared package MUST remain unchanged, so no consumer file requires an import change.
- **FR-006**: Every existing consumer of the shared package MUST type-check and pass its tests after consolidation.
- **FR-007**: Every case where the hand-maintained and generated versions of a shape have already diverged MUST be enumerated and its resolution recorded, rather than resolved implicitly.
- **FR-008**: Reintroducing a hand-maintained duplicate of a generated shape MUST be detected by an automated check.
- **FR-009**: Uncommitted in-flight edits to the affected files MUST be reconciled into the consolidated result rather than discarded.

#### Documentation consistency

- **FR-010**: For each topic covered by the repository's context documents, exactly one document MUST state the operative rule; others MUST refer to it rather than restate it.
- **FR-011**: The governing principles document and the agent instructions document MUST NOT contradict each other on shared type definitions after this change.
- **FR-012**: The agent instructions MUST accurately describe what the quality command covers, and MUST NOT claim coverage for languages absent from the repository. **Delivered by the companion workflow-gates feature**, which changes what that command runs; this feature only verifies the result (see Dependencies).
- **FR-013**: The agent instructions MUST state the rule that work reaches the trunk via a branch and a change request.
- **FR-014**: The agent instructions MUST state which working copy is authoritative and how isolated working copies are created and retired.
- **FR-015**: Every rule stated in the context documents MUST be true of the repository at the time the change lands. This includes claims about directories and file locations, not only claims about process.
- **FR-016**: If the governing principles document is amended, the amendment MUST follow that document's own procedure, including a version bump, an updated change summary, and a re-check of dependent templates and command prompts in the same change.
- **FR-017**: The governing principles document states that design documents are written under a directory that **does not exist**; the repository uses a different one. This statement MUST be corrected, and because it lives in the governing principles, the correction MUST follow the amendment procedure in FR-016.
- **FR-018**: The agent instructions describe a project directory for implementation plans that **does not exist**. The claim MUST be removed or corrected to the directory actually used.
- **FR-019**: The agent instructions identify the backend data-transfer types as living in a **single file**, in two places. They are spread across ten files in one directory. Both references MUST be corrected to the directory, so an agent adding a field does not look in the wrong place — and so the instructions agree with the edit-time tooling, which already matches the whole directory. The governing principles make no file-level claim here and need no change on this point.

#### Single agent tooling stack

- **FR-020**: The repository MUST declare exactly one supported agent stack.
- **FR-021**: Exactly one copy of each specification command definition MUST exist.
- **FR-022**: The declared configuration MUST name the same stack that is actually installed.
- **FR-023**: The removed stack's directory and manifests MUST be deleted, and no remaining configuration, script, or document may reference them.
- **FR-024**: All specification commands MUST continue to function after consolidation, including resolution of their helper scripts and templates.
- **FR-025**: A future upgrade of the specification tooling MUST target the single declared stack by default and MUST NOT reinstall the removed one.

### Key Entities

- **Generated type**: a shared type derived automatically from a backend definition; its authority is the backend definition, and it is never edited by hand.
- **Consumer-only type**: a shared type with no backend counterpart, maintained by hand and explicitly marked as such.
- **Narrowing**: an additional constraint a consumer needs that generation cannot express, expressed on top of a generated type rather than as a second copy of it.
- **Context document**: a document an agent reads to learn the rules — the governing principles, the agent instructions, and the command prompts.
- **Operative rule**: a statement of what must be done on a given topic. Each operative rule has exactly one home document; other documents may reference it but not restate it.
- **Agent stack**: the set of command prompts, manifests, and configuration belonging to one agent tool. Exactly one is supported.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The number of shared type shapes defined in two places drops from 56 to 0.
- **SC-002**: Adding a field to a backend data-transfer type requires exactly one file edit plus regeneration, down from two hand edits today.
- **SC-003**: 100% of shared types are either generated from a backend definition or explicitly labelled consumer-only; no type is ambiguous about which it is.
- **SC-004**: Zero consumer files require an import change as a result of consolidation, out of the 47 that import the shared package.
- **SC-005**: All existing type checks and test suites pass after consolidation, with no reduction in the strictness of any type a consumer relies on.
- **SC-006**: Zero pairs of context documents state conflicting rules on the same topic, verified by enumerating rules per topic.
- **SC-007**: Zero statements in the context documents are false of the repository, verified statement by statement.
- **SC-008**: Exactly one copy of each specification command definition exists, down from two.
- **SC-009**: Zero references to the removed agent stack remain anywhere in the repository.
- **SC-010**: An agent given the same task in two separate sessions follows the same workflow rules, because only one version of each rule is available to read.
- **SC-011**: A reader can determine which document owns any given rule in under one minute, because ownership is stated rather than inferred.

## Assumptions

- The preferred resolution is to eliminate the hand-maintained duplicates and treat generation as authoritative, consistent with the existing governing principle; amending the principle to bless the hybrid is the fallback only if elimination proves to weaken type safety in a way that cannot be recovered by layering narrowings.
- The existing drift check between backend definitions and generated output is **not** sufficient on its own. Two further checks are needed and are in scope: one that fails when a duplicate shape is reintroduced, and one that fails when a narrowing is missing so a type would silently weaken. No new *generator* is introduced.
- Correcting a false statement inside the governing principles is itself an amendment of that document, so it triggers the amendment procedure (FR-016) even though no principle changes meaning. Bringing the repository into compliance with Principle III does not.
- Consumer-facing narrowings that generation flattens are assumed recoverable by layering constraints on the generated types, without restating any field list. Two forms are expected: a nullability wrapper that names fields but no types, and a per-field override for the few constraints generation cannot express at all.
- FR-025 is met by configuration alone. Verifying how a future tooling upgrade behaves is impractical without running that upgrade, so it is accepted as unverified rather than given a task that cannot be executed.
- The retained agent stack is the one whose command definitions are already present and in use in this session's tooling; the other is removed.
- Removing the second stack is assumed safe because it is not referenced by any automated workflow; this assumption is verified by search before deletion, per FR-020.
- The 14 shared types with no backend counterpart are assumed genuinely consumer-only and are retained by hand rather than pushed into backend definitions.
- Uncommitted work touching the shared type files is assumed to be intentional and is folded into the consolidation rather than reverted.
- The branch-and-change-request rule referenced by the agent instructions is defined by the companion workflow-gates feature; this feature documents the rule, it does not implement its enforcement.
- No behaviour of the running system changes as a result of this feature; it is a source-organisation and documentation change with type-level effects only.

## Dependencies

The two features overlap on edits to the same documents. Each edit has exactly one owner, so neither feature waits on the other and neither writes over the other's work.

| Edit | Owner | The other feature's role |
|---|---|---|
| Description of what the quality command covers, incl. removing the false claim about an absent language | **workflow-gates** — it changes what the command runs, so it must describe the result | this feature verifies the description is accurate at the time it lands |
| Removing the dead script that invokes a non-existent build target | **workflow-gates** — same reason | none |
| Wording of the branch-and-change-request rule, and the ownership map for every rule | **this feature** — document consistency is its subject | workflow-gates states the minimal rule its own enforcement needs; this feature owns the final wording and the ownership table |
| Worktree lifecycle rules (FR-014) | **this feature, solely** — workflow-gates originally carried a duplicate task with no requirement behind it; it has been removed | none |
| Un-excluding the agent configuration directory from version control | **whichever lands first** — the edit is identical | the second finds it already present |

Ordering: either may land first. If workflow-gates lands second, it must leave the quality-command description accurate; if this feature lands second, it must not revert the description workflow-gates wrote.

## Out of Scope

- Enforcing the branch-and-change-request rule, adding linters, adding automated checks that run the integration or end-to-end suites, and adding edit-time repair behaviour — all belong to the companion workflow-gates feature.
- Restructuring the specification pipeline into effort tiers, spec lifecycle status tracking, retiring stale isolated working copies and branches, and permission-list hygiene — deferred.
- Changing the meaning, shape, or field set of any type beyond what is required to remove duplication and preserve existing strictness.
- Adding new shared types or new backend definitions.
- Any change to runtime behaviour.
