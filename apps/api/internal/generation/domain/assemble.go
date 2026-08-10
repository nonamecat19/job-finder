package domain

import "strings"

// Assemble builds the document to export from the run's master snapshot and
// the user's current selection: exactly the selected, available items, in
// `position` order, with the wording the workspace displayed (FR-018).
//
// It follows MergeTailored's discipline — deep-clone first, then write only
// section *contents*. It never creates or removes a section key and never
// touches `_order`, so an entry's company, dates, education and every other
// out-of-scope field reach the PDF byte-identical to the master. Experience
// entries stay in master order; an entry with nothing selected keeps its
// place with an empty highlight list rather than disappearing.
//
// No model call, no trimming, no padding: the selection is the shape.
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

	for _, sec := range sections {
		selected := selectedInOrder(sec)
		switch sec.Kind {
		case SectionKindSummary:
			// Only written when the master already has a summary section: a
			// key this document never had is a new section, not a content
			// edit, and adding one would place it wherever `_order` did not
			// ask for it.
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
		}
	}

	return doc, nil
}

// selectedInOrder is the one place the inclusion rule lives: selected, not
// unavailable, in `position` order. Sorted by insertion rather than sort.Slice
// so equal positions keep their incoming order.
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

// assembleSkillGroups rebuilds the skills section from the selected items.
// A profile-origin item resolves through its SourceIndex back to the master's
// own group map, so every field on that group (and not just label/details)
// survives verbatim; an AI-origin item, which has no master group behind it,
// is parsed from its "Label: details" display text.
func assembleSkillGroups(masterGroups []map[string]any, selected []Item) []any {
	out := make([]any, 0, len(selected))
	for _, it := range selected {
		if it.Origin == OriginProfile {
			if it.SourceIndex != nil && *it.SourceIndex >= 0 && *it.SourceIndex < len(masterGroups) {
				out = append(out, masterGroups[*it.SourceIndex])
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
