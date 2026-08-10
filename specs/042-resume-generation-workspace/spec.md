# Feature Specification: Resume Generation Workspace

**Feature Branch**: `042-resume-generation-workspace`

**Created**: 2026-08-10

**Status**: Draft

**Input**: User description: "i need to fully rework the resume generation UX. should be separate route for that, similar to editor. should be generated items on left (summary, works, skills). ai should only sort the achiementes (if needed 4, then pick top 8 and sort them by relevance), not fabricate them. also suggest your own (not selected by default but user can include it, they can be fabricated). same for skills section"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Review the generated resume as an inspectable list, not a finished blob (Priority: P1)

A user pastes a vacancy and lands on a dedicated generation workspace — its own route,
laid out like the profile editor. The left side shows the generated resume broken into
its parts: the summary, one block per work experience with its achievement bullets, and
the skills section. Every item is visible as an individual, addressable element the user
can read, keep, or drop. Nothing about the run is a black box: for each achievement the
user can see it came from their master profile, and for each item the AI added on its own
initiative, the user can see that too.

**Why this priority**: This is the whole reworked surface. Today the result of a run is a
single generated document plus a diff-like proposal list; the user cannot see the resume
as the structured thing it is, and cannot act on individual items. Without this layout,
none of the selection behaviour below has anywhere to live.

**Independent Test**: Can be fully tested by navigating to the generation route with a
vacancy, running generation, and asserting the left pane renders a summary block, one
block per master work entry that has selected achievements, and a skills block — each
item individually identifiable and toggleable. Delivers a reviewable, structured result
even before any of the suggestion behaviour exists.

**Acceptance Scenarios**:

1. **Given** a user with a master profile and a vacancy, **When** they open the generation
   route and run a generation, **Then** the left pane shows three section groups —
   Summary, Work Experience, Skills — with the generated content rendered as discrete
   items rather than as one block of text.
2. **Given** a completed generation, **When** the user looks at any work experience block,
   **Then** they see that entry's achievement bullets in the order the AI ranked them,
   each labelled as coming from their master profile.
3. **Given** a completed generation, **When** the user toggles any individual item off,
   **Then** the item is excluded from the resume that will be exported, and the change is
   reflected without re-running generation.
4. **Given** the user is on the generation route, **When** they navigate away and return
   to the same generation, **Then** their selection state is still what they left it as.

---

### User Story 2 - AI ranks the user's real achievements and never invents them (Priority: P1)

For each work entry, the AI's job is ranking, not writing. It is shown the full list of
the user's real achievement bullets for that entry and returns an ordering of them by
relevance to the vacancy, plus how many belong on the page. If the entry needs 4 bullets
but has 8 available, the AI picks the top 8 by relevance, orders them, and the top 4 are
the ones selected — the rest stay visible and unselected so the user can promote one. The
selected bullets are the user's own wording, character for character.

**Why this priority**: This is the trust boundary the user is asking for and the reason
the rework exists. An achievement that appears in the "your material" part of the list
must be the user's material — if ranked output can silently reword or merge bullets, the
distinction between "yours" and "the AI's" collapses and the whole workspace is
meaningless.

**Independent Test**: Can be fully tested by running generation against a master profile
with known bullets and asserting that every item rendered in the "from your profile"
group is byte-identical to a master bullet, that no master bullet appears twice, and that
the ordering differs from master order when the vacancy justifies it. Delivers grounded
selection independent of the suggestion feature.

**Acceptance Scenarios**:

1. **Given** a work entry with 8 master achievement bullets and a target of 4 for this
   run, **When** generation completes, **Then** all 8 bullets are ranked and shown, the
   top 4 are selected by default, and the remaining 4 are shown unselected below them.
2. **Given** a work entry with 3 master achievement bullets and a target of 4, **When**
   generation completes, **Then** all 3 are shown and selected, and no fourth bullet is
   conjured to fill the gap.
