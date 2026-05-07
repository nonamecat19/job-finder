package generation

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// RendercvMaster is the hand-tuned rendercv YAML config: theme, templates,
// locale, design, settings + the actual CV content. We only ever tailor the
// *content* sections (summary/skills/experience.highlights) and leave every
// structural/design field byte-for-byte intact, so it's represented as a
// generic map (parsed straight from YAML) rather than a fully-typed struct —
// same "pass everything else through untouched" contract as the TS
// `RendercvMaster` loose type. Mirrors rendercv-tailor.ts.
type RendercvMaster map[string]any

type GroundingLevel string

const (
	GroundingStrict     GroundingLevel = "strict"
	GroundingModerate   GroundingLevel = "moderate"
	GroundingAggressive GroundingLevel = "aggressive"
)

// ParseGroundingLevel mirrors the TS fallback: anything other than
// "strict"/"aggressive" becomes "moderate".
func ParseGroundingLevel(s string) GroundingLevel {
	switch GroundingLevel(s) {
	case GroundingStrict, GroundingAggressive:
		return GroundingLevel(s)
	default:
		return GroundingModerate
	}
}

// TailoredSkillGroup / TailoredExperience / TailoredSections are the only
// content the LLM is allowed to produce — everything else in the master
// (companies, dates, education, design, templates, locale, header) is
// preserved by mergeTailored, never sent back by the model. Mirrors
// tailoredSectionsSchema.
type TailoredSkillGroup struct {
	Index   int    `json:"index" jsonschema_description:"0-based index of the skill group in the master, unchanged"`
	Details string `json:"details" jsonschema_description:"comma-separated skills for this group, reordered/tailored"`
}

type TailoredExperience struct {
	Company    string   `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
	Highlights []string `json:"highlights" jsonschema_description:"selected, reordered, rephrased highlights"`
}

type TailoredSections struct {
	Summary    string               `json:"summary" jsonschema_description:"one-paragraph professional summary rewritten for the vacancy"`
	Skills     []TailoredSkillGroup `json:"skills" jsonschema_description:"one entry per master skill group, same indexes"`
	Experience []TailoredExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company"`
}

var levelRules = map[GroundingLevel]string{
	GroundingStrict: "GROUNDING = STRICT. Use ONLY skills and facts already present in the master profile. " +
		"You may reorder, trim and rephrase, but you must NOT introduce any technology, tool or " +
		"skill token that does not already appear in the master. Do not invent achievements.",
	GroundingModerate: "GROUNDING = MODERATE. You may reorder, trim and rephrase freely, and you MAY add a skill " +
		"or reframe a highlight for technology that is directly ADJACENT to the existing stack " +
		"(e.g. the vacancy asks for Terraform and the master already lists AWS, Docker and " +
		"Kubernetes). Never add technology unrelated to the demonstrated experience, and never " +
		"invent employers, dates, projects or metrics.",
	GroundingAggressive: "GROUNDING = AGGRESSIVE. Maximize keyword match with the vacancy: you may add any skills the " +
		"vacancy requires and frame highlights toward them. Still never invent employers, dates, " +
		"degrees or numeric metrics that are not in the master.",
}

func cvSections(master RendercvMaster) map[string]any {
	cv, _ := master["cv"].(map[string]any)
	if cv == nil {
		return nil
	}
	sections, _ := cv["sections"].(map[string]any)
	return sections
}

