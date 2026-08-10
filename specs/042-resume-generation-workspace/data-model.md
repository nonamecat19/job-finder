# Phase 1 Data Model: Resume Generation Workspace

Entities are the spec's Key Entities made concrete. Three tables, not five: **Section** and
**Work Entry Block** are the same thing at two granularities and collapse into one
`generation_sections` row per section with a nullable `entry_key`; **Selection State** is not a
table but the `selected` + `position` columns on `generation_items`, because a selection that
lives anywhere other than on the item it selects can drift from it.

---

## 1. `generation_runs`

One tailoring attempt against one vacancy for one profile.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | `gen_random_uuid()` |
| `profile_id` | `uuid` NOT NULL | → `"Profile"("id")` ON DELETE CASCADE |
| `job_id` | `uuid` NULL | → `"Job"("id")` ON DELETE CASCADE; NULL for a pasted vacancy |
| `vacancy_company` | `text` NULL | |
| `vacancy_title` | `text` NULL | |
| `vacancy_text` | `text` NOT NULL | the posting the ranking was made against |
| `master_snapshot` | `jsonb` NOT NULL | the `RendercvMaster` this run ranked. **Not a reference** — see §5 |
| `master_content_hash` | `text` NOT NULL | for FR-022 staleness detection |
| `shape_config` | `jsonb` NOT NULL | resolved once at run start (R3, 031-FR-006) |
| `grounding_level` | `text` NOT NULL | governs the summary only on this path (R1) |
| `summary_option_id` | `text` NULL | 034 choice, resolved once at run start |
| `analysis` | `jsonb` NULL | the `VacancyAnalysis`; drives per-section rerun without re-analysing |
| `state` | `text` NOT NULL | `running` \| `ready` \| `partial` \| `failed`; default `running` |
| `activity_id` | `uuid` NULL | → `"ActivityRun"("id")` ON DELETE SET NULL |
| `export_document_id` | `uuid` NULL | → `"GeneratedDocument"("id")` ON DELETE SET NULL |
| `export_status` | `text` NULL | `rendering` \| `exported` \| `blocked` \| `error` |
| `export_report` | `jsonb` NULL | overflow report when `blocked` (R5) |
| `created_at`, `updated_at` | `timestamptz` NOT NULL | `now()`; `updated_at` set explicitly per UPDATE, matching every other table in this repo |

**Validation**

- exactly one of `job_id` / `vacancy_text`-only origin; `vacancy_text` is required either way
  because the ranking must be explainable after the job row changes.
- `state = 'ready'` requires every section row to be `ready`. `partial` means at least one
  section is `failed` and at least one is `ready` — the "generation fails partway" edge case:
  completed sections are shown and the failed ones offer a per-section retry.

**Indexes**

```sql
CREATE INDEX generation_runs_profile_created_idx ON generation_runs (profile_id, created_at DESC);
CREATE INDEX generation_runs_job_idx ON generation_runs (job_id) WHERE job_id IS NOT NULL;
```

**No uniqueness constraint on `(profile_id, job_id)`** — deliberately unlike `00036`'s
`tailored_drafts_profile_job_key`. 020 needed one draft per job because accept/reject mutated a
shared baseline; a run here is immutable input plus per-item selections, so several runs against
the same vacancy are a comparison, not a conflict. The workspace opens the newest by default.

---

## 2. `generation_sections`

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `run_id` | `uuid` NOT NULL | → `generation_runs(id)` ON DELETE CASCADE |
| `kind` | `text` NOT NULL | `summary` \| `experience` \| `skills` |
| `entry_key` | `text` NULL | for `experience`, the exact master company name; NULL for `summary`/`skills` |
| `entry_label` | `text` NULL | display label (position, dates) copied from master |
| `position` | `int` NOT NULL | master order — never model-chosen (028-FR-003) |
| `target_count` | `int` NOT NULL | N for this section; 0 where not applicable |
| `state` | `text` NOT NULL | `running` \| `ready` \| `failed` |
| `error` | `text` NULL | shown with the per-section retry |
| `fallback_used` | `bool` NOT NULL DEFAULT false | true when a rejected ranking fell back to master order (FR-010) — SC-007 is measured from this column |

**Constraints**

```sql
UNIQUE (run_id, kind, COALESCE(entry_key, ''))
CHECK (kind <> 'experience' OR entry_key IS NOT NULL)
```

**Why `entry_key` is the company name, not a master row id.** The master is an opaque
`RendercvMaster` map with no stable per-entry identity — `MergeTailored` already keys experience
by `norm(company)` and the ranking prompt already addresses entries the same way. Introducing a
synthetic id here would be a second identity scheme for the same thing.

---

## 3. `generation_items`

One candidate for inclusion. The spec's **Ranked Item**.