3. **Given** any generated run, **When** each selected profile-sourced bullet is compared
   to the master profile, **Then** its text is identical to a master bullet — no
   rewording, no merging of two bullets, no added metrics.
4. **Given** the AI returns an ordering that omits or duplicates a master bullet, **When**
   the result is assembled, **Then** the run is treated as invalid and retried or falls
   back to master order rather than presenting a lossy list to the user.

---

### User Story 3 - AI suggestions are offered, clearly marked, and off by default (Priority: P2)

Alongside the user's real achievements, the AI offers its own suggested bullets for each
work entry — material the user's profile does not contain but that the vacancy calls for.
These are visually distinct, explicitly marked as AI-written and unverified, and are
never selected by default. The user may include one, in which case it becomes part of the
resume and can be edited into something true before export. The same applies to skills:
suggested skills the user does not have are offered separately and unselected.

**Why this priority**: This is the escape hatch that makes strict grounding livable — the
user gets ideas without the system ever putting words in their mouth. It depends on
Stories 1 and 2 existing, and the workspace is already useful without it.

**Independent Test**: Can be fully tested by running generation on a vacancy demanding
skills and experience the profile lacks, and asserting suggestions appear in a separate
marked group, that the exported resume with no user action contains none of them, and
that including one adds it to the export. Delivers optional AI assistance without
weakening grounding.

**Acceptance Scenarios**:

1. **Given** a completed generation, **When** the user views a work entry, **Then** AI
   suggested bullets appear in their own group, visually distinguished from profile
   bullets, each unselected and marked as AI-written.
2. **Given** the user exports without touching any suggestion, **Then** the resulting
   resume contains zero AI-suggested content — only their own material.
3. **Given** the user includes an AI-suggested bullet, **When** they export, **Then** the
   bullet appears in the resume in the position they placed it, and the workspace records
   that this item was AI-originated.
4. **Given** a vacancy asking for skills absent from the profile, **When** generation
   completes, **Then** those skills appear as unselected suggestions in the skills
   section, separate from the user's real skills, which remain ranked and selected.
5. **Given** the user includes an AI-suggested item, **When** they edit its text, **Then**
   the edited text is what is exported.

---

### User Story 4 - Skills are ranked, not rewritten (Priority: P2)

The skills section follows the same contract as achievements: the user's real skill groups
and their skills come through intact, ordered by relevance to the vacancy, with nothing
dropped silently and nothing invented inside the user's own groups. If the page can only
fit a subset of groups, the least relevant groups are the ones left out — visibly, as
unselected items the user can restore.

**Why this priority**: Skills are the second-most-fabricated part of a tailored resume and
the easiest to lie on. Same trust rule, smaller surface than achievements, so it ships
after the achievement path is proven.

**Independent Test**: Can be tested by running generation with a profile whose skills are
known and asserting the rendered skill set is exactly a permutation/subset of the master
skills, with every omission visible as an unselected item.

**Acceptance Scenarios**:

1. **Given** a master profile with known skill groups, **When** generation completes,
   **Then** every rendered skill in the "from your profile" group matches a master skill
   exactly, and no master skill is missing from the list entirely — omitted ones appear
   unselected.
2. **Given** the resume must fit a page limit, **When** groups must be dropped, **Then**
   the dropped groups are the lowest-ranked ones and are shown unselected rather than
   removed from view.

---

### User Story 5 - Export what I approved (Priority: P2)

When the user is satisfied with the left-side selection, they export. What is produced is
exactly the set of items selected in the workspace, in the order shown — the AI does not
get another pass at the document afterwards, and nothing re-condenses or rewords the
approved text on the way to the file.

**Why this priority**: Selection is worthless if a later stage overrides it. Ships with
the first usable slice but is listed after it because the workspace has review value even
before export is wired.

**Independent Test**: Can be tested by making a known set of toggles, exporting, and
asserting the exported document's content is exactly the selected items in the displayed
order.

**Acceptance Scenarios**:

