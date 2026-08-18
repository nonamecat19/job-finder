package application

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/generation/domain"
)

func experienceCompanies(master domain.RendercvMaster) []string {
	entries := domain.AsSliceOfMaps(domain.CvSections(master)["experience"])
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, domain.StringField(e, "company"))
	}
	return out
}

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

func offsetSuggestionItems(items []domain.Item, offset int) []domain.Item {
	out := make([]domain.Item, len(items))
	for i, it := range items {
		it.Rank += offset
		it.Position += offset
		out[i] = it
	}
	return out
}

func (s *Service) persistSuggestions(ctx context.Context, runID pgtype.UUID, experience map[string][]domain.Item, skills []domain.Item, target map[pgtype.UUID]bool, oldBySection map[pgtype.UUID][]domain.Item) error {
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
		if target != nil && !target[sec.ID] {
			continue
		}
		switch {
		case sec.Kind == string(domain.SectionKindExperience) && sec.EntryKey != nil:
			suggested, ok := experience[*sec.EntryKey]
			if !ok || len(suggested) == 0 {
				continue
			}
			final := preserveMatchedSelections(oldBySection[sec.ID], offsetSuggestionItems(suggested, countBySection[sec.ID]))
			if err := s.persistWorkspaceItems(ctx, sec.ID, final); err != nil {
				return err
			}
		case sec.Kind == string(domain.SectionKindSkills) && len(skills) > 0:
			final := preserveMatchedSelections(oldBySection[sec.ID], offsetSuggestionItems(skills, countBySection[sec.ID]))
			if err := s.persistWorkspaceItems(ctx, sec.ID, final); err != nil {
				return err
			}
		}
	}
	return nil
}
