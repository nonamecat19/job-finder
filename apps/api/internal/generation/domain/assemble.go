package domain

import "strings"

func Assemble(master RendercvMaster, sections []Section) (RendercvMaster, error) {
	doc, err := DeepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	cvSections := CvSections(doc)
	if cvSections == nil {
		return doc, nil
	}

	byCompany := map[string]map[string]any{}
	for _, e := range AsSliceOfMaps(cvSections["experience"]) {
		byCompany[norm(StringField(e, "company"))] = e
	}
	masterSkills := AsSliceOfMaps(cvSections["skills"])
	masterProjects := AsSliceOfMaps(cvSections["projects"])
	masterCertifications := AsSliceOfMaps(cvSections["certifications"])
	masterEducation := AsSliceOfMaps(cvSections["education"])

	for _, sec := range sections {

		if !sec.Enabled {
			switch sec.Kind {
			case SectionKindExperience:
				if sec.EntryKey != nil {
					removeExperienceEntry(cvSections, *sec.EntryKey)
				}
			case SectionKindSummary:
				RemoveSection(cvSections, "summary")
			case SectionKindSkills:
				RemoveSection(cvSections, "skills")
			case SectionKindProjects:
				RemoveSection(cvSections, "projects")
			case SectionKindCertifications:
				RemoveSection(cvSections, "certifications")
			case SectionKindEducation:
				RemoveSection(cvSections, "education")
			}
			continue
		}

		selected := selectedInOrder(sec)
		switch sec.Kind {
		case SectionKindSummary:

			if _, ok := cvSections["summary"]; !ok {
				continue
			}
			texts := make([]any, 0, len(selected))
			for _, it := range selected {
				if t := strings.TrimSpace(it.EffectiveText()); t != "" {
					texts = append(texts, t)
				}
			}
			cvSections["summary"] = texts
		case SectionKindExperience:
			if sec.EntryKey == nil {
				continue
			}
			entry, ok := byCompany[norm(*sec.EntryKey)]
			if !ok {
				continue
			}
			texts := make([]string, 0, len(selected))
			for _, it := range selected {
				texts = append(texts, it.EffectiveText())
			}
			entry["highlights"] = cleanHighlights(texts)
		case SectionKindSkills:
			if _, ok := cvSections["skills"]; !ok {
				continue
			}
			cvSections["skills"] = assembleSkillGroups(masterSkills, selected)
		case SectionKindProjects:
			if _, ok := cvSections["projects"]; !ok {
				continue
			}
			cvSections["projects"] = assembleBySourceIndex(masterProjects, selected)
		case SectionKindCertifications:
			if _, ok := cvSections["certifications"]; !ok {
				continue
			}
			cvSections["certifications"] = assembleBySourceIndex(masterCertifications, selected)
		case SectionKindEducation:
			if _, ok := cvSections["education"]; !ok {
				continue
			}
			cvSections["education"] = assembleBySourceIndex(masterEducation, selected)
		}
	}

	return doc, nil
}

func removeExperienceEntry(cvSections map[string]any, entryKey string) {
	key := norm(entryKey)
	entries := AsSliceOfMaps(cvSections["experience"])
	filtered := make([]any, 0, len(entries))
	for _, e := range entries {
		if norm(StringField(e, "company")) == key {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		RemoveSection(cvSections, "experience")
		return
	}
	cvSections["experience"] = filtered
}

func selectedInOrder(sec Section) []Item {
	out := make([]Item, 0, len(sec.Items))
	for _, it := range sec.Items {
		if !it.Selected || it.Unavailable {
			continue
		}
		i := len(out)
		for i > 0 && out[i-1].Position > it.Position {
			i--
		}
		out = append(out, Item{})
		copy(out[i+1:], out[i:])
		out[i] = it
	}
	return out
}

func assembleBySourceIndex(master []map[string]any, selected []Item) []any {
	out := make([]any, 0, len(selected))
	for _, it := range selected {
		if it.Origin != OriginProfile || it.SourceIndex == nil {
			continue
		}
		if idx := *it.SourceIndex; idx >= 0 && idx < len(master) {
			out = append(out, master[idx])
		}
	}
	return out
}

func applyDroppedEntries(group map[string]any, it Item) (map[string]any, bool) {
	if len(it.DroppedEntries) == 0 {
		return group, true
	}
	dropped := make(map[string]bool, len(it.DroppedEntries))
	for _, d := range it.DroppedEntries {
		dropped[strings.TrimSpace(d)] = true
	}
	kept := make([]string, 0, len(it.DroppedEntries))
	for _, e := range splitSkillEntries(StringField(group, "details")) {
		if !dropped[e] {
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		return nil, false
	}
	out := make(map[string]any, len(group))
	for k, v := range group {
		out[k] = v
	}
	out["details"] = strings.Join(kept, ", ")
	return out, true
}

func assembleSkillGroups(masterGroups []map[string]any, selected []Item) []any {
	out := make([]any, 0, len(selected))
	for _, it := range selected {
		if it.Origin == OriginProfile {
			if it.SourceIndex != nil && *it.SourceIndex >= 0 && *it.SourceIndex < len(masterGroups) {
				group, keep := applyDroppedEntries(masterGroups[*it.SourceIndex], it)
				if keep {
					out = append(out, group)
				}
			}
			continue
		}
		label, details, _ := strings.Cut(it.EffectiveText(), ":")
		group := map[string]any{"label": strings.TrimSpace(label)}
		if d := strings.TrimSpace(details); d != "" {
			group["details"] = d
		}
		out = append(out, group)
	}
	return out
}
