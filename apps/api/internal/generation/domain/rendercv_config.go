package domain

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// SectionOrderKey is the synthetic cv.sections key ParseRendercv writes the
// original section key order under, so downstream mapping can preserve the
// author's intended order rather than the map's random iteration order.
const SectionOrderKey = "_order"

func ParseRendercv(yamlText string) (RendercvMaster, error) {
	var master map[string]any
	if err := yaml.Unmarshal([]byte(yamlText), &master); err != nil {
		return nil, fmt.Errorf("parse rendercv yaml: %w", err)
	}
	normalized := NormalizeYAMLMap(master)
	m, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("rendercv yaml: expected a top-level object")
	}
	cv, _ := m["cv"].(map[string]any)
	if cv == nil {
		return nil, fmt.Errorf("rendercv yaml: missing required 'cv' block")
	}
	name, _ := cv["name"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("rendercv yaml: missing required 'cv.name' field")
	}

	if sections, ok := cv["sections"].(map[string]any); ok {
		if order := sectionOrderFromYAML(yamlText); len(order) > 0 {
			orderAny := make([]any, len(order))
			for i, k := range order {
				orderAny[i] = k
			}
			sections[SectionOrderKey] = orderAny
		}
	}

	return RendercvMaster(m), nil
}

// RemoveSection deletes a section from cv.sections and drops its entry from
// the synthetic _order list, so no stale order entry names a section that no
// longer exists. It lives beside the code that builds the order, because that
// is where the invariant is owned.
func RemoveSection(sections map[string]any, key string) {
	if sections == nil {
		return
	}
	delete(sections, key)

	order, ok := sections[SectionOrderKey].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(order))
	for _, entry := range order {
		if s, ok := entry.(string); ok && s == key {
			continue
		}
		filtered = append(filtered, entry)
	}
	sections[SectionOrderKey] = filtered
}

// sectionOrderFromYAML walks the raw YAML (before it's flattened into an
// unordered map[string]any) to recover the original cv.sections key order,
// which rendercv uses to decide the order sections are rendered in.
func sectionOrderFromYAML(yamlText string) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	cvNode := findYAMLMapValue(root, "cv")
	sectionsNode := findYAMLMapValue(cvNode, "sections")
	if sectionsNode == nil || sectionsNode.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(sectionsNode.Content)/2)
	for i := 0; i+1 < len(sectionsNode.Content); i += 2 {
		keys = append(keys, sectionsNode.Content[i].Value)
	}
	return keys
}

func findYAMLMapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func RendercvToText(master RendercvMaster) string {
	sections := CvSections(master)
	if sections == nil {
		return ""
	}
	var parts []string

	cv, _ := master["cv"].(map[string]any)
	if cv != nil {
		if name, ok := cv["name"].(string); ok && name != "" {
			parts = append(parts, name)
		}
		if headline, ok := cv["headline"].(string); ok && headline != "" {
			parts = append(parts, headline)
		}
	}

	if summary, ok := sections["summary"]; ok {
		if items, ok := summary.([]any); ok {
			for _, item := range items {
				if s, ok := item.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
		}
	}

	if skillsRaw, ok := sections["skills"]; ok {
		for _, g := range AsSliceOfMaps(skillsRaw) {
			label := StringField(g, "label")
			details := StringField(g, "details")
			if label != "" || details != "" {
				parts = append(parts, strings.TrimSpace(label+" "+details))
			}
		}
	}

	if expRaw, ok := sections["experience"]; ok {
		for _, e := range AsSliceOfMaps(expRaw) {
			company := StringField(e, "company")
			position := StringField(e, "position")
			if company != "" || position != "" {
				parts = append(parts, strings.TrimSpace(position+" at "+company))
			}
			for _, h := range StringSliceField(e, "highlights") {
				if h != "" {
					parts = append(parts, h)
				}
			}
		}
	}

	return strings.Join(parts, "\n")
}
