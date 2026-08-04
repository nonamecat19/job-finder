package tailoring

import (
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/generation/domain"
)

type ProposalValidator func(proposal domain.EditProposal, baseline domain.RendercvMaster) error

var validators = map[string]ProposalValidator{
	"summary":               validateSummary,
	"experience_highlights": validateExperienceHighlights,
	"skill_change":          validateSkillChange,
	"skill_group_add":       validateSkillGroupAdd,
	"skill_group_remove":    validateSkillGroupRemove,
}

func ValidateProposal(proposal domain.EditProposal, baseline domain.RendercvMaster) error {
	v, ok := validators[proposal.FieldType]
	if !ok {
		return fmt.Errorf("unknown field_type %q", proposal.FieldType)
	}
	return v(proposal, baseline)
}

func validateSummary(p domain.EditProposal, _ domain.RendercvMaster) error {
	if p.FieldKey != "summary" {
		return fmt.Errorf("summary proposal must have fieldKey='summary'")
	}
	return nil
}

func validateExperienceHighlights(p domain.EditProposal, baseline domain.RendercvMaster) error {
	parts := strings.SplitN(p.FieldKey, "::", 2)
	if len(parts) != 2 {
		return fmt.Errorf("experience_highlights fieldKey must be 'company::index'")
	}
	sections := domain.CvSections(baseline)
	found := false
	for _, e := range domain.AsSliceOfMaps(sections["experience"]) {
		if domain.StringField(e, "company") == parts[0] {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("company %q not in baseline", parts[0])
	}
	return nil
}

func validateSkillChange(p domain.EditProposal, baseline domain.RendercvMaster) error {
	parts := strings.SplitN(p.FieldKey, "::", 2)
	if len(parts) != 2 {
		return fmt.Errorf("skill_change fieldKey must be 'groupLabel::token'")
	}
	sections := domain.CvSections(baseline)
	found := false
	for _, g := range domain.AsSliceOfMaps(sections["skills"]) {
		if domain.StringField(g, "label") == parts[0] {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill group %q not in baseline", parts[0])
	}
	return nil
}

func validateSkillGroupAdd(p domain.EditProposal, baseline domain.RendercvMaster) error {
	if p.BeforeValue != "" {
		return fmt.Errorf("skill_group_add before_value must be empty")
	}
	if p.AfterValue == "" {
		return fmt.Errorf("skill_group_add after_value must not be empty")
	}
	allowed := domain.MasterSkillTokens(baseline)
	for _, t := range tokenize(p.AfterValue) {
		if !allowed[t] {
			return fmt.Errorf("skill_group_add token %q not in master skill tokens", t)
		}
	}
	return nil
}

func validateSkillGroupRemove(p domain.EditProposal, baseline domain.RendercvMaster) error {
	if p.AfterValue != "" {
		return fmt.Errorf("skill_group_remove after_value must be empty")
	}
	sections := domain.CvSections(baseline)
	found := false
	for _, g := range domain.AsSliceOfMaps(sections["skills"]) {
		if domain.StringField(g, "label") == p.FieldKey {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill group %q not in baseline", p.FieldKey)
	}
	return nil
}

func tokenize(details string) []string {
	parts := strings.FieldsFunc(details, func(r rune) bool { return r == ',' || r == '/' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if n := strings.ToLower(strings.TrimSpace(p)); n != "" {
			out = append(out, n)
		}
	}
	return out
}
