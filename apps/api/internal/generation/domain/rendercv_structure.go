package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StructureKind identifies a class of structural-integrity violation
// (feature 028). Block-sequence and experience-order invariants are enforced
// deterministically by MergeTailored and never produce violations; the only
// kind emitted today is the text-asserted years contradiction.
type StructureKind string

const (
	// StructureTotalExperienceYears flags generated text (summary or a bullet)
	// that asserts a numeric total years-of-experience figure contradicting
	// the master's derivable total.
	StructureTotalExperienceYears StructureKind = "total_experience_years"
)

// StructureViolation is a single structural-integrity violation returned by
// VerifyStructureIntegrity. It is not persisted; the tailoring loop consumes
// it to drive the targeted re-prompt and strip-and-log fallback.
type StructureViolation struct {
	Kind    StructureKind
	Path    string
	Message string
}

// yearRe extracts a 4-digit year from a free-text date string like "2020-01",
// "Jan 2020", "2018-06" or "Present". Returns ok=false when no year is found.
var yearRe = regexp.MustCompile(`(?:^|\D)(\d{4})(?:\D|$)`)

func parseExperienceYear(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	m := yearRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	y, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return y, true
}

// deriveTotalExperienceYears computes the master's total years of experience
// the way any reader would: the sum of (endYear - startYear) per experience
// entry, clamped to >= 0, with an empty or "Present" end date treated as the
// current year. Entries with no parseable dates contribute 0 — the derivation
// is conservative and only ever flags a contradiction, never an absence.
func deriveTotalExperienceYears(master RendercvMaster) int {
	sections := CvSections(master)
	now := time.Now().Year()
	total := 0
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		startYear, ok := parseExperienceYear(StringField(e, "start_date"))
		if !ok {
			continue
		}
		endStr := StringField(e, "end_date")
		endYear, ok := parseExperienceYear(endStr)
		if !ok || strings.EqualFold(strings.TrimSpace(endStr), "present") {
			endYear = now
		}
		span := endYear - startYear
		if span < 0 {
			span = 0
		}
		total += span
	}
	return total
}

// yearsAssertionRe matches numeric years-of-experience assertions in free
// text. Group 1 is an optional qualifier ("over", "more than", ...), group 2
// the asserted number, group 3 an optional "of experience" suffix. A match is
// only treated as an assertion when either the qualifier or the "of
// experience" suffix is present — a bare "12 years" is too ambiguous to flag.
var yearsAssertionRe = regexp.MustCompile(`(?i)\b((?:over|more than|almost|nearly|about|above|just over)\s+)?(\d{1,3})\+?\s+years?\b(\s+of\s+(?:professional\s+|relevant\s+)?experience)?`)

// yearsAssertion is one located numeric years claim within a text field.
type yearsAssertion struct {
	start, end int
	number     int
	text       string
}

// findYearsAssertions locates every numeric years-of-experience assertion in
// s, returning its byte span, parsed number and matched text.
func findYearsAssertions(s string) []yearsAssertion {
	idxs := yearsAssertionRe.FindAllStringSubmatchIndex(s, -1)
	out := make([]yearsAssertion, 0, len(idxs))
	for _, m := range idxs {
		qualStart, ofExpStart := m[2], m[6]
		if qualStart < 0 && ofExpStart < 0 {
			continue // bare "N years" with no experience context
		}
		n, err := strconv.Atoi(s[m[4]:m[5]])
		if err != nil {
			continue
		}
		out = append(out, yearsAssertion{start: m[0], end: m[1], number: n, text: s[m[0]:m[1]]})
	}
	return out
}

