package keyword

// Package keyword — truthful rephrase suggester (spec 008-1 §5, task 008-5).
//
// For each missing-required term where the profile holds *adjacent* evidence,
// the suggester asks the model to reframe an EXISTING profile bullet so the
// term surfaces honestly. The hard constraint (spec §5.1): it may only reframe
// experience already present in the profile — never introduce a skill,
// employer, title, date, or metric absent from the source.
//
// Two independent guards enforce that, defence-in-depth:
//
//  1. Evidence gate (before any model call): if no profile bullet is adjacent
//     to the term, we return an explicit "no honest rephrase available" result
//     and never prompt the model. We do not soften this into a suggestion.
//  2. Grounding post-check (after each model call): every generation is routed
//     through verifyRephraseGrounding, which rejects the output if it mentions
//     a technology/proper-noun or a metric/number not traceable to the source.
//     This mirrors the philosophy of generation.verifyGrounding (spec §5.2) —
//     the LLM may reorder/rephrase/omit, never invent — adapted to free-text
//     bullet reframing.
//
// On rejection the suggester retries once with the violation fed back into the
// prompt (spec §5.2 step 4); a second failure yields the no-rephrase result.
// Every rejected generation is logged (spec §5.2 step 5).

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// ReasonNoHonestRephrase is the explicit result when the profile has no
// adjacent evidence for a term, or every grounded generation was rejected.
// It is returned verbatim rather than softened into a speculative suggestion.
const ReasonNoHonestRephrase = "no-honest-rephrase-available"

// rephraseAttempts is the total number of model attempts per term: the first
// plus one retry with the grounding violation fed back (spec §5.2 step 4).
const rephraseAttempts = 2

// RephraseSuggestion is the per-term output of the suggester.
type RephraseSuggestion struct {
	// Term / Canonical identify the missing-required JD term this addresses.
	Term      string `json:"term"`
	Canonical string `json:"canonical"`
	// Rephrase is the honest reframed bullet, or nil when none is available.
	Rephrase *string `json:"rephrase"`
	// SourceBullet is the existing profile bullet the rephrase was derived
	// from — kept for traceability so a reviewer can confirm the reframing.
	SourceBullet string `json:"sourceBullet,omitempty"`
	// Reason is empty on success, else ReasonNoHonestRephrase.
	Reason string `json:"reason,omitempty"`
}

// RephraseModel is the minimal LLM port the suggester needs. The production
// adapter (ProviderRephraseModel) wraps llm.Provider; unit tests inject a
// deterministic fake, keeping the suggester's logic testable without a model.
type RephraseModel interface {
	// Rephrase returns a single reframed bullet for the given prompt.
	Rephrase(ctx context.Context, prompt string) (string, error)
}

// EvidenceFinder decides whether the profile contains experience adjacent to a
// missing JD term that is worth reframing. Production may use embedding
// similarity (spec §3.1); the default LexicalEvidenceFinder uses token overlap
// so the suggester is deterministic and dependency-free by default.
type EvidenceFinder interface {
	// FindEvidence returns the best adjacent profile bullet and true, or "" and
	// false when no bullet is adjacent to the term.
	FindEvidence(term DiffTerm, bullets []string) (bullet string, ok bool)
}

// Suggester generates truthful rephrases for missing-required terms.
type Suggester struct {
	model  RephraseModel
	finder EvidenceFinder
	log    *slog.Logger
}

// NewSuggester returns a suggester backed by model, the default lexical
// evidence finder, and the default slog logger. Use WithEvidenceFinder /
// WithLogger to override.
func NewSuggester(model RephraseModel) *Suggester {
	return &Suggester{
		model:  model,
		finder: LexicalEvidenceFinder{},
		log:    slog.Default(),
	}
}

// WithEvidenceFinder swaps the adjacency strategy (e.g. an embedding finder).
func (s *Suggester) WithEvidenceFinder(f EvidenceFinder) *Suggester {
	if f != nil {
		s.finder = f
	}
	return s
}

// WithLogger sets the logger used for rejected-generation observability.
func (s *Suggester) WithLogger(l *slog.Logger) *Suggester {
	if l != nil {
		s.log = l
	}
	return s
}

// SuggestAll produces one suggestion per missing-required term. profileBullets
// are the user's existing resume bullet lines, verbatim. The returned slice is
// aligned 1:1 with missingRequired (including no-rephrase results) so callers
// can render a complete gap report.
func (s *Suggester) SuggestAll(ctx context.Context, missingRequired []DiffTerm, profileBullets []string) []RephraseSuggestion {
	out := make([]RephraseSuggestion, 0, len(missingRequired))
	for _, t := range missingRequired {
		out = append(out, s.Suggest(ctx, t, profileBullets))
	}
	return out
}

