# Data Model — Feature 028: Strict Resume Structure Preservation During AI Tailoring

## No schema change

This feature enforces three structural-integrity invariants **in the tailoring/merge layer**, not in the data layer. There is **no new table, no new column, no migration**. The invariants operate on the existing `RendercvMaster` (generic `map[string]any`) and the existing `generated_documents.content` jsonb that feature 020's direct-tailor path already writes.

This section documents the *contract changes to existing in-memory structures* (the `TailoredSections` payload the LLM produces and `MergeTailored` applies), since those are the data shapes the feature alters.

## Changed in-memory structures

### `domain.TailoredSections` (`apps/api/internal/generation/domain/rendercv.go`)

The LLM output payload. Feature 028 **removes three fields** that granted the AI structural mutation power, and **removes one field** from `TailoredExperience`.

**Before (current)**:

```go
type TailoredSections struct {
    Summary         string               `json:"summary"`
    Skills          []TailoredSkillGroup `json:"skills"`
    Experience      []TailoredExperience `json:"experience"`
    SectionsToDrop  []string             `json:"sectionsToDrop,omitempty"`   // REMOVED
    ExperienceOrder []string             `json:"experienceOrder,omitempty"`  // REMOVED
}

type TailoredExperience struct {
    Company    string   `json:"company"`
    Highlights []string `json:"highlights"`
    Drop       bool     `json:"drop,omitempty"`   // REMOVED
}
```

**After (feature 028)**:

```go
type TailoredSections struct {
    Summary    string               `json:"summary"`
    Skills     []TailoredSkillGroup `json:"skills"`
    Experience []TailoredExperience `json:"experience"`
}

type TailoredExperience struct {
    Company    string   `json:"company"`
    Highlights []string `json:"highlights"`
}
```

