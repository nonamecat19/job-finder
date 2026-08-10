package domain

// RankedExperience is one master experience entry's ranking: the K bullet
// indices (contracts/llm-contracts.md §1) the ranking stage judged most
// relevant, most relevant first.
//
// Deliberately absent: any free-text field. A field whose only legal value
// equals its source is a field with no purpose except to carry a violation
// (research.md R1) — the same move 028 made for SectionsToDrop/Drop and 035
// made for TailoredSelection's summary field, applied here to the last text
// channel that still reached the profile-sourced group.
type RankedExperience struct {
	Company string `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
	Ranking []int  `json:"ranking" jsonschema_description:"the K bullet [index] values for THIS entry, most relevant to the vacancy first"`
}

// RankedProject is RankedExperience's counterpart for the master's projects.
type RankedProject struct {
	Name    string `json:"name" jsonschema_description:"project name copied EXACTLY from the master"`
	Ranking []int  `json:"ranking" jsonschema_description:"the K bullet [index] values for THIS project, most relevant to the vacancy first"`
}

// RankedSkills orders the master's skill *groups* by relevance. Group
// contents are never returned here — they come from the master untouched,
// ordered within a group by the existing deterministic RankSkills (§2a of
// the domain doc) — this is only the between-group order.
type RankedSkills struct {
	GroupOrder []int `json:"groupOrder" jsonschema_description:"the [index] of each master skill group, most relevant first"`
}

// RankedSelection is the ranking stage's whole response — indices only, for
// every master experience entry, project and the skill-group order. This is
// what `llm.CompleteStructured[T]` unmarshals into, so the type is the
// contract: a rephrased bullet, a suggestion or a drop cannot be expressed
// because there is no field to hold one (contracts/llm-contracts.md §1).
type RankedSelection struct {
	Experience []RankedExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company, in the EXACT order shown"`
	Projects   []RankedProject    `json:"projects" jsonschema_description:"one entry per master project, keyed by name"`
	Skills     RankedSkills       `json:"skills" jsonschema_description:"the relevance order of the master's skill groups"`
}

// ExperienceSuggestions is one master company's worth of suggested
// achievement bullets — the suggestion stage's per-entry response
// (contracts/llm-contracts.md §2). Deliberately absent: any index field. A
// suggestion cannot claim to be one of the user's bullets, which is the
// mirror image of RankedExperience's missing text field — together the two
// types make the profile/AI distinction a property of the wire format.
type ExperienceSuggestions struct {
	Company string   `json:"company" jsonschema_description:"company name copied EXACTLY from the list below"`
	Bullets []string `json:"bullets" jsonschema_description:"achievement bullets the vacancy calls for that this candidate's profile does not contain"`
}

// SuggestionSet is the suggestion stage's whole response: achievement
// suggestions per company and a flat list of suggested skills. Routed
// through the existing generation-select task key (research.md R4) and run
// concurrently with the summary stage — it never sees the master's bullet
// text or skill tokens, only company names and skill group labels, so a
// suggestion cannot be a paraphrase of material it was never shown.
type SuggestionSet struct {
	Experience []ExperienceSuggestions `json:"experience"`
	Skills     []string                `json:"skills" jsonschema_description:"skills the vacancy asks for that the profile does not list"`
}

// SeedRankedItems builds one experience (or project) section's items from a
// verified ranking (research.md R2): the top min(target, available) ranked
// indices are selected, the remainder of the ranking is unselected, and any
// master bullet the ranking left out — when available exceeds K — is
// appended afterward in master order, unselected. Nothing is ever hidden.
//
// The caller is expected to have already verified `ranking` with
// VerifyRanking; an out-of-range or repeated index here is skipped rather
// than trusted, so a caller that forgot to verify fails safe instead of
// panicking or duplicating a bullet.
func SeedRankedItems(highlights []string, target int, ranking []int) []Item {
	n := target
	if n > len(highlights) {
		n = len(highlights)
	}
	if n < 0 {
		n = 0
	}

	items := make([]Item, 0, len(highlights))
	seen := make(map[int]bool, len(ranking))
	pos := 0
	for _, idx := range ranking {
		if idx < 0 || idx >= len(highlights) || seen[idx] {
			continue
		}
		seen[idx] = true
		i := idx
		items = append(items, Item{
			Origin:      OriginProfile,
			Kind:        ItemKindAchievement,
			SourceIndex: &i,
			SourceText:  highlights[idx],
			Rank:        pos,
			Position:    pos,
			Selected:    pos < n,
		})
		pos++
	}
	for i, h := range highlights {
		if seen[i] {
			continue
		}
		idx := i
		items = append(items, Item{
			Origin:      OriginProfile,
			Kind:        ItemKindAchievement,
			SourceIndex: &idx,
			SourceText:  h,
			Rank:        pos,
			Position:    pos,
			Selected:    false,
		})
		pos++
	}
	return items
}