// Suggest produces a truthful rephrase for a single missing-required term, or
// an explicit no-rephrase result. It never returns an ungrounded suggestion.
func (s *Suggester) Suggest(ctx context.Context, term DiffTerm, profileBullets []string) RephraseSuggestion {
	res := RephraseSuggestion{Term: term.Term, Canonical: term.Canonical}

	// Guard 1 — evidence gate. No adjacent evidence ⇒ explicit no-rephrase,
	// and crucially no model call (nothing honest to say).
	bullet, ok := s.finder.FindEvidence(term, profileBullets)
	if !ok {
		res.Reason = ReasonNoHonestRephrase
		return res
	}

	// Grounding source sets, computed once. Skills/proper-nouns may come from
	// anywhere in the profile; metrics/numbers must come from the specific
	// source bullet being reframed (a reframing cannot borrow a figure from an
	// unrelated bullet).
	allowedProper := properNounSet(profileBullets)
	sourceNums := numberSet(bullet)

	var lastViol []string
	for attempt := 0; attempt < rephraseAttempts; attempt++ {
		prompt := buildRephrasePrompt(term, bullet, lastViol)
		raw, err := s.model.Rephrase(ctx, prompt)
		if err != nil {
			// A model/transport error is not a truthful suggestion; log and
			// fall back to the honest no-rephrase result.
			s.log.WarnContext(ctx, "keyword: rephrase model error",
				"term", term.Term, "error", err)
			res.Reason = ReasonNoHonestRephrase
			return res
		}
		out := strings.TrimSpace(raw)

		// Guard 2 — grounding post-check.
		lastViol = verifyRephraseGrounding(bullet, allowedProper, sourceNums, out)
		if len(lastViol) == 0 {
			r := out
			res.Rephrase = &r
			res.SourceBullet = bullet
			return res
		}

		// Reject rather than emit; log the violating output for prompt
		// debugging and observability (spec §5.2 step 5).
		s.log.WarnContext(ctx, "keyword: rejected rephrase generation (grounding violation)",
			"term", term.Term,
			"attempt", attempt+1,
			"violations", strings.Join(lastViol, "; "),
			"output", out)
	}

	res.Reason = ReasonNoHonestRephrase
	return res
}

// buildRephrasePrompt builds the generation-time grounding prompt (spec §5.2
// step 2): the model may only reframe the source bullet, and any prior
// grounding violation is fed back so the retry can correct it.
func buildRephrasePrompt(term DiffTerm, sourceBullet string, priorViolations []string) string {
	want := term.Term
	if term.Canonical != "" {
		want = term.Canonical
	}
	var b strings.Builder
	b.WriteString("Reframe the candidate's EXISTING resume bullet so that it honestly surfaces the target skill/term the job wants.\n\n")
	b.WriteString("STRICT RULES:\n")
	b.WriteString("- Rephrase ONLY the experience shown in the source bullet below.\n")
	b.WriteString("- Do NOT add any skill, technology, employer, job title, date, or metric that is not already in the source bullet.\n")
	b.WriteString("- If the source bullet does not genuinely support the target term, do not stretch it — return the bullet unchanged.\n")
	b.WriteString("- Output the single reframed bullet as plain text. No preamble, no quotes, no bullet marker.\n\n")
	b.WriteString("TARGET TERM: " + want + "\n\n")
	b.WriteString("SOURCE BULLET:\n" + sourceBullet + "\n")
	if len(priorViolations) > 0 {
		b.WriteString("\nYour previous attempt violated the no-invention rule:\n- ")
		b.WriteString(strings.Join(priorViolations, "\n- "))
		b.WriteString("\nRegenerate using only what the source bullet contains.\n")
	}
	return b.String()
}

