// Grounding verification: every employer, institution, degree and date in
// a tailored resume must already exist in the master profile.
package domain

import (
	"strings"

	"github.com/job-finder/api/internal/dto"
)

// verifyGrounding is a post-generation check: every employer, institution,
// degree and date in the tailored resume must already exist in the master
// profile. The LLM may reorder/rephrase/omit — never invent. Mirrors
// grounding.ts's verifyGrounding exactly.
func VerifyGrounding(master, tailored dto.JsonResume) []string {
	var violations []string

	employers := map[string]bool{}
	workDates := map[string]bool{}
	for _, w := range master.Work {
		employers[norm(w.Name)] = true
		if w.StartDate != nil {
			workDates[norm(*w.StartDate)] = true
		}
		if w.EndDate != nil {
			workDates[norm(*w.EndDate)] = true
		}
	}

	institutions := map[string]bool{}
	eduDates := map[string]bool{}
	for _, e := range master.Education {
		institutions[norm(e.Institution)] = true
		if e.StartDate != nil {
			eduDates[norm(*e.StartDate)] = true
		}
		if e.EndDate != nil {
			eduDates[norm(*e.EndDate)] = true
		}
	}

	projectNames := map[string]bool{}
	for _, p := range master.Projects {
		projectNames[norm(p.Name)] = true
	}

	for _, w := range tailored.Work {
		if !employers[norm(w.Name)] {
			violations = append(violations, `employer "`+w.Name+`" not in master profile`)
		}
		for _, d := range []*string{w.StartDate, w.EndDate} {
			if d != nil && !workDates[norm(*d)] {
				violations = append(violations, `work date "`+*d+`" (`+w.Name+`) not in master profile`)
			}
		}
	}

	for _, e := range tailored.Education {
		if !institutions[norm(e.Institution)] {
			violations = append(violations, `institution "`+e.Institution+`" not in master profile`)
		}
		for _, d := range []*string{e.StartDate, e.EndDate} {
			if d != nil && !eduDates[norm(*d)] {
				violations = append(violations, `education date "`+*d+`" (`+e.Institution+`) not in master profile`)
			}
		}
	}

	for _, p := range tailored.Projects {
		if !projectNames[norm(p.Name)] {
			violations = append(violations, `project "`+p.Name+`" not in master profile`)
		}
	}

	return violations
}

func VerifyGroundingFromRendercv(master RendercvMaster, tailored dto.JsonResume) []string {
	var violations []string

	sections := CvSections(master)

	employers := map[string]bool{}
	workDates := map[string]bool{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		employers[norm(StringField(e, "company"))] = true
		if d := StringField(e, "start_date"); d != "" {
			workDates[norm(d)] = true
		}
		if d := StringField(e, "end_date"); d != "" {
			workDates[norm(d)] = true
		}
	}

	institutions := map[string]bool{}
	eduDates := map[string]bool{}
	for _, e := range AsSliceOfMaps(sections["education"]) {
		institutions[norm(StringField(e, "institution"))] = true
		if d := StringField(e, "start_date"); d != "" {
			eduDates[norm(d)] = true
		}
		if d := StringField(e, "end_date"); d != "" {
			eduDates[norm(d)] = true
		}
	}

	projectNames := map[string]bool{}
	for _, p := range AsSliceOfMaps(sections["projects"]) {
		projectNames[norm(StringField(p, "name"))] = true
	}

	for _, w := range tailored.Work {
		if !employers[norm(w.Name)] {
			violations = append(violations, `employer "`+w.Name+`" not in master profile`)
		}
		for _, d := range []*string{w.StartDate, w.EndDate} {
			if d != nil && !workDates[norm(*d)] {
				violations = append(violations, `work date "`+*d+`" (`+w.Name+`) not in master profile`)
			}
		}
	}

	for _, e := range tailored.Education {
		if !institutions[norm(e.Institution)] {
			violations = append(violations, `institution "`+e.Institution+`" not in master profile`)
		}
		for _, d := range []*string{e.StartDate, e.EndDate} {
			if d != nil && !eduDates[norm(*d)] {
				violations = append(violations, `education date "`+*d+`" (`+e.Institution+`) not in master profile`)
			}
		}
	}

	for _, p := range tailored.Projects {
		if !projectNames[norm(p.Name)] {
			violations = append(violations, `project "`+p.Name+`" not in master profile`)
		}
	}

	return violations
}

func norm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
