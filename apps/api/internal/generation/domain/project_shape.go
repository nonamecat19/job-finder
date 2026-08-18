package domain

import "sort"

const (
	ProjectLevelTop3     = "top3"
	ProjectLevelTop5     = "top5"
	ProjectLevelAll      = "all"
	ProjectLevelRelevant = "relevant"
)

const (
	autoProjectMin = 2
	autoProjectMax = 5
)

func autoProjectCap(size int) int {
	cap := (size + 1) / 2
	if cap < autoProjectMin {
		cap = autoProjectMin
	}
	if cap > autoProjectMax {
		cap = autoProjectMax
	}
	if cap > size {
		cap = size
	}
	return cap
}

func RankProjects(doc RendercvMaster, analysis VacancyAnalysis, cfg ShapeConfig) {
	sections := CvSections(doc)
	if sections == nil {
		return
	}
	items, ok := sections["projects"].([]any)
	if !ok || len(items) < 2 {
		return
	}
	if cfg.ProjectsMax <= 0 || len(items) <= cfg.ProjectsMax {
		return
	}

	ranked := make([]any, 0, len(items))
	for _, i := range RankedProjectOrder(doc, analysis) {
		ranked = append(ranked, items[i])
	}
	sections["projects"] = ranked
}

func RankedProjectOrder(doc RendercvMaster, analysis VacancyAnalysis) []int {
	items, _ := CvSections(doc)["projects"].([]any)
	if len(items) == 0 {
		return nil
	}

	evidence := BuildProfileEvidence(doc)
	scores := make([]int, len(items))
	backing := make([]int, len(items))
	order := make([]int, len(items))
	for i, item := range items {
		order[i] = i
		p, ok := item.(map[string]any)
		if !ok {
			continue
		}
		scores[i], backing[i] = projectScore(p, analysis, evidence)
	}
	sort.SliceStable(order, func(a, b int) bool {
		i, j := order[a], order[b]
		if scores[i] != scores[j] {
			return scores[i] > scores[j]
		}
		return backing[i] > backing[j]
	})
	return order
}

func projectScore(p map[string]any, analysis VacancyAnalysis, evidence ProfileEvidence) (relevance, backing int) {
	for _, text := range projectTexts(p) {
		relevance += skillEntryScore(text, analysis)
		backing += evidence.Score(text)
	}
	return relevance, backing
}

func projectTexts(p map[string]any) []string {
	texts := make([]string, 0, 4)
	if name := StringField(p, "name"); name != "" {
		texts = append(texts, name)
	}
	if summary := StringField(p, "summary"); summary != "" {
		texts = append(texts, summary)
	}
	texts = append(texts, StringSliceField(p, "highlights")...)
	return texts
}

func TrimProjectHighlights(doc RendercvMaster, analysis VacancyAnalysis) bool {
	sections := CvSections(doc)
	if sections == nil {
		return false
	}
	changed := false
	for _, p := range AsSliceOfMaps(sections["projects"]) {
		highlights := StringSliceField(p, "highlights")
		if len(highlights) == 0 {
			continue
		}
		kept := projectKeep(highlights, StringField(p, "project_level"), analysis)
		if len(kept) < len(highlights) {
			p["highlights"] = toAnySlice(kept)
			changed = true
		}
	}
	return changed
}

func projectKeep(highlights []string, level string, analysis VacancyAnalysis) []string {
	switch level {
	case ProjectLevelAll:
		return highlights
	case ProjectLevelTop3:
		return highlights[:minInt(3, len(highlights))]
	case ProjectLevelTop5:
		return highlights[:minInt(5, len(highlights))]
	case ProjectLevelRelevant:
		if matched := matchedEntries(highlights, analysis); len(matched) > 0 {
			return matched
		}
		return highlights[:1]
	default:
		keep := len(matchedEntries(highlights, analysis))
		if cap := autoProjectCap(len(highlights)); cap > keep {
			keep = cap
		}
		return highlights[:minInt(keep, len(highlights))]
	}
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
