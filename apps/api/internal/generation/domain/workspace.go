package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	ItemKindAchievement ItemKind = "achievement"
	ItemKindSkillGroup  ItemKind = "skill_group"
	ItemKindSummary     ItemKind = "summary"
)

// SectionKind is one of the three section granularities data-model.md §2
// collapses into a single table.
type SectionKind string

const (
	SectionKindSummary    SectionKind = "summary"
	SectionKindExperience SectionKind = "experience"
	SectionKindSkills     SectionKind = "skills"
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
	Items        []Item
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
