package domain

import (
	"strings"

	"gopkg.in/yaml.v3"
)

type OrderedYAMLMap struct {
	Keys   []string
	Values map[string]any
}

func (o OrderedYAMLMap) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range o.Keys {
		var keyNode, valNode yaml.Node
		if err := keyNode.Encode(k); err != nil {
			return nil, err
		}
		if err := valNode.Encode(o.Values[k]); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &keyNode, &valNode)
	}
	return node, nil
}

var defaultSectionOrder = []string{
	"summary",
	"experience",
	"skills",
	"projects",
	"education",
	"certifications",
	"publications",
}

func SortByDefaultSectionOrder(keys []string) []string {
	rank := func(k string) int {
		for i, d := range defaultSectionOrder {
			if k == d {
				return i
			}
		}
		return len(defaultSectionOrder)
	}
	sorted := append([]string(nil), keys...)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			ri, rj := rank(sorted[i]), rank(sorted[j])
			if ri > rj || (ri == rj && sorted[i] > sorted[j]) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}

func PrepareMasterForMarshal(master RendercvMaster) (RendercvMaster, error) {
	cloned, err := DeepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	sections := CvSections(cloned)
	if sections == nil {
		return cloned, nil
	}

	delete(sections, SectionOrderKey)
	embedEntryLinks(sections, "projects")
	embedEntryLinks(sections, "certifications")

	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	order := SortByDefaultSectionOrder(keys)

	if cv, ok := cloned["cv"].(map[string]any); ok {
		cv["sections"] = OrderedYAMLMap{Keys: order, Values: sections}
	}

	hideFooter(cloned)

	return cloned, nil
}

func embedEntryLinks(sections map[string]any, sectionKey string) {
	for _, e := range AsSliceOfMaps(sections[sectionKey]) {
		url := StringField(e, "url")
		if url == "" {
			continue
		}
		name := StringField(e, "name")
		if name == "" || strings.Contains(name, "](") {
			continue
		}
		e["name"] = "[" + name + "](" + url + ")"
		delete(e, "url")
	}
}

func hideFooter(master RendercvMaster) {
	design, _ := master["design"].(map[string]any)
	if design == nil {
		design = map[string]any{}
		master["design"] = design
	}
	page, _ := design["page"].(map[string]any)
	if page == nil {
		page = map[string]any{}
		design["page"] = page
	}
	setDefault(page, "show_footer", false)
}
