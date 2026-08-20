package domain

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type RendercvMaster map[string]any

type GroundingLevel string

const (
	GroundingStrict     GroundingLevel = "strict"
	GroundingModerate   GroundingLevel = "moderate"
	GroundingAggressive GroundingLevel = "aggressive"
)

func ParseGroundingLevel(s string) GroundingLevel {
	switch GroundingLevel(s) {
	case GroundingStrict, GroundingAggressive:
		return GroundingLevel(s)
	default:
		return GroundingModerate
	}
}

type VacancyHints struct {
	RequiredSkills  []string `json:"requiredSkills,omitempty"`
	NiceToHave      []string `json:"niceToHave,omitempty"`
	ExperienceLevel string   `json:"experienceLevel,omitempty"`
}

type VacancyAnalysis struct {
	RequiredSkills      []string `json:"requiredSkills"      jsonschema_description:"skills explicitly required/mandatory in the vacancy"`
	NiceToHaveSkills    []string `json:"niceToHaveSkills"    jsonschema_description:"preferred but not required skills"`
	ExperienceLevel     string   `json:"experienceLevel"     jsonschema_description:"junior|mid|senior|lead|staff|principal"`
	KeyResponsibilities []string `json:"keyResponsibilities" jsonschema_description:"top 3-5 responsibilities mentioned"`
	IndustryKeywords    []string `json:"industryKeywords"    jsonschema_description:"domain/industry terms to match (e.g. fintech, SaaS, healthcare)"`
	SeniorityKeywords   []string `json:"seniorityKeywords"   jsonschema_description:"leadership/ownership indicators found in the vacancy"`

	// TargetTitle and TargetCompany are the vacancy's own header, carried
	// alongside the analysis rather than produced by it. `json:"-"` keeps them
	// out of the response schema, so no stage can rewrite the role it is
	// tailoring for, and it keeps them out of the persisted analysis, so the
	// rerun path re-reads them from the run rather than trusting a snapshot.
	TargetTitle   string `json:"-"`
	TargetCompany string `json:"-"`
}

type HighlightRef struct {
	SourceIndex int    `json:"sourceIndex" jsonschema_description:"0-based index of the bullet in the numbered list shown for this entry"`
	Rephrased   string `json:"rephrased,omitempty" jsonschema_description:"optional rewording of THAT bullet only; omit to keep the master's wording"`
}

type TailoredExperience struct {
	Company    string         `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
	Highlights []HighlightRef `json:"highlights" jsonschema_description:"the most relevant bullets for this entry, by index, in the order they should appear"`
}

type TailoredProject struct {
	Name       string         `json:"name" jsonschema_description:"project name copied EXACTLY from the master"`
	Highlights []HighlightRef `json:"highlights" jsonschema_description:"the most relevant bullets for THIS project, by index into its own bullet list"`
}

type TailoredSkillGroupAdd struct {
	Label   string `json:"label" jsonschema_description:"name for the new skill group"`
	Details string `json:"details" jsonschema_description:"comma-separated skills drawn ONLY from the master skill tokens"`
}

type SkillChange struct {
	GroupLabel     string  `json:"groupLabel" jsonschema_description:"the existing group label to modify"`
	AddTokens      string  `json:"addTokens" jsonschema_description:"comma-separated tokens to add, drawn ONLY from master skill tokens"`
	RemoveTokens   string  `json:"removeTokens" jsonschema_description:"comma-separated tokens to remove from this group"`
	ReplaceDetails *string `json:"replaceDetails,omitempty" jsonschema_description:"full replacement details string; when set, AddTokens/RemoveTokens are ignored"`
}

type TailoredSelection struct {
	Experience []TailoredExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company"`
	Projects   []TailoredProject    `json:"projects" jsonschema_description:"the most vacancy-relevant master projects, keyed by name; empty unless a project limit is configured"`

	SkillGroupsToAdd    []TailoredSkillGroupAdd `json:"skillGroupsToAdd,omitempty" jsonschema_description:"new skill groups to propose adding, populated from master skill tokens"`
	SkillGroupsToRemove []string                `json:"skillGroupsToRemove,omitempty" jsonschema_description:"existing group labels to propose removing"`
	SkillChanges        []SkillChange           `json:"skillChanges,omitempty" jsonschema_description:"per-token add/remove within existing groups"`
}

type TailoredSummary struct {
	Summary string `json:"summary" jsonschema_description:"2-3 sentence professional summary targeting the vacancy"`
}

type TailoredSections struct {
	Summary string `json:"summary" jsonschema_description:"2-3 sentence professional summary targeting the vacancy"`
	TailoredSelection
}