// numberRe matches metric/number tokens: integers/decimals with optional
// thousands separators and an optional trailing percent (e.g. "40%", "3",
// "1,200", "99.9%").
var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*%?`)

// verifyRephraseGrounding is the keyword-diff analogue of
// generation.verifyGrounding (spec §5.2 step 3). It rejects a rephrase whose
// content is not traceable to the source. Returns human-readable violations;
// an empty slice means the output is grounded.
//
// Two entity classes are checked — the ones an inventive model reaches for:
//
//   - Proper nouns (capitalized mid-sentence tokens, acronyms, mixed-case tech
//     like "JavaScript"): technologies, employers, product/tool names. Every
//     one must already appear in the profile (allowedProper). This catches a
//     model tempted to insert a JD technology absent from the profile.
//   - Metrics/numbers: every numeric token must appear in the source bullet
//     (sourceNums). This catches fabricated figures ("improved perf by 40%").
func verifyRephraseGrounding(sourceBullet string, allowedProper, sourceNums map[string]bool, rephrase string) []string {
	if strings.TrimSpace(rephrase) == "" {
		return []string{"empty rephrase"}
	}
	var violations []string

	for _, p := range properNounsInText(rephrase) {
		if !allowedProper[lowerASCII(p)] {
			violations = append(violations, fmt.Sprintf("proper noun / technology %q not in profile", p))
		}
	}
	for _, n := range numberRe.FindAllString(rephrase, -1) {
		if !sourceNums[normNumber(n)] {
			violations = append(violations, fmt.Sprintf("metric/number %q not in source bullet", n))
		}
	}
	return violations
}

// properNounsInText returns the capitalized, non-sentence-initial tokens of s
// (plus any all-caps or mixed-case tech tokens), which stand in for the
// technology / employer / product names a rephrase must not invent. The first
// word of each sentence is skipped because its capitalization is grammatical,
// not a proper noun.
func properNounsInText(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, sentence := range splitSentences(s) {
		tokens := tokenRe.FindAllString(sentence, -1)
		for i, tok := range tokens {
			if !looksProper(tok) {
				continue
			}
			// A grammatically-capitalized first word (e.g. "Built ...") is not
			// a proper noun; a mixed-case/all-caps token ("JavaScript", "AWS")
			// is, even in first position.
			if i == 0 && isPlainCapitalized(tok) {
				continue
			}
			key := lowerASCII(tok)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tok)
		}
	}
	return out
}

// sentenceSplitRe splits on sentence-ending punctuation followed by space.
var sentenceSplitRe = regexp.MustCompile(`[.!?]\s+`)

func splitSentences(s string) []string { return sentenceSplitRe.Split(s, -1) }

// looksProper reports whether a token carries any uppercase letter (proper
// noun, acronym, or mixed-case technology name).
func looksProper(tok string) bool {
	for _, r := range tok {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

// isPlainCapitalized reports whether a token is capitalized only in the
// leading position (e.g. "Built") — ordinary sentence-initial casing rather
// than a proper noun like "JavaScript" or "AWS".
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

// properNounSet collects every word token across the profile bullets, indexed
// lowercased. A proper noun / technology in a rephrase is grounded only if it
// appears here — i.e. only if the profile itself contains it.
//
// The target term is deliberately NOT auto-allowed: surfacing a term the
// profile does not actually contain is an overclaim, even when a lexically
// adjacent bullet passed the evidence gate. The honest move is to reframe the
// adjacent experience in the profile's own words, so grounding requires the
// term (or its tokens) to be present in the source.
func properNounSet(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		// Index raw word tokens so a lowercase profile mention still grounds a
		// capitalized mention in the rephrase.
		for _, w := range tokenRe.FindAllString(b, -1) {
			set[lowerASCII(w)] = true
		}
	}
	return set
}

// numberSet returns the normalized numeric tokens present in text.
func numberSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, n := range numberRe.FindAllString(text, -1) {
		set[normNumber(n)] = true
	}
	return set
}

// normNumber normalizes a numeric token for comparison: strips a trailing
// percent and thousands separators so "1,200" and "1200" compare equal.
func normNumber(n string) string {
	n = strings.TrimSuffix(n, "%")
	return strings.ReplaceAll(n, ",", "")
}

// LexicalEvidenceFinder is the default, deterministic EvidenceFinder. A profile
// bullet is "adjacent" to a term when they share at least one significant
// (non-stopword) token after stemming — e.g. the term "React Native" is
// adjacent to a bullet mentioning "React". This is intentionally conservative:
// it never invents a link where the profile shares no vocabulary with the term.
type LexicalEvidenceFinder struct{}

// FindEvidence returns the first bullet sharing a stemmed token with the term.
func (LexicalEvidenceFinder) FindEvidence(term DiffTerm, bullets []string) (string, bool) {
	wants := termStems(term)
	if len(wants) == 0 {
		return "", false
	}
	for _, b := range bullets {
		for _, w := range filterStopwords(tokenRe.FindAllString(b, -1)) {
			if wants[stem(w)] {
				return b, true
			}
		}
	}
	return "", false
}

// termStems returns the set of stemmed significant tokens of a term.
func termStems(term DiffTerm) map[string]bool {
	src := term.Canonical
	if src == "" {
		src = term.Term
	}
	out := map[string]bool{}
	for _, w := range filterStopwords(tokenRe.FindAllString(src, -1)) {
		if s := stem(w); s != "" {
			out[s] = true
		}
	}
	return out
}
