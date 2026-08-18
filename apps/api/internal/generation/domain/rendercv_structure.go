package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type StructureKind string

const (
	StructureTotalExperienceYears StructureKind = "total_experience_years"

	StructureHighlightDrift StructureKind = "highlight_drift"
)

type StructureViolation struct {
	Kind    StructureKind
	Path    string
	Message string
}

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

func DeriveTotalExperienceYears(master RendercvMaster) int {
	return DeriveTotalExperienceYearsAsOf(master, time.Now().Year())
}

func DeriveTotalExperienceYearsAsOf(master RendercvMaster, now int) int {
	sections := CvSections(master)
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

var yearsAssertionRe = regexp.MustCompile(`(?i)\b((?:over|more than|almost|nearly|about|above|just over)\s+)?(\d{1,3})\+?\s+years?\b(\s+of\s+(?:professional\s+|relevant\s+)?experience)?`)

type yearsAssertion struct {
	start, end int
	number     int
	text       string
}

func findYearsAssertions(s string) []yearsAssertion {
	idxs := yearsAssertionRe.FindAllStringSubmatchIndex(s, -1)
	out := make([]yearsAssertion, 0, len(idxs))
	for _, m := range idxs {
		qualStart, ofExpStart := m[2], m[6]
		if qualStart < 0 && ofExpStart < 0 {
			continue
		}
		n, err := strconv.Atoi(s[m[4]:m[5]])
		if err != nil {
			continue
		}
		out = append(out, yearsAssertion{start: m[0], end: m[1], number: n, text: s[m[0]:m[1]]})
	}
	return out
}

func VerifyStructureIntegrity(master, merged RendercvMaster) []StructureViolation {
	var violations []StructureViolation
	total := DeriveTotalExperienceYears(master)
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

var yearsClausePrefixRe = regexp.MustCompile(`(?i)(?:and |with |for |of |including |having )$`)

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
			continue
		}
		b.WriteString(s[prev:sp.start])
		prev = sp.end
	}
	b.WriteString(s[prev:])
	return strings.TrimSpace(b.String())
}

func VerifyHighlightGrounding(master, merged RendercvMaster) []StructureViolation {
	var violations []StructureViolation
	masterSections := CvSections(master)
	mergedSections := CvSections(merged)
	if masterSections == nil || mergedSections == nil {
		return violations
	}
	masterCompanyHighlights := map[string][]string{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanyHighlights[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	for i, e := range AsSliceOfMaps(mergedSections["experience"]) {
		company := StringField(e, "company")
		name := company
		if name == "" {
			name = strconv.Itoa(i)
		}
		masterBullets := masterCompanyHighlights[norm(company)]
		for j, h := range StringSliceField(e, "highlights") {
			if !lcsCovered(h, masterBullets) {
				violations = append(violations, StructureViolation{
					Kind:    StructureHighlightDrift,
					Path:    fmt.Sprintf("cv.sections.experience[%s].highlights[%d]", name, j),
					Message: fmt.Sprintf("highlight not grounded in master: %q", truncateStr(h, 60)),
				})
			}
		}
	}
	return violations
}

func StripUngroundedHighlights(master, merged RendercvMaster) RendercvMaster {
	masterSections := CvSections(master)
	mergedSections := CvSections(merged)
	if mergedSections == nil {
		return merged
	}
	masterCompanyHighlights := map[string][]string{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanyHighlights[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	for _, e := range AsSliceOfMaps(mergedSections["experience"]) {
		company := norm(StringField(e, "company"))
		masterBullets := masterCompanyHighlights[company]
		if len(masterBullets) == 0 {
			continue
		}
		highlights := StringSliceField(e, "highlights")
		changed := false
		for i, h := range highlights {
			if lcsCovered(h, masterBullets) {
				continue
			}
			replacement := bestOverlapBullet(h, masterBullets)
			if replacement != "" {
				highlights[i] = replacement
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

func VerifyHighlightProvenance(master, merged RendercvMaster) []string {
	masterSections := CvSections(master)
	mergedSections := CvSections(merged)
	if mergedSections == nil {
		return nil
	}
	masterCompanyHighlights := map[string][]string{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanyHighlights[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}

	var violations []string
	for _, e := range AsSliceOfMaps(mergedSections["experience"]) {
		company := StringField(e, "company")
		masterBullets := masterCompanyHighlights[norm(company)]
		if len(masterBullets) == 0 {
			continue
		}
		claimedBy := map[string]string{}
		for _, h := range StringSliceField(e, "highlights") {
			source := bestOverlapBullet(h, masterBullets)
			if source == "" {
				continue
			}
			if first, ok := claimedBy[source]; ok {
				violations = append(violations, fmt.Sprintf(
					"experience %q: two highlights derive from the same master bullet %q: %q and %q",
					company, truncateStr(source, 60), truncateStr(first, 60), truncateStr(h, 60)))
				continue
			}
			claimedBy[source] = h
		}
	}
	return violations
}

func bestOverlapBullet(proposed string, masterBullets []string) string {
	proposedWords := wordSet(proposed)
	best := ""
	bestOverlap := -1
	for _, b := range masterBullets {
		masterWords := wordSet(b)
		overlap := 0
		for w := range proposedWords {
			if masterWords[w] {
				overlap++
			}
		}
		if overlap > bestOverlap {
			bestOverlap = overlap
			best = b
		}
	}
	return best
}