1. **Given** a workspace state with specific items selected and ordered, **When** the user
   exports, **Then** the exported document contains those items, in that order, with that
   wording.
2. **Given** the selected content exceeds the page limit, **When** the user attempts to
   export, **Then** the workspace tells them how much is over and which items are the
   least relevant candidates to drop, and does not silently rewrite or trim anything.

---

### Edge Cases

- **Master profile has no achievements for an entry**: the entry renders with an empty
  achievement list and an explicit "no bullets in your profile for this role" state, plus
  any AI suggestions — never a fabricated bullet presented as the user's.
- **Every item deselected in a section**: the section is omitted from the export, and the
  workspace warns before export that the resume has no summary / no skills.
- **Generation fails partway**: sections that completed are shown; failed sections show an
  error with a per-section retry, rather than discarding the whole run.
- **Master profile changes after a generation exists**: the workspace flags that the
  underlying profile has changed and offers a re-run; stale selections referencing removed
  bullets are shown as unavailable rather than silently dropped.
- **AI returns a suggestion identical to an existing profile bullet**: it is deduplicated
  and shown only once, in the profile-sourced group.
- **User includes a suggestion and never edits it**: it exports as written, and the
  workspace has warned at include-time that AI-written content is unverified.
- **Target bullet count larger than 2x available**: all available bullets are selected; no
  padding.
- **A run with zero AI suggestions**: suggestion groups render an empty state, not a
  broken or missing section.

## Requirements *(mandatory)*

### Functional Requirements

#### Route and layout

- **FR-001**: System MUST provide a dedicated resume generation route, distinct from the
  existing tailor surface, reachable from the main navigation and from a job's detail view.
- **FR-002**: The generation route MUST use a two-pane layout in the manner of the profile
  editor: generated resume items on the left, vacancy/context and controls on the right.
- **FR-003**: The left pane MUST group items into Summary, Work Experience (one block per
  master entry), and Skills.
- **FR-004**: Every generated item MUST be individually addressable — selectable,
  deselectable, and reorderable within its section — without re-running generation.
- **FR-005**: Each item MUST display its origin: sourced from the user's master profile, or
  AI-suggested.

#### Grounded selection

- **FR-006**: For each work entry, the AI's contribution MUST be a ranking of the entry's
  existing master achievement bullets — an ordered reference list, never new bullet text
  in the profile-sourced group.
- **FR-007**: When an entry's target bullet count is N and more than N master bullets
  exist, the system MUST rank and present up to 2N bullets (e.g. target 4 → present up to
  8), selecting the top N and leaving the remainder visible and unselected.
- **FR-008**: When fewer than N master bullets exist, the system MUST present and select
  all of them and MUST NOT generate filler to reach N.
- **FR-009**: Text of a profile-sourced item, as shown and as exported, MUST be identical
  to the corresponding master profile text.
- **FR-010**: A ranking that omits, duplicates, or invents a bullet MUST be rejected, with
  the system retrying or falling back to master order; a lossy list MUST NOT reach the user.
- **FR-011**: Skills MUST follow FR-006 through FR-010: the user's skill groups and skills
  are ranked, never rewritten, with omissions shown as unselected rather than hidden.

#### AI suggestions

- **FR-012**: System MUST offer AI-suggested achievement bullets per work entry and
  AI-suggested skills, derived from the vacancy, in groups separate from profile-sourced
  items.
- **FR-013**: AI-suggested items MUST be unselected by default and MUST require an explicit
  user action to be included.
- **FR-014**: AI-suggested items MUST be visually and semantically marked as AI-written and
  unverified wherever they appear, including after inclusion.
- **FR-015**: Users MUST be able to edit the text of an included AI-suggested item before
  export.
- **FR-016**: AI suggestions MUST NOT be subject to the grounding rules that bind
  profile-sourced items — they may propose content absent from the profile — but MUST NOT
  be presented as the user's own material.
