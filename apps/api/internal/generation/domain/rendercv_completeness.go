package domain

import (
	"fmt"
	"strings"
	"unicode"
)

type CompletenessReport struct {
	RequiredMissing    []string
	NiceToHaveRetained float64
	NiceToHaveMissing  []string
	BulletShortfalls   map[string]int
	StructuralFallback bool
	Shortfall          bool

	structuralDetail string
}

const niceToHaveRetentionFloor = 0.80

// VerifyCompleteness is a pure function of (master, merged, analysis, cfg): it
// answers whether the selection stage kept the material the vacancy actually
// wants, which is what decides retry/escalation (FR-006, FR-007).
func VerifyCompleteness(master, merged RendercvMaster, analysis VacancyAnalysis, cfg ShapeConfig) CompletenessReport {
	report := CompletenessReport{NiceToHaveRetained: 1}

	masterTokens := orderedSkillTokens(master)
	mergedTokens := orderedSkillTokens(merged)

	if len(analysis.RequiredSkills) == 0 {
		// A thin analysis makes "every required skill retained" vacuously true,
		// which would silently disable the whole gate (research R7). Fall back
		// to a structural floor and record that it happened.
		report.StructuralFallback = true
		masterGroups := len(AsSliceOfMaps(CvSections(master)["skills"]))
		mergedGroups := len(AsSliceOfMaps(CvSections(merged)["skills"]))
		if masterGroups != mergedGroups {
			report.Shortfall = true
			report.structuralDetail = fmt.Sprintf("skill groups %d, want %d", mergedGroups, masterGroups)
		}
	} else {
		niceTotal := 0
		for _, t := range masterTokens {
			switch {
			case matchesAnySkill(t, analysis.RequiredSkills):
				if !tokenRetained(t, mergedTokens) {
					report.RequiredMissing = append(report.RequiredMissing, t)
				}
			case matchesAnySkill(t, analysis.NiceToHaveSkills):
				niceTotal++
				if !tokenRetained(t, mergedTokens) {
					report.NiceToHaveMissing = append(report.NiceToHaveMissing, t)
				}
			}
		}
		// A master with no nice-to-have matches has nothing to lose, so it stays
		// fully retained rather than dropping to zero.
		if niceTotal > 0 {
			report.NiceToHaveRetained = float64(niceTotal-len(report.NiceToHaveMissing)) / float64(niceTotal)
		}
		if len(report.RequiredMissing) > 0 || report.NiceToHaveRetained < niceToHaveRetentionFloor {
			report.Shortfall = true
		}
	}

	if shortfalls := bulletShortfalls(master, merged, cfg); len(shortfalls) > 0 {
		report.BulletShortfalls = shortfalls
		report.Shortfall = true
	}

	return report
}

func (r CompletenessReport) Reason() string {
	var parts []string
	if len(r.RequiredMissing) > 0 {
		parts = append(parts, "required skills missing: "+strings.Join(r.RequiredMissing, ", "))
	}
	if r.NiceToHaveRetained < niceToHaveRetentionFloor {
		parts = append(parts, fmt.Sprintf("nice-to-have retention %.0f%% below %.0f%% (missing: %s)",
			r.NiceToHaveRetained*100, niceToHaveRetentionFloor*100, strings.Join(r.NiceToHaveMissing, ", ")))
	}
	if len(r.BulletShortfalls) > 0 {
		var bullets []string
		for _, company := range sortedKeys(r.BulletShortfalls) {
			bullets = append(bullets, fmt.Sprintf("%s (%d)", company, r.BulletShortfalls[company]))
		}
		parts = append(parts, "highlights below minimum: "+strings.Join(bullets, ", "))
	}
	if r.structuralDetail != "" {
		parts = append(parts, "structural check failed: "+r.structuralDetail)
	}
	if len(parts) == 0 {
		return ""
	}
	reason := strings.Join(parts, "; ")
	if r.StructuralFallback {
		reason += " (vacancy analysis had no required skills; structural check used)"
	}
	return reason
}

// bulletShortfalls reports only shortfalls the model could have avoided. A
// company whose master entry holds fewer highlights than the minimum can never
// meet it, so flagging it would blame the model for a master gap — the same
// distinction ApplyHardLimits draws before recording a Shortfall.
func bulletShortfalls(master, merged RendercvMaster, cfg ShapeConfig) map[string]int {
	if cfg.ExperienceBulletsMin <= 0 {
		return nil
	}
	masterCounts := map[string]int{}
	for _, e := range AsSliceOfMaps(CvSections(master)["experience"]) {
		masterCounts[norm(StringField(e, "company"))] = len(StringSliceField(e, "highlights"))
	}

	var out map[string]int
	for _, e := range AsSliceOfMaps(CvSections(merged)["experience"]) {
		company := StringField(e, "company")
		available, inMaster := masterCounts[norm(company)]
		if !inMaster || available < cfg.ExperienceBulletsMin {
			continue
		}
		if got := len(StringSliceField(e, "highlights")); got < cfg.ExperienceBulletsMin {
			if out == nil {
				out = map[string]int{}
			}
			out[company] = got
		}
	}
	return out
}

func orderedSkillTokens(master RendercvMaster) []string {
	seen := map[string]bool{}
	var out []string
	for _, g := range AsSliceOfMaps(CvSections(master)["skills"]) {
		for _, t := range tokens(StringField(g, "details")) {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	return out
}

func tokenRetained(masterToken string, mergedTokens []string) bool {
	for _, m := range mergedTokens {
		if phrasesMatch(masterToken, m) {
			return true
		}
	}
	return false
}

// matchesAnySkill compares tokenized phrases on both sides: vacancy skills are
// free text ("React Native", "Docker/Kubernetes") and must be split the same
// way master skill details are before anything can be said to match.
func matchesAnySkill(masterToken string, vacancySkills []string) bool {
	for _, s := range vacancySkills {
		for _, v := range tokens(s) {
			if phrasesMatch(masterToken, v) {
				return true
			}
		}
	}
	return false
}

// phrasesMatch treats two skill phrases as the same skill when one's words are
// a subset of the other's — "react native" matches "react native (expo)" while
// substring comparison would also match "go" against "django".
func phrasesMatch(a, b string) bool {
	aw, bw := phraseWords(a), phraseWords(b)
	if len(aw) == 0 || len(bw) == 0 {
		return false
	}
	return isSubset(aw, bw) || isSubset(bw, aw)
}

func isSubset(sub, super map[string]bool) bool {
	for w := range sub {
		if !super[w] {
			return false
		}
	}
	return true
}

func phraseWords(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		set[norm(w)] = true
	}
	return set
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
