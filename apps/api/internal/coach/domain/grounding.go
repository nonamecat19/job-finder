package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/job-finder/api/internal/keyword"
)

// seniorityLevel maps job-title seniority prefixes to a numeric rank.
// Higher rank = more senior. Used to detect seniority inflation.
var seniorityLevel = map[string]int{
	"junior":        0,
	"associate":     0,
	"entry":         0,
	"mid":           1,
	"senior":        2,
	"lead":          3,
	"staff":         3,
	"principal":     4,
	"architect":     4,
	"distinguished": 4,
}

// seniorityRe matches a seniority prefix at the start of a job title.
var seniorityRe = regexp.MustCompile(`(?i)\b(junior|associate|entry|mid|senior|lead|staff|principal|architect|distinguished)\b`)

// dateRangeRe matches date ranges like "2022–2024", "2020-2023", "Jan 2020–Present".
var dateRangeRe = regexp.MustCompile(`(\d{4})\s*(?:[–\-]|to)\s*(\d{4}|present|now|current)`)

var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*%?`)
var sentenceSplitRe = regexp.MustCompile(`[.!?]\s+`)
var tokenRe = regexp.MustCompile(`\b[\w']+\b`)

// VerifyRephraseGrounding checks that every proper noun, number, seniority
// claim, and duration claim in the rephrase is traceable to the source.
// Returns violations; empty slice means grounded.
func VerifyRephraseGrounding(sourceBullet string, allowedProper, sourceNums map[string]bool, rephrase string, sourceSeniority string, sourceDateRange string) []string {
	if strings.TrimSpace(rephrase) == "" {
		return []string{"empty rephrase"}
	}
	var violations []string

	// Check proper nouns (technologies, employers, product names)
	for _, p := range PropernounsInText(rephrase) {
		if !allowedProper[LowerASCII(p)] {
			violations = append(violations, fmt.Sprintf("proper noun / technology %q not in source bullet", p))
		}
	}

	// Check metrics/numbers
	for _, n := range numberRe.FindAllString(rephrase, -1) {
		if !sourceNums[NormNumber(n)] {
			violations = append(violations, fmt.Sprintf("metric/number %q not in source bullet", n))
		}
	}

	// Check seniority inflation: if the source has a seniority level, the
	// rephrase must not claim a higher level.
	if sourceSeniority != "" {
		rephraseSeniority := ExtractSeniority(rephrase)
		if rephraseSeniority != "" && seniorityLevel[rephraseSeniority] > seniorityLevel[sourceSeniority] {
			violations = append(violations, fmt.Sprintf("seniority inflated: source is %q, rephrase claims %q", sourceSeniority, rephraseSeniority))
		}
	}

	// Check duration inflation: if the source label has a date range, the
	// rephrase must not claim a longer duration.
	if sourceDateRange != "" {
		rephraseDuration := ExtractDateRange(rephrase)
		if rephraseDuration != "" {
			srcYears := ParseDurationYears(sourceDateRange)
			rpYears := ParseDurationYears(rephraseDuration)
			if rpYears > srcYears {
				violations = append(violations, fmt.Sprintf("duration inflated: source is %q (%d years), rephrase claims %q (%d years)", sourceDateRange, srcYears, rephraseDuration, rpYears))
			}
		}
	}

	return violations
}

func splitSentences(s string) []string { return sentenceSplitRe.Split(s, -1) }

// PropernounsInText returns the capitalized, non-sentence-initial tokens of s.
func PropernounsInText(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, sentence := range splitSentences(s) {
		tokens := tokenRe.FindAllString(sentence, -1)
		for i, tok := range tokens {
			if !looksProper(tok) {
				continue
			}
			if i == 0 && isPlainCapitalized(tok) {
				continue
			}
			key := LowerASCII(tok)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tok)
		}
	}
	return out
}

func looksProper(tok string) bool {
	for _, r := range tok {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func isPlainCapitalized(tok string) bool {
	if tok == "" {
		return false
	}
	r := []rune(tok)
	if !(r[0] >= 'A' && r[0] <= 'Z') {
		return false
	}
	for _, c := range r[1:] {
		if c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

// PropernounSet collects every word token across bullets, indexed lowercased.
func PropernounSet(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		for _, w := range tokenRe.FindAllString(b, -1) {
			set[LowerASCII(w)] = true
		}
	}
	return set
}

// NumberSet returns the normalized numeric tokens present in text.
func NumberSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, n := range numberRe.FindAllString(text, -1) {
		set[NormNumber(n)] = true
	}
	return set
}

func NormNumber(n string) string {
	n = strings.TrimSuffix(n, "%")
	return strings.ReplaceAll(n, ",", "")
}

// ExtractSeniority returns the first seniority-level token found in text,
// lowercased, or "" if none.
func ExtractSeniority(text string) string {
	m := seniorityRe.FindString(text)
	if m == "" {
		return ""
	}
	return strings.ToLower(m)
}

// ExtractDateRange returns the first matched date range from text (e.g.
// "2022–2024") or "" if none.
func ExtractDateRange(text string) string {
	return dateRangeRe.FindString(text)
}

// ParseDurationYears extracts the number of whole years from a date range
// string like "2022–2024" (returns 2) or "2020–Present" (returns 0 for
// unknown end). Returns 0 on parse failure.
func ParseDurationYears(dr string) int {
	m := dateRangeRe.FindStringSubmatch(dr)
	if len(m) < 3 {
		return 0
	}
	start := parseInt(m[1])
	if start == 0 {
		return 0
	}
	endStr := strings.ToLower(m[2])
	if endStr == "present" || endStr == "now" || endStr == "current" {
		return 0 // Can't verify open-ended ranges
	}
	end := parseInt(endStr)
	if end == 0 {
		return 0
	}
	if end > start {
		return end - start
	}
	return 0
}

func parseInt(s string) int {
	var n int
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n = n*10 + int(r-'0')
		} else {
			return 0
		}
	}
	return n
}

func LowerASCII(s string) string { return strings.ToLower(s) }

func ProximityRank(p keyword.Proximity) int {
	switch p {
	case keyword.ProximityClose:
		return 0
	case keyword.ProximityModerate:
		return 1
	case keyword.ProximityDistant:
		return 2
	default:
		return 3
	}
}

// Stem is a minimal stemmer (mirrors keyword.stem).
func Stem(s string) string {
	s = strings.TrimSuffix(s, "ing")
	s = strings.TrimSuffix(s, "ed")
	s = strings.TrimSuffix(s, "s")
	return s
}

// TokenRe is the word tokenizer regex used to scan bullet text for adjacent
// term matches.
var TokenRe = tokenRe