- **FR-017**: A suggestion duplicating an existing profile item MUST be suppressed from the
  suggestion group.

#### Export and state

- **FR-018**: Export MUST produce exactly the selected items, in the displayed order, with
  the displayed wording; no post-selection rewording, condensing, or re-ranking may occur.
- **FR-019**: When selected content exceeds the page budget, the system MUST report the
  overflow and indicate the lowest-ranked candidates for removal, and MUST NOT resolve it
  by rewriting or silently dropping content.
- **FR-020**: Workspace state — the run, its ranked items, and the user's selections and
  edits — MUST persist so the user can leave and return to the same generation.
- **FR-021**: Users MUST be able to re-run generation for the whole resume or for a single
  section, and MUST be warned that re-running replaces the AI's ordering for that section
  (their explicit selections and edits are preserved where the underlying item still exists).
- **FR-022**: System MUST detect that the master profile changed since the run and offer a
  re-run, marking selections whose source item no longer exists as unavailable.
- **FR-023**: Every run MUST record, per exported item, whether it was profile-sourced or
  AI-originated, so an exported resume's provenance is auditable after the fact.

### Key Entities

- **Generation Run**: One tailoring attempt against one vacancy for one profile. Holds the
  vacancy context, the model/grounding settings used, timestamps, per-section status, and
  a snapshot reference to the master profile version it ranked.
- **Section**: Summary, Work Experience, or Skills. Owns an ordered list of items and a
  target size (e.g. bullets per entry, groups per page).
- **Ranked Item**: One candidate for inclusion. Carries origin (profile-sourced with a
  reference to its master item, or AI-suggested), rank/relevance position, selected state,
  displayed text, and — for AI-suggested items that were included — the user's edited text.
- **Work Entry Block**: Groups ranked achievement items under one master employment entry,
  with that entry's target bullet count.
- **Selection State**: The user's per-item include/exclude and ordering decisions for a
  run; the sole input to export.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of profile-sourced items in an exported resume match the user's master
  profile text exactly — zero fabricated content can appear without an explicit user
  inclusion action.
- **SC-002**: A user can go from pasted vacancy to an approved, exported resume in under
  3 minutes, including reviewing and adjusting selections.
- **SC-003**: For a work entry with N target bullets and ≥2N available, the user is shown
  at least 2N ranked candidates, of which exactly N are pre-selected.
- **SC-004**: 0% of AI-suggested items appear in an export where the user took no action on
  them, measured across a corpus of generation runs.
- **SC-005**: Users can identify the origin of any item in the workspace (theirs vs
  AI-written) without opening a detail view, verified by task-based usability checks with
  90% correct identification.
- **SC-006**: Adjusting a selection updates the previewed resume with no additional model
  run and no perceptible delay (under 1 second).
- **SC-007**: Rejected/retried rankings (omitted or duplicated bullets) occur in under 5%
  of runs, and no such run reaches the user with a lossy list.

## Assumptions

- The existing tailor surface remains available during the transition and is retired once
  the new workspace covers its use cases; cover-letter generation stays where it is and is
  out of scope for this rework.
- The vacancy is supplied the same way as today (pasted text, or carried in from a job
  detail view); no new vacancy ingestion is part of this feature.
- The summary remains AI-written prose rather than a ranked selection — it has no
  equivalent "list of the user's own summaries" to rank — and stays subject to the existing
  grounding rules, presented as a single item the user can accept, edit, or drop.
- "Top 8 for a target of 4" generalizes to "present up to twice the target": the number in
  the request is an example of the ratio, not a fixed constant.
- Page-fit limits and target bullet counts come from the existing resume shape settings;
  this feature does not introduce new layout configuration.
- Existing grounding levels continue to govern profile-sourced content; AI suggestions are
  a separate, explicitly-unverified channel and are not governed by them.
- Provenance recording reuses the existing run/document history rather than introducing a
  new audit system.
- One user per instance; no sharing, review-by-others, or collaborative approval flows.
