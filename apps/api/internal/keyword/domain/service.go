package domain

import (
	"fmt"
	"regexp"
	"strings"
)

type RegexExtractor struct{}

func NewExtractor() *RegexExtractor {
	return &RegexExtractor{}
}

var _ Extractor = (*RegexExtractor)(nil)

var (
	requiredHeaders = []string{
		"requirements", "qualifications", "what you'll need", "what you need",
		"minimum qualifications", "what we're looking for", "about you",
		"who you are", "responsibilities", "what you'll do", "the role",
		"role description", "what you will do",
	}
	preferredHeaders = []string{
		"nice to have", "bonus", "preferred", "preferred qualifications",
		"bonus points", "nice-to-have", "desired", "ideal but not required",
	}
)

var headerRe = regexp.MustCompile(`(?m)^(?:\s*(?:#{1,3}\s*|\*\*)?)([A-Z][A-Za-z0-9 /&+'.()-]{2,60}?)(?:\s*\*\*)?\s*[:\s]*$`)

var bulletRe = regexp.MustCompile(`(?m)^\s*(?:[-*•]|\d+[.)])\s+(.*)$`)

var stripPunctRe = regexp.MustCompile(`[^\w\s/-]`)

var tokenRe = regexp.MustCompile(`[A-Za-z0-9]+(?:[./-][A-Za-z0-9]+)*`)

func (e *RegexExtractor) Extract(jd string) (*ExtractResult, error) {
	if strings.TrimSpace(jd) == "" {
		return nil, fmt.Errorf("keyword: empty job description")
	}
	sections := splitSections(jd)
	seen := make(map[string]ExtractedTerm)
	for _, sec := range sections {
		for _, hit := range extractPhrases(sec.body) {
			term := normalizeTerm(hit.phrase)
			if term.Term == "" {
				continue
			}
			term.Polarity = sec.polarity
			term.Section = sec.header
			term.Evidence = hit.evidence
			if prev, ok := seen[term.Canonical]; ok {
				if prev.Polarity != PolarityRequired && term.Polarity == PolarityRequired {
					prev.Polarity = PolarityRequired
					seen[term.Canonical] = prev
				}
				continue
			}
			seen[term.Canonical] = term
		}
	}
	terms := make([]ExtractedTerm, 0, len(seen))
	for _, t := range seen {
		terms = append(terms, t)
	}
	return &ExtractResult{Terms: terms}, nil
}

type section struct {
	header   string
	body     string
	polarity Polarity
}

func splitSections(jd string) []section {
	locs := headerRe.FindAllStringIndex(jd, -1)
	if len(locs) == 0 {
		return []section{{header: "", body: jd, polarity: PolarityRequired}}
	}
	var out []section
	prev := 0
	for i, loc := range locs {
		body := strings.TrimSpace(jd[prev:loc[0]])
		headerText := strings.TrimSpace(headerRe.FindString(jd[loc[0]:loc[1]]))
		headerText = strings.Trim(headerText, "*#: ")
		if i > 0 && body != "" {
			out = append(out, section{
				header:   out[len(out)-1].header,
				body:     body,
				polarity: out[len(out)-1].polarity,
			})
		}
		polarity := classifyHeader(headerText)
		out = append(out, section{header: headerText, body: "", polarity: polarity})
		prev = loc[1]
		if i == len(locs)-1 {
			tail := strings.TrimSpace(jd[prev:])
			if tail != "" {
				out = append(out, section{header: headerText, body: tail, polarity: polarity})
			}
		}
	}
	if len(out) == 0 {
		return []section{{header: "", body: jd, polarity: PolarityRequired}}
	}
	return out
}

func classifyHeader(h string) Polarity {
	hl := lowerASCII(h)
	for _, p := range preferredHeaders {
		if strings.Contains(hl, p) {
			return PolarityPreferred
		}
	}
	for _, r := range requiredHeaders {
		if strings.Contains(hl, r) {
			return PolarityRequired
		}
	}
	return PolarityRequired
}

type phraseHit struct {
	phrase   string
	evidence string
}

func extractPhrases(body string) []phraseHit {
	var candidates []string
	for _, m := range bulletRe.FindAllStringSubmatch(body, -1) {
		candidates = append(candidates, m[1])
	}
	if len(candidates) == 0 {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				candidates = append(candidates, line)
			}
		}
	}

	var hits []phraseHit
	for _, c := range candidates {
		evidence := collapseSpace(strings.TrimSpace(c))
		for _, p := range mergeMultiWord(c) {
			hits = append(hits, phraseHit{phrase: p, evidence: evidence})
		}
	}
	return hits
}

func mergeMultiWord(candidate string) []string {
	clean := stripPunctRe.ReplaceAllString(candidate, " ")
	clean = collapseSpace(clean)
	tokens := tokenRe.FindAllString(clean, -1)
	if len(tokens) == 0 {
		return nil
	}
	tokens = filterStopwords(tokens)

	var out []string
	i := 0
	for i < len(tokens) {
		best := 1
		maxSpan := 4
		if maxSpan > len(tokens)-i {
			maxSpan = len(tokens) - i
		}
		for span := maxSpan; span > 1; span-- {
			if i+span > len(tokens) {
				continue
			}
			phrase := strings.Join(tokens[i:i+span], " ")
			if _, ok := synonymMap[titleCaseWords(strings.ToLower(phrase))]; ok {
				best = span
				break
			}
			if _, ok := canonicalByAlias[lowerASCII(phrase)]; ok {
				best = span
				break
			}
		}
		phrase := strings.Join(tokens[i:i+best], " ")
		i += best
		if best == 1 && isNoise(phrase) {
			continue
		}
		out = append(out, phrase)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeTerm(raw string) ExtractedTerm {
	clean := stripPunctRe.ReplaceAllString(raw, " ")
	clean = collapseSpace(strings.TrimSpace(clean))
	if clean == "" {
		return ExtractedTerm{}
	}
	canonical := resolveAlias(clean)
	stemmed := stem(canonical)
	return ExtractedTerm{
		Term:      clean,
		Canonical: canonical,
		Stemmed:   stemmed,
	}
}
