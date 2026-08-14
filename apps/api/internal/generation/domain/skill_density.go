package domain

import "strings"

// Skill density levels, stored per skill group in the profile as
// "skills_level". They bound how much of a group's details list a generated
// resume renders. The count-based levels keep the group's most vacancy-
// relevant skills up to a cap: "top5" (5), "top10" (~10), "top15" (~15) and
// "top20" (~20). "all" keeps everything, and "relevant" keeps only the
// entries the vacancy asks for.
//
// A group with no level (or an unrecognised one) is *auto*: the count comes
// from the group itself — see autoSkillCap.
const (
	SkillLevelTop5     = "top5"
	SkillLevelTop10    = "top10"
	SkillLevelTop15    = "top15"
	SkillLevelTop20    = "top20"
	SkillLevelAll      = "all"
	SkillLevelRelevant = "relevant"
)

// Auto density bounds. A group's own size is the best available statement of
// how much the candidate has to say on that subject: a two-line "Languages"
// group and a thirty-entry "Backend" group are not the same claim, and giving
// both the same fixed cap flattens that. autoSkillCap keeps half of a group,
// floored at autoSkillMin so a small group never collapses to one entry and
// ceilinged at autoSkillMax so a long one cannot eat the page.
//
// The vacancy still wins: every entry the vacancy asks for is kept even when
// there are more of them than the cap. The cap only decides how much
// *unasked-for* depth follows the matches, in the relevance order RankSkills
// produced.
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

// TrimSkillGroups bounds the rendered document's skills down to each group's
// authored density level, and returns whether it changed anything.
//
// It must run after RankSkills: the ordering that function produces is the
// relevance order the trim relies on. The count-based levels keep that order's
// first N entries (fewer if the group is smaller): "top10" keeps ten, "top15"
// fifteen and "top20" twenty. "all" keeps everything, and "relevant" keeps
// only the entries the vacancy asks for. An unset or unknown level is auto:
// every vacancy match plus enough of the ranked remainder to reach
// autoSkillCap, so the count follows the profile's own depth in that group.
//
// Callers feed it master-fresh skills: every tailoring retry re-merges from
// the master before re-ranking, so the trim runs once per group. Trimming an
// already-trimmed group would trim it again — the fixed cap is deliberately
// not idempotent, and no pipeline path depends on it being so.
//
// Pinned groups are facts about the candidate, not tailoring targets, and are
// never trimmed — the same exemption skill_ranking.go and ApplyHardLimits
// already make. A "relevant" group with nothing matching is dropped from the
// document entirely; the profile and the workspace still hold it in full.
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
			// Keep only the entries the vacancy asks for. A group with no
			// match says nothing about this vacancy and is dropped.
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
			// Auto (unset or unknown level): every vacancy match, then as much
			// of the ranked remainder as the group's own size justifies.
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

// matchedEntries keeps the entries the vacancy asks for, in the order they
// arrive — which is the relevance order RankSkills produced.
func matchedEntries(entries []string, analysis VacancyAnalysis) []string {
	matched := make([]string, 0, len(entries))
	for _, e := range entries {
		if skillEntryScore(e, analysis) > 0 {
			matched = append(matched, e)
		}
	}
	return matched
}

// topCapFor returns the cap a count-based density level applies.
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
