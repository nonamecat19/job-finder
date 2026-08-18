package domain

import "strings"

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
