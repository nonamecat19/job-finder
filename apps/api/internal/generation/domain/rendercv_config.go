package domain

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	return RendercvMaster(m), nil
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