// VerifyStructureIntegrity checks the merged resume's structural integrity
// against the master (feature 028 invariant 3b): it regex-scans
// cv.sections.summary[0] and each cv.sections.experience[].highlights[] for
// numeric years-of-experience assertions and flags any figure that
// contradicts the master's derivable total. Invariants 1 and 2 (block
// sequence, experience order) are enforced by MergeTailored and can never
// violate here.
func VerifyStructureIntegrity(master, merged RendercvMaster) []StructureViolation {
	var violations []StructureViolation
	total := deriveTotalExperienceYears(master)
	sections := CvSections(merged)
	if sections == nil {
		return violations
	}

	if summary := StringSliceField(sections, "summary"); len(summary) > 0 {
		for _, a := range findYearsAssertions(summary[0]) {
			if a.number != total {
				violations = append(violations, StructureViolation{
					Kind:    StructureTotalExperienceYears,
					Path:    "cv.sections.summary[0]",
					Message: fmt.Sprintf("summary asserts %q but master's experience spans %d years", a.text, total),
				})
			}
		}
	}

	for i, e := range AsSliceOfMaps(sections["experience"]) {
		name := StringField(e, "company")
		if name == "" {
			name = strconv.Itoa(i)
		}
		for j, h := range StringSliceField(e, "highlights") {
			for _, a := range findYearsAssertions(h) {
				if a.number != total {
					violations = append(violations, StructureViolation{
						Kind:    StructureTotalExperienceYears,
						Path:    fmt.Sprintf("cv.sections.experience[%s].highlights[%d]", name, j),
						Message: fmt.Sprintf("highlight asserts %q but master's experience spans %d years", a.text, total),
					})
				}
			}
		}
	}
	return violations
}

// StripStructureViolations is the strip-and-flag fallback for the
// text-asserted-years invariant: after a targeted re-prompt fails, it removes
// every offending numeric years claim from the merged resume's summary and
// experience highlights so the emitted resume never carries a contradicting
// figure. The caller logs the intervention on the activity row.
func StripStructureViolations(merged RendercvMaster, violations []StructureViolation) RendercvMaster {
	if len(violations) == 0 {
		return merged
	}
	sections := CvSections(merged)
	if sections == nil {
		return merged
	}

	if summary := StringSliceField(sections, "summary"); len(summary) > 0 {
		if cleaned := stripYearsAssertions(summary[0]); cleaned != summary[0] {
			sections["summary"] = []any{cleaned}
		}
	}

	for _, e := range AsSliceOfMaps(sections["experience"]) {
		highlights := StringSliceField(e, "highlights")
		changed := false
		for i, h := range highlights {
			if cleaned := stripYearsAssertions(h); cleaned != h {
				highlights[i] = cleaned
				changed = true
			}
		}
		if changed {
			anyHighlights := make([]any, 0, len(highlights))
			for _, h := range highlights {
				anyHighlights = append(anyHighlights, h)
			}
			e["highlights"] = anyHighlights
		}
	}
	return merged
}

// yearsClausePrefixRe matches a connector immediately before a years claim
// ("with ", "for ", "and "), so stripping the claim leaves grammatical text.
var yearsClausePrefixRe = regexp.MustCompile(`(?i)(?:and |with |for |of |including |having )$`)

// stripYearsAssertions removes every numeric years-of-experience claim from a
// single text field, including adjacent connectors and trailing punctuation.
func stripYearsAssertions(s string) string {
	matches := findYearsAssertions(s)
	if len(matches) == 0 {
		return s
	}
	type span struct{ start, end int }
	var spans []span
	for _, a := range matches {
		start, end := a.start, a.end
		if loc := yearsClausePrefixRe.FindStringIndex(s[:start]); loc != nil {
			start = loc[0]
		}
		for end < len(s) && strings.ContainsRune(" .,;!?)\t", rune(s[end])) {
			end++
		}
		tail := strings.ToLower(s[end:])
		for _, w := range []string{" and ", " with ", " also ", " including "} {
			if strings.HasPrefix(tail, w) {
				end += len(w)
				break
			}
		}
		if end > start {
			spans = append(spans, span{start, end})
		}
	}
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, sp := range spans {
		if sp.start < prev {
			continue // overlapping matches — keep the first cut
		}
		b.WriteString(s[prev:sp.start])
		prev = sp.end
	}
	b.WriteString(s[prev:])
	return strings.TrimSpace(b.String())
}
