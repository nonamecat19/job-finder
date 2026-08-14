package domain

import "sort"

// Project density levels, stored per project entry in the profile as
// "project_level". They bound how many bullets a project renders, the same way
// "skills_level" bounds a skill group's details list: "top3" (3), "top5" (5),
// "all" (everything), "relevant" (only the bullets that name something the
// vacancy asks for). An unset or unrecognised level is auto — see
// autoProjectCap.
const (
	ProjectLevelTop3     = "top3"
	ProjectLevelTop5     = "top5"
	ProjectLevelAll      = "all"
	ProjectLevelRelevant = "relevant"
)

// Auto bullet bounds for a project, read from the project's own bullet count:
// a project the candidate wrote six bullets about carries more than one they
// wrote two about, and a fixed cap would erase that difference.
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

// RankProjects orders the projects section by how much of the vacancy each
// project covers, so the cap in ApplyHardLimits keeps the relevant projects
// rather than whichever ones were written first.
//
// Same contract as RankSkills: nothing is added, reworded or removed — the
// output is a permutation of the input — and the reorder only happens when a
// cap is actually going to drop something. Without a cap every project renders
// and the master's authored order is the user's own choice of what to lead
// with; there is no decision for the score to make.
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

// RankedProjectOrder is the relevance order over the master's projects, as
// indices into the master's own list — the form the workspace needs, where the
// order is a *selection* boundary over items the user can still promote rather
// than a rewrite of the section.
//
// RankProjects is this order applied in place; the generation workspace reads
// the indices directly. Both stay one definition of "most relevant project".
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

// projectScore is a project's vacancy relevance and, as the tiebreaker, how
// much of the rest of the profile echoes it. Relevance sums over the bullets
// rather than taking the best one: a project touching four required skills is
// a better answer to this vacancy than one touching a single required skill,
// which is the opposite of the per-skill-entry case where summing would just
// reward listing more words in one entry.
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

// TrimProjectHighlights bounds every project's bullets to its authored density
// level and reports whether it changed anything.
//
// It runs after the selection stage has already ordered each project's bullets
// by relevance, so a count-based level keeps that order's first N. "relevant"
// keeps the bullets naming something the vacancy asks for — and, when none do,
// keeps the leading bullet rather than leaving a bare project title on the
// page: an entry with a name and nothing under it reads as an omission, and
// dropping the project entirely is the cap's decision to make, not this one's.
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

// projectKeep returns the bullets a level keeps, in the order they were
// authored. The count-based levels take a prefix, so they never reorder;
// "relevant" filters, which drops non-matching bullets from the middle but
// still leaves the survivors in their original sequence.
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
