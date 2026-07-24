package generation

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/job-finder/api/internal/llm"
	"github.com/job-finder/api/internal/strutil"
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
	Drop       bool     `json:"drop,omitempty" jsonschema_description:"true if this entire entry is irrelevant to the vacancy and should be omitted"`
}

// TailoredSections is the enhanced output of Step 2 (content selection).
// In addition to summary/skills/experience it now carries section-drop
// decisions and experience ordering so mergeTailored can rearrange and prune
// the master YAML accordingly.
type TailoredSections struct {
	Summary         string               `json:"summary" jsonschema_description:"2-3 sentence professional summary targeting the vacancy"`
	Skills          []TailoredSkillGroup `json:"skills" jsonschema_description:"one entry per master skill group, same indexes, vacancy-required skills first"`
	Experience      []TailoredExperience `json:"experience" jsonschema_description:"one entry per master experience entry, keyed by company"`
	SectionsToDrop  []string             `json:"sectionsToDrop,omitempty" jsonschema_description:"section keys to omit (e.g. patents, invited_talks, publications). Never include summary, experience, education, or skills"`
	ExperienceOrder []string             `json:"experienceOrder,omitempty" jsonschema_description:"companies in desired display order, most relevant to the vacancy first"`
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

// protectedSections can never be dropped by the LLM — they are core to any
// resume regardless of the target role.
var protectedSections = map[string]bool{
	"summary":    true,
	"experience": true,
	"education":  true,
	"skills":     true,
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

// toAnySlice converts a slice of maps back to []any for YAML marshalling.
func toAnySlice(maps []map[string]any) []any {
	out := make([]any, len(maps))
	for i, m := range maps {
		out[i] = m
	}
	return out
}

// ---------------------------------------------------------------------------
// Step 1: Vacancy Analysis
// ---------------------------------------------------------------------------

// buildAnalyzePrompt constructs the prompt for Step 1. When VacancyHints are
// provided, they are included so the LLM can validate/refine them rather than
// starting from scratch.
func buildAnalyzePrompt(vacancy string, hints *VacancyHints) string {
	vac := strutil.Truncate(vacancy, 6000)

	var b strings.Builder
	b.WriteString("Analyze this job vacancy and extract structured requirements.\n\n")
	b.WriteString("VACANCY TEXT:\n")
	b.WriteString(vac)
	b.WriteString("\n")

	if hints != nil {
		b.WriteString("\nPROVIDED HINTS (validate and refine these):\n")
		if len(hints.RequiredSkills) > 0 {
			b.WriteString("  Required skills (provided): ")
			b.WriteString(strings.Join(hints.RequiredSkills, ", "))
			b.WriteString("\n")
		}
		if len(hints.NiceToHave) > 0 {
			b.WriteString("  Nice-to-have skills (provided): ")
			b.WriteString(strings.Join(hints.NiceToHave, ", "))
			b.WriteString("\n")
		}
		if hints.ExperienceLevel != "" {
			b.WriteString("  Experience level (provided): ")
			b.WriteString(hints.ExperienceLevel)
			b.WriteString("\n")
		}
		b.WriteString("\nVerify the hints against the vacancy text. Add any missing required/nice-to-have skills. Correct the experience level if the hints seem wrong.\n")
	}

	b.WriteString("\nReturn a VacancyAnalysis with:\n")
	b.WriteString("- requiredSkills: skills explicitly listed as required/mandatory\n")
	b.WriteString("- niceToHaveSkills: preferred but not required\n")
	b.WriteString("- experienceLevel: one of junior|mid|senior|lead|staff|principal\n")
	b.WriteString("- keyResponsibilities: top 3-5 responsibilities\n")
	b.WriteString("- industryKeywords: domain terms (e.g. fintech, healthcare, SaaS, e-commerce)\n")
	b.WriteString("- seniorityKeywords: leadership indicators (e.g. mentor, lead team, architecture decisions)")

	return b.String()
}

// analyzeVacancy calls the LLM to produce a VacancyAnalysis from the raw
// vacancy text (optionally enriched with caller-provided hints).
func analyzeVacancy(ctx context.Context, lc llm.Provider, model, vacancy string, hints *VacancyHints) (VacancyAnalysis, error) {
	prompt := buildAnalyzePrompt(vacancy, hints)
	return llm.CompleteStructured[VacancyAnalysis](ctx, lc, prompt, &llm.CompleteOptions{
		System: "You are a job-market analyst who extracts structured requirements from vacancy descriptions. Be precise and concise.",
		Model:  model,
	})
}

// ---------------------------------------------------------------------------
// Step 2: Content Selection & Tailoring
// ---------------------------------------------------------------------------

// buildSelectPrompt constructs the prompt for Step 2. It receives the vacancy
// analysis from Step 1 and the full master resume content, and asks the LLM
// to select, reorder, rephrase and optionally drop content.
func buildSelectPrompt(master RendercvMaster, analysis VacancyAnalysis, level GroundingLevel, prevViolations []string) string {
	sections := CvSections(master)
	skills := AsSliceOfMaps(sections["skills"])
	experience := AsSliceOfMaps(sections["experience"])
	sectionKeys := sectionKeys(sections)

	// Format vacancy analysis
	var analysisLines []string
	analysisLines = append(analysisLines, "REQUIRED SKILLS: "+strings.Join(analysis.RequiredSkills, ", "))
	if len(analysis.NiceToHaveSkills) > 0 {
		analysisLines = append(analysisLines, "NICE-TO-HAVE: "+strings.Join(analysis.NiceToHaveSkills, ", "))
	}
	analysisLines = append(analysisLines, "EXPERIENCE LEVEL: "+analysis.ExperienceLevel)
	if len(analysis.KeyResponsibilities) > 0 {
		analysisLines = append(analysisLines, "KEY RESPONSIBILITIES:")
		for _, r := range analysis.KeyResponsibilities {
			analysisLines = append(analysisLines, "  - "+r)
		}
	}
	if len(analysis.IndustryKeywords) > 0 {
		analysisLines = append(analysisLines, "INDUSTRY: "+strings.Join(analysis.IndustryKeywords, ", "))
	}
	if len(analysis.SeniorityKeywords) > 0 {
		analysisLines = append(analysisLines, "SENIORITY SIGNALS: "+strings.Join(analysis.SeniorityKeywords, ", "))
	}

	// Format skill groups
	var skillLines []string
	for i, s := range skills {
		skillLines = append(skillLines, fmt.Sprintf("  [%d] %s: %s", i, StringField(s, "label"), StringField(s, "details")))
	}

	// Format experience
	var expLines []string
	for _, e := range experience {
		line := "  - company: " + StringField(e, "company")
		if pos := StringField(e, "position"); pos != "" {
			line += " (" + pos + ")"
		}
		if loc := StringField(e, "location"); loc != "" {
			line += " | " + loc
		}
		expLines = append(expLines, line)
		for _, h := range StringSliceField(e, "highlights") {
			expLines = append(expLines, "      • "+h)
		}
	}

	var b strings.Builder
	b.WriteString("Given this vacancy analysis, tailor the candidate's resume content.\n\n")
	b.WriteString("VACANCY ANALYSIS:\n")
	b.WriteString(strings.Join(analysisLines, "\n"))
	b.WriteString("\n\n")
	b.WriteString(levelRules[level])
	b.WriteString("\n\nHARD RULES (all levels):\n")
	b.WriteString("- Return skills as one entry per group, using the SAME [index] shown below.\n")
	b.WriteString("- Return experience keyed by the EXACT company name shown below; do not add companies.\n")
	b.WriteString("- For each experience entry, select the TOP 3-5 most relevant highlights and rephrase them to emphasize what the vacancy asks for.\n")
	b.WriteString("- Set drop: true only for entries with score below 3 (completely irrelevant to the role).\n")
	b.WriteString("- Reorder experience: most relevant company first.\n")
	b.WriteString("- Reorder skills within each group: vacancy-required skills first.\n")
	b.WriteString("- Keep highlights concise, one achievement each, no fabricated numbers.\n")
	b.WriteString("- Decide which sections to drop. Section keys available: ")
	b.WriteString(strings.Join(sectionKeys, ", "))
	b.WriteString("\n- NEVER drop: summary, experience, education, skills.\n")
	b.WriteString("- Drop academic sections (patents, invited_talks, publications) for non-academic roles.\n")
	b.WriteString("- Drop projects if they are irrelevant to the vacancy.\n\n")
	b.WriteString("Generate a tailored summary (2-3 sentences) that:\n")
	b.WriteString("- Opens with the candidate's seniority level and domain expertise\n")
	b.WriteString("- References 2-3 key skills from the vacancy's required skills\n")
	b.WriteString("- Mentions one quantified achievement from the selected experience\n\n")
	b.WriteString("SKILL GROUPS (master):\n")
	b.WriteString(strings.Join(skillLines, "\n"))
	b.WriteString("\n\nEXPERIENCE (master):\n")
	b.WriteString(strings.Join(expLines, "\n"))

	if len(prevViolations) > 0 {
		b.WriteString("\n\nYour previous attempt violated grounding rules:\n- ")
		b.WriteString(strings.Join(prevViolations, "\n- "))
		b.WriteString("\nRegenerate without these violations.")
	}

	return b.String()
}

// sectionKeys returns the sorted list of section key names from the master.
func sectionKeys(sections map[string]any) []string {
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

// selectAndTailor calls the LLM to produce a TailoredSections from the master
// resume content and the vacancy analysis.
func selectAndTailor(ctx context.Context, lc llm.Provider, model string, master RendercvMaster, analysis VacancyAnalysis, level GroundingLevel, prevViolations []string) (TailoredSections, error) {
	prompt := buildSelectPrompt(master, analysis, level, prevViolations)
	return llm.CompleteStructured[TailoredSections](ctx, lc, prompt, &llm.CompleteOptions{
		System: "You are an expert resume writer who never fabricates information. " +
			"You select, reorder and rephrase existing content to match a specific vacancy.",
		Model: model,
	})
}

// ---------------------------------------------------------------------------
// Merge: apply LLM output to master
// ---------------------------------------------------------------------------

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
func deepCloneYAML(master RendercvMaster) (RendercvMaster, error) {
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

// mergeTailored deep-clones the master and applies all tailoring decisions:
// - Overwrites summary
// - Overwrites skill-group details (vacancy-required first)
// - Replaces experience highlights per entry
// - Drops experience entries marked with Drop: true
// - Reorders experience entries per ExperienceOrder
// - Drops non-protected sections per SectionsToDrop
// Companies, positions, dates, education, design, templates, locale, settings
// and header are taken verbatim from the master — the LLM never touches them.
func mergeTailored(master RendercvMaster, payload TailoredSections) (RendercvMaster, error) {
	merged, err := deepCloneYAML(master)
	if err != nil {
		return nil, err
	}
	sections := CvSections(merged)
	if sections == nil {
		return merged, nil
	}

	// 1. Replace summary
	sections["summary"] = []any{strings.TrimSpace(payload.Summary)}

	// 2. Replace skill details (preserve group order, only change content)
	skills := AsSliceOfMaps(sections["skills"])
	for _, s := range payload.Skills {
		if s.Index >= 0 && s.Index < len(skills) {
			skills[s.Index]["details"] = strings.TrimSpace(s.Details)
		}
	}

	// 3. Apply experience changes: highlights, drops, reorder
	experience := AsSliceOfMaps(sections["experience"])
	byCompany := map[string]map[string]any{}
	for _, e := range experience {
		byCompany[norm(StringField(e, "company"))] = e
	}

	// Apply highlight replacements and mark drops
	for _, pe := range payload.Experience {
		if target, ok := byCompany[norm(pe.Company)]; ok {
			if pe.Drop {
				target["_drop"] = true
				continue
			}
			highlights := make([]any, 0, len(pe.Highlights))
			for _, h := range pe.Highlights {
				if trimmed := strings.TrimSpace(h); trimmed != "" {
					highlights = append(highlights, trimmed)
				}
			}
			target["highlights"] = highlights
		}
	}

	// Filter out dropped entries
	kept := make([]map[string]any, 0, len(experience))
	for _, e := range experience {
		if _, drop := e["_drop"]; !drop {
			kept = append(kept, e)
		}
	}

	// Apply reorder
	if len(payload.ExperienceOrder) > 0 {
		ordered := make([]map[string]any, 0, len(kept))
		seen := map[string]bool{}
		for _, name := range payload.ExperienceOrder {
			if e, ok := byCompany[norm(name)]; ok {
				if _, drop := e["_drop"]; !drop {
					ordered = append(ordered, e)
					seen[norm(name)] = true
				}
			}
		}
		// Append remaining entries not in ExperienceOrder
		for _, e := range kept {
			if !seen[norm(StringField(e, "company"))] {
				ordered = append(ordered, e)
			}
		}
		kept = ordered
	}

	// Clean up internal marker and write back
	for _, e := range kept {
		delete(e, "_drop")
	}
	sections["experience"] = toAnySlice(kept)

	// 4. Drop non-protected sections
	for _, key := range payload.SectionsToDrop {
		if !protectedSections[key] {
			delete(sections, key)
		}
	}

	return merged, nil
}
