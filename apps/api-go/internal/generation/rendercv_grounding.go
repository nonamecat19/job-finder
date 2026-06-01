package generation

// verifyRendercvGrounding is the post-merge grounding check for the RenderCV
// path. Because mergeTailored only ever replaces summary/skill-details/
// highlights and copies companies, dates, education and design verbatim,
// the structural rules (no invented employer/date/degree) are guaranteed by
// construction. The one machine-checkable content rule is the STRICT skill
// constraint: no skill token may appear that isn't already in the master.
// We still defensively verify merged experience companies are a subset of
// the master. Mirrors verifyRendercvGrounding in rendercv-grounding.ts.
func verifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel) []string {
	var violations []string

	masterSections := cvSections(master)
	masterCompanies := map[string]bool{}
	for _, e := range asSliceOfMaps(masterSections["experience"]) {
		masterCompanies[norm(stringField(e, "company"))] = true
	}

	mergedSections := cvSections(merged)
	for _, e := range asSliceOfMaps(mergedSections["experience"]) {
		company := stringField(e, "company")
		if !masterCompanies[norm(company)] {
			violations = append(violations, `company "`+company+`" not in master profile`)
		}
	}

	if level == GroundingStrict {
		allowed := masterSkillTokens(master)
		for _, g := range asSliceOfMaps(mergedSections["skills"]) {
			label := stringField(g, "label")
			for _, t := range tokens(stringField(g, "details")) {
				if !allowed[t] {
					violations = append(violations, `skill "`+t+`" (`+label+`) not in master profile (strict grounding)`)
				}
			}
		}
	}

	return violations
}
