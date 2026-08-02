package domain

import (
	"strings"
	"unicode"
)

// verifyRendercvGrounding is the post-merge grounding check for the RenderCV
// path. It verifies:
// - No fabricated companies (experience entries must exist in master)
// - No added sections (merged sections are a subset of master sections)
// - On strict grounding, no skill tokens outside the master skill pool.
//
// It does not check structural invariants (block sequence, experience order,
// total experience years) — those are enforced by MergeTailored and
// VerifyStructureIntegrity (see rendercv_structure.go).
func VerifyRendercvGrounding(master, merged RendercvMaster, level GroundingLevel) []string {
	var violations []string

	masterSections := CvSections(master)
	mergedSections := CvSections(merged)

	// 1. Check experience companies in merged are all from master
	masterCompanies := map[string]bool{}
	for _, e := range AsSliceOfMaps(masterSections["experience"]) {
		masterCompanies[norm(StringField(e, "company"))] = true
	}
	for _, e := range AsSliceOfMaps(mergedSections["experience"]) {
		company := StringField(e, "company")
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

	// 3. Check project names in merged are all from master. Projects gained a
	// tailoring path, so they need the same no-fabrication guard companies
	// have: the model may select among the master's projects, never add one.
	masterProjects := map[string]bool{}
	for _, p := range AsSliceOfMaps(masterSections["projects"]) {
		masterProjects[norm(StringField(p, "name"))] = true
	}
	for _, p := range AsSliceOfMaps(mergedSections["projects"]) {
		name := StringField(p, "name")
		if !masterProjects[norm(name)] {
			violations = append(violations, `project "`+name+`" not in master profile`)
		}
	}

	// 4. Strict skill grounding: no tokens outside the master skill pool
	if level == GroundingStrict {
		allowed := masterSkillTokens(master)
		for _, g := range AsSliceOfMaps(mergedSections["skills"]) {
			label := StringField(g, "label")
			for _, t := range tokens(StringField(g, "details")) {
				if !allowed[t] {
					violations = append(violations, `skill "`+t+`" (`+label+`) not in master profile (strict grounding)`)
				}
			}
		}

		// 5. Strict project grounding: each project's highlights must come
		// from that same project's own master bullets — a bullet borrowed
		// from a *different* project is still a misattribution.
		masterProjectTokens := map[string]map[string]bool{}
		for _, p := range AsSliceOfMaps(masterSections["projects"]) {
			masterProjectTokens[norm(StringField(p, "name"))] = wordTokens(StringSliceField(p, "highlights"))
		}
		for _, p := range AsSliceOfMaps(mergedSections["projects"]) {
			name := StringField(p, "name")
			allowed, ok := masterProjectTokens[norm(name)]
			if !ok {
				continue // already reported as an unknown project above
			}
			for t := range wordTokens(StringSliceField(p, "highlights")) {
				if !allowed[t] {
					violations = append(violations, `project highlight token "`+t+`" (`+name+`) not in master profile (strict grounding)`)
				}
			}
		}
	}

	return violations
}

// wordTokens is the set of normalised words across a project's bullets, used
// as the strict-grounding pool for that project's highlights. Words shorter
// than four characters are ignored: connectives ("and", "the", "for") carry no
// grounding signal and would only produce noise.
func wordTokens(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		for _, w := range strings.FieldsFunc(b, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsNumber(r)
		}) {
			if n := norm(w); len(n) >= 4 {
				set[n] = true
			}
		}
	}
	return set
}
