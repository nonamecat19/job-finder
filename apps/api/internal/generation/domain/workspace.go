package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// ItemOrigin distinguishes a profile-sourced candidate (text byte-identical
// to the master, per FR-009) from an AI-suggested one (unverified, editable,
// unselected by default). See data-model.md §3.
type ItemOrigin string

const (
	OriginProfile ItemOrigin = "profile"
	OriginAI      ItemOrigin = "ai"
)

// ItemKind drives which badge/affordance the client renders for an item.
type ItemKind string

const (
	ItemKindAchievement   ItemKind = "achievement"
	ItemKindSkillGroup    ItemKind = "skill_group"
	ItemKindSummary       ItemKind = "summary"
	ItemKindProject       ItemKind = "project"
	ItemKindCertification ItemKind = "certification"
	ItemKindEducation     ItemKind = "education"
)

// SectionKind is one of the section granularities data-model.md §2 collapses
// into a single table.
type SectionKind string

const (
	SectionKindSummary        SectionKind = "summary"
	SectionKindExperience     SectionKind = "experience"
	SectionKindSkills         SectionKind = "skills"
	SectionKindProjects       SectionKind = "projects"
	SectionKindCertifications SectionKind = "certifications"
	SectionKindEducation      SectionKind = "education"
)

// RunState is a generation_runs.state value (data-model.md §4).
type RunState string

const (
	RunRunning RunState = "running"
	RunReady   RunState = "ready"
	RunPartial RunState = "partial"
	RunFailed  RunState = "failed"
)

// SectionState is a generation_sections.state value.
type SectionState string

const (
	SectionRunning SectionState = "running"
	SectionReady   SectionState = "ready"
	SectionFailed  SectionState = "failed"
)

// Item is one candidate for inclusion — the spec's "Ranked Item"
// (data-model.md §3), independent of the sqlc row shape so the seeder and
// the assembler can be tested without a database.
type Item struct {
	ID          string
	SectionID   string
	Origin      ItemOrigin
	Kind        ItemKind
	SourceIndex *int
	SourceText  string
	EditedText  *string
	Rank        int
	Position    int
	Selected    bool
	Unavailable bool
	// DroppedEntries names the individual entries of a skill group's details
	// the user switched off (empty for every other kind of item). It is a
	// selection inside the item, the same way Selected is a selection inside
	// the section: nothing is reworded, only left out, so FR-009's
	// byte-identical rule still holds for what remains.
	DroppedEntries []string
}

// SkillEntries splits a skill-group item's display text into its individual
// entries and reports which of them are still included — the per-skill
// counterpart to Selected, and the one place the drop rule is applied.
//
// Only meaningful for a profile-origin item in a skills section: an AI
// suggestion is a single skill whose own text may contain commas
// ("Nest.js fundamentals (modules, guards, pipes)"), and splitting it would
// invent entries the model never wrote.
func (i Item) SkillEntries() []SkillEntry {
	_, details, found := strings.Cut(i.SourceText, ":")
	if !found {
		details = i.SourceText
	}
	entries := splitSkillEntries(details)
	dropped := make(map[string]bool, len(i.DroppedEntries))
	for _, d := range i.DroppedEntries {
		dropped[strings.TrimSpace(d)] = true
	}
	out := make([]SkillEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, SkillEntry{Text: e, Selected: !dropped[e]})
	}
	return out
}

// SkillEntry is one entry of a skill group's details plus whether the user
// kept it.
type SkillEntry struct {
	Text     string
	Selected bool
}

// EffectiveText is `edited_text ?? source_text` — computed here, never
// stored, so there is exactly one place this rule can drift (data-model.md
// §3). A profile-origin item never has EditedText set (enforced by the
// CHECK constraint and by SeedFromMaster never setting it), so this is a
// no-op identity for every profile item and only ever substitutes for an
// edited AI item.
func (i Item) EffectiveText() string {
	if i.EditedText != nil && *i.EditedText != "" {
		return *i.EditedText
	}
	return i.SourceText
}

// Edited reports whether the user has edited this item — only meaningful
// for origin='ai' items, per FR-015.
func (i Item) Edited() bool {
	return i.EditedText != nil && *i.EditedText != ""
}

// Section is one generation_sections row plus its items, in position order.
type Section struct {
	ID           string
	Kind         SectionKind
	EntryKey     *string
	EntryLabel   *string
	Position     int
	TargetCount  int
	State        SectionState
	Error        *string
	FallbackUsed bool
	// Enabled excludes the whole section from export/preview when false
	// (Assemble skips it outright), independent of any item's own
	// selection. Every constructor MUST set this explicitly — Go's zero
	// value is false, which would silently disable a freshly built section.
	Enabled bool
	Items   []Item
}

// NormalizeText exports grounding.go's norm() for callers outside this
// package that need the same normalisation basis — currently the rerun
// selection-preservation match (data-model.md §4: an AI item is matched to
// its replacement "by normalised source_text"), which must agree with R6's
// suppression check's idea of "the same text".
func NormalizeText(s string) string {
	return norm(s)
}

// ContentHash is a stable digest of a master resume, used for FR-022
// staleness detection: a run's snapshot hash compared against the profile's
// current hash. encoding/json sorts map keys on marshal, so this is
// deterministic across processes for the same logical content.
func ContentHash(master RendercvMaster) (string, error) {
	b, err := json.Marshal(map[string]any(master))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