func CurrentSummary(doc RendercvMaster) *TailoredSummary {
	sections := CvSections(doc)
	if sections == nil {
		return nil
	}
	items, ok := sections["summary"].([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	s, ok := items[0].(string)
	if !ok || s == "" {
		return nil
	}
	return &TailoredSummary{Summary: s}
}

func SelectedHighlights(base RendercvMaster, sel TailoredSelection, level GroundingLevel) []string {
	sections := CvSections(base)
	byCompany := map[string][]string{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		byCompany[norm(StringField(e, "company"))] = StringSliceField(e, "highlights")
	}
	byProject := map[string][]string{}
	for _, p := range AsSliceOfMaps(sections["projects"]) {
		byProject[norm(StringField(p, "name"))] = StringSliceField(p, "highlights")
	}

	var out []string
	for _, e := range sel.Experience {
		out = append(out, ResolveHighlights(byCompany[norm(e.Company)], e.Highlights, level)...)
	}
	for _, p := range sel.Projects {
		out = append(out, ResolveHighlights(byProject[norm(p.Name)], p.Highlights, level)...)
	}
	return out
}

func ResolveHighlights(sources []string, refs []HighlightRef, level GroundingLevel) []string {
	out := make([]string, 0, len(refs))
	seen := make(map[int]bool, len(refs))
	for _, ref := range refs {
		if ref.SourceIndex < 0 || ref.SourceIndex >= len(sources) || seen[ref.SourceIndex] {
			continue
		}
		seen[ref.SourceIndex] = true
		out = append(out, resolveHighlight(sources[ref.SourceIndex], ref, level))
	}
	return out
}

func resolveHighlight(source string, ref HighlightRef, level GroundingLevel) string {
	rephrased := strings.TrimSpace(ref.Rephrased)
	if rephrased == "" || level == GroundingStrict {
		return source
	}
	if !lcsCovered(rephrased, []string{source}) || len(ungroundedMetrics(rephrased, []string{source})) > 0 {
		return source
	}
	return rephrased
}

func SkillGroupLabels(master RendercvMaster) []string {
	var out []string
	for _, g := range AsSliceOfMaps(CvSections(master)["skills"]) {
		if label := StringField(g, "label"); label != "" {
			out = append(out, label)
		}
	}
	return out
}

type SummaryBrief struct {
	Analysis         VacancyAnalysis
	TotalYears       int
	Highlights       []string
	SkillGroupLabels []string
	// SkillLines are the candidate's ranked skill groups as "Label: a, b, c".
	// Group labels alone name no technology, so a summary written from them
	// could only reach for whatever the selected highlights happened to
	// mention — which is how a Vue vacancy produced a summary that never said
	// Vue. These are the tailored groups, so the vacancy's own skills lead.
	SkillLines  []string
	SentenceMin int
	SentenceMax int

	PreviousViolations []string
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

func SectionKeys(sections map[string]any) []string {
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

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

func DropUngroundedSkillTokens(pool RendercvMaster, doc RendercvMaster) {
	allowed := MasterSkillTokens(pool)
	for _, g := range AsSliceOfMaps(CvSections(doc)["skills"]) {
		var kept []string
		for _, entry := range strings.Split(StringField(g, "details"), ",") {
			parts := tokens(entry)
			if len(parts) == 0 {
				continue
			}
			grounded := true
			for _, p := range parts {
				if !allowed[p] {
					grounded = false
					break
				}
			}
			if grounded {
				kept = append(kept, strings.TrimSpace(entry))
			}
		}
		g["details"] = strings.Join(kept, ", ")
	}
}

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

func MergeTailored(master RendercvMaster, payload TailoredSelection, summary *TailoredSummary, level GroundingLevel) (RendercvMaster, error) {
	merged, err := DeepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	sections := CvSections(merged)
	if sections == nil {
		return merged, nil
	}

	if summary != nil {
		sections["summary"] = []any{strings.TrimSpace(summary.Summary)}
	}

	byCompany := map[string]map[string]any{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		byCompany[norm(StringField(e, "company"))] = e
	}
	for _, pe := range payload.Experience {
		if target, ok := byCompany[norm(pe.Company)]; ok {
			target["highlights"] = cleanHighlights(ResolveHighlights(StringSliceField(target, "highlights"), pe.Highlights, level))
		}
	}

	byProject := map[string]map[string]any{}
	for _, p := range AsSliceOfMaps(sections["projects"]) {
		byProject[norm(StringField(p, "name"))] = p
	}
	for _, pp := range payload.Projects {
		if target, ok := byProject[norm(pp.Name)]; ok {
			target["highlights"] = cleanHighlights(ResolveHighlights(StringSliceField(target, "highlights"), pp.Highlights, level))
		}
	}

	return merged, nil
}

func cleanHighlights(in []string) []any {
	out := make([]any, 0, len(in))
	for _, h := range in {
		if trimmed := strings.TrimSpace(h); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func (b SummaryBrief) WithViolations(violations []string) SummaryBrief {
	b.PreviousViolations = violations
	return b
}
