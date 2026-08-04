package domain

import (
	"gopkg.in/yaml.v3"
)

// OrderedYAMLMap marshals a map[string]any with a fixed key order. yaml.v3
// marshals plain maps in sorted key order, which loses the section order
// rendercv relies on. Used only for cv.sections: rendercv renders sections
// in the order they appear in the YAML, so losing that order (e.g. via
// generic map[string]any marshalling) silently reshuffles the rendered
// resume.
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

// defaultSectionOrder is the resume's fixed, always-enforced section order.
// The business requirement is that resume structure is always the same
// regardless of what order a master config's own sections happen to be
// authored in — so this order wins outright rather than merely filling gaps
// in a captured _order. Sections not listed here (custom/extra sections)
// sort alphabetically after these.
var defaultSectionOrder = []string{
	"summary",
	"experience",
	"skills",
	"projects",
	"education",
	"certifications",
	"publications",
}

// SortByDefaultSectionOrder sorts section keys into the fixed resume order:
// known sections (see defaultSectionOrder) always come first in that order,
// anything else follows alphabetically.
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

// PrepareMasterForMarshal deep-clones master and replaces cv.sections with an
// OrderedYAMLMap so the final YAML written for `rendercv render` always uses
// the fixed section order (see defaultSectionOrder)
// instead of whatever order the source master config's sections happen to
// be authored/stored in, or the alphabetical order plain map marshalling
// would produce.
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

	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	order := SortByDefaultSectionOrder(keys)

	if cv, ok := cloned["cv"].(map[string]any); ok {
		cv["sections"] = OrderedYAMLMap{Keys: order, Values: sections}
	}
	return cloned, nil
}