- `SectionsToDrop` removed → AI cannot drop non-protected blocks (invariant #1).
- `ExperienceOrder` removed → AI cannot reorder job entries (invariant #2).
- `Drop` removed → AI cannot drop job entries (invariant #2).
- `TailoredSkillGroup` is **unchanged** (`Index int`, `Details string`) — skills remain the one place where group-internal reordering is allowed (vacancy-required skills first), per feature 020 FR-001.

### `packages/shared/src/generated.ts` (tygo-regenerated)

After the Go struct change, `make tygo-generate` regenerates the TS mirror. The `TailoredSections`/`TailoredExperience` TS types lose the three removed fields automatically.

### `packages/shared/src/index.ts` (hand-mirror)

Per AGENTS.md convention, `index.ts` is hand-maintained (not auto-imported from `generated.ts`) until feature 024 removes the duplicate. Any hand-maintained `TailoredSections`/`TailoredExperience` mirror in `index.ts` must have `sectionsToDrop`, `experienceOrder`, and `drop` removed in the same change.

## New in-memory structures

### `domain.StructureViolations` (new, `apps/api/internal/generation/domain/rendercv_structure.go`)

A typed list of structural-integrity violations returned by `VerifyStructureIntegrity`. Not persisted — consumed by the tailoring loop to drive the single text-years re-prompt and the strip-and-flag fallback (research R5). The block-sequence/experience-order/dropped-jobs invariants are enforced by merge-layer removal (research R1/R5) and **never produce violations**; only the text-asserted-years check produces violations.

```go
type StructureViolation struct {
    Kind    StructureKind   // "total_experience_years"
    Path    string           // e.g. "cv.sections.summary[0]" or "cv.sections.experience[Acme].highlights[2]"
    Message string           // human-readable, e.g. "summary asserts '12 years' but master spans 5 years"
}
type StructureKind string
const (
    StructureTotalExperienceYears StructureKind = "total_experience_years"
)
```

## Unchanged entities (reused, no schema change)

- **`RendercvMaster`** (`domain.RendercvMaster = map[string]any`) — unchanged; the master resume config. The structural invariants read its `cv.sections["_order"]` key (for block order, research R3) and its `cv.sections.experience[].start_date/end_date` (for the total-experience-years derivation, research R4).
- **`generated_documents`** table — unchanged; the direct-tailor path writes the merged `RendercvMaster` as `content` jsonb and the PDF as `pdf_path`, exactly as today.
- **`dto.Resume` / `dto.Section` / `dto.Entry`** — unchanged; the resume DTO is not affected. The constraint is on the *tailoring payload*, not the resume data model. Dates (`Date`, `StartDate`, `EndDate`) remain on `dto.Entry` and stay outside the AI allow-list (research R4).
- **`protectedSections`** map (`rendercv.go:104-109`) — kept but its role shrinks: it was previously consulted by `MergeTailored` to decide which sections the AI *could* drop. After feature 028 removes `SectionsToDrop`, no sections can be dropped at all, so `protectedSections` becomes a defensive reference (and a guard against any future reintroduction of drop capability). It is **not removed** to avoid touching unrelated code, but it is no longer on the enforcement path.

## Enforcement points (no DB involved)

| Invariant | Where enforced | How |
|-----------|---------------|-----|
| #1 Block sequence immutable | `MergeTailored` (removal of `SectionsToDrop` handling) | The `delete(sections, key)` loop at rendercv.go:345-350 is removed; no section is ever dropped. Section order was already preserved by the untouched `cv.sections["_order"]` key (research R3). |
| #1 Block sequence: no add/rename/reorder | `MergeTailored` (structural) | The function only mutates section *contents* (summary, skills, experience highlights) and never writes/removes section keys or the `_order` key. No code path exists to add/rename/reorder a section. |
| #2 Experience order + no job drops | `MergeTailored` (removal of `ExperienceOrder` + `Drop` handling) | The `kept` filter (rendercv.go:310-316) becomes a no-op (no `_drop` markers set), and the reorder block (rendercv.go:318-337) is removed. `sections["experience"]` is rewritten from the master-order `experience` slice with only per-entry `highlights` changed. |
| #3 Dates unchanged | `MergeTailored` (existing allow-list) | `MergeTailored` never writes `start_date`/`end_date`/`date`/`company`/`position`/`location` — they pass through verbatim from the deep-cloned master. Already true; feature 028 makes the rationale explicit in code comments and the plan. |
| #3 No text-asserted years figure | `VerifyStructureIntegrity` (new, post-merge) + single re-prompt + strip fallback | A regex-based check scans `merged.cv.sections.summary[0]` and each `merged.cv.sections.experience[].highlights[]` for numeric years-of-experience assertions (e.g. "over N years", "N+ years", "N years of experience") and compares against the master's derivable total. On mismatch: one targeted re-prompt (feed violation back), then strip the offending clause and log on the activity. |

## Total experience years derivation (reference, for the text check)

There is no stored total. When the text-asserted-years check needs the master's "true" total to compare against, it derives it the same way any reader would:

- For each `cv.sections.experience[]` entry with `start_date` and/or `end_date` (free-text, e.g. "2020-01" or "Jan 2020 – Present"): parse the year from `start_date` and `end_date` (treat "Present"/empty end as the current year).
- Total = sum of per-entry `(endYear - startYear)`, clamped to ≥0, rounded down.
- This is a **read-only derivation used only by the text check**, never persisted and never exposed to the AI as a field it can edit. If an entry has no parseable dates, it contributes 0 (the check is conservative — it only flags a *contradiction*, not an absence).

The derivation lives in `domain/rendercv_structure.go` alongside `VerifyStructureIntegrity` so the invariant and its helper stay together.

## Migration note

**No migration.** Goose migration version numbers must remain unique and sequential (constitution); this feature introduces none. The largest migration today is `00032_batch_ingest.sql`; feature 028 does not add `00033`. (Audited: `ls apps/api/internal/db/migrations/` — no `edit_proposals`/`tailored_drafts` tables exist; feature 020's data model is specified-only.)