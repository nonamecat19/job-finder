package generation

import (
	"fmt"

	"github.com/job-finder/api/internal/dto"
)

// orderedSectionKeys returns cv.sections keys in the fixed resume order
// (see domain.SortByDefaultSectionOrder) — same canonical order
// PrepareMasterForMarshal enforces, but non-destructive (does not
// consume/delete the order key). Ignores any captured _order: the business
// requirement is a resume structure that's always the same, not whatever
// order the source master config's sections happen to be authored in.
func orderedSectionKeys(sections map[string]any) []string {
	keys := make([]string, 0, len(sections))
	for k := range sections {
		if k == sectionOrderKey {
			continue
		}
		keys = append(keys, k)
	}
	return sortByDefaultSectionOrder(keys)
}

func hasKey(m map[string]any, key string) bool {
	v, ok := m[key]
	return ok && v != nil
}

// inferEntryType classifies a section's entries by field shape. Sections are
// homogeneous in RenderCV (one entry type per section), so the first
// classifiable entry decides the whole section. Returns "" when no entry
// matches a known shape (an unrecognized section, FR-009).
func inferEntryType(rawEntries []any) dto.EntryType {
	for _, re := range rawEntries {
		if _, ok := re.(string); ok {
			return dto.EntryText
		}
		m, ok := re.(map[string]any)
		if !ok {
			continue
		}
		switch {
		case hasKey(m, "institution") || hasKey(m, "degree") || hasKey(m, "area"):
			return dto.EntryEducation
		case hasKey(m, "company") || hasKey(m, "position"):
			return dto.EntryExperience
		case hasKey(m, "title") && hasKey(m, "authors"):
			return dto.EntryPublication
		case hasKey(m, "label") && hasKey(m, "details"):
			return dto.EntryOneLine
		case hasKey(m, "bullet"):
			return dto.EntryBullet
		case hasKey(m, "number"):
			return dto.EntryNumbered
		case hasKey(m, "reversed_number"):
			return dto.EntryReversedNumbered
		case hasKey(m, "name"):
			return dto.EntryNormal
		}
	}
	return ""
}

func strPtr(m map[string]any, key string, consumed map[string]bool) *string {
	if v, ok := m[key]; ok {
		consumed[key] = true
		if s, ok := v.(string); ok && s != "" {
			return &s
		}
	}
	return nil
}

func strSlice(m map[string]any, key string, consumed map[string]bool) []string {
	if _, ok := m[key]; ok {
		consumed[key] = true
	}
	return StringSliceField(m, key)
}

// mapEntry converts one raw RenderCV entry (string, for text entries, or a
// map) into a typed dto.Entry, retaining any fields not recognized for the
// section's entryType under Unrecognized (FR-009).
func mapEntry(entryType dto.EntryType, raw any) dto.Entry {
	if entryType == dto.EntryText {
		if s, ok := raw.(string); ok {
			return dto.Entry{Text: &s}
		}
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return dto.Entry{Unrecognized: map[string]any{"_value": raw}}
	}

	e := dto.Entry{}
	consumed := map[string]bool{}

	switch entryType {
	case dto.EntryEducation:
		e.Institution = strPtr(m, "institution", consumed)
		e.Area = strPtr(m, "area", consumed)
		e.Degree = strPtr(m, "degree", consumed)
		e.Date = strPtr(m, "date", consumed)
		e.StartDate = strPtr(m, "start_date", consumed)
		e.EndDate = strPtr(m, "end_date", consumed)
		e.Location = strPtr(m, "location", consumed)
		e.Summary = strPtr(m, "summary", consumed)
		e.Highlights = strSlice(m, "highlights", consumed)
	case dto.EntryExperience:
		e.Company = strPtr(m, "company", consumed)
		e.Position = strPtr(m, "position", consumed)
		e.Date = strPtr(m, "date", consumed)
		e.StartDate = strPtr(m, "start_date", consumed)
		e.EndDate = strPtr(m, "end_date", consumed)
		e.Location = strPtr(m, "location", consumed)
		e.Summary = strPtr(m, "summary", consumed)
		e.Highlights = strSlice(m, "highlights", consumed)
	case dto.EntryNormal:
		e.Name = strPtr(m, "name", consumed)
		e.URL = strPtr(m, "url", consumed)
		e.Date = strPtr(m, "date", consumed)
		e.StartDate = strPtr(m, "start_date", consumed)
		e.EndDate = strPtr(m, "end_date", consumed)
		e.Location = strPtr(m, "location", consumed)
		e.Summary = strPtr(m, "summary", consumed)
		e.Highlights = strSlice(m, "highlights", consumed)
	case dto.EntryPublication:
		e.Title = strPtr(m, "title", consumed)
		e.Authors = strSlice(m, "authors", consumed)
		e.Doi = strPtr(m, "doi", consumed)
		e.URL = strPtr(m, "url", consumed)
		e.Journal = strPtr(m, "journal", consumed)
		e.Date = strPtr(m, "date", consumed)
		e.Summary = strPtr(m, "summary", consumed)
	case dto.EntryOneLine:
		e.Label = strPtr(m, "label", consumed)
		e.Details = strPtr(m, "details", consumed)
	case dto.EntryBullet:
		e.Bullet = strPtr(m, "bullet", consumed)
	case dto.EntryNumbered:
		e.Number = strPtr(m, "number", consumed)
	case dto.EntryReversedNumbered:
		e.ReversedNumber = strPtr(m, "reversed_number", consumed)
	}

	if len(consumed) < len(m) {
		leftover := map[string]any{}
		for k, v := range m {
			if !consumed[k] {
				leftover[k] = v
			}
		}
		e.Unrecognized = leftover
	}
	return e
}

