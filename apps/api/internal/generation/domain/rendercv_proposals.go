package domain

import (
	"fmt"
	"strings"
)

type EditProposal struct {
	FieldType    string
	FieldKey     string
	BeforeValue  string
	AfterValue   string
	Traceability Traceability
	Status       string
	DroppedReason string
}

type Traceability struct {
	Source string
	Path   string
}

func PayloadToProposals(baseline RendercvMaster, payload TailoredSections, jobPosting string) []EditProposal {
	var proposals []EditProposal
	sections := CvSections(baseline)

	// 1. Summary diff
	baselineSummary := ""
	if raw, ok := sections["summary"]; ok {
		if items, ok := raw.([]any); ok && len(items) > 0 {
			if s, ok := items[0].(string); ok {
				baselineSummary = strings.TrimSpace(s)
			}
		}
	}
	proposedSummary := strings.TrimSpace(payload.Summary)
	if proposedSummary != "" && proposedSummary != baselineSummary {
		proposals = append(proposals, EditProposal{
			FieldType:   "summary",
			FieldKey:    "summary",
			BeforeValue: baselineSummary,
			AfterValue:  proposedSummary,
			Traceability: Traceability{Source: "master", Path: "cv.sections.summary"},
			Status:      "pending",
		})
	}

	// 2. Experience highlights per company (LCS diff)
	baselineCompanies := map[string][]string{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		baselineCompanies[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	for _, pe := range payload.Experience {
		company := pe.Company
		baselineHighlights := baselineCompanies[norm(company)]
		for i, h := range pe.Highlights {
			before := ""
			if i < len(baselineHighlights) {
				before = baselineHighlights[i]
			}
			if strings.TrimSpace(h) != strings.TrimSpace(before) {
				proposals = append(proposals, EditProposal{
					FieldType:   "experience_highlights",
					FieldKey:    fmt.Sprintf("%s::%d", company, i),
					BeforeValue: before,
					AfterValue:  strings.TrimSpace(h),
					Traceability: Traceability{
						Source: "master",
						Path:   fmt.Sprintf("cv.sections.experience[%s].highlights[%d]", company, i),
					},
					Status: "pending",
				})
			}
		}
	}

	// 3. Skill group additions
	for _, add := range payload.SkillGroupsToAdd {
		proposals = append(proposals, EditProposal{
			FieldType:   "skill_group_add",
			FieldKey:    add.Label,
			BeforeValue: "",
			AfterValue:  add.Details,
			Traceability: Traceability{
				Source: "master",
				Path:   fmt.Sprintf("cv.sections.skills[%s]", add.Label),
			},
			Status: "pending",
		})
	}

	// 4. Skill group removals
	for _, label := range payload.SkillGroupsToRemove {
		proposals = append(proposals, EditProposal{
			FieldType:   "skill_group_remove",
			FieldKey:    label,
			BeforeValue: label,
			AfterValue:  "",
			Traceability: Traceability{
				Source: "master",
				Path:   fmt.Sprintf("cv.sections.skills[%s]", label),
			},
			Status: "pending",
		})
	}

	// 5. Skill changes (per-token add/remove within existing groups)
	for _, sc := range payload.SkillChanges {
		if sc.AddTokens != "" {
			for _, t := range tokens(sc.AddTokens) {
				proposals = append(proposals, EditProposal{
					FieldType:   "skill_change",
					FieldKey:    fmt.Sprintf("%s::%s", sc.GroupLabel, t),
					BeforeValue: "",
					AfterValue:  t,
					Traceability: Traceability{
						Source: "master",
						Path:   fmt.Sprintf("cv.sections.skills[%s]", sc.GroupLabel),
					},
					Status: "pending",
				})
			}
		}
		if sc.RemoveTokens != "" {
			for _, t := range tokens(sc.RemoveTokens) {
				proposals = append(proposals, EditProposal{
					FieldType:   "skill_change",
					FieldKey:    fmt.Sprintf("%s::%s", sc.GroupLabel, t),
					BeforeValue: t,
					AfterValue:  "",
					Traceability: Traceability{
						Source: "master",
						Path:   fmt.Sprintf("cv.sections.skills[%s]", sc.GroupLabel),
					},
					Status: "pending",
				})
			}
		}
	}

	return proposals
}
