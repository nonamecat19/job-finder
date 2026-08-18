package domain

type RankedExperience struct {
	Company string `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
	Ranking []int  `json:"ranking" jsonschema_description:"the K bullet [index] values for THIS entry, most relevant to the vacancy first"`
}

type RankedProject struct {
	Name    string `json:"name" jsonschema_description:"project name copied EXACTLY from the master"`
	Ranking []int  `json:"ranking" jsonschema_description:"the K bullet [index] values for THIS project, most relevant to the vacancy first"`
}

type RankedSkills struct {
	GroupOrder []int `json:"groupOrder" jsonschema_description:"the [index] of each master skill group, most relevant first"`
}

type RankedSelection struct {
	Experience []RankedExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company, in the EXACT order shown"`
	Projects   []RankedProject    `json:"projects" jsonschema_description:"one entry per master project, keyed by name"`
	Skills     RankedSkills       `json:"skills" jsonschema_description:"the relevance order of the master's skill groups"`
}

type ExperienceSuggestions struct {
	Company string   `json:"company" jsonschema_description:"company name copied EXACTLY from the list below"`
	Bullets []string `json:"bullets" jsonschema_description:"achievement bullets the vacancy calls for that this candidate's profile does not contain"`
}

type SuggestionSet struct {
	Experience []ExperienceSuggestions `json:"experience"`
	Skills     []string                `json:"skills" jsonschema_description:"skills the vacancy asks for that the profile does not list"`
}

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
