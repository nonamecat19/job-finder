package domain

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// VerifyTailoredSectionsGrounding checks the LLM's TailoredSections payload
// against the master before any merge. It enforces the tighter rules from
// feature 020: SkillGroupsToAdd tokens must all be in MasterSkillTokens;
// SkillGroupsToRemove labels must exist in the master; SkillChanges.AddTokens
// must be in MasterSkillTokens; per-company Highlights whose LCS-matched set
// doesn't cover the proposed bullets are rejected.
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
		// A reference names the bullet it rephrases, so each rewording is held
		// against that one bullet rather than against anything the company ever
		// did. Merging two bullets into one claim used to satisfy this check;
		// now it cannot be expressed.
		for _, ref := range pe.Highlights {
			if ref.SourceIndex < 0 || ref.SourceIndex >= len(masterHighlights) {
				violations = append(violations, fmt.Sprintf(`experience %q highlight index %d has no bullet in the master`, pe.Company, ref.SourceIndex))
				continue
			}
			h := strings.TrimSpace(ref.Rephrased)
			if h == "" {
				continue // the master's own wording, verbatim
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

// ungroundedMetrics returns the numbers a proposed bullet asserts that none of
// its master sources contain.
//
// This is the hole lcsCovered cannot see: wordSet drops tokens shorter than
// three characters, so "40%" reduces to "40" and is discarded before the
// overlap is counted. A model could take a real bullet, attach a metric the
// candidate never claimed, and pass the drift check on the surrounding words
// alone — which is the single most damaging thing a generated resume can do,
// because the number is exactly what a hiring manager asks about.
//
// Sources are the master bullets the proposal may draw from — the single
// bullet a reference names when checked pre-merge, and every bullet of the same
// company or project when checked on the finished document, where the mapping
// is no longer available.
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

// metricFingerprints reduces the numeric claims in a string to comparable
// keys — "1,200" and "1200" are the same claim, "$2M" and "2" are not.
//
// Digits embedded in an identifier ("S3", "p95", "EC2") are skipped: they name
// a technology rather than assert a quantity, and treating them as metrics
// would flag a bullet for mentioning the same service the master mentions
// elsewhere.
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
		// The match itself can extend past the unit — the regex's optional
		// separator consumes the space in "12 years" — so the boundary check
		// uses the number's own end, not the match's.
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

// VerifyRendercvGrounding is the post-merge grounding check for the RenderCV
// path. It verifies:
//   - No fabricated companies (experience entries must exist in master)
//   - No added sections (merged sections are a subset of master sections)
//   - No fabricated project names
//   - Skill tokens outside the master skill pool — at ALL grounding levels
//     (033 FR-001). Under strict, every non-master token is a violation. Under
//     moderate/aggressive, a non-master token is allowed only when it appears in
//     the vacancy analysis AND the master already has a skill in the same group
//     (adjacency); otherwise it is a violation.
//   - Experience highlights that have drifted too far from the master's bullets
//     — at ALL grounding levels (033 FR-002), via the lcsCovered ≥50% word-overlap
//     check.
//   - On strict grounding, project highlight tokens must come from that same
//     project's own master bullets.
//
// It does not check structural invariants (block sequence, experience order,
// total experience years) — those are enforced by MergeTailored and
// VerifyStructureIntegrity (see rendercv_structure.go).
func VerifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel, analysis VacancyAnalysis) []string {
	var violations []string

	masterSections := CvSections(master)
	mergedSections := CvSections(merged)

	// 1. Check experience companies in merged are all from master
	masterCompanies := map[string]bool{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanies[norm(StringField(e, "company"))] = true
	}
	// Also build per-company master highlights for the drift check (FR-002).
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
		// 4. Highlight drift: every highlight must have ≥50% word-overlap
		// with at least one master bullet for the same company (033 FR-002).
		masterBullets := masterCompanyHighlights[companyKey]
		for _, h := range StringSliceField(e, "highlights") {
			if !lcsCovered(h, masterBullets) {
				violations = append(violations, `experience "`+company+`" highlight not grounded in master: "`+truncateStr(h, 60)+`"`)
			}
			// 5. Metric grounding: a number the master never claimed is a
			// fabrication the word-overlap check above cannot detect.
			for _, m := range ungroundedMetrics(h, masterBullets) {
				violations = append(violations, `experience "`+company+`" highlight asserts metric "`+m+`" absent from the master's bullets: "`+truncateStr(h, 60)+`"`)
			}
		}
	}

	// 2. Verify sections in merged are a subset of master sections (no added sections)
	masterSectionKeys := map[string]bool{}
	for k := range masterSections {
		masterSectionKeys[k] = true
	}
	// Sorted, not map-ordered: these strings are fed back into the retry prompt
	// (service.go tailorRendercvResume -> prevViolations -> buildSelectPrompt),
	// so map iteration order would make the retry request text differ between
	// runs on identical input. That breaks request-hash-keyed replay.
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

	// 3. Check project names in merged are all from master. Projects gained a
	// tailoring path, so they need the same no-fabrication guard companies
	// have: the model may select among the master's projects, never add one.
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
		// Metrics are checked at every grounding level: an invented number is
		// not a stylistic liberty that a looser level should tolerate.
		own := masterProjectHighlights[norm(name)]
		for _, h := range StringSliceField(p, "highlights") {
			for _, m := range ungroundedMetrics(h, own) {
				violations = append(violations, `project "`+name+`" highlight asserts metric "`+m+`" absent from the master's bullets: "`+truncateStr(h, 60)+`"`)
			}
		}
	}

	// 4. Skill-token grounding at ALL levels (033 FR-001). Under strict, any
	// non-master token is a violation. Under moderate/aggressive, a non-master
	// token is allowed only when it is vacancy-required (or nice-to-have) AND
	// the master already has a skill in the same group (adjacency allowance).
	allowed := MasterSkillTokens(master)
	// Build vacancy-required set for adjacency check.
	vacancySkills := map[string]bool{}
	for _, s := range analysis.RequiredSkills {
		vacancySkills[norm(s)] = true
	}
	for _, s := range analysis.NiceToHaveSkills {
		vacancySkills[norm(s)] = true
	}
	// Build per-group label set from master, so adjacency can check "does the
	// master have ANY skill in this group?" — if yes, a vacancy-required token
	// in the same group is considered adjacent.
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
			// Moderate/aggressive: allow if vacancy-required AND the master
			// already has a skill in this group (adjacency).
			if vacancySkills[t] && masterGroupHasSkills[norm(label)] {
				continue
			}
			violations = append(violations, `skill "`+t+`" (`+label+`) not in master profile`)
		}
	}

	// 5. Strict project grounding: each project's highlights must come
	// from that same project's own master bullets — a bullet borrowed
	// from a *different* project is still a misattribution.
	if level == GroundingStrict {
		masterProjectTokens := map[string]map[string]bool{}
		for _, p := range AsSliceOfMaps(masterSections["projects"]) {
			masterProjectTokens[norm(StringField(p, "name"))] = wordTokens(StringSliceField(p, "highlights"))
		}
		for _, p := range AsSliceOfMaps(mergedSections["projects"]) {
			name := StringField(p, "name")
			projectAllowed, ok := masterProjectTokens[norm(name)]
			if !ok {
				continue // already reported as an unknown project above
			}
			// Sorted for the same reason as the added-section check above:
			// these violations reach the retry prompt, so map order would make
			// the retry request non-reproducible on identical input.
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

// VerifySummaryGrounding checks a summary written by its own stage against the
// master profile and the brief it was written from (035 FR-008). Because the
// summary now arrives alone rather than buried in a combined response, all
// three of its failure modes can be checked precisely:
//   - a skill the master does not list,
//   - a numeric achievement metric none of the selected highlights supports,
//   - a years-of-experience figure contradicting DeriveTotalExperienceYears.
//
// It returns one violation string per problem; an empty slice means the summary
// is grounded. The caller re-prompts once and strips on a second failure
// (035 FR-009).
func VerifySummaryGrounding(master RendercvMaster, summary TailoredSummary, brief SummaryBrief) []string {
	var violations []string
	text := strings.TrimSpace(summary.Summary)
	if text == "" {
		return violations
	}

	// The years figure is asserted with numbers too, so both checks below need
	// to know where those assertions sit: check 3 owns them, check 2 skips them.
	yearsAssertions := findYearsAssertions(text)

	// 1. Skill tokens. A summary is prose, not a comma-separated details
	// field, so the candidate terms are picked out by shape first (acronym,
	// dotted or symbol-bearing name, or a mid-sentence proper noun) and only
	// then checked against the master. Anything the master mentions anywhere —
	// skill tokens, group labels, companies, projects, education, the selected
	// highlights — counts as grounded, which is the same judgement
	// DropUngroundedSkillTokens and VerifyRendercvGrounding make: grounded
	// means traceable to the master, not absent from a curated list.
	allowed := MasterSkillTokens(master)
	grounded := summaryGroundedVocabulary(master, brief)
	for _, c := range summarySkillCandidates(text) {
		if allowed[norm(c)] || grounded[normSkillWord(c)] {
			continue
		}
		violations = append(violations, `summary skill "`+c+`" not in master skill tokens`)
	}

	// 2. Numeric metrics. Every achievement number the summary asserts must
	// come from an achievement the selection stage actually kept.
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

	// 3. Years figure, against the same derivation the merged document is held
	// to (feature 028), using its parser rather than a second regex.
	total := DeriveTotalExperienceYears(master)
	for _, a := range yearsAssertions {
		if a.number != total {
			violations = append(violations, fmt.Sprintf("summary asserts %q but master's experience spans %d years", a.text, total))
		}
	}

	return violations
}

// summarySkillCandidates picks the terms in a prose summary that assert a
// technology or skill. Ordinary English is lower-case mid-sentence and carries
// none of the shapes below, so it is never a candidate — the guard against
// flagging "delivered", "platform" or a capitalised first word.
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
	// A numeric token is an assertion, not a skill: "8+" is the years figure,
	// which check 3 owns. Without this, every summary that opens with the
	// years figure the prompt *requires* is flagged as an ungrounded skill.
	if strings.IndexFunc(word, unicode.IsDigit) >= 0 && !hasLetter(word) {
		return false
	}
	if strings.ContainsAny(word, "+#") {
		return true // C++, C#, F#
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
		return true // AWS, SQL, CI, GCP
	}
	if i := strings.Index(word, "."); i > 0 && i < len(word)-1 {
		return true // Node.js, Vue.js
	}
	return !sentenceStart && unicode.IsUpper([]rune(word)[0])
}

// summaryGroundedVocabulary is every word the master profile and the brief's
// selected material already contain. The vacancy analysis is deliberately left
// out: a skill the vacancy asks for but the master never demonstrates is
// precisely the fabrication this check exists to catch.
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

// numberRe matches a number with its optional currency prefix and unit suffix:
// "40%", "$2M", "3x", "12".
var numberRe = regexp.MustCompile(`(?i)(\$?)(\d[\d,]*(?:\.\d+)?)\s?(%|[xkmb]\b)?`)

// metricCues are the words that turn a bare number into an achievement claim
// ("a team of 12", "cut build time by half"). Without a cue or a unit a number
// is a version or an incidental count, not a metric.
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

// numberLiterals is every number in a string, normalised to its digits. Used
// on the brief's highlights, where any number present is support enough.
func numberLiterals(s string) []string {
	var out []string
	for _, m := range numberRe.FindAllStringSubmatch(s, -1) {
		out = append(out, strings.ReplaceAll(m[2], ",", ""))
	}
	return out
}

// metricClaims is the subset of numbers in a string that assert an achievement
// metric — those carrying a unit or currency marker, or preceded by a cue word.
func metricClaims(s string) []metricClaim {
	var out []metricClaim
	for _, m := range numberRe.FindAllStringSubmatchIndex(s, -1) {
		start, end := m[0], m[1]
		if start > 0 {
			prev := rune(s[start-1])
			if unicode.IsLetter(prev) || unicode.IsNumber(prev) {
				continue // part of a token like "S3" or "p95"
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

// wordTokens is the set of normalised words across a project's bullets, used
// as the strict-grounding pool for that project's highlights. Words shorter
// than four characters are ignored: connectives ("and", "the", "for") carry no
// grounding signal and would only produce noise.
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

// StripSummaryViolations removes the sentences carrying ungrounded claims from
// a summary that failed verification twice. Stripping keeps the run alive with
// a shorter but honest summary — the same "strip and log" contract the
// years-assertion check uses, rather than failing the whole document or
// shipping a fabricated claim (035 FR-009).
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

// quotedTerm pulls the offending term out of a violation string, which this
// package formats with the term in double quotes.
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
