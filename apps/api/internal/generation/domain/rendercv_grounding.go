package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

func VerifyTailoredSectionsGrounding(master RendercvMaster, payload TailoredSections) []string {
	var violations []string
	allowed := MasterSkillTokens(master)
	sections := CvSections(master)

	for _, add := range payload.SkillGroupsToAdd {
		for _, t := range tokens(add.Details) {
			if !allowed[t] {
				violations = append(violations, `skill_group_add "`+add.Label+`" token "`+t+`" not in master skill tokens`)
			}
		}
	}

	masterGroupLabels := map[string]bool{}
	for _, g := range AsSliceOfMaps(sections["skills"]) {
		masterGroupLabels[norm(StringField(g, "label"))] = true
	}
	for _, label := range payload.SkillGroupsToRemove {
		if !masterGroupLabels[norm(label)] {
			violations = append(violations, `skill_group_remove "`+label+`" not in master skill groups`)
		}
	}

	for _, sc := range payload.SkillChanges {
		for _, t := range tokens(sc.AddTokens) {
			if !allowed[t] {
				violations = append(violations, `skill_change "`+sc.GroupLabel+`" add token "`+t+`" not in master skill tokens`)
			}
		}
	}

	masterCompanies := map[string][]string{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		masterCompanies[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	for _, pe := range payload.Experience {
		masterHighlights, ok := masterCompanies[norm(pe.Company)]
		if !ok {
			violations = append(violations, `experience "`+pe.Company+`" not in master`)
			continue
		}

		for _, ref := range pe.Highlights {
			if ref.SourceIndex < 0 || ref.SourceIndex >= len(masterHighlights) {
				violations = append(violations, fmt.Sprintf(`experience %q highlight index %d has no bullet in the master`, pe.Company, ref.SourceIndex))
				continue
			}
			h := strings.TrimSpace(ref.Rephrased)
			if h == "" {
				continue
			}
			source := []string{masterHighlights[ref.SourceIndex]}
			if !lcsCovered(h, source) {
				violations = append(violations, `experience "`+pe.Company+`" highlight not grounded in the bullet it rephrases: "`+truncateStr(h, 60)+`"`)
			}
			for _, m := range ungroundedMetrics(h, source) {
				violations = append(violations, `experience "`+pe.Company+`" highlight asserts metric "`+m+`" absent from the bullet it rephrases: "`+truncateStr(h, 60)+`"`)
			}
		}
	}

	return violations
}

func ungroundedMetrics(proposed string, sources []string) []string {
	if strings.IndexFunc(proposed, unicode.IsDigit) < 0 {
		return nil
	}
	allowed := map[string]bool{}
	for _, s := range sources {
		for _, f := range metricFingerprints(s) {
			allowed[f] = true
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range metricFingerprints(proposed) {
		if allowed[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

func metricFingerprints(s string) []string {
	var out []string
	for _, m := range numberRe.FindAllStringSubmatchIndex(s, -1) {
		start := m[0]
		if start > 0 {
			if prev := rune(s[start-1]); unicode.IsLetter(prev) || unicode.IsDigit(prev) {
				continue
			}
		}
		digits := strings.ReplaceAll(s[m[4]:m[5]], ",", "")
		currency := s[m[2]:m[3]]
		unit := ""

		end := m[5]
		if m[6] >= 0 {
			unit = s[m[6]:m[7]]
			end = m[7]
		}
		if end < len(s) && unicode.IsLetter(rune(s[end])) {
			continue
		}
		out = append(out, strings.ToLower(currency+digits+unit))
	}
	return out
}

func lcsCovered(proposed string, masterBullets []string) bool {
	proposedWords := wordSet(proposed)
	for _, b := range masterBullets {
		masterWords := wordSet(b)
		overlap := 0
		for w := range proposedWords {
			if masterWords[w] {
				overlap++
			}
		}
		if overlap >= len(proposedWords)/2 {
			return true
		}
	}
	return len(proposedWords) == 0
}

func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if n := norm(w); len(n) >= 3 {
			set[n] = true
		}
	}
	return set
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func VerifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel, analysis VacancyAnalysis) []string {
	var violations []string

	masterSections := CvSections(master)
	mergedSections := CvSections(merged)

	masterCompanies := map[string]bool{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanies[norm(StringField(e, "company"))] = true
	}

	masterCompanyHighlights := map[string][]string{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanyHighlights[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	for _, e := range AsSliceOfMaps(mergedSections["experience"]) {
		company := StringField(e, "company")
		companyKey := norm(company)
		if !masterCompanies[companyKey] {
			violations = append(violations, `company "`+company+`" not in master profile`)
		}

		masterBullets := masterCompanyHighlights[companyKey]
		for _, h := range StringSliceField(e, "highlights") {
			if !lcsCovered(h, masterBullets) {
				violations = append(violations, `experience "`+company+`" highlight not grounded in master: "`+truncateStr(h, 60)+`"`)
			}

			for _, m := range ungroundedMetrics(h, masterBullets) {
				violations = append(violations, `experience "`+company+`" highlight asserts metric "`+m+`" absent from the master's bullets: "`+truncateStr(h, 60)+`"`)
			}
		}
	}

	masterSectionKeys := map[string]bool{}
	for k := range masterSections {
		masterSectionKeys[k] = true
	}

	var addedSections []string
	for k := range mergedSections {
		if !masterSectionKeys[k] {
			addedSections = append(addedSections, k)
		}
	}
	sort.Strings(addedSections)
	for _, k := range addedSections {
		violations = append(violations, `unexpected section "`+k+`" added to merged resume`)
	}

	masterProjects := map[string]bool{}
	masterProjectHighlights := map[string][]string{}
	for _, p := range AsSliceOfMaps(masterSections["projects"]) {
		key := norm(StringField(p, "name"))
		masterProjects[key] = true
		masterProjectHighlights[key] = StringSliceField(p, "highlights")
	}
	for _, p := range AsSliceOfMaps(mergedSections["projects"]) {
		name := StringField(p, "name")
		if !masterProjects[norm(name)] {
			violations = append(violations, `project "`+name+`" not in master profile`)
			continue
		}

		own := masterProjectHighlights[norm(name)]
		for _, h := range StringSliceField(p, "highlights") {
			for _, m := range ungroundedMetrics(h, own) {
				violations = append(violations, `project "`+name+`" highlight asserts metric "`+m+`" absent from the master's bullets: "`+truncateStr(h, 60)+`"`)
			}
		}
	}

	allowed := MasterSkillTokens(master)

	vacancySkills := map[string]bool{}
	for _, s := range analysis.RequiredSkills {
		vacancySkills[norm(s)] = true
	}
	for _, s := range analysis.NiceToHaveSkills {
		vacancySkills[norm(s)] = true
	}

	masterGroupHasSkills := map[string]bool{}
	for _, g := range AsSliceOfMaps(masterSections["skills"]) {
		label := norm(StringField(g, "label"))
		if len(tokens(StringField(g, "details"))) > 0 {
			masterGroupHasSkills[label] = true
		}
	}
	for _, g := range AsSliceOfMaps(mergedSections["skills"]) {
		label := StringField(g, "label")
		for _, t := range tokens(StringField(g, "details")) {
			if allowed[t] {
				continue
			}
			if level == GroundingStrict {
				violations = append(violations, `skill "`+t+`" (`+label+`) not in master profile (strict grounding)`)
				continue
			}

			if vacancySkills[t] && masterGroupHasSkills[norm(label)] {
				continue
			}
			violations = append(violations, `skill "`+t+`" (`+label+`) not in master profile`)
		}
	}

	if level == GroundingStrict {
		masterProjectTokens := map[string]map[string]bool{}
		for _, p := range AsSliceOfMaps(masterSections["projects"]) {
			masterProjectTokens[norm(StringField(p, "name"))] = wordTokens(StringSliceField(p, "highlights"))
		}
		for _, p := range AsSliceOfMaps(mergedSections["projects"]) {
			name := StringField(p, "name")
			projectAllowed, ok := masterProjectTokens[norm(name)]
			if !ok {
				continue
			}

			var ungrounded []string
			for t := range wordTokens(StringSliceField(p, "highlights")) {
				if !projectAllowed[t] {
					ungrounded = append(ungrounded, t)
				}
			}
			sort.Strings(ungrounded)
			for _, t := range ungrounded {
				violations = append(violations, `project highlight token "`+t+`" (`+name+`) not in master profile (strict grounding)`)
			}
		}
	}

	return violations
}

func VerifySummaryGrounding(master RendercvMaster, summary TailoredSummary, brief SummaryBrief) []string {
	var violations []string
	text := strings.TrimSpace(summary.Summary)
	if text == "" {
		return violations
	}

	yearsAssertions := findYearsAssertions(text)

	allowed := MasterSkillTokens(master)
	grounded := summaryGroundedVocabulary(master, brief)
	for _, c := range summarySkillCandidates(text) {
		if allowed[norm(c)] || grounded[normSkillWord(c)] {
			continue
		}
		violations = append(violations, `summary skill "`+c+`" not in master skill tokens`)
	}

	briefNumbers := map[string]bool{}
	for _, h := range brief.Highlights {
		for _, n := range numberLiterals(h) {
			briefNumbers[n] = true
		}
	}
	for _, m := range metricClaims(text) {
		if withinYearsAssertion(m.start, yearsAssertions) || briefNumbers[m.value] {
			continue
		}
		violations = append(violations, `summary metric "`+m.text+`" not supported by the selected highlights`)
	}

	total := DeriveTotalExperienceYears(master)
	for _, a := range yearsAssertions {
		if a.number != total {
			violations = append(violations, fmt.Sprintf("summary asserts %q but master's experience spans %d years", a.text, total))
		}
	}

	return violations
}

func summarySkillCandidates(text string) []string {
	var out []string
	seen := map[string]bool{}
	sentenceStart := true
	for _, raw := range strings.Fields(text) {
		word := strings.Trim(raw, `.,;:!?()[]{}"'`)
		endsSentence := strings.HasSuffix(raw, ".") || strings.HasSuffix(raw, "!") || strings.HasSuffix(raw, "?")
		if isSkillShaped(word, sentenceStart) && !seen[normSkillWord(word)] {
			seen[normSkillWord(word)] = true
			out = append(out, word)
		}
		sentenceStart = endsSentence
	}
	return out
}

func isSkillShaped(word string, sentenceStart bool) bool {
	if len([]rune(word)) < 2 {
		return false
	}

	if strings.IndexFunc(word, unicode.IsDigit) >= 0 && !hasLetter(word) {
		return false
	}
	if strings.ContainsAny(word, "+#") {
		return true
	}
	letters, uppers := 0, 0
	for _, r := range word {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				uppers++
			}
		}
	}
	if letters == 0 {
		return false
	}
	if letters == uppers && letters >= 2 {
		return true
	}
	if i := strings.Index(word, "."); i > 0 && i < len(word)-1 {
		return true
	}
	return !sentenceStart && unicode.IsUpper([]rune(word)[0])
}

func summaryGroundedVocabulary(master RendercvMaster, brief SummaryBrief) map[string]bool {
	set := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			addVocabWords(t, set)
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(map[string]any(master))
	for _, h := range brief.Highlights {
		addVocabWords(h, set)
	}
	for _, l := range brief.SkillGroupLabels {
		addVocabWords(l, set)
	}
	return set
}

func addVocabWords(s string, set map[string]bool) {
	set[norm(s)] = true
	for _, w := range strings.FieldsFunc(s, isSkillWordSeparator) {
		if n := normSkillWord(w); n != "" {
			set[n] = true
		}
	}
}

func isSkillWordSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '.' && r != '+' && r != '#'
}

func normSkillWord(w string) string {
	return strings.ToLower(strings.Trim(w, ".'"))
}

var numberRe = regexp.MustCompile(`(?i)(\$?)(\d[\d,]*(?:\.\d+)?)\s?(%|[xkmb]\b)?`)

var metricCues = map[string]bool{
	"by": true, "of": true, "to": true, "from": true, "over": true, "under": true,
	"across": true, "than": true, "serving": true, "handling": true, "saving": true,
	"supporting": true, "processing": true,
}

type metricClaim struct {
	start int
	text  string
	value string
}

func numberLiterals(s string) []string {
	var out []string
	for _, m := range numberRe.FindAllStringSubmatch(s, -1) {
		out = append(out, strings.ReplaceAll(m[2], ",", ""))
	}
	return out
}

func metricClaims(s string) []metricClaim {
	var out []metricClaim
	for _, m := range numberRe.FindAllStringSubmatchIndex(s, -1) {
		start, end := m[0], m[1]
		if start > 0 {
			prev := rune(s[start-1])
			if unicode.IsLetter(prev) || unicode.IsNumber(prev) {
				continue
			}
		}
		hasUnit := m[2] != m[3] || m[6] != m[7]
		if !hasUnit && !metricCues[lastWord(s[:start])] {
			continue
		}
		out = append(out, metricClaim{
			start: start,
			text:  strings.TrimSpace(s[start:end]),
			value: strings.ReplaceAll(s[m[4]:m[5]], ",", ""),
		})
	}
	return out
}

func lastWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return normSkillWord(fields[len(fields)-1])
}

func withinYearsAssertion(pos int, assertions []yearsAssertion) bool {
	for _, a := range assertions {
		if pos >= a.start && pos < a.end {
			return true
		}
	}
	return false
}

func wordTokens(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		for _, w := range strings.FieldsFunc(b, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}) {
			if n := norm(w); len(n) >= 4 {
				set[n] = true
			}
		}
	}
	return set
}

func StripSummaryViolations(summary TailoredSummary, violations []string) TailoredSummary {
	if len(violations) == 0 {
		return summary
	}
	offending := map[string]bool{}
	for _, v := range violations {
		if q := quotedTerm(v); q != "" {
			offending[strings.ToLower(q)] = true
		}
	}
	if len(offending) == 0 {
		return TailoredSummary{}
	}
	var kept []string
	for _, sentence := range splitSentences(summary.Summary) {
		lower := strings.ToLower(sentence)
		clean := true
		for term := range offending {
			if strings.Contains(lower, term) {
				clean = false
				break
			}
		}
		if clean {
			kept = append(kept, sentence)
		}
	}
	return TailoredSummary{Summary: strings.TrimSpace(strings.Join(kept, " "))}
}

func quotedTerm(violation string) string {
	first := strings.Index(violation, `"`)
	if first < 0 {
		return ""
	}
	rest := violation[first+1:]
	last := strings.Index(rest, `"`)
	if last < 0 {
		return ""
	}
	return rest[:last]
}

func splitSentences(text string) []string {
	var out []string
	start := 0
	for i, r := range text {
		if r == '.' || r == '!' || r == '?' {
			s := strings.TrimSpace(text[start : i+1])
			if s != "" {
				out = append(out, s)
			}
			start = i + 1
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		out = append(out, tail)
	}
	return out
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
