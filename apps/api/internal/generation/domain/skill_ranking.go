package domain

import (
	"sort"
	"strings"
)

const (
	requiredSkillWeight   = 2
	niceToHaveSkillWeight = 1
)

func RankSkills(doc RendercvMaster, analysis VacancyAnalysis, cfg ShapeConfig) {
	sections := CvSections(doc)
	if sections == nil {
		return
	}
	groups := AsSliceOfMaps(sections["skills"])
	if len(groups) == 0 {
		return
	}

	evidence := BuildProfileEvidence(doc)

	for _, g := range groups {
		if IsPinnedSkillGroup(StringField(g, "label")) {
			continue
		}
		g["details"] = rankDetails(StringField(g, "details"), analysis, evidence)
	}

	if cfg.SkillsMaxGroups > 0 && len(groups) > cfg.SkillsMaxGroups {
		sections["skills"] = rankGroups(sections["skills"], analysis, evidence)
	}
}

func rankDetails(details string, analysis VacancyAnalysis, evidence ProfileEvidence) string {
	entries := splitSkillEntries(details)
	if len(entries) < 2 {
		return details
	}
	scores := make([]int, len(entries))
	backing := make([]int, len(entries))
	order := make([]int, len(entries))
	for i, e := range entries {
		scores[i] = skillEntryScore(e, analysis)
		backing[i] = evidence.Score(e)
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		i, j := order[a], order[b]
		if scores[i] != scores[j] {
			return scores[i] > scores[j]
		}
		return backing[i] > backing[j]
	})

	ranked := make([]string, 0, len(entries))
	for _, i := range order {
		ranked = append(ranked, entries[i])
	}
	return strings.Join(ranked, ", ")
}

func rankGroups(raw any, analysis VacancyAnalysis, evidence ProfileEvidence) []any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	required := make([]int, len(items))
	scores := make([]int, len(items))
	backing := make([]int, len(items))
	pinned := make([]bool, len(items))
	order := make([]int, len(items))
	for i, item := range items {
		order[i] = i
		g, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if IsPinnedSkillGroup(StringField(g, "label")) {
			pinned[i] = true
			continue
		}
		for _, e := range splitSkillEntries(StringField(g, "details")) {
			score := skillEntryScore(e, analysis)
			if score == requiredSkillWeight {
				required[i]++
			}
			scores[i] += score
			backing[i] += evidence.Score(e)
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		i, j := order[a], order[b]
		if pinned[i] != pinned[j] {
			return !pinned[i]
		}
		if required[i] != required[j] {
			return required[i] > required[j]
		}
		if scores[i] != scores[j] {
			return scores[i] > scores[j]
		}
		return backing[i] > backing[j]
	})

	out := make([]any, 0, len(items))
	for _, i := range order {
		out = append(out, items[i])
	}
	return out
}

func skillEntryScore(entry string, analysis VacancyAnalysis) int {
	best := 0
	for _, t := range tokens(entry) {
		switch {
		case matchesAnySkill(t, analysis.RequiredSkills):
			if requiredSkillWeight > best {
				best = requiredSkillWeight
			}
		case matchesAnySkill(t, analysis.NiceToHaveSkills):
			if niceToHaveSkillWeight > best {
				best = niceToHaveSkillWeight
			}
		}
	}
	return best
}

func splitSkillEntries(details string) []string {
	var out []string
	for _, e := range strings.Split(details, ",") {
		if trimmed := strings.TrimSpace(e); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