func asSliceOfMaps(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func stringSliceField(m map[string]any, key string) []string {
	raw, _ := m[key].([]any)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// buildTailorPrompt mirrors buildTailorPrompt in rendercv-tailor.ts.
func buildTailorPrompt(master RendercvMaster, vacancy string, level GroundingLevel) string {
	sections := cvSections(master)
	skills := asSliceOfMaps(sections["skills"])
	experience := asSliceOfMaps(sections["experience"])

	var skillLines []string
	for i, s := range skills {
		skillLines = append(skillLines, fmt.Sprintf("  [%d] %s: %s", i, stringField(s, "label"), stringField(s, "details")))
	}
	var expLines []string
	for _, e := range experience {
		line := "  - company: " + stringField(e, "company")
		if pos := stringField(e, "position"); pos != "" {
			line += " (" + pos + ")"
		}
		expLines = append(expLines, line)
		for _, h := range stringSliceField(e, "highlights") {
			expLines = append(expLines, "      • "+h)
		}
	}
	currentSummary := strings.Join(stringSliceField(sections, "summary"), " ")

	vac := vacancy
	if len(vac) > 6000 {
		vac = vac[:6000]
	}

	return "Tailor this candidate's resume content to the target vacancy by selecting, reordering and " +
		"rephrasing what the master profile already contains.\n\n" +
		levelRules[level] + "\n\n" +
		"HARD RULES (all levels):\n" +
		"- Return skills as one entry per group, using the SAME [index] shown below.\n" +
		"- Return experience keyed by the EXACT company name shown below; do not add or drop companies.\n" +
		"- Emphasize what the vacancy asks for; move the most relevant items first.\n" +
		"- Keep highlights concise, one achievement each, no fabricated numbers.\n\n" +
		"CURRENT SUMMARY:\n" + currentSummary + "\n\n" +
		"SKILL GROUPS:\n" + strings.Join(skillLines, "\n") + "\n\n" +
		"EXPERIENCE:\n" + strings.Join(expLines, "\n") + "\n\n" +
		"TARGET VACANCY:\n" + vac
}

// tokens splits a comma/slash-separated details string into skill tokens.
func tokens(details string) []string {
	parts := strings.FieldsFunc(details, func(r rune) bool { return r == ',' || r == '/' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if n := norm(p); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// masterSkillTokens is the union of every skill token in the master (all groups).
func masterSkillTokens(master RendercvMaster) map[string]bool {
	set := map[string]bool{}
	sections := cvSections(master)
	for _, g := range asSliceOfMaps(sections["skills"]) {
		for _, t := range tokens(stringField(g, "details")) {
			set[t] = true
		}
	}
	return set
}

// deepCloneYAML round-trips through YAML to deep-copy a generic map — the Go
// equivalent of `structuredClone(master)`.
func deepCloneYAML(master RendercvMaster) (RendercvMaster, error) {
	b, err := yaml.Marshal(map[string]any(master))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return normalizeYAMLMap(out).(map[string]any), nil
}

// normalizeYAMLMap recursively converts map[any]any / map[string]any produced
// by yaml.v3 (which can yield either depending on key types) into
// map[string]any so later type assertions (`.(map[string]any)`) succeed
// uniformly, matching plain-JSON-object semantics used everywhere else.
func normalizeYAMLMap(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeYAMLMap(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[toStringKey(k)] = normalizeYAMLMap(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeYAMLMap(val)
		}
		return out
	default:
		return v
	}
}

func toStringKey(k any) string {
	switch v := k.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// mergeTailored deep-clones the master and overwrites ONLY summary,
// skill-group details, and experience highlights. Companies, positions,
// dates, education, design, templates, locale, settings and header are
// taken verbatim from the master — the LLM never touches them. Mirrors
// mergeTailored in rendercv-tailor.ts.
func mergeTailored(master RendercvMaster, payload TailoredSections) (RendercvMaster, error) {
	merged, err := deepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	sections := cvSections(merged)
	if sections == nil {
		return merged, nil
	}

	sections["summary"] = []any{strings.TrimSpace(payload.Summary)}

	skills := asSliceOfMaps(sections["skills"])
	for _, s := range payload.Skills {
		if s.Index >= 0 && s.Index < len(skills) {
			skills[s.Index]["details"] = strings.TrimSpace(s.Details)
		}
	}

	experience := asSliceOfMaps(sections["experience"])
	byCompany := map[string]map[string]any{}
	for _, e := range experience {
		byCompany[norm(stringField(e, "company"))] = e
	}
	for _, e := range payload.Experience {
		if target, ok := byCompany[norm(e.Company)]; ok {
			highlights := make([]any, 0, len(e.Highlights))
			for _, h := range e.Highlights {
				if trimmed := strings.TrimSpace(h); trimmed != "" {
					highlights = append(highlights, trimmed)
				}
			}
			target["highlights"] = highlights
		}
	}

	return merged, nil
}