| Column | Type | Notes |
|---|---|---|
| `id` | `uuid` PK | |
| `section_id` | `uuid` NOT NULL | → `generation_sections(id)` ON DELETE CASCADE |
| `origin` | `text` NOT NULL | `profile` \| `ai` |
| `source_index` | `int` NULL | for `origin='profile'`: index into that entry's master bullet list. NOT NULL when `origin='profile'` |
| `source_text` | `text` NOT NULL | for `profile`, the master's text **copied at run time** from the snapshot; for `ai`, the model's suggestion |
| `edited_text` | `text` NULL | user's edit. **Only legal when `origin='ai'`** — this is FR-009 and FR-015 in one constraint |
| `rank` | `int` NOT NULL | relevance position within the section; the ordering the model returned |
| `position` | `int` NOT NULL | the user's display order; seeded from `rank` |
| `selected` | `bool` NOT NULL | seeded true for the top N profile items, **always false for `ai`** at creation (FR-013) |
| `unavailable` | `bool` NOT NULL DEFAULT false | set when the master changed and this source item no longer exists (FR-022) |
| `created_at`, `updated_at` | `timestamptz` NOT NULL | |

**Constraints — the ones that carry a functional requirement**

```sql
CHECK (origin <> 'profile' OR source_index IS NOT NULL)
CHECK (origin <> 'profile' OR edited_text IS NULL)   -- FR-009: profile text is not editable
UNIQUE (section_id, origin, source_index)            -- FR-010: no master bullet twice
CREATE INDEX generation_items_section_pos_idx ON generation_items (section_id, position);
```

`edited_text IS NULL` for profile items is where FR-009 stops being a rule and becomes a schema
fact. Combined with R1's deletion of `rephrased`, there is no representable path from a model
response or a user action to a profile-sourced item whose text differs from the master's.

**Effective text** is `COALESCE(edited_text, source_text)`, computed in the domain layer, never
stored — a stored copy is a second source of truth that can drift from the edit that produced it.

**Skills** use the same table: one `skills` section, one item per skill *group*, with
`source_index` indexing the master's group list and `source_text` holding `"Label: details"` for
display. Ordering within a group is computed by the existing deterministic `RankSkills`, not by
the model (§2a of the domain doc), so it needs no rows.

**Summary** is one section with exactly one `origin='ai'` item — the written prose of spec
assumption 3 — selected by default, editable, and *not* marked "unverified AI suggestion": it is
grounded output subject to the existing summary grounding checks, which is a different thing from
a suggestion. The UI distinguishes the two badges accordingly (see contracts).

---

## 4. State transitions

```text
run:      running ──all sections ready──> ready ──export──> ready (export_status set)
             │
             ├──some ready, some failed──> partial ──per-section retry──> running
             └──all failed──────────────> failed

section:  running ──ranking accepted─────> ready
             │    ├─rejected, retry ok───> ready
             │    └─rejected twice───────> ready (fallback_used = true)   ← FR-010 fallback
             └──stage error──────────────> failed

item:     created ──toggle──> selected ⇄ unselected
                   ──reorder──> position changed
                   ──edit (ai only)──> edited_text set
                   ──master changed, source gone──> unavailable = true
```

A rerun of a section (FR-021) **deletes and recreates its items**, except that the user's
explicit decisions are re-applied where the underlying item still exists: a profile item is
matched by `source_index`, an AI item by normalised `source_text`, and a matched item keeps its
`selected`, `position` and `edited_text`. Anything unmatched is gone, which is what "re-running
replaces the AI's ordering for that section" means.

---

## 5. Why the master is snapshotted rather than referenced

`master_snapshot` holds the whole `RendercvMaster` the run ranked, and `source_text` is copied
onto each item at creation.

Without it, FR-022's "selections whose source item no longer exists are shown as unavailable"
cannot be implemented — the workspace would have no way to render an item whose source the user
just deleted, and would have to drop it silently, which is the exact behaviour FR-022 forbids.
With it, staleness is `master_content_hash != current hash`, unavailability is per-item, and an
export from a stale run still produces the document the user approved rather than a document
assembled from a profile they have since changed.

The cost is a jsonb copy per run of the same order as the `tailored_drafts.baseline` column 020
already accepted for the same reason.

---

## 6. Relationship to existing tables

- `"Profile"`, `"Job"`, `"ActivityRun"`, `"GeneratedDocument"` are referenced, unchanged. An
  exported run produces an ordinary `GeneratedDocument` row, so `GET /api/documents` and the PDF
  download work with no change — the same additive property 020 relied on.
- `resume_shape_setting` is read at run start and snapshotted into `shape_config`; the settings
  row is never read again for that run.
- `tailored_drafts` / `edit_proposals` are **dropped** (research R8), pending the explicit
  approval that research notes.

**FR-023 (auditable provenance) needs no new table**: `generation_items.origin` on the run that
produced a `GeneratedDocument`, reachable through `generation_runs.export_document_id`, is the
per-exported-item record. The stage/model provenance columns added by migration `00038` continue
to record which model served each stage.
