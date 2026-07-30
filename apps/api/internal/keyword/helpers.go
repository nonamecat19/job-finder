package keyword

import "strings"

// lowerASCII lowercases ASCII letters only. We avoid strings.ToLower because it
// would case-fold non-ASCII scripts in JD text we don't want to mutate
// (e.g. Cyrillic employer names); alias matching is intentionally ASCII-only.
func lowerASCII(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// titleCaseWords upper-cases the first ASCII letter of each space-separated
// word, leaving the rest untouched. This exists in place of the deprecated
// strings.Title (removed guidance: golang.org/x/text/cases) because we only
// ever call it on already-lowercased, space-joined ASCII synonym-map keys —
// the Unicode word-boundary subtleties strings.Title mishandles don't apply
// here, so pulling in x/text/cases would be one dependency for zero benefit.
func titleCaseWords(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		b := []byte(w)
		if b[0] >= 'a' && b[0] <= 'z' {
			b[0] -= 'a' - 'A'
		}
		words[i] = string(b)
	}
	return strings.Join(words, " ")
}

// collapseSpace turns runs of whitespace into a single space.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// stopwords are dropped from candidate phrases so "experience with Kubernetes"
// yields "Kubernetes", not the filler words. These are connector words, not
// JD skills, so removing them cannot change polarity classification.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true,
	"to": true, "with": true, "in": true, "on": true, "for": true, "is": true,
	"are": true, "be": true, "you": true, "your": true, "our": true, "we": true,
	"experience": true, "knowledge": true, "understanding": true, "ability": true,
	"strong": true, "good": true, "excellent": true, "solid": true, "familiarity": true,
	"background": true, "skills": true, "skill": true, "using": true, "use": true,
	"working": true, "work": true, "well": true, "as": true, "at": true, "least": true,
	"years": true, "year": true, "plus": true, "bonus": true, "points": true,
	"preferred": true, "required": true, "nice": true, "have": true, "has": true,
}

func filterStopwords(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t == "" || stopwords[lowerASCII(t)] {
			continue
		}
		out = append(out, t)
	}
	return out
}

// noiseWords are common English verbs/nouns that appear in responsibility
// prose ("build", "deploy", "manage") but are not themselves JD skills. The
// extractor is recall-biased by design, so this is a small, deliberately
// conservative denylist — we only drop words that would never be a meaningful
// required/preferred term in a keyword diff.
var noiseWords = map[string]bool{
	"build": true, "deploy": true, "develop": true, "create": true, "manage": true,
	"design": true, "implement": true, "maintain": true, "support": true, "lead": true,
	"work": true, "working": true, "collaborate": true, "communicate": true, "write": true,
	"features": true, "feature": true, "services": true, "service": true, "systems": true,
	"system": true, "team": true, "teams": true, "product": true, "products": true,
	"code": true, "problems": true, "problem": true, "solutions": true, "solution": true,
	"projects": true, "project": true, "customers": true, "customer": true, "users": true,
	"user": true, "business": true, "data": true, "ensure": true, "help": true,
	"provide": true, "drive": true, "own": true, "deliver": true, "improve": true,
}

// isNoise reports whether a single lowercase token is non-skill prose.
func isNoise(t string) bool {
	return noiseWords[lowerASCII(t)]
}

// stem is a small, deterministic English stemmer. We deliberately avoid pulling
// in a full Porter2 dependency for this task; the rules below cover the
// inflectional endings that matter for JD/resume term matching (spec §2.1):
// plurals and common suffix variation. It is conservative — it only strips a
// suffix when the remainder is still a plausible word — so "docker" never
// collapses to "dock".
func stem(s string) string {
	w := lowerASCII(strings.TrimSpace(s))
	if w == "" {
		return w
	}
	// Preserve known acronyms / casing-sensitive canonicals verbatim.
	if _, ok := canonicalByAlias[w]; ok && len(w) <= 5 {
		return w
	}
	rules := []struct {
		suffix string
		minLen int
		repl   string
	}{
		{"ization", 7, "ize"},
		{"ational", 8, "ate"},
		{"fulness", 8, "ful"},
		{"ousness", 8, "ous"},
		{"tional", 7, "tion"},
		{"iveness", 8, "ive"},
		{"ologies", 9, "ology"},
		{"ically", 7, "ic"},
		{"ation", 6, "ate"},
		{"ement", 6, ""},
		{"ment", 5, ""},
		{"ence", 6, ""},
		{"ance", 5, ""},
		{"able", 5, ""},
		{"ible", 5, ""},
		{"ing", 4, ""},
		{"ies", 4, "y"},
		{"ive", 4, ""},
		{"ize", 4, ""},
		{"ise", 4, ""},
		{"ed", 3, ""},
		{"ly", 3, ""},
		{"s", 2, ""},
	}
	for _, r := range rules {
		if len(w) > r.minLen && strings.HasSuffix(w, r.suffix) {
			trimmed := w[:len(w)-len(r.suffix)] + r.repl
			if len(trimmed) >= 2 {
				return trimmed
			}
		}
	}
	return w
}
