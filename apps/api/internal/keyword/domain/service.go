package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// RegexExtractor is the deterministic, dependency-free implementation of the
// Extractor port. It follows the spec 008-1 §1.4 algorithm: split the JD into
// sections by headers, classify each section's default polarity, extract
// skill-like tokens, normalize them, and tag each with polarity + section.
type RegexExtractor struct{}

// NewExtractor returns the production extractor. (NewService-style constructor
// kept for symmetry with sibling packages and future dependency injection.)
func NewExtractor() *RegexExtractor {
	return &RegexExtractor{}
}

// Compile-time check that RegexExtractor satisfies the port.
var _ Extractor = (*RegexExtractor)(nil)

// Section headers and the polarity they imply by default (spec 008-1 §1.3).
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

// Inline polarity markers (spec 008-1 §1.1 / §1.2).
var (
	requiredMarkers = []string{
		"must have", "required", "essential", "minimum of", "requires",
		"prerequisite", "necessary", "need to", "mandatory", "you will",
		"you are responsible for", "you'll", "the ideal candidate will have",
		"at least", "years of experience", "years experience", "years of",
	}
	preferredMarkers = []string{
		"nice to have", "preferred", "a plus", "bonus", "desired",
		"ideal but not required", "would be great", "familiarity with",
		"knowledge of", "experience with", "plus if",
	}
)

// headerRe matches a section header: a line that is short, possibly bolded
// (markdown **...** or leading #), and ends before a blank line.
var headerRe = regexp.MustCompile(`(?m)^(?:\s*(?:#{1,3}\s*|\*\*)?)([A-Z][A-Za-z0-9 /&+'.()-]{2,60}?)(?:\s*\*\*)?\s*[:\s]*$`)

// bulletRe matches list items ("- ", "* ", "• ", "1. ").
var bulletRe = regexp.MustCompile(`(?m)^\s*(?:[-*•]|\d+[.)])\s+(.*)$`)

// stripPunctRe removes punctuation except hyphens inside known compounds.
var stripPunctRe = regexp.MustCompile(`[^\w\s/-]`)

// tokenRe splits a phrase into word tokens, keeping hyphenated compounds.
var tokenRe = regexp.MustCompile(`[A-Za-z0-9]+(?:[./-][A-Za-z0-9]+)*`)

// Extract implements Extractor.
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
			// Dedup by canonical term: a skill required somewhere in the JD
			// outranks a preferred-only mention, so required wins on conflict
			// (the diff should not hide a hard requirement behind a "nice to
			// have" repeat). First sighting keeps its section for traceability.
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

// section is a slice of the JD with its inferred default polarity.
type section struct {
	header   string
	body     string
	polarity Polarity
}

// splitSections partitions the JD by headers and assigns each section a default
// polarity from its header (spec 008-1 §1.3). Text before the first header
// defaults to required.
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

// classifyHeader returns the default polarity implied by a header string.
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

// phraseHit is a single extracted phrase paired with the raw candidate
// line/bullet it came from, so downstream steps (the must-have classifier)
// can inspect the surrounding phrasing.
type phraseHit struct {
	phrase   string
	evidence string
}

// extractPhrases pulls skill-like phrases out of a section body. It collects
// bullet items and falls back to sentence fragments, then merges adjacent
// single tokens into known multi-word terms (synonym map keys) so
// "machine learning" is not split into "machine" + "learning" (spec §1.4.4).
// Each phrase keeps a reference to the candidate line it originated from.
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

// mergeMultiWord splits a candidate into word tokens, then greedily folds
// consecutive tokens into a known canonical multi-word term when one exists.
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
			if _, ok := synonymMap[strings.Title(strings.ToLower(phrase))]; ok {
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
		// Drop single-token non-skill prose so the diff isn't polluted with
		// generic verbs; keep multi-word phrases (they may be real skills).
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

// normalizeTerm runs a single term through the normalization sequence
// (spec 008-1 §2.4): lowercase, strip punctuation, acronym/synonym expansion
// to canonical, then stem.
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
		// Polarity is assigned by the caller (depends on section context).
	}
}
