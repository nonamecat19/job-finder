package domain

import "strings"

const (
	SkillLevelTop5     = "top5"
	SkillLevelTop10    = "top10"
	SkillLevelTop15    = "top15"
	SkillLevelTop20    = "top20"
	SkillLevelAll      = "all"
	SkillLevelRelevant = "relevant"
)

const (
	autoSkillMin = 4
	autoSkillMax = 12
)

func autoSkillCap(size int) int {
	cap := (size + 1) / 2
	if cap < autoSkillMin {
		cap = autoSkillMin
	}
	if cap > autoSkillMax {
		cap = autoSkillMax
	}
	if cap > size {
		cap = size
	}
	return cap
}

func TrimSkillGroups(doc RendercvMaster, analysis VacancyAnalysis) bool {
	sections := CvSections(doc)
	raw, ok := sections["skills"].([]any)
	if !ok || len(raw) == 0 {
		return false
	}
	changed := false
	kept := make([]any, 0, len(raw))
	for _, item := range raw {
		g, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		if IsPinnedSkillGroup(StringField(g, "label")) {
			kept = append(kept, g)
			continue
		}
		level := StringField(g, "skills_level")
		entries := splitSkillEntries(StringField(g, "details"))
		switch level {
		case SkillLevelAll:
			kept = append(kept, g)
			continue
		case SkillLevelTop5, SkillLevelTop10, SkillLevelTop15, SkillLevelTop20:
			cap := topCapFor(level)
			if len(entries) > cap {
				g["details"] = strings.Join(entries[:cap], ", ")
				changed = true
			}
		case SkillLevelRelevant:

			matched := matchedEntries(entries, analysis)
			if len(matched) == 0 {
				changed = true
				continue
			}
			if len(matched) < len(entries) {
				g["details"] = strings.Join(matched, ", ")
				changed = true
			}
		default:

			keep := len(matchedEntries(entries, analysis))
			if cap := autoSkillCap(len(entries)); cap > keep {
				keep = cap
			}
			if keep < len(entries) {
				g["details"] = strings.Join(entries[:keep], ", ")
				changed = true
			}
		}
		kept = append(kept, g)
	}
	if len(kept) == 0 {
		delete(sections, "skills")
		return true
	}
	sections["skills"] = kept
	return changed
}

func matchedEntries(entries []string, analysis VacancyAnalysis) []string {
	matched := make([]string, 0, len(entries))
	for _, e := range entries {
		if skillEntryScore(e, analysis) > 0 {
			matched = append(matched, e)
		}
	}
	return matched
}

func topCapFor(level string) int {
	switch level {
	case SkillLevelTop5:
		return 5
	case SkillLevelTop10:
		return 10
	case SkillLevelTop15:
		return 15
	case SkillLevelTop20:
		return 20
	default:
		return 0
	}
}