// MasterToResume converts the generic RendercvMaster (map[string]any) into
// the typed, structured dto.Resume the client edits. Never drops data: any
// section/entry that doesn't match a known shape is retained under
// Unrecognized (FR-009).
func MasterToResume(master RendercvMaster) (dto.Resume, error) {
	resume := dto.Resume{Sections: []dto.Section{}}
	cv, _ := master["cv"].(map[string]any)
	if cv == nil {
		return resume, nil
	}

	resume.Name = StringField(cv, "name")
	consumedTop := map[string]bool{"name": true, "sections": true}
	resume.Headline = strPtr(cv, "headline", consumedTop)
	resume.Location = strPtr(cv, "location", consumedTop)
	resume.Email = strPtr(cv, "email", consumedTop)
	resume.Phone = strPtr(cv, "phone", consumedTop)
	resume.Website = strPtr(cv, "website", consumedTop)
	resume.Photo = strPtr(cv, "photo", consumedTop)

	if raw, ok := cv["social_networks"]; ok {
		consumedTop["social_networks"] = true
		for _, m := range AsSliceOfMaps(raw) {
			resume.SocialNetworks = append(resume.SocialNetworks, dto.SocialNetwork{
				Network:  StringField(m, "network"),
				Username: StringField(m, "username"),
			})
		}
	}
	if raw, ok := cv["custom_connections"]; ok {
		consumedTop["custom_connections"] = true
		for _, m := range AsSliceOfMaps(raw) {
			resume.CustomConnections = append(resume.CustomConnections, dto.CustomConnection{
				Placeholder: StringField(m, "placeholder"),
				URL:         StringField(m, "url"),
				Icon:        StringField(m, "fontawesome_icon"),
			})
		}
	}

	sections, _ := cv["sections"].(map[string]any)
	if sections != nil {
		for _, key := range orderedSectionKeys(sections) {
			rawEntries, _ := sections[key].([]any)
			entryType := inferEntryType(rawEntries)
			section := dto.Section{Name: key, EntryType: entryType}
			for _, re := range rawEntries {
				section.Entries = append(section.Entries, mapEntry(entryType, re))
			}
			resume.Sections = append(resume.Sections, section)
		}
	}

	leftover := map[string]any{}
	for k, v := range cv {
		if !consumedTop[k] {
			leftover[k] = v
		}
	}
	if len(leftover) > 0 {
		resume.Unrecognized = leftover
	}

	return resume, nil
}

func setOrDelete(m map[string]any, key string, v *string) {
	if v != nil && *v != "" {
		m[key] = *v
	} else {
		delete(m, key)
	}
}

func setSliceOrDelete(m map[string]any, key string, v []string) {
	if len(v) > 0 {
		out := make([]any, len(v))
		for i, s := range v {
			out[i] = s
		}
		m[key] = out
	} else {
		delete(m, key)
	}
}

