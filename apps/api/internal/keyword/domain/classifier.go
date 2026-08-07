package domain

import (
	"regexp"
	"sort"
	"strings"
)

type Class string

const (
	ClassMustHave   Class = "must_have"
	ClassNiceToHave Class = "nice_to_have"
)

type Signal string

const (
	SignalRequiredSection   Signal = "required_section"
	SignalMustHavePhrase    Signal = "must_have_phrase"
	SignalRequiredPhrase    Signal = "required_phrase"
	SignalYearsOfExperience Signal = "years_of_experience"
	SignalPreferredPhrase   Signal = "preferred_phrase"
)

type ClassifiedTerm struct {
	ExtractedTerm
	Class   Class    `json:"class"`
	Signals []Signal `json:"signals,omitempty"`
}

var yearsOfExperienceRe = regexp.MustCompile(`(?i)\b\d+\s*\+?\s*(?:years?|yrs?)\b`)

var mustHavePhrases = []string{"must have", "must-have", "must possess", "must be able"}

var requiredPhrases = []string{
	"required", "requires", "essential", "mandatory", "minimum of",
	"at least", "prerequisite", "proficiency in", "proven",
}

var preferredPhrases = []string{
	"nice to have", "nice-to-have", "a plus", "is a plus", "bonus",
	"preferred", "desired", "would be great", "ideal but not required",
}

var mustWordRe = regexp.MustCompile(`(?i)\bmust\b`)

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

func containsAny(haystack string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}

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
