package domain

import (
	"regexp"
	"sort"
	"strings"
)

// Class is the hard must-have vs nice-to-have classification of an extracted
// term. It is a coarser, decision-oriented view over the extractor's Polarity:
// the Fit-Gap Coach (spec 009) counts only must-haves in its failure summary
// (009 spec §3), so a term needs a single boolean-ish verdict, backed by the
// concrete JD phrasing signal(s) that produced it.
type Class string

const (
	// ClassMustHave marks a term the JD frames as a hard requirement.
	ClassMustHave Class = "must_have"
	// ClassNiceToHave marks a term the JD frames as optional / a bonus.
	ClassNiceToHave Class = "nice_to_have"
)

// Signal names the kind of JD phrasing that justified a must-have verdict.
// These are stable identifiers (not human prose) so tests and callers can
// assert on why a term was classified.
type Signal string

const (
	// SignalRequiredSection: the term sits in a section whose header implies a
	// requirement (e.g. "Requirements", "Qualifications") — section polarity.
	SignalRequiredSection Signal = "required_section"
	// SignalMustHavePhrase: an explicit "must have" / "must" phrase on the line.
	SignalMustHavePhrase Signal = "must_have_phrase"
	// SignalRequiredPhrase: an explicit "required"/"essential"/"mandatory"
	// style phrase on the line.
	SignalRequiredPhrase Signal = "required_phrase"
	// SignalYearsOfExperience: a years-of-experience qualifier on the line
	// (e.g. "5+ years", "at least 3 yrs"), a strong hard-requirement signal.
	SignalYearsOfExperience Signal = "years_of_experience"
	// SignalPreferredPhrase: an explicit optional marker ("a plus", "nice to
	// have", "bonus"). Informational — it never upgrades to must-have and is
	// reported so callers can see the optional framing.
	SignalPreferredPhrase Signal = "preferred_phrase"
)

// ClassifiedTerm is an ExtractedTerm with its hard must-have verdict and the
// phrasing signals that produced it.
type ClassifiedTerm struct {
	ExtractedTerm
	Class   Class    `json:"class"`
	Signals []Signal `json:"signals,omitempty"`
}

// yearsOfExperienceRe matches years-of-experience qualifiers such as
// "5+ years", "3 yrs", "at least 2 year". A digit followed by an optional "+"
// and a years word is the reliable, language-light signal.
var yearsOfExperienceRe = regexp.MustCompile(`(?i)\b\d+\s*\+?\s*(?:years?|yrs?)\b`)

// mustHavePhrases are the strongest inline hard-requirement markers.
var mustHavePhrases = []string{"must have", "must-have", "must possess", "must be able"}

// requiredPhrases are other inline hard-requirement markers.
var requiredPhrases = []string{
	"required", "requires", "essential", "mandatory", "minimum of",
	"at least", "prerequisite", "proficiency in", "proven",
}

// preferredPhrases are inline optional markers. They are informational only.
var preferredPhrases = []string{
	"nice to have", "nice-to-have", "a plus", "is a plus", "bonus",
	"preferred", "desired", "would be great", "ideal but not required",
}

// classifierWordBoundary lets us require whole-word "must" without matching
// "mustard" etc. Cheap and deterministic; no full tokenizer needed here.
var mustWordRe = regexp.MustCompile(`(?i)\bmust\b`)

// Classify assigns each extracted term a hard must-have / nice-to-have verdict.
// A term is a must-have when EITHER its section implies a requirement OR its
// source line carries an inline hard signal (must have, required, or a
// years-of-experience qualifier). Inline hard signals can promote a term the
// section framed as optional; an optional marker alone never demotes a term
// the JD otherwise frames as required (a hard requirement stated once outranks
// a softer repeat — mirrors the extractor's required-wins dedup in Extract).
func Classify(res *ExtractResult) []ClassifiedTerm {
	if res == nil {
		return nil
	}
	out := make([]ClassifiedTerm, 0, len(res.Terms))
	for _, t := range res.Terms {
		class, signals := classifyTerm(t)
		out = append(out, ClassifiedTerm{ExtractedTerm: t, Class: class, Signals: signals})
	}
	return out
}

// classifyTerm computes the verdict and supporting signals for one term.
func classifyTerm(t ExtractedTerm) (Class, []Signal) {
	set := map[Signal]bool{}

	if t.Polarity == PolarityRequired {
		set[SignalRequiredSection] = true
	}

	ev := lowerASCII(t.Evidence)
	if ev != "" {
		if containsAny(ev, mustHavePhrases) || mustWordRe.MatchString(ev) {
			set[SignalMustHavePhrase] = true
		}
		if containsAny(ev, requiredPhrases) {
			set[SignalRequiredPhrase] = true
		}
		if yearsOfExperienceRe.MatchString(ev) {
			set[SignalYearsOfExperience] = true
		}
		if containsAny(ev, preferredPhrases) {
			set[SignalPreferredPhrase] = true
		}
	}

	hardSignal := set[SignalRequiredSection] || set[SignalMustHavePhrase] ||
		set[SignalRequiredPhrase] || set[SignalYearsOfExperience]

	class := ClassNiceToHave
	if hardSignal {
		class = ClassMustHave
	}
	return class, sortedSignals(set)
}

// containsAny reports whether haystack contains any of the given substrings.
func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

// sortedSignals flattens the signal set into a stable, sorted slice so output
// is deterministic across runs (map iteration order is not).
func sortedSignals(set map[Signal]bool) []Signal {
	if len(set) == 0 {
		return nil
	}
	out := make([]Signal, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
