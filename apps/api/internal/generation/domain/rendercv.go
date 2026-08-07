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
}

type TailoredSkillGroup struct {
	Index   int    `json:"index" jsonschema_description:"0-based index of the skill group in the master, unchanged"`
	Details string `json:"details" jsonschema_description:"comma-separated skills for this group, reordered so vacancy-required skills come first"`
}

type TailoredExperience struct {
	Company    string   `json:"company" jsonschema_description:"company name copied EXACTLY from the master"`
	Highlights []string `json:"highlights" jsonschema_description:"selected, reordered, rephrased highlights (top 3-5 most relevant)"`
}

type TailoredProject struct {
	Name       string   `json:"name" jsonschema_description:"project name copied EXACTLY from the master"`
	Highlights []string `json:"highlights" jsonschema_description:"selected, rephrased highlights drawn only from THIS project's own master bullets"`
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

// TailoredSelection is what the selection stage returns: the mechanical work
// of choosing and reordering master content. It deliberately has no summary
// field. The summary is written by a separate, more expensive stage, and the
// page-fitting passes reuse this same type — so a page-fit response that tries
// to reword the summary has nowhere to put it and is discarded at unmarshal
// rather than merged (035 FR-005/FR-010).
type TailoredSelection struct {
	Skills     []TailoredSkillGroup `json:"skills" jsonschema_description:"one entry per master skill group, same indexes, vacancy-required skills first"`
	Experience []TailoredExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company"`
	Projects   []TailoredProject    `json:"projects" jsonschema_description:"the most vacancy-relevant master projects, keyed by name; empty unless a project limit is configured"`

	SkillGroupsToAdd    []TailoredSkillGroupAdd `json:"skillGroupsToAdd,omitempty" jsonschema_description:"new skill groups to propose adding, populated from master skill tokens"`
	SkillGroupsToRemove []string                `json:"skillGroupsToRemove,omitempty" jsonschema_description:"existing group labels to propose removing"`
	SkillChanges        []SkillChange           `json:"skillChanges,omitempty" jsonschema_description:"per-token add/remove within existing groups"`
}

// TailoredSummary is what the summary stage returns — the one part of a
// tailored resume that is written rather than selected, and the only part
// worth a premium model.
type TailoredSummary struct {
	Summary string `json:"summary" jsonschema_description:"2-3 sentence professional summary targeting the vacancy"`
}

// TailoredSections is the merged shape the render path consumes, assembled
// from a TailoredSelection plus a TailoredSummary. It is no longer any single
// model's response type.
type TailoredSections struct {
	Summary string `json:"summary" jsonschema_description:"2-3 sentence professional summary targeting the vacancy"`
	TailoredSelection
}

// CurrentSummary reads the summary already on a document. The structure and
// page-fit re-prompts merge a fresh selection onto the master, which would
// otherwise drop the premium-written summary and silently replace it with the
// master's — carrying it through is what keeps it immutable (035 FR-010).
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

// SelectedHighlights flattens the achievements the selection stage kept, in
// document order. It is what lets the summary reference a real achievement
// without being handed the master profile.
func SelectedHighlights(sel TailoredSelection) []string {
	var out []string
	for _, e := range sel.Experience {
		out = append(out, e.Highlights...)
	}
	for _, p := range sel.Projects {
		out = append(out, p.Highlights...)
	}
	return out
}

// SkillGroupLabels lists the master's skill group labels — the candidate's
// areas of competence, without the token-level detail the summary cannot use.
func SkillGroupLabels(master RendercvMaster) []string {
	var out []string
	for _, g := range AsSliceOfMaps(CvSections(master)["skills"]) {
		if label := StringField(g, "label"); label != "" {
			out = append(out, label)
		}
	}
	return out
}

// SummaryBrief is the premium stage's entire input. It carries what a summary
// needs to reference and nothing else — notably not the master profile, which
// is ~5k tokens and would triple the cost of the one call priced at premium
// rates (035 FR-004, research R3).
type SummaryBrief struct {
	Analysis         VacancyAnalysis
	TotalYears       int
	Highlights       []string
	SkillGroupLabels []string
	SentenceMin      int
	SentenceMax      int
	// PreviousViolations feeds a failed attempt's grounding violations back
	// into the re-prompt.
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

// MergeTailored applies a selection onto the master, and a summary only when
// one is supplied. A nil summary means "leave what is already there" — that is
// the page-fitting case, where the premium-written summary must survive
// untouched (035 FR-010).
func MergeTailored(master RendercvMaster, payload TailoredSelection, summary *TailoredSummary) (RendercvMaster, error) {
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

	skills := AsSliceOfMaps(sections["skills"])
	for _, s := range payload.Skills {
		if s.Index >= 0 && s.Index < len(skills) && !IsPinnedSkillGroup(StringField(skills[s.Index], "label")) {
			skills[s.Index]["details"] = strings.TrimSpace(s.Details)
		}
	}

	byCompany := map[string]map[string]any{}
	for _, e := range AsSliceOfMaps(sections["experience"]) {
		byCompany[norm(StringField(e, "company"))] = e
	}
	for _, pe := range payload.Experience {
		if target, ok := byCompany[norm(pe.Company)]; ok {
			target["highlights"] = cleanHighlights(pe.Highlights)
		}
	}

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

func cleanHighlights(in []string) []any {
	out := make([]any, 0, len(in))
	for _, h := range in {
		if trimmed := strings.TrimSpace(h); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// WithViolations returns a copy of the brief carrying the grounding violations
// a previous attempt produced, so the re-prompt is told what it got wrong
// rather than being asked the same question twice (035 FR-009).
func (b SummaryBrief) WithViolations(violations []string) SummaryBrief {
	b.PreviousViolations = violations
	return b
}
