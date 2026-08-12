package domain

import "strings"

// Skill density levels, stored per skill group in the profile as
// "skills_level". They bound how much of a group's details list a generated
// resume renders; a group without a level keeps everything, which is also
// what "all" means. An unknown level is treated as "all" — a lenient default
// rather than a rejected profile.
const (
	SkillLevelAll      = "all"
	SkillLevelMedium   = "medium"
	SkillLevelRelevant = "relevant"
)

// SkillLevels the rendered document's skills down to each group's authored
// density level, and returns whether it changed anything.
//
// It must run after RankSkills: the ordering that function produces is the
// relevance order the trim relies on. "medium" keeps the top half of each
// group (ceil, so a single-skill group never empties), "relevant" keeps only
// the entries the vacancy asks for, and "all" (the default for an unset or
// unknown level) keeps everything.
//
// Callers feed it master-fresh skills: every tailoring retry re-merges from
// the master before re-ranking, so the trim runs once per group. Trimming an
// already-trimmed group would halve it again — the proportional cut is
// deliberately not idempotent, and no pipeline path depends on it being so.
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
		if level == "" || level == SkillLevelAll {
			kept = append(kept, g)
			continue
		}
		entries := splitSkillEntries(StringField(g, "details"))
		switch level {
		case SkillLevelMedium:
			half := (len(entries) + 1) / 2
			if half >= len(entries) {
				kept = append(kept, g)
				continue
			}
			g["details"] = strings.Join(entries[:half], ", ")
			changed = true
		case SkillLevelRelevant:
			matched := make([]string, 0, len(entries))
			for _, e := range entries {
				if skillEntryScore(e, analysis) > 0 {
					matched = append(matched, e)
				}
			}
			if len(matched) == 0 {
				changed = true
				continue
			}
			if len(matched) < len(entries) {
				g["details"] = strings.Join(matched, ", ")
				changed = true
			}
		default:
			kept = append(kept, g)
			continue
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
