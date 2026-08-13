package domain

import (
	"fmt"
	"strings"
	"testing"
)

// TrimSkillGroups is the render-time enforcement of each skill group's
// authored density level. These tests pin the levels against the relevance
// order RankSkills produces, the "relevant" default for an unset or unknown
// level, the pinned-group exemption and the empty-"relevant"-group drop.

func skillsDoc(groups ...[3]string) RendercvMaster {
	raw := make([]any, 0, len(groups))
	for _, g := range groups {
		m := map[string]any{"label": g[0], "details": g[1]}
		if g[2] != "" {
			m["skills_level"] = g[2]
		}
		raw = append(raw, m)
	}
	return RendercvMaster{"cv": map[string]any{"sections": map[string]any{"skills": raw}}}
}

func groupDetails(t *testing.T, doc RendercvMaster) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, g := range AsSliceOfMaps(CvSections(doc)["skills"]) {
		out[StringField(g, "label")] = StringField(g, "details")
	}
	return out
}

func TestTrimSkillGroups_NoLevelDefaultsToRelevant(t *testing.T) {
	doc := skillsDoc([3]string{"Backend", "Go, Node.js, Kafka", ""})
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	if !TrimSkillGroups(doc, analysis) {
		t.Error("changed = false, want true (unset level defaults to relevant-only)")
	}
	if got := groupDetails(t, doc)["Backend"]; got != "Go" {
		t.Errorf("details = %q, want %q", got, "Go")
	}
}

func TestTrimSkillGroups_UnknownLevelDefaultsToRelevant(t *testing.T) {
	doc := skillsDoc([3]string{"Backend", "Go, Node.js, Kafka", "medium"})
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	if !TrimSkillGroups(doc, analysis) {
		t.Error("changed = false, want true (legacy 'medium' behaves as relevant-only)")
	}
	if got := groupDetails(t, doc)["Backend"]; got != "Go" {
		t.Errorf("details = %q, want %q", got, "Go")
	}
}

func TestTrimSkillGroups_CountCaps(t *testing.T) {
	skills := make([]string, 25)
	for i := range skills {
		skills[i] = fmt.Sprintf("s%02d", i)
	}
	cases := []struct {
		level string
		cap   int
	}{
		{SkillLevelTop5, 5},
		{SkillLevelTop10, 10},
		{SkillLevelTop15, 15},
		{SkillLevelTop20, 20},
	}
	for _, c := range cases {
		t.Run(c.level, func(t *testing.T) {
			doc := skillsDoc([3]string{"Backend", strings.Join(skills, ", "), c.level})
			// No vacancy skills match, so the ranked order is the authored order.
			RankSkills(doc, VacancyAnalysis{}, ShapeConfig{SkillsEnabled: true})
			TrimSkillGroups(doc, VacancyAnalysis{})

			got := splitSkillEntries(groupDetails(t, doc)["Backend"])
			if len(got) != c.cap {
				t.Errorf("kept %d skills, want %d (%q)", len(got), c.cap, strings.Join(got, ", "))
			}
		})
	}
}

func TestTrimSkillGroups_AllKeepsEverything(t *testing.T) {
	doc := skillsDoc([3]string{"Backend", "Go, Node.js, Kafka, Redis, Fiber, NATS", SkillLevelAll})
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	if changed := TrimSkillGroups(doc, analysis); changed {
		t.Error("changed = true, want false (all keeps everything)")
	}
	// RankSkills still orders by relevance; "all" just skips the cap.
	want := "Go, Node.js, Kafka, Redis, Fiber, NATS"
	if got := groupDetails(t, doc)["Backend"]; got != want {
		t.Errorf("details = %q, want %q", got, want)
	}
}

func TestTrimSkillGroups_RelevantKeepsOnlyMatchedSkills(t *testing.T) {
	doc := skillsDoc(
		[3]string{"Backend", "Go, Redis, Node.js", SkillLevelRelevant},
		[3]string{"Frontend", "React, Vue, Next.js", SkillLevelRelevant},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go", "Next.js"}, NiceToHaveSkills: []string{"Redis"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	TrimSkillGroups(doc, analysis)

	details := groupDetails(t, doc)
	if got := details["Backend"]; got != "Go, Redis" {
		t.Errorf("Backend details = %q, want %q", got, "Go, Redis")
	}
	if got := details["Frontend"]; got != "Next.js" {
		t.Errorf("Frontend details = %q, want %q", got, "Next.js")
	}
}

func TestTrimSkillGroups_RelevantDropsGroupWithNoMatch(t *testing.T) {
	doc := skillsDoc(
		[3]string{"Backend", "Go", SkillLevelRelevant},
		[3]string{"Mobile", "React Native, Swift", SkillLevelRelevant},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	if !TrimSkillGroups(doc, analysis) {
		t.Error("changed = false, want true (group dropped)")
	}
	if _, ok := groupDetails(t, doc)["Mobile"]; ok {
		t.Error("Mobile group still present, want it dropped")
	}
	if got := groupDetails(t, doc)["Backend"]; got != "Go" {
		t.Errorf("Backend details = %q, want %q", got, "Go")
	}
}

func TestTrimSkillGroups_DroppingAllGroupsRemovesSection(t *testing.T) {
	doc := skillsDoc([3]string{"Mobile", "React Native", SkillLevelRelevant})
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	TrimSkillGroups(doc, analysis)
	if _, ok := CvSections(doc)["skills"]; ok {
		t.Error("skills section still present, want it removed")
	}
}

func TestTrimSkillGroups_PinnedGroupNeverTrimmed(t *testing.T) {
	doc := skillsDoc(
		[3]string{"Spoken Languages", "English, Ukrainian", SkillLevelRelevant},
		[3]string{"Backend", "Go, Redis", SkillLevelRelevant},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	TrimSkillGroups(doc, analysis)

	details := groupDetails(t, doc)
	if got := details["Spoken Languages"]; got != "English, Ukrainian" {
		t.Errorf("pinned group trimmed: %q", got)
	}
	if got := details["Backend"]; got != "Go" {
		t.Errorf("Backend details = %q, want %q", got, "Go")
	}
}

func TestTrimSkillGroups_ExpandPathPreservesTrimmedDetails(t *testing.T) {
	doc := skillsDoc(
		[3]string{"Backend", "Go, Redis, Node.js, Kafka", SkillLevelTop20},
		[3]string{"Frontend", "React, Vue, Next.js", SkillLevelRelevant},
	)
	analysis := VacancyAnalysis{RequiredSkills: []string{"Go", "Next.js"}, NiceToHaveSkills: []string{"Redis"}}

	RankSkills(doc, analysis, ShapeConfig{SkillsEnabled: true})
	TrimSkillGroups(doc, analysis)
	before := strings.Join(sortedGroupDetails(t, doc), "|")

	// The expand path merges on top of an already-trimmed document and then
	// runs DropUngroundedSkillTokens + ApplyHardLimits only — no RankSkills,
	// no trim. Its skill section must reach the page exactly as trimmed.
	DropUngroundedSkillTokens(doc, doc)
	ApplyHardLimits(doc, doc, ShapeConfig{SkillsEnabled: true})
	after := strings.Join(sortedGroupDetails(t, doc), "|")
	if before != after {
		t.Errorf("expand-path stages changed the trimmed details:\n before %q\n after  %q", before, after)
	}
}

func sortedGroupDetails(t *testing.T, doc RendercvMaster) []string {
	t.Helper()
	details := groupDetails(t, doc)
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sortStrings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+":"+details[k])
	}
	return out
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
