package domain

import "strings"

// SuppressDuplicateSuggestions removes suggestion content that duplicates
// material the master already has (FR-017, resume-generation.md § 4.3). It never
// touches the master or any profile item — only the candidates inside
// `suggestions` are filtered, deterministically, so the same input produces
// the same suppression on every machine (the 038 corpus requires this).
//
// A suggested bullet is dropped when its normalised form (norm()) matches a
// master bullet for the SAME entry — matched by company via norm() — or when
// its word-set containment (wordSet()) against any master bullet for that
// entry is >= 0.9: at least 90% of the suggested bullet's significant words
// already appear in a single master bullet for the same company.
//
// A suggested skill is dropped when its normalised token (tokens()) exactly
// matches any master skill token — the same comparison basis
// DropUngroundedSkillTokens already uses for skill grounding.
func SuppressDuplicateSuggestions(suggestions SuggestionSet, master RendercvMaster) SuggestionSet {
	sections := CvSections(master)
	byCompany := map[string][]string{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		byCompany[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	masterSkillTokens := MasterSkillTokens(master)

	out := SuggestionSet{}
	for _, exp := range suggestions.Experience {
		masterBullets := byCompany[norm(exp.Company)]
		var kept []string
		for _, b := range exp.Bullets {
			b = strings.TrimSpace(b)
			if b == "" || duplicatesMasterBullet(b, masterBullets) {
				continue
			}
			kept = append(kept, b)
		}
		out.Experience = append(out.Experience, ExperienceSuggestions{Company: exp.Company, Bullets: kept})
	}

	for _, s := range suggestions.Skills {
		s = strings.TrimSpace(s)
		if s == "" || duplicatesMasterSkill(s, masterSkillTokens) {
			continue
		}
		out.Skills = append(out.Skills, s)
	}

	return out
}

// duplicatesMasterBullet is R6's suppression test for one candidate against
// one entry's master bullets: an exact normalised-form match, or >= 0.9
// word-set containment (the fraction of the candidate's own significant
// words that also appear in that master bullet).
func duplicatesMasterBullet(candidate string, masterBullets []string) bool {
	normCandidate := norm(candidate)
	candidateWords := wordSet(candidate)
	for _, b := range masterBullets {
		if norm(b) == normCandidate {
			return true
		}
		if wordSetContainment(candidateWords, wordSet(b)) >= 0.9 {
			return true
		}
	}
	return false
}

func duplicatesMasterSkill(skill string, masterTokens map[string]bool) bool {
	for _, t := range tokens(skill) {
		if masterTokens[t] {
			return true
		}
	}
	return false
}

// wordSetContainment is the fraction of `a`'s words that also appear in `b`
// — asymmetric, from the candidate's perspective, matching "word-set
// containment against a master bullet" (resume-generation.md § 4.3). An empty candidate
// contains nothing to check, so it is never treated as a duplicate here.
func wordSetContainment(a, b map[string]bool) float64 {
	if len(a) == 0 {
		return 0
	}
	overlap := 0
	for w := range a {
		if b[w] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(a))
}