// entryToRaw is the inverse of mapEntry: renders a typed dto.Entry back into
// the plain YAML-shaped value (string for text entries, map otherwise) for
// its section's entryType, re-attaching any Unrecognized fields verbatim.
func entryToRaw(entryType dto.EntryType, e dto.Entry) any {
	if entryType == dto.EntryText {
		if e.Text != nil {
			return *e.Text
		}
		return ""
	}

	m := map[string]any{}
	for k, v := range e.Unrecognized {
		m[k] = v
	}

	switch entryType {
	case dto.EntryEducation:
		setOrDelete(m, "institution", e.Institution)
		setOrDelete(m, "area", e.Area)
		setOrDelete(m, "degree", e.Degree)
		setOrDelete(m, "date", e.Date)
		setOrDelete(m, "start_date", e.StartDate)
		setOrDelete(m, "end_date", e.EndDate)
		setOrDelete(m, "location", e.Location)
		setOrDelete(m, "summary", e.Summary)
		setSliceOrDelete(m, "highlights", e.Highlights)
	case dto.EntryExperience:
		setOrDelete(m, "company", e.Company)
		setOrDelete(m, "position", e.Position)
		setOrDelete(m, "date", e.Date)
		setOrDelete(m, "start_date", e.StartDate)
		setOrDelete(m, "end_date", e.EndDate)
		setOrDelete(m, "location", e.Location)
		setOrDelete(m, "summary", e.Summary)
		setSliceOrDelete(m, "highlights", e.Highlights)
	case dto.EntryNormal:
		setOrDelete(m, "name", e.Name)
		setOrDelete(m, "url", e.URL)
		setOrDelete(m, "date", e.Date)
		setOrDelete(m, "start_date", e.StartDate)
		setOrDelete(m, "end_date", e.EndDate)
		setOrDelete(m, "location", e.Location)
		setOrDelete(m, "summary", e.Summary)
		setSliceOrDelete(m, "highlights", e.Highlights)
	case dto.EntryPublication:
		setOrDelete(m, "title", e.Title)
		setSliceOrDelete(m, "authors", e.Authors)
		setOrDelete(m, "doi", e.Doi)
		setOrDelete(m, "url", e.URL)
		setOrDelete(m, "journal", e.Journal)
		setOrDelete(m, "date", e.Date)
		setOrDelete(m, "summary", e.Summary)
	case dto.EntryOneLine:
		setOrDelete(m, "label", e.Label)
		setOrDelete(m, "details", e.Details)
	case dto.EntryBullet:
		setOrDelete(m, "bullet", e.Bullet)
	case dto.EntryNumbered:
		setOrDelete(m, "number", e.Number)
	case dto.EntryReversedNumbered:
		setOrDelete(m, "reversed_number", e.ReversedNumber)
	}
	return m
}

// ResumeToMaster is the inverse of MasterToResume: it applies a structured
// dto.Resume onto the existing RendercvMaster (preserving any non-cv blocks —
// design/locale/settings — byte-for-byte, per Assumptions in spec 009), and
// writes cv.sections' key order back through the same _order convention
// PrepareMasterForMarshal expects, rather than a parallel marshal path.
func ResumeToMaster(resume dto.Resume, existing RendercvMaster) (RendercvMaster, error) {
	base, err := deepCloneYAML(existing)
	if err != nil {
		return nil, fmt.Errorf("resume to master: clone existing: %w", err)
	}
	if base == nil {
		base = map[string]any{}
	}
	cv, _ := base["cv"].(map[string]any)
	if cv == nil {
		cv = map[string]any{}
	}

	cv["name"] = resume.Name
	setOrDelete(cv, "headline", resume.Headline)
	setOrDelete(cv, "location", resume.Location)
	setOrDelete(cv, "email", resume.Email)
	setOrDelete(cv, "phone", resume.Phone)
	setOrDelete(cv, "website", resume.Website)
	setOrDelete(cv, "photo", resume.Photo)

	if len(resume.SocialNetworks) > 0 {
		sn := make([]any, len(resume.SocialNetworks))
		for i, s := range resume.SocialNetworks {
			sn[i] = map[string]any{"network": s.Network, "username": s.Username}
		}
		cv["social_networks"] = sn
	} else {
		delete(cv, "social_networks")
	}
	if len(resume.CustomConnections) > 0 {
		cc := make([]any, len(resume.CustomConnections))
		for i, c := range resume.CustomConnections {
			m := map[string]any{"placeholder": c.Placeholder, "url": c.URL}
			if c.Icon != "" {
				m["fontawesome_icon"] = c.Icon
			}
			cc[i] = m
		}
		cv["custom_connections"] = cc
	} else {
		delete(cv, "custom_connections")
	}

	for k, v := range resume.Unrecognized {
		cv[k] = v
	}

	sections := map[string]any{}
	order := make([]any, 0, len(resume.Sections))
	for _, sec := range resume.Sections {
		entries := make([]any, 0, len(sec.Entries))
		for _, e := range sec.Entries {
			entries = append(entries, entryToRaw(sec.EntryType, e))
		}
		sections[sec.Name] = entries
		order = append(order, sec.Name)
	}
	sections[sectionOrderKey] = order
	cv["sections"] = sections
	base["cv"] = cv

	return RendercvMaster(base), nil
}
