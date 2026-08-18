package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type ItemOrigin string

const (
	OriginProfile ItemOrigin = "profile"
	OriginAI      ItemOrigin = "ai"
)

type ItemKind string

const (
	ItemKindAchievement   ItemKind = "achievement"
	ItemKindSkillGroup    ItemKind = "skill_group"
	ItemKindSummary       ItemKind = "summary"
	ItemKindProject       ItemKind = "project"
	ItemKindCertification ItemKind = "certification"
	ItemKindEducation     ItemKind = "education"
)

type SectionKind string

const (
	SectionKindSummary        SectionKind = "summary"
	SectionKindExperience     SectionKind = "experience"
	SectionKindSkills         SectionKind = "skills"
	SectionKindProjects       SectionKind = "projects"
	SectionKindCertifications SectionKind = "certifications"
	SectionKindEducation      SectionKind = "education"
)

type RunState string

const (
	RunRunning RunState = "running"
	RunReady   RunState = "ready"
	RunPartial RunState = "partial"
	RunFailed  RunState = "failed"
)

type SectionState string

const (
	SectionRunning SectionState = "running"
	SectionReady   SectionState = "ready"
	SectionFailed  SectionState = "failed"
)

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

	DroppedEntries []string
}

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

type SkillEntry struct {
	Text     string
	Selected bool
}

func (i Item) EffectiveText() string {
	if i.EditedText != nil && *i.EditedText != "" {
		return *i.EditedText
	}
	return i.SourceText
}

func (i Item) Edited() bool {
	return i.EditedText != nil && *i.EditedText != ""
}

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

	Enabled bool
	Items   []Item
}

func NormalizeText(s string) string {
	return norm(s)
}

func ContentHash(master RendercvMaster) (string, error) {
	b, err := json.Marshal(map[string]any(master))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
