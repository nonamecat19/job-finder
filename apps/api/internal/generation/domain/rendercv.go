package domain

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

// VacancyHints carries structured vacancy data parsed by the caller (e.g. an
// AI intake step or the dashboard form). When present, Step 1 reuses these
// instead of extracting them from raw text, improving accuracy and speed.
type VacancyHints struct {
	RequiredSkills  []string `json:"requiredSkills,omitempty"`
	NiceToHave      []string `json:"niceToHave,omitempty"`
	ExperienceLevel string   `json:"experienceLevel,omitempty"` // junior|mid|senior|lead|staff|principal
}

// VacancyAnalysis is the structured output of Step 1 (vacancy analysis).
// It feeds into Step 2 so the content-selection prompt has a clean,
// machine-readable view of what the vacancy demands.
type VacancyAnalysis struct {
	RequiredSkills      []string `json:"requiredSkills"      jsonschema_description:"skills explicitly required/mandatory in the vacancy"`
	NiceToHaveSkills    []string `json:"niceToHaveSkills"    jsonschema_description:"preferred but not required skills"`
	ExperienceLevel     string   `json:"experienceLevel"     jsonschema_description:"junior|mid|senior|lead|staff|principal"`
	KeyResponsibilities []string `json:"keyResponsibilities" jsonschema_description:"top 3-5 responsibilities mentioned"`
	IndustryKeywords    []string `json:"industryKeywords"    jsonschema_description:"domain/industry terms to match (e.g. fintech, SaaS, healthcare)"`
	SeniorityKeywords   []string `json:"seniorityKeywords"   jsonschema_description:"leadership/ownership indicators found in the vacancy"`
}

// TailoredSkillGroup / TailoredExperience / TailoredSections are the only
// content the LLM is allowed to produce — everything else in the master
// (companies, dates, education, design, templates, locale, header) is
// preserved by mergeTailored, never sent back by the model. Mirrors
// tailoredSectionsSchema.
type TailoredSkillGroup struct {
	Index   int    `json:"index" jsonschema_description:"0-based index of the skill group in the master, unchanged"`
	Details string `json:"details" jsonschema_description:"comma-separated skills for this group, reordered so vacancy-required skills come first"`
}

type TailoredExperience struct {
	Company    string   `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
	Highlights []string `json:"highlights" jsonschema_description:"selected, reordered, rephrased highlights (top 3-5 most relevant)"`
}

// TailoredProject is the LLM-facing projection of a master project: its name
// is a lookup key only (copied exactly from the master, never used as content)
// and highlights must come from that project's own master bullets. url,
// start_date, end_date and the stored name always come from the master clone,
// so the model structurally cannot corrupt a project's identity.
type TailoredProject struct {
	Name       string   `json:"name" jsonschema_description:"project name copied EXACTLY from the master"`
	Highlights []string `json:"highlights" jsonschema_description:"selected, rephrased highlights drawn only from THIS project's own master bullets"`
}

// TailoredSkillGroupAdd is a proposed new skill group the AI may suggest
// when the job posting emphasizes a domain the user has tagged but hasn't
// grouped. Details must be drawn from MasterSkillTokens only.
type TailoredSkillGroupAdd struct {
	Label   string `json:"label" jsonschema_description:"name for the new skill group"`
	Details string `json:"details" jsonschema_description:"comma-separated skills drawn ONLY from the master skill tokens"`
}

// SkillChange is a per-token add/remove within an existing skill group.
type SkillChange struct {
	GroupLabel     string  `json:"groupLabel" jsonschema_description:"the existing group label to modify"`
	AddTokens      string  `json:"addTokens" jsonschema_description:"comma-separated tokens to add, drawn ONLY from master skill tokens"`
	RemoveTokens   string  `json:"removeTokens" jsonschema_description:"comma-separated tokens to remove from this group"`
	ReplaceDetails *string `json:"replaceDetails,omitempty" jsonschema_description:"full replacement details string; when set, AddTokens/RemoveTokens are ignored"`
}

// TailoredSections is the output of Step 2 (content selection). It carries
// only the content fields the LLM may change — summary, skill details,
// experience highlights and (only when a project limit is configured) project
// highlights. Feature 028 removed SectionsToDrop, ExperienceOrder and
// TailoredExperience.Drop: the AI may not add, remove, rename or reorder
// resume blocks, nor reorder or drop job entries.
type TailoredSections struct {
	Summary    string               `json:"summary" jsonschema_description:"2-3 sentence professional summary targeting the vacancy"`
	Skills     []TailoredSkillGroup `json:"skills" jsonschema_description:"one entry per master skill group, same indexes, vacancy-required skills first"`
	Experience []TailoredExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company"`
	Projects   []TailoredProject    `json:"projects" jsonschema_description:"the most vacancy-relevant master projects, keyed by name; empty unless a project limit is configured"`

	SkillGroupsToAdd    []TailoredSkillGroupAdd `json:"skillGroupsToAdd,omitempty" jsonschema_description:"new skill groups to propose adding, populated from master skill tokens"`
	SkillGroupsToRemove []string                `json:"skillGroupsToRemove,omitempty" jsonschema_description:"existing group labels to propose removing"`
	SkillChanges        []SkillChange           `json:"skillChanges,omitempty" jsonschema_description:"per-token add/remove within existing groups"`
}

