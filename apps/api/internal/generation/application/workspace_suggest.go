package application

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/generation/domain"
)

// experienceCompanies lists the master's experience company names in master
// order — the suggestion stage's entry identities (contracts/llm-contracts.md
// §2), never its bullet text.
func experienceCompanies(master domain.RendercvMaster) []string {
	entries := domain.AsSliceOfMaps(domain.CvSections(master)["experience"])
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, domain.StringField(e, "company"))
	}
	return out
}

// buildSuggestionItems turns a raw SuggestionSet into the AI-origin items
// each section will receive (T054): entries whose company does not match a
// master company are dropped first (a suggestion the model attached to an
// employer the candidate never had is not a suggestion for THIS profile),
// then domain.SuppressDuplicateSuggestions removes anything that duplicates
// a master bullet or skill (T052, FR-017).
//
// Rank/Position are 0-based WITHIN the AI group here; persistSuggestions
// offsets them past each section's existing item count so every AI item is
// ranked after every profile item in its section, per contracts §2 rule 4.
// Every returned item has Selected = false (FR-013/SC-004, T050) — there is
// no path in this function that sets it any other way.
func buildSuggestionItems(suggestions domain.SuggestionSet, master domain.RendercvMaster) (experience map[string][]domain.Item, skills []domain.Item) {
	companyByKey := map[string]string{}
	for _, company := range experienceCompanies(master) {
		companyByKey[strings.ToLower(strings.TrimSpace(company))] = company
	}

	matched := domain.SuggestionSet{Skills: suggestions.Skills}
	for _, exp := range suggestions.Experience {
		canonical, ok := companyByKey[strings.ToLower(strings.TrimSpace(exp.Company))]
		if !ok {
			continue
		}
		matched.Experience = append(matched.Experience, domain.ExperienceSuggestions{Company: canonical, Bullets: exp.Bullets})
	}

	suppressed := domain.SuppressDuplicateSuggestions(matched, master)

	experience = map[string][]domain.Item{}
	for _, exp := range suppressed.Experience {
		if len(exp.Bullets) == 0 {
			continue
		}
		items := make([]domain.Item, 0, len(exp.Bullets))
		for i, b := range exp.Bullets {
			items = append(items, domain.Item{
				Origin: domain.OriginAI, Kind: domain.ItemKindAchievement,
				SourceText: b, Rank: i, Position: i, Selected: false,
			})
		}
		experience[exp.Company] = items
	}

	for i, sk := range suppressed.Skills {
		skills = append(skills, domain.Item{
			Origin: domain.OriginAI, Kind: domain.ItemKindSkillGroup,
			SourceText: sk, Rank: i, Position: i, Selected: false,
		})
	}
	return experience, skills
}

// offsetSuggestionItems shifts a 0-based AI-group item list past a section's
// existing item count, so persisted AI items always sort after every
// profile item already in that section.
func offsetSuggestionItems(items []domain.Item, offset int) []domain.Item {
	out := make([]domain.Item, len(items))
	for i, it := range items {
		it.Rank += offset
		it.Position += offset
		out[i] = it
	}
	return out
}

// persistSuggestions writes buildSuggestionItems' output: each matched
// experience section gets its suggested bullets appended after its existing
// items, and the skills section gets its suggested skills appended the same
// way. A section with no survivors for it is left untouched — the client
// renders that as the suggestion group's empty state (T055), not an error.
func (s *Service) persistSuggestions(ctx context.Context, runID pgtype.UUID, experience map[string][]domain.Item, skills []domain.Item) error {
	if len(experience) == 0 && len(skills) == 0 {
		return nil
	}
	sections, err := s.q.ListSectionsByRun(ctx, runID)
	if err != nil {
		return err
	}
	items, err := s.q.ListItemsByRun(ctx, runID)
	if err != nil {
		return err
	}
	countBySection := make(map[pgtype.UUID]int, len(sections))
	for _, it := range items {
		countBySection[it.SectionID]++
	}

	for _, sec := range sections {
		switch {
		case sec.Kind == string(domain.SectionKindExperience) && sec.EntryKey != nil:
			suggested, ok := experience[*sec.EntryKey]
			if !ok || len(suggested) == 0 {
				continue
			}
			if err := s.persistWorkspaceItems(ctx, sec.ID, offsetSuggestionItems(suggested, countBySection[sec.ID])); err != nil {
				return err
			}
		case sec.Kind == string(domain.SectionKindSkills) && len(skills) > 0:
			if err := s.persistWorkspaceItems(ctx, sec.ID, offsetSuggestionItems(skills, countBySection[sec.ID])); err != nil {
				return err
			}
		}
	}
	return nil
}
