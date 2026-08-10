package domain

import "strings"

// SeedFromMaster builds the workspace's starting sections and items directly
// from the master resume, with no model call: one summary section (empty —
// filled by the summary stage), one experience section per master entry
// keyed by exact company name in master order, and one skills section.
//
// Every item is origin='profile', rank and position both equal the source
// index (master order), and the top min(N, A) items per experience entry are
// selected — this is not throwaway scaffolding: FR-010 requires master-order
// seeding as the fallback when a ranking is rejected, so this function is the
// path both the "no ranking yet" (this phase) and "ranking rejected twice"
// cases share.
//
// An entry with zero master bullets still gets its own section with an empty
// item list, never skipped — the client renders it as an explicit empty
// state rather than a missing block.
func SeedFromMaster(master RendercvMaster, cfg ShapeConfig) []Section {
	out := make([]Section, 0, 2)
	position := 0

	out = append(out, Section{
		Kind:     SectionKindSummary,
		Position: position,
		State:    SectionRunning,
		Items:    []Item{},
	})
	position++

	sections := CvSections(master)
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		company := StringField(e, "company")
		label := entryLabel(e)
		highlights := StringSliceField(e, "highlights")

		n := cfg.ExperienceBulletsMin
		if n > len(highlights) {
			n = len(highlights)
		}

		items := make([]Item, 0, len(highlights))
		for i, h := range highlights {
			idx := i
			items = append(items, Item{
				Origin:      OriginProfile,
				Kind:        ItemKindAchievement,
				SourceIndex: &idx,
				SourceText:  h,
				Rank:        i,
				Position:    i,
				Selected:    i < n,
			})
		}

		key := company
		out = append(out, Section{
			Kind:        SectionKindExperience,
			EntryKey:    &key,
			EntryLabel:  &label,
			Position:    position,
			TargetCount: cfg.ExperienceBulletsMin,
			State:       SectionReady,
			Items:       items,
		})
		position++
	}

	skillGroups := AsSliceOfMaps(sections["skills"])
	skillItems := make([]Item, 0, len(skillGroups))
	for i, g := range skillGroups {
		idx := i
		skillItems = append(skillItems, Item{
			Origin:      OriginProfile,
			Kind:        ItemKindSkillGroup,
			SourceIndex: &idx,
			SourceText:  skillGroupText(g),
			Rank:        i,
			Position:    i,
			Selected:    true,
		})
	}
	out = append(out, Section{
		Kind:     SectionKindSkills,
		Position: position,
		State:    SectionReady,
		Items:    skillItems,
	})

	return out
}

// entryLabel is the display label copied from master: "Senior Engineer ·
// 2021–2024", degrading gracefully when either half is missing.
func entryLabel(e map[string]any) string {
	position := StringField(e, "position")
	start := StringField(e, "start_date")
	end := StringField(e, "end_date")
	if start == "" {
		return position
	}
	if end == "" {
		end = "present"
	}
	dates := start + "–" + end
	if position == "" {
		return dates
	}
	return position + " · " + dates
}

// skillGroupText renders a skill group as "Label: details" for display —
// data-model.md §3's format for a skills-section item's source_text.
func skillGroupText(g map[string]any) string {
	label := strings.TrimSpace(StringField(g, "label"))
	details := strings.TrimSpace(StringField(g, "details"))
	if label == "" {
		return details
	}
	if details == "" {
		return label
	}
	return label + ": " + details
}
