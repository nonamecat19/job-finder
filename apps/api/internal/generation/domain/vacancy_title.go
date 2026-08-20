package domain

import "strings"

// VacancyTarget is the posting's own header — the fields a job board fills in
// even when it never serves the body.
type VacancyTarget struct {
	Title   string
	Company string
}

// A vacancy title is the one part of a posting that is never truncated, never
// boilerplate and never optional: "Senior Full Stack Developer (Vue.js/Node.js)"
// names the stack even when the body the scraper captured is a company blurb.
// Until now the title reached only the export filename, so a run whose analysis
// came back with no required skills at all ranked the candidate's Vue bullets
// last and wrote a summary about a different stack.
//
// TitleRequiredSkills reads the title as a source of *required* skills, but it
// can only ever return skills the candidate already claims: it returns the
// master's own skill entries, so a title naming a technology the candidate does
// not have adds nothing. Grounding is preserved by construction — this widens
// what the vacancy asks for, never what the candidate offers.
func TitleRequiredSkills(master RendercvMaster, title string) []string {
	titleWords := significantWords(title)
	if len(titleWords) == 0 {
		return nil
	}

	var out []string
	seen := map[string]bool{}
	for _, g := range AsSliceOfMaps(CvSections(master)["skills"]) {
		for _, entry := range splitSkillEntries(StringField(g, "details")) {
			key := norm(entry)
			if seen[key] {
				continue
			}
			for w := range significantWords(entry) {
				if titleWords[w] {
					seen[key] = true
					out = append(out, entry)
					break
				}
			}
		}
	}
	return out
}

// MergeTitleSkills folds the title's skills into the analysis' required list,
// skipping any the model already found. The match is the same one ranking uses,
// so "Vue.js" from a title does not double up an existing "Vue 3".
func MergeTitleSkills(analysis VacancyAnalysis, titleSkills []string) VacancyAnalysis {
	for _, s := range titleSkills {
		if matchesAnySkill(norm(s), analysis.RequiredSkills) {
			continue
		}
		analysis.RequiredSkills = append(analysis.RequiredSkills, s)
	}
	return analysis
}

// significantWords drops the words that say what kind of role a title is rather
// than which technology it is about. Without this, "Web Developer" matches
// "Web Performance" and every ".js" entry matches every other one through the
// shared "js" — a match on noise is worse than no match, because it dilutes the
// required list the whole ranking pass is scored against.
func significantWords(s string) map[string]bool {
	out := map[string]bool{}
	for w := range phraseWords(s) {
		if w == "" || titleNoiseWords[w] {
			continue
		}
		out[w] = true
	}
	return out
}

var titleNoiseWords = map[string]bool{
	"js": true, "dev": true, "developer": true, "engineer": true,
	"programmer": true, "software": true, "specialist": true, "expert": true,
	"architect": true, "manager": true, "team": true, "lead": true,
	"senior": true, "middle": true, "mid": true, "junior": true,
	"principal": true, "staff": true, "intern": true, "trainee": true,
	"full": true, "fullstack": true, "stack": true, "part": true,
	"time": true, "web": true, "remote": true, "hybrid": true, "onsite": true,
	"and": true, "or": true, "with": true, "for": true, "the": true,
	"a": true, "an": true, "in": true, "of": true, "to": true,
}

// VacancyIsThin reports a posting that carries no requirements to tailor
// against. It is the difference between "the vacancy asks for nothing unusual"
// and "we never got the vacancy" — a scraper that stores a listing teaser, or a
// posting behind a login wall, both land here. A run in this state still
// produces a resume, because the master content is real, but it is the master's
// own ordering dressed as a tailored document, and saying so is the only thing
// that stops a user sending it.
func VacancyIsThin(vacancyText string, analysis VacancyAnalysis) bool {
	if len(analysis.RequiredSkills) > 0 || len(analysis.NiceToHaveSkills) > 0 {
		return false
	}
	return len(strings.Fields(vacancyText)) < thinVacancyWords
}

// thinVacancyWords is deliberately generous: a real posting that asks for no
// named skill at all is rare, and a run should not be flagged for being terse.
// Djinni's listing teaser runs to roughly 80 words.
const thinVacancyWords = 120

// RankedSkillLines renders the candidate's skill groups the way the summary
// needs to read them — "Frontend: Vue 3, React, ..." — after ordering them
// against the vacancy. It works on a clone, so ranking for a prompt never
// mutates the document being assembled.
func RankedSkillLines(master RendercvMaster, analysis VacancyAnalysis, cfg ShapeConfig) []string {
	clone, err := DeepCloneYAML(master)
	if err != nil {
		clone = master
	} else {
		RankSkills(clone, analysis, cfg)
	}
	var out []string
	for _, g := range AsSliceOfMaps(CvSections(clone)["skills"]) {
		label, details := StringField(g, "label"), StringField(g, "details")
		switch {
		case label != "" && details != "":
			out = append(out, label+": "+details)
		case details != "":
			out = append(out, details)
		}
	}
	return out
}
