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

	out = append(out, Section{
		Kind:     SectionKindSkills,
		Position: position,
		State:    SectionReady,
		Items:    SeedSkillItems(AsSliceOfMaps(sections["skills"]), nil, cfg.SkillsMaxGroups),
	})

	return out
}

// SeedSkillItems builds the skills section's items from the master's skill
// groups, in the order `groupOrder` names (a verified RankedSkills.GroupOrder,
// or nil for master order). It is SeedRankedItems' counterpart for skills, and
// obeys the same two rules:
//
//   - The result is always a permutation of `groups`. Nothing is added,
//     reworded or dropped — the model supplies an order over indices, never
//     text, so a lost or invented group is not expressible.
//   - maxGroups is a *selection* boundary, not a removal (FR-011, US4 AS-2):
//     the groups past it stay in the list as unselected items the user can
//     promote, rather than disappearing the way ApplyHardLimits' render-time
//     cap makes them.
//
// Pinned groups ("Spoken Languages") keep the position the candidate authored
// them at and are always selected: they are a fact about the candidate, not a
// tailoring target, so neither the ranking nor the cap moves them — the same
// exemption capSkillGroups gives them at render time.
func SeedSkillItems(groups []map[string]any, groupOrder []int, maxGroups int) []Item {
	pinned := make([]bool, len(groups))
	pinnedCount := 0
	for i, g := range groups {
		if IsPinnedSkillGroup(StringField(g, "label")) {
			pinned[i] = true
			pinnedCount++
		}
	}

	seen := make(map[int]bool, len(groups))
	ranked := make([]int, 0, len(groups))
	for _, idx := range groupOrder {
		if idx < 0 || idx >= len(groups) || seen[idx] || pinned[idx] {
			continue
		}
		seen[idx] = true
		ranked = append(ranked, idx)
	}
	for i := range groups {
		if !pinned[i] && !seen[i] {
			ranked = append(ranked, i)
		}
	}

	slots := len(ranked)
	if maxGroups > 0 {
		slots = maxGroups - pinnedCount
		if slots < 0 {
			slots = 0
		}
	}

	items := make([]Item, 0, len(groups))
	next := 0
	for i := range groups {
		source, selected := i, true
		if !pinned[i] {
			source = ranked[next]
			next++
			selected = next <= slots
		}
		idx := source
		items = append(items, Item{
			Origin:      OriginProfile,
			Kind:        ItemKindSkillGroup,
			SourceIndex: &idx,
			SourceText:  skillGroupText(groups[source]),
			Rank:        i,
			Position:    i,
			Selected:    selected,
		})
	}
	return items
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
