package generation

import (
	"gopkg.in/yaml.v3"
)

// orderedYAMLMap marshals a map[string]any with a fixed key order. yaml.v3
// marshals plain maps in sorted key order, which loses the section order
// rendercv relies on. Used only for cv.sections: rendercv renders sections
// in the order they appear in the YAML, so losing that order (e.g. via
// generic map[string]any marshalling) silently reshuffles the rendered
// resume.
type orderedYAMLMap struct {
	keys   []string
	values map[string]any
}

func (o orderedYAMLMap) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range o.keys {
		var keyNode, valNode yaml.Node
		if err := keyNode.Encode(k); err != nil {
			return nil, err
		}
		if err := valNode.Encode(o.values[k]); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &keyNode, &valNode)
	}
	return node, nil
}

// PrepareMasterForMarshal deep-clones master and replaces cv.sections with an
// orderedYAMLMap so the final YAML written for `rendercv render` preserves
// the original section order (summary, skills, experience, education, ...)
// instead of the alphabetical order plain map marshalling would produce.
// Falls back to sorted order for any section not covered by the captured
// _order (e.g. missing entirely, or a section added after tailoring).
func PrepareMasterForMarshal(master RendercvMaster) (RendercvMaster, error) {
	cloned, err := deepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	sections := CvSections(cloned)
	if sections == nil {
		return cloned, nil
	}

	orderRaw, _ := sections[sectionOrderKey].([]any)
	delete(sections, sectionOrderKey)

	order := make([]string, 0, len(orderRaw))
	seen := map[string]bool{}
	for _, k := range orderRaw {
		s, _ := k.(string)
		if s == "" || seen[s] {
			continue
		}
		if _, ok := sections[s]; !ok {
			continue
		}
		order = append(order, s)
		seen[s] = true
	}

	rest := make([]string, 0, len(sections))
	for k := range sections {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	for i := 0; i < len(rest); i++ {
		for j := i + 1; j < len(rest); j++ {
			if rest[i] > rest[j] {
				rest[i], rest[j] = rest[j], rest[i]
			}
		}
	}
	order = append(order, rest...)

	if cv, ok := cloned["cv"].(map[string]any); ok {
		cv["sections"] = orderedYAMLMap{keys: order, values: sections}
	}
	return cloned, nil
}