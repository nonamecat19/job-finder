package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/job-finder/api/internal/keyword"
)

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

var seniorityRe = regexp.MustCompile(`(?i)\b(junior|associate|entry|mid|senior|lead|staff|principal|architect|distinguished)\b`)

var dateRangeRe = regexp.MustCompile(`(\d{4})\s*(?:[–\-]|to)\s*(\d{4}|present|now|current)`)

var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*%?`)
var sentenceSplitRe = regexp.MustCompile(`[.!?]\s+`)
var tokenRe = regexp.MustCompile(`\b[\w']+\b`)

func VerifyRephraseGrounding(sourceBullet string, allowedProper, sourceNums map[string]bool, rephrase string, sourceSeniority string, sourceDateRange string) []string {
	if strings.TrimSpace(rephrase) == "" {
		return []string{"empty rephrase"}
	}
	var violations []string

	for _, p := range PropernounsInText(rephrase) {
		if !allowedProper[LowerASCII(p)] {
			violations = append(violations, fmt.Sprintf("proper noun / technology %q not in source bullet", p))
		}
	}

	for _, n := range numberRe.FindAllString(rephrase, -1) {
		if !sourceNums[NormNumber(n)] {
			violations = append(violations, fmt.Sprintf("metric/number %q not in source bullet", n))
		}
	}

	if sourceSeniority != "" {
		rephraseSeniority := ExtractSeniority(rephrase)
		if rephraseSeniority != "" && seniorityLevel[rephraseSeniority] > seniorityLevel[sourceSeniority] {
			violations = append(violations, fmt.Sprintf("seniority inflated: source is %q, rephrase claims %q", sourceSeniority, rephraseSeniority))
		}
	}

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
	if r[0] < 'A' || r[0] > 'Z' {
		return false
	}
	for _, c := range r[1:] {
		if c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

func PropernounSet(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		for _, w := range tokenRe.FindAllString(b, -1) {
			set[LowerASCII(w)] = true
		}
	}
	return set
}

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

func ExtractSeniority(text string) string {
	m := seniorityRe.FindString(text)
	if m == "" {
		return ""
	}
	return strings.ToLower(m)
}

func ExtractDateRange(text string) string {
	return dateRangeRe.FindString(text)
}

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
		return 0
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

func Stem(s string) string {
	s = strings.TrimSuffix(s, "ing")
	s = strings.TrimSuffix(s, "ed")
	s = strings.TrimSuffix(s, "s")
	return s
}

var TokenRe = tokenRe
