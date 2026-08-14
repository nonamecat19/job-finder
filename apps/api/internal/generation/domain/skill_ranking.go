package domain

import (
	"sort"
	"strings"
)

// Skill ordering is a code decision, not a model decision.
//
// The select prompt used to ask the model to "reorder skills within each group,
// vacancy-required skills first" and to return a rewritten details string per
// group. That handed a cheap model a free-text field over content it had no
// business rewriting: a reorder and a quiet deletion look identical in the
// response, and only the completeness gate stood between a dropped skill and
// the rendered document.
//
// Ranking is arithmetic. The model still decides what the vacancy wants — that
// is the analysis stage's judgement call — but turning that into an order over
// the candidate's own skills is a pure function here: reproducible, testable,
// and structurally incapable of dropping or inventing a skill, because its
// output is a permutation of its input.
const (
	requiredSkillWeight   = 2
	niceToHaveSkillWeight = 1
)

// RankSkills reorders skills inside every group of doc so the ones the vacancy
// asks for come first, and — only when a group cap is configured — reorders the
// groups themselves by how much of the vacancy they cover, so the cap that
// follows keeps the relevant groups rather than the first ones written.
//
// Nothing is added, reworded or removed. Pinned groups ("Spoken Languages") are
// left exactly as authored: they are a fact about the candidate, not a
// tailoring target.
func RankSkills(doc RendercvMaster, analysis VacancyAnalysis, cfg ShapeConfig) {
	sections := CvSections(doc)
	if sections == nil {
		return
	}
	groups := AsSliceOfMaps(sections["skills"])
	if len(groups) == 0 {
		return
	}

	// Two signals, in this order: what the vacancy asks for, then what the
	// candidate's own experience evidences. See profile_evidence.go for why
	// the second one exists and why it can never overturn the first.
	evidence := BuildProfileEvidence(doc)

	for _, g := range groups {
		if IsPinnedSkillGroup(StringField(g, "label")) {
			continue
		}
		g["details"] = rankDetails(StringField(g, "details"), analysis, evidence)
	}

	// Without a cap every group renders, so their order is the master's
	// authored order and there is no choice to make. With a cap, something has
	// to lose a slot, and that choice belongs to the score rather than to
	// whichever group the user happened to write first.
	if cfg.SkillsMaxGroups > 0 && len(groups) > cfg.SkillsMaxGroups {
		sections["skills"] = rankGroups(sections["skills"], analysis, evidence)
	}
}

// rankDetails reorders one group's comma-separated skills by vacancy
// relevance, breaking ties on how much the candidate's own experience
// evidences the skill and, for entries the profile evidences equally, on the
// master's order. Each entry's original text is preserved.
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

// rankGroups orders skill groups by the vacancy relevance of their contents.
// Pinned groups sort after the ranked ones and keep their relative order; the
// cap in ApplyHardLimits keeps them regardless of position.
//
// The first key is how many *required* skills a group carries, not the summed
// score: summing lets two nice-to-haves outweigh one hard requirement, and the
// group that loses that comparison loses a required skill off the page
// entirely when the cap bites. Summed score breaks a tie in required coverage,
// and profile evidence breaks a tie in that.
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

// skillEntryScore is the relevance of one skill entry: the best match any of
// its tokens makes against the vacancy. Max rather than sum, so "Docker /
// Kubernetes / Helm" does not outrank a single required skill by listing more
// words.
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

// splitSkillEntries splits a details field the way it is written rather than
// the way it is matched: tokens() normalises for comparison, which would strip
// the casing and spacing the document has to render.
func splitSkillEntries(details string) []string {
	var out []string
	for _, e := range strings.Split(details, ",") {
		if trimmed := strings.TrimSpace(e); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
