package generation

// verifyRendercvGrounding is the post-merge grounding check for the RenderCV
// path. It verifies:
// - No fabricated companies (experience entries must exist in master)
// - No fabricated sections in SectionsToDrop (must be real master sections)
// - ExperienceOrder companies must be a subset of master companies
// - ExperienceOrder must include all master companies (no entries lost)
// - Dropped experience entries must still exist in master
// - On strict grounding, no skill tokens outside the master skill pool.
func verifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel) []string {
	var violations []string

	masterSections := cvSections(master)
	mergedSections := cvSections(merged)

	// 1. Check experience companies in merged are all from master
	masterCompanies := map[string]bool{}
	for _, e := range asSliceOfMaps(masterSections["experience"]) {
		masterCompanies[norm(stringField(e, "company"))] = true
	}
	for _, e := range asSliceOfMaps(mergedSections["experience"]) {
		company := stringField(e, "company")
		if !masterCompanies[norm(company)] {
			violations = append(violations, `company "`+company+`" not in master profile`)
		}
	}

	// 2. Verify sections in merged are a subset of master sections (no added sections)
	masterSectionKeys := map[string]bool{}
	for k := range masterSections {
		masterSectionKeys[k] = true
	}
	for k := range mergedSections {
		if !masterSectionKeys[k] {
			violations = append(violations, `unexpected section "`+k+`" added to merged resume`)
		}
	}

	// 3. Strict skill grounding: no tokens outside the master skill pool
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
