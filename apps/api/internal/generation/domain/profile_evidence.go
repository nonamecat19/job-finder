package domain

import "strings"

type ProfileEvidence map[string]int

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
