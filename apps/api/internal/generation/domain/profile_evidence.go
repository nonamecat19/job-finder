package domain

import "strings"

// Profile evidence is the second ranking signal, under vacancy relevance.
//
// Vacancy relevance is a coarse score: required (2), nice-to-have (1), or
// unmentioned (0), and a candidate's skill group collapses into a handful of
// ties at each level. Master order broke those ties, which means the order the
// user happened to type decided what a reader sees first — and, once a density
// cap trims the tail, what survives at all.
//
// The profile itself is a better tiebreaker than typing order: a skill the
// candidate actually wrote about in their experience bullets is a skill they
// can talk about in an interview, and one that appears in three roles is
// stronger evidence than one that appears in none. ProfileEvidence counts
// those mentions once per document; the ranking functions read the counts.
//
// It is a *tiebreaker*, never an override. A required skill the profile never
// mentions elsewhere still outranks a well-evidenced skill the vacancy did not
// ask for — the vacancy decides relevance, the profile only decides the order
// within one relevance level.
type ProfileEvidence map[string]int

// BuildProfileEvidence counts how often each word appears in the parts of the
// document that describe what the candidate did: experience and project
// summaries and bullets, plus education highlights. Skill group details are
// deliberately excluded — every skill appears there exactly once, so counting
// them would add the same constant to every entry and change no order.
func BuildProfileEvidence(doc RendercvMaster) ProfileEvidence {
	ev := ProfileEvidence{}
	sections := CvSections(doc)
	if sections == nil {
		return ev
	}
	for _, key := range []string{"experience", "projects", "education"} {
		for _, e := range AsSliceOfMaps(sections[key]) {
			ev.add(StringField(e, "summary"))
			ev.add(StringField(e, "position"))
			for _, h := range StringSliceField(e, "highlights") {
				ev.add(h)
			}
		}
	}
	return ev
}

func (ev ProfileEvidence) add(text string) {
	if text == "" {
		return
	}
	for _, w := range strings.FieldsFunc(text, isSkillWordSeparator) {
		if n := normSkillWord(w); n != "" {
			ev[n]++
		}
	}
}

// Score is how well the profile evidences one phrase: the mention count of its
// rarest word, so "React Native" scores by the weaker half rather than
// inheriting the count of a common word like "React". A phrase whose words
// never appear scores 0.
func (ev ProfileEvidence) Score(phrase string) int {
	best := -1
	for _, w := range strings.FieldsFunc(phrase, isSkillWordSeparator) {
		n := normSkillWord(w)
		if n == "" {
			continue
		}
		c := ev[n]
		if best < 0 || c < best {
			best = c
		}
	}
	if best < 0 {
		return 0
	}
	return best
}
