package domain

func CompactDesign(master RendercvMaster) {
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
	setDefault(page, "size", "a4")
	setDefault(page, "top_margin", "0.45in")
	setDefault(page, "bottom_margin", "0.45in")
	setDefault(page, "left_margin", "0.55in")
	setDefault(page, "right_margin", "0.55in")

	typography, _ := design["typography"].(map[string]any)
	if typography == nil {
		typography = map[string]any{}
		design["typography"] = typography
	}
	setDefault(typography, "line_spacing", "0.5em")

	fontSize, _ := typography["font_size"].(map[string]any)
	if fontSize == nil {
		fontSize = map[string]any{}
		typography["font_size"] = fontSize
	}
	setDefault(fontSize, "body", "9pt")
	setDefault(fontSize, "name", "24pt")
	setDefault(fontSize, "headline", "9pt")
	setDefault(fontSize, "connections", "9pt")
	setDefault(fontSize, "section_titles", "1.2em")
}

func setDefault(m map[string]any, key string, val any) {
	if _, ok := m[key]; !ok {
		m[key] = val
	}
}
