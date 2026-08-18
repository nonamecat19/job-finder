package domain

import (
	"strconv"
	"strings"
)

func SeedFromMaster(master RendercvMaster, cfg ShapeConfig) []Section {
	out := make([]Section, 0, 2)
	position := 0

	if cfg.SummaryEnabled {
		out = append(out, Section{
			Kind:     SectionKindSummary,
			Position: position,
			State:    SectionRunning,
			Enabled:  true,
			Items:    []Item{},
		})
		position++
	}

	sections := CvSections(master)
	if cfg.ExperienceEnabled {
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
				Enabled:     true,
				Items:       items,
			})
			position++
		}
	}

	if cfg.SkillsEnabled {
		out = append(out, Section{
			Kind:     SectionKindSkills,
			Position: position,
			State:    SectionReady,
			Enabled:  true,
			Items:    SeedSkillItems(AsSliceOfMaps(sections["skills"]), nil, cfg.SkillsMaxGroups),
		})
		position++
	}

	if projects := AsSliceOfMaps(sections["projects"]); cfg.ProjectsEnabled && len(projects) > 0 {
		out = append(out, Section{
			Kind:        SectionKindProjects,
			Position:    position,
			TargetCount: cfg.ProjectsMax,
			State:       SectionReady,
			Enabled:     true,
			Items:       SeedProjectItems(projects, nil, cfg.ProjectsMax),
		})
		position++
	}

	if certs := AsSliceOfMaps(sections["certifications"]); cfg.CertificationsEnabled && len(certs) > 0 {
		out = append(out, Section{
			Kind:     SectionKindCertifications,
			Position: position,
			State:    SectionReady,
			Enabled:  true,
			Items:    SeedCertificationItems(certs, nil),
		})
		position++
	}

	if edu := AsSliceOfMaps(sections["education"]); cfg.EducationEnabled && len(edu) > 0 {
		out = append(out, Section{
			Kind:     SectionKindEducation,
			Position: position,
			State:    SectionReady,
			Enabled:  true,
			Items:    SeedEducationItems(edu, nil),
		})
	}

	return out
}

func SeedProjectItems(projects []map[string]any, order []int, projectsMax int) []Item {
	seen := make(map[int]bool, len(projects))
	ranked := make([]int, 0, len(projects))
	for _, idx := range order {
		if idx < 0 || idx >= len(projects) || seen[idx] {
			continue
		}
		seen[idx] = true
		ranked = append(ranked, idx)
	}
	for i := range projects {
		if !seen[i] {
			ranked = append(ranked, i)
		}
	}

	slots := len(ranked)
	if projectsMax > 0 {
		slots = projectsMax
	}

	items := make([]Item, 0, len(projects))
	for i, source := range ranked {
		idx := source
		items = append(items, Item{
			Origin:      OriginProfile,
			Kind:        ItemKindProject,
			SourceIndex: &idx,
			SourceText:  projectItemText(projects[source]),
			Rank:        i,
			Position:    i,
			Selected:    i < slots,
		})
	}
	return items
}

func projectItemText(p map[string]any) string {
	name := strings.TrimSpace(StringField(p, "name"))
	if inner, _, found := strings.Cut(strings.TrimPrefix(name, "["), "]("); found {
		name = inner
	}
	n := len(StringSliceField(p, "highlights"))
	if n == 0 {
		return name
	}
	if n == 1 {
		return name + " · 1 bullet"
	}
	return name + " · " + strconv.Itoa(n) + " bullets"
}

func SeedCertificationItems(certs []map[string]any, order []int) []Item {
	return seedWholeSectionItems(certs, order, ItemKindCertification, skillGroupText)
}

func SeedEducationItems(edu []map[string]any, order []int) []Item {
	return seedWholeSectionItems(edu, order, ItemKindEducation, educationItemText)
}

func seedWholeSectionItems(entries []map[string]any, order []int, kind ItemKind, text func(map[string]any) string) []Item {
	seen := make(map[int]bool, len(entries))
	ranked := make([]int, 0, len(entries))
	for _, idx := range order {
		if idx < 0 || idx >= len(entries) || seen[idx] {
			continue
		}
		seen[idx] = true
		ranked = append(ranked, idx)
	}
	for i := range entries {
		if !seen[i] {
			ranked = append(ranked, i)
		}
	}

	items := make([]Item, 0, len(entries))
	for i, source := range ranked {
		idx := source
		items = append(items, Item{
			Origin:      OriginProfile,
			Kind:        kind,
			SourceIndex: &idx,
			SourceText:  text(entries[source]),
			Rank:        i,
			Position:    i,
			Selected:    true,
		})
	}
	return items
}

func educationItemText(e map[string]any) string {
	degree := strings.TrimSpace(StringField(e, "degree"))
	institution := strings.TrimSpace(StringField(e, "institution"))
	date := educationDate(e)

	head := degree
	if institution != "" {
		if head != "" {
			head += " — " + institution
		} else {
			head = institution
		}
	}
	if date != "" {
		if head != "" {
			return head + ", " + date
		}
		return date
	}
	return head
}

func educationDate(e map[string]any) string {
	if d := strings.TrimSpace(StringField(e, "date")); d != "" {
		return d
	}
	start := strings.TrimSpace(StringField(e, "start_date"))
	if start == "" {
		return ""
	}
	end := strings.TrimSpace(StringField(e, "end_date"))
	if end == "" {
		end = "present"
	}
	return start + "–" + end
}

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