var LevelRules = map[GroundingLevel]string{
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

// Feature 028 removed the AI's section-drop capability (SectionsToDrop no
// longer exists on TailoredSections), so no resume block can ever be dropped
// during tailoring and no protected-sections guard is needed on the merge path.
func CvSections(master RendercvMaster) map[string]any {
	cv, _ := master["cv"].(map[string]any)
	if cv == nil {
		return nil
	}
	sections, _ := cv["sections"].(map[string]any)
	return sections
}

func AsSliceOfMaps(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// IsPinnedSkillGroup reports whether a skill group is exempt from tailoring.
// The spoken-languages group states a fact about the candidate (which human
// languages they speak, at what level) that no vacancy can change, so it is
// carried over from the master verbatim: never rewritten by the merge and
// never dropped by the group cap.
func IsPinnedSkillGroup(label string) bool {
	return strings.Contains(norm(label), "spoken language")
}

func StringField(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func StringSliceField(m map[string]any, key string) []string {
	raw, _ := m[key].([]any)
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// SectionKeys returns the sorted list of section key names from the master.
func SectionKeys(sections map[string]any) []string {
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	// Sort for deterministic output
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
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

// MasterSkillTokens is the union of every skill token in the master (all groups).
func MasterSkillTokens(master RendercvMaster) map[string]bool {
	set := map[string]bool{}
	sections := CvSections(master)
	for _, g := range AsSliceOfMaps(sections["skills"]) {
		for _, t := range tokens(StringField(g, "details")) {
			set[t] = true
		}
	}
	return set
}

// deepCloneYAML round-trips through YAML to deep-copy a generic map — the Go
// equivalent of `structuredClone(master)`.
func DeepCloneYAML(master RendercvMaster) (RendercvMaster, error) {
	b, err := yaml.Marshal(map[string]any(master))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return NormalizeYAMLMap(out).(map[string]any), nil
}

// NormalizeYAMLMap recursively converts map[any]any / map[string]any produced
// by yaml.v3 (which can yield either depending on key types) into
// map[string]any so later type assertions (`.(map[string]any)`) succeed
// uniformly, matching plain-JSON-object semantics used everywhere else.
func NormalizeYAMLMap(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = NormalizeYAMLMap(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[toStringKey(k)] = NormalizeYAMLMap(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = NormalizeYAMLMap(val)
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

// mergeTailored deep-clones the master and applies the only tailoring
// decisions the AI is allowed to make:
// - Overwrites summary
// - Overwrites skill-group details (vacancy-required first)
// - Replaces experience highlights per entry
//
// Feature 028: the resume's structure is immutable. No section is ever
// added, removed, renamed or reordered (the cv.sections["_order"] key is
// untouched), and experience entries keep the master's authored order and
// identity — the AI may not reorder or drop jobs. Companies, positions,
// dates, education, design, templates, locale, settings and header are taken
// verbatim from the deep-cloned master — the LLM never touches them.
func MergeTailored(master RendercvMaster, payload TailoredSections) (RendercvMaster, error) {
	merged, err := DeepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	sections := CvSections(merged)
	if sections == nil {
		return merged, nil
	}

	// 1. Replace summary
	sections["summary"] = []any{strings.TrimSpace(payload.Summary)}

	// 2. Replace skill details (preserve group order, only change content).
	// Pinned groups — the spoken languages — keep the master's details even
	// when the model returns a rewrite for that index.
	skills := AsSliceOfMaps(sections["skills"])
	for _, s := range payload.Skills {
		if s.Index >= 0 && s.Index < len(skills) && !IsPinnedSkillGroup(StringField(skills[s.Index], "label")) {
			skills[s.Index]["details"] = strings.TrimSpace(s.Details)
		}
	}

	// 3. Replace experience highlights only. The master's experience slice is
	// kept in its authored order and identity; mutating each entry's map in
	// place changes the description bullets without ever rewriting the slice.
	// start_date/end_date/company/position/location/summary pass through
	// verbatim from the deep-cloned master.
	byCompany := map[string]map[string]any{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		byCompany[norm(StringField(e, "company"))] = e
	}
	for _, pe := range payload.Experience {
		if target, ok := byCompany[norm(pe.Company)]; ok {
			target["highlights"] = cleanHighlights(pe.Highlights)
		}
	}

	// 4. Replace project highlights only, the same way. name/url/start_date/
	// end_date pass through from the clone, so a model that returns a wrong
	// link or date cannot affect the document; an unknown project name simply
	// finds no target here and is reported by the grounding check. An empty
	// Projects payload — the default path — leaves the master's projects
	// exactly as authored.
	byProject := map[string]map[string]any{}
	for _, p := range AsSliceOfMaps(sections["projects"]) {
		byProject[norm(StringField(p, "name"))] = p
	}
	for _, pp := range payload.Projects {
		if target, ok := byProject[norm(pp.Name)]; ok {
			target["highlights"] = cleanHighlights(pp.Highlights)
		}
	}

	return merged, nil
}

// cleanHighlights trims each bullet and drops the empty ones.
func cleanHighlights(in []string) []any {
	out := make([]any, 0, len(in))
	for _, h := range in {
		if trimmed := strings.TrimSpace(h); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
